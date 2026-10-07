package handler

import (
	"slices"
	"strconv"
	"strings"

	"github.com/gofiber/fiber/v2"

	"github.com/securepatrol/backend/internal/middleware"
	"github.com/securepatrol/backend/internal/model"
	"github.com/securepatrol/backend/internal/notification"
	"github.com/securepatrol/backend/internal/repository"
	"github.com/securepatrol/backend/internal/service"
	"github.com/securepatrol/backend/internal/settings"
)

// ── Notification rules ───────────────────────────────────────────────────────

func (h *Handler) eventTypes(c *fiber.Ctx) error {
	return c.JSON(fiber.Map{"event_types": notification.EventTypes, "channels": []string{"email", "whatsapp"}})
}

func (h *Handler) listRules(c *fiber.Ctx) error {
	site, err := h.siteFor(c, c.Query("site_id"))
	if err != nil {
		return h.fail(c, err)
	}
	rules, err := repository.ListNotificationRules(c.Context(), h.DB, site.ID)
	if err != nil {
		return h.fail(c, err)
	}
	return c.JSON(rules)
}

func (h *Handler) createRule(c *fiber.Ctx) error {
	var b model.NotificationRule
	if err := parse(c, &b); err != nil {
		return h.fail(c, err)
	}
	site, err := h.siteFor(c, b.SiteID)
	if err != nil {
		return h.fail(c, err)
	}
	if !slices.Contains(notification.EventTypes, b.EventType) {
		return h.fail(c, service.BadRequest("invalid_event_type", "unknown event type %q", b.EventType))
	}
	b.Recipient = strings.TrimSpace(b.Recipient)
	switch b.Channel {
	case "email":
		if !strings.Contains(b.Recipient, "@") {
			return h.fail(c, service.BadRequest("invalid_recipient", "recipient must be an e-mail address"))
		}
	case "whatsapp":
		digits := strings.Map(func(r rune) rune {
			if r >= '0' && r <= '9' {
				return r
			}
			return -1
		}, b.Recipient)
		if len(digits) < 8 {
			return h.fail(c, service.BadRequest("invalid_recipient", "recipient must be a phone number in international format, e.g. 62812…"))
		}
		b.Recipient = digits
	default:
		return h.fail(c, service.BadRequest("invalid_channel", "channel must be email or whatsapp"))
	}
	rule, err := repository.CreateNotificationRule(c.Context(), h.DB, model.NotificationRule{
		SiteID: site.ID, EventType: b.EventType, Channel: b.Channel, Recipient: b.Recipient, Enabled: true,
	})
	if err != nil {
		return h.fail(c, err)
	}
	return c.Status(fiber.StatusCreated).JSON(rule)
}

func (h *Handler) ruleFor(c *fiber.Ctx) (*model.NotificationRule, error) {
	r, err := repository.GetNotificationRule(c.Context(), h.DB, c.Params("id"))
	if err != nil {
		return nil, err
	}
	if _, err := h.siteFor(c, r.SiteID); err != nil {
		return nil, err
	}
	return r, nil
}

func (h *Handler) updateRule(c *fiber.Ctx) error {
	r, err := h.ruleFor(c)
	if err != nil {
		return h.fail(c, err)
	}
	var b struct {
		Enabled bool `json:"enabled"`
	}
	if err := parse(c, &b); err != nil {
		return h.fail(c, err)
	}
	out, err := repository.SetNotificationRuleEnabled(c.Context(), h.DB, r.ID, b.Enabled)
	if err != nil {
		return h.fail(c, err)
	}
	return c.JSON(out)
}

func (h *Handler) deleteRule(c *fiber.Ctx) error {
	r, err := h.ruleFor(c)
	if err != nil {
		return h.fail(c, err)
	}
	if err := repository.DeleteNotificationRule(c.Context(), h.DB, r.ID); err != nil {
		return h.fail(c, err)
	}
	return c.SendStatus(fiber.StatusNoContent)
}

// ── Delivery log (FR-NOT-04) ─────────────────────────────────────────────────

// realmSiteIDs returns the IDs of the caller's sites, or just one when
// site_id is given.
func (h *Handler) realmSiteIDs(c *fiber.Ctx) ([]string, error) {
	if id := c.Query("site_id"); id != "" {
		site, err := h.siteFor(c, id)
		if err != nil {
			return nil, err
		}
		return []string{site.ID}, nil
	}
	sites, err := repository.ListSitesByRealm(c.Context(), h.DB, middleware.GetPrincipal(c).Realm)
	if err != nil {
		return nil, err
	}
	ids := make([]string, len(sites))
	for i, s := range sites {
		ids[i] = s.ID
	}
	return ids, nil
}

func (h *Handler) listNotificationLog(c *fiber.Ctx) error {
	ids, err := h.realmSiteIDs(c)
	if err != nil {
		return h.fail(c, err)
	}
	limit, _ := strconv.Atoi(c.Query("limit", "200"))
	if limit <= 0 || limit > 1000 {
		limit = 200
	}
	rows, err := repository.ListNotificationLog(c.Context(), h.DB, ids, c.Query("status"), limit)
	if err != nil {
		return h.fail(c, err)
	}
	return c.JSON(rows)
}

func (h *Handler) retryNotification(c *fiber.Ctx) error {
	l, err := repository.GetNotificationLog(c.Context(), h.DB, c.Params("id"))
	if err != nil {
		return h.fail(c, err)
	}
	if l.SiteID == nil {
		return h.fail(c, repository.ErrNotFound)
	}
	if _, err := h.siteFor(c, *l.SiteID); err != nil {
		return h.fail(c, err)
	}
	if l.Status != "failed" {
		return h.fail(c, service.Conflict("not_failed", "only failed deliveries can be retried"))
	}
	if err := repository.RequeueNotification(c.Context(), h.DB, l.ID); err != nil {
		return h.fail(c, err)
	}
	if err := notification.Enqueue(c.Context(), h.Redis, l.ID); err != nil {
		h.Log.Warn().Err(err).Str("id", l.ID).Msg("enqueue retry failed; sweeper will pick it up")
	}
	return c.JSON(fiber.Map{"status": "queued"})
}

// ── Channel settings (FR-NOT-02) ─────────────────────────────────────────────

func (h *Handler) getChannels(c *fiber.Ctx) error {
	ch, err := h.Settings.Masked(c.Context())
	if err != nil {
		return h.fail(c, err)
	}
	return c.JSON(ch)
}

func (h *Handler) putChannels(c *fiber.Ctx) error {
	var b settings.Channels
	if err := parse(c, &b); err != nil {
		return h.fail(c, err)
	}
	if err := h.Settings.Save(c.Context(), b, actor(c)); err != nil {
		return h.fail(c, err)
	}
	return h.getChannels(c)
}

// POST /settings/channels/test {channel, recipient}
func (h *Handler) testChannel(c *fiber.Ctx) error {
	var b struct {
		Channel   string `json:"channel"`
		Recipient string `json:"recipient"`
	}
	if err := parse(c, &b); err != nil {
		return h.fail(c, err)
	}
	if err := notification.SendTest(c.Context(), h.Settings, b.Channel, strings.TrimSpace(b.Recipient)); err != nil {
		return c.Status(fiber.StatusBadGateway).JSON(fiber.Map{"error": err.Error(), "code": "delivery_failed"})
	}
	return c.JSON(fiber.Map{"sent": true})
}

// ── Backups (NFR-07) ─────────────────────────────────────────────────────────

func (h *Handler) listBackups(c *fiber.Ctx) error {
	rows, err := repository.ListBackups(c.Context(), h.DB, 60)
	if err != nil {
		return h.fail(c, err)
	}
	return c.JSON(rows)
}

// POST /backups — queues a manual backup; the worker runs it within a minute.
func (h *Handler) createBackup(c *fiber.Ctx) error {
	who := actor(c)
	b, err := repository.CreateBackup(c.Context(), h.DB, service.BackupObjectKey(service.Now()), "manual", &who)
	if err != nil {
		return h.fail(c, err)
	}
	return c.Status(fiber.StatusAccepted).JSON(b)
}
