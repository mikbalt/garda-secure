package handler

import (
	"errors"
	"slices"
	"strconv"
	"strings"

	"github.com/gofiber/fiber/v2"

	"github.com/securepatrol/backend/internal/notification"
	"github.com/securepatrol/backend/internal/repository"
	"github.com/securepatrol/backend/internal/service"
)

// Install-wide operations alerts: who is told when a backup fails or is
// stale, an SOS cannot be delivered, or a scheduled job keeps failing. Like
// the channel settings they are install-wide and need the platform-admin
// role (BE-S3).

// GET /notifications/ops-event-types
func (h *Handler) opsEventTypes(c *fiber.Ctx) error {
	return c.JSON(fiber.Map{"event_types": notification.OpsEventTypes, "channels": notification.ChannelNames()})
}

// GET /notifications/ops-rules
func (h *Handler) listOpsRules(c *fiber.Ctx) error {
	rules, err := repository.ListOpsAlertRules(c.Context(), h.DB)
	if err != nil {
		return h.fail(c, err)
	}
	return c.JSON(rules)
}

// POST /notifications/ops-rules {event_type, channel, recipient}
func (h *Handler) createOpsRule(c *fiber.Ctx) error {
	var b struct {
		EventType string `json:"event_type"`
		Channel   string `json:"channel"`
		Recipient string `json:"recipient"`
	}
	if err := parse(c, &b); err != nil {
		return h.fail(c, err)
	}
	if !slices.Contains(notification.OpsEventTypes, b.EventType) {
		return h.fail(c, service.BadRequest("invalid_event_type", "unknown ops event type %q", b.EventType))
	}
	recipient, err := normalizeRecipient(b.Channel, b.Recipient)
	if err != nil {
		return h.fail(c, err)
	}
	who := actor(c)
	rule, err := repository.CreateOpsAlertRule(c.Context(), h.DB, repository.OpsAlertRule{
		EventType: b.EventType, Channel: b.Channel, Recipient: recipient, CreatedBy: &who,
	})
	if err != nil {
		return h.fail(c, err)
	}
	return c.Status(fiber.StatusCreated).JSON(rule)
}

// PUT /notifications/ops-rules/:id {enabled}
func (h *Handler) updateOpsRule(c *fiber.Ctx) error {
	var b struct {
		Enabled bool `json:"enabled"`
	}
	if err := parse(c, &b); err != nil {
		return h.fail(c, err)
	}
	rule, err := repository.SetOpsAlertRuleEnabled(c.Context(), h.DB, c.Params("id"), b.Enabled)
	if err != nil {
		return h.fail(c, err)
	}
	return c.JSON(rule)
}

// DELETE /notifications/ops-rules/:id
func (h *Handler) deleteOpsRule(c *fiber.Ctx) error {
	if err := repository.DeleteOpsAlertRule(c.Context(), h.DB, c.Params("id")); err != nil {
		return h.fail(c, err)
	}
	return c.SendStatus(fiber.StatusNoContent)
}

// GET /notifications/ops-log?status=&limit= — deliveries of ops alerts.
func (h *Handler) listOpsLog(c *fiber.Ctx) error {
	limit, _ := strconv.Atoi(c.Query("limit", "200"))
	if limit <= 0 || limit > 1000 {
		limit = 200
	}
	rows, err := repository.ListOpsNotificationLog(c.Context(), h.DB, c.Query("status"), limit)
	if err != nil {
		return h.fail(c, err)
	}
	return c.JSON(rows)
}

// POST /notifications/ops-log/:id/retry — failed ops deliveries only.
func (h *Handler) retryOpsNotification(c *fiber.Ctx) error {
	l, err := repository.GetNotificationLog(c.Context(), h.DB, c.Params("id"))
	if err != nil {
		return h.fail(c, err)
	}
	if l.SiteID != nil {
		return h.fail(c, repository.ErrNotFound) // site deliveries use /notifications/log
	}
	if l.Status != "failed" {
		return h.fail(c, service.Conflict("not_failed", "only failed deliveries can be retried"))
	}
	if err := repository.RequeueNotification(c.Context(), h.DB, l.ID); err != nil {
		return h.fail(c, err)
	}
	if err := notification.Enqueue(c.Context(), h.Redis, l.ID); err != nil {
		h.Log.Warn().Err(err).Str("id", l.ID).Msg("wake-up for retry failed; the worker polls postgres")
	}
	return c.JSON(fiber.Map{"status": "queued"})
}

// normalizeRecipient validates a recipient for a registered channel
// (notification.Channels, F10); WhatsApp numbers become digits in
// international format (08... -> 628..., D-023).
func normalizeRecipient(channel, recipient string) (string, error) {
	out, err := notification.NormalizeRecipient(channel, recipient)
	switch {
	case errors.Is(err, notification.ErrUnknownChannel):
		return "", service.BadRequest("invalid_channel", "channel must be one of %s", strings.Join(notification.ChannelNames(), ", "))
	case errors.Is(err, notification.ErrInvalidRecipient):
		msg := strings.TrimPrefix(err.Error(), notification.ErrInvalidRecipient.Error()+": ")
		return "", service.BadRequest("invalid_recipient", "%s", msg)
	case err != nil:
		return "", err
	}
	return out, nil
}
