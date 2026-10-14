package handler

import (
	"context"
	"errors"
	"slices"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5"

	"github.com/securepatrol/backend/internal/metrics"
	"github.com/securepatrol/backend/internal/middleware"
	"github.com/securepatrol/backend/internal/model"
	"github.com/securepatrol/backend/internal/notification"
	"github.com/securepatrol/backend/internal/push"
	"github.com/securepatrol/backend/internal/repository"
	"github.com/securepatrol/backend/internal/service"
)

// Push limits.
const (
	maxPushSubscriptions = 20 // per user
	maxPushLabel         = 100
	maxFCMToken          = 4096
	pushTestPerMinute    = 3
	pushTestTimeout      = 15 * time.Second
)

// pushUserOf is the audience snapshot of a web principal: realm, roles and
// the allowed sites of the token claim (nil = whole realm, D-042).
func pushUserOf(p *middleware.Principal) model.PushUser {
	var sites []string
	if p.SitesRestricted {
		sites = append([]string{}, p.Sites...)
	}
	return model.PushUser{
		Realm: p.Realm, UserID: p.Subject, Username: p.Username,
		Roles: append([]string{}, p.Roles...), Sites: sites,
	}
}

// ── Audience refresh ─────────────────────────────────────────────────────────

// audienceRefreshEvery throttles snapshot refreshes per user and process.
const audienceRefreshEvery = 10 * time.Minute

type audienceSeen struct {
	fingerprint string
	at          time.Time
}

var audienceCache sync.Map // realm|sub → audienceSeen

// refreshDue reports whether a user's snapshot should be written now: their
// roles/sites changed, or the last refresh is older than the throttle.
func refreshDue(key, fingerprint string, now time.Time) bool {
	if v, ok := audienceCache.Load(key); ok {
		s := v.(audienceSeen)
		if s.fingerprint == fingerprint && now.Sub(s.at) < audienceRefreshEvery {
			return false
		}
	}
	audienceCache.Store(key, audienceSeen{fingerprint, now})
	return true
}

// RefreshPushAudience keeps the push audience snapshot of web users current:
// roles and sites of the latest token (a cheap UPDATE of an existing row,
// at most every 10 minutes per user unless they change, off the request
// path). Users who never used push have no row and cost nothing.
func (h *Handler) RefreshPushAudience() fiber.Handler {
	return func(c *fiber.Ctx) error {
		p := middleware.GetPrincipal(c)
		if p == nil || p.HasRole(middleware.RoleDevice) || p.ClientID != h.webClient() {
			return c.Next()
		}
		u := pushUserOf(p)
		fp := strings.Join(u.Roles, ",") + "|" + strings.Join(u.Sites, ",") + "|" + boolStr(u.Sites == nil) + "|" + u.Username
		if refreshDue(u.Realm+"|"+u.UserID, fp, time.Now()) {
			go func() {
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				if err := repository.RefreshPushUser(ctx, h.DB, u); err != nil {
					h.Log.Warn().Err(err).Str("user", u.UserID).Msg("refresh push audience failed")
				}
			}()
		}
		return c.Next()
	}
}

func (h *Handler) webClient() string {
	if h.Cfg != nil && h.Cfg.KeycloakWebClient != "" {
		return h.Cfg.KeycloakWebClient
	}
	return middleware.DefaultWebClient
}

func boolStr(b bool) string {
	if b {
		return "1"
	}
	return "0"
}

// ── Config, subscriptions, preferences, test ─────────────────────────────────

// GET /push/config — which transports the server has.
func (h *Handler) pushConfig(c *fiber.Ctx) error {
	ps := h.Notifier.Push()
	return c.JSON(fiber.Map{
		"webpush": fiber.Map{"enabled": ps.WebEnabled(), "vapid_public_key": vapidKey(ps)},
		"fcm":     fiber.Map{"enabled": ps.FCMEnabled()},
	})
}

func vapidKey(ps *push.Service) string {
	if !ps.WebEnabled() {
		return ""
	}
	return ps.Web.PublicKey()
}

var errPushDisabled = service.Conflict("push_disabled", "web push is not configured on this server")

// POST /push/subscriptions {kind, endpoint, keys: {p256dh, auth}, label}
func (h *Handler) createPushSubscription(c *fiber.Ctx) error {
	ps := h.Notifier.Push()
	if !ps.WebEnabled() {
		return h.fail(c, errPushDisabled)
	}
	var b struct {
		Kind     string `json:"kind"`
		Endpoint string `json:"endpoint"`
		Keys     struct {
			P256dh string `json:"p256dh"`
			Auth   string `json:"auth"`
		} `json:"keys"`
		Label string `json:"label"`
	}
	if err := parse(c, &b); err != nil {
		return h.fail(c, err)
	}
	if b.Kind != "" && b.Kind != "webpush" {
		return h.fail(c, service.BadRequest("invalid_kind", "kind must be webpush").WithField("kind"))
	}
	endpoint, err := push.ValidateEndpoint(strings.TrimSpace(b.Endpoint), ps.Web.Hosts())
	if err != nil {
		return h.fail(c, service.BadRequest("invalid_endpoint", "%s", err.Error()).WithField("endpoint"))
	}
	if err := push.ValidateKeys(b.Keys.P256dh, b.Keys.Auth); err != nil {
		return h.fail(c, service.BadRequest("invalid_keys", "%s", err.Error()).WithField("keys"))
	}
	label := strings.TrimSpace(strings.Map(dropControl, b.Label))
	if err := service.CheckLengths(service.S("label", label, maxPushLabel)); err != nil {
		return h.fail(c, err)
	}
	u := pushUserOf(middleware.GetPrincipal(c))
	var id string
	var res repository.SaveResult
	err = pgx.BeginFunc(c.Context(), h.DB, func(tx pgx.Tx) error {
		if err := repository.UpsertPushUser(c.Context(), tx, u); err != nil {
			return err
		}
		var err error
		id, res, err = repository.SavePushSubscription(c.Context(), tx, model.PushSubscription{
			Realm: u.Realm, UserID: u.UserID, Endpoint: endpoint, P256dh: b.Keys.P256dh, Auth: b.Keys.Auth, Label: label,
		})
		if err != nil {
			return err
		}
		n, err := repository.CountPushSubscriptions(c.Context(), tx, u.Realm, u.UserID)
		if err != nil {
			return err
		}
		if n > maxPushSubscriptions {
			return service.Conflict("too_many_subscriptions", "at most %d devices per user; remove one first", maxPushSubscriptions)
		}
		return nil
	})
	if err != nil {
		return h.fail(c, err)
	}
	return created(c, res != repository.SubExisting, fiber.Map{"id": id})
}

func dropControl(r rune) rune {
	if unicode.IsControl(r) {
		return -1
	}
	return r
}

// GET /push/subscriptions — the caller's own browsers.
func (h *Handler) listPushSubscriptions(c *fiber.Ctx) error {
	p := middleware.GetPrincipal(c)
	subs, err := repository.ListPushSubscriptions(c.Context(), h.DB, p.Realm, p.Subject)
	if err != nil {
		return h.fail(c, err)
	}
	return c.JSON(subs)
}

// DELETE /push/subscriptions/:id — only the caller's own.
func (h *Handler) deletePushSubscription(c *fiber.Ctx) error {
	p := middleware.GetPrincipal(c)
	if err := repository.DeleteOwnPushSubscription(c.Context(), h.DB, c.Params("id"), p.Realm, p.Subject); err != nil {
		return h.fail(c, err)
	}
	return c.SendStatus(fiber.StatusNoContent)
}

// pushPrefsView is GET/PUT /push/preferences.
func pushPrefsView(roles []string, prefs map[string]bool) fiber.Map {
	return fiber.Map{
		"events":    notification.EffectivePushPrefs(roles, prefs),
		"available": notification.AvailablePushEvents(roles),
	}
}

// GET /push/preferences — effective on/off per event the caller can receive.
func (h *Handler) getPushPreferences(c *fiber.Ctx) error {
	p := middleware.GetPrincipal(c)
	prefs, err := repository.GetPushPrefs(c.Context(), h.DB, p.Realm, p.Subject)
	if err != nil {
		return h.fail(c, err)
	}
	return c.JSON(pushPrefsView(p.Roles, prefs))
}

// PUT /push/preferences {events: {event: bool}} — partial updates.
func (h *Handler) putPushPreferences(c *fiber.Ctx) error {
	p := middleware.GetPrincipal(c)
	var b struct {
		Events map[string]bool `json:"events"`
	}
	if err := parse(c, &b); err != nil {
		return h.fail(c, err)
	}
	available := notification.AvailablePushEvents(p.Roles)
	for ev := range b.Events {
		if !slices.Contains(available, ev) {
			return h.fail(c, service.BadRequest("invalid_event", "%q is not a push event for your roles", ev).WithField("events"))
		}
	}
	var prefs map[string]bool
	err := pgx.BeginFunc(c.Context(), h.DB, func(tx pgx.Tx) error {
		if err := repository.UpsertPushUser(c.Context(), tx, pushUserOf(p)); err != nil {
			return err
		}
		var err error
		prefs, err = repository.MergePushPrefs(c.Context(), tx, p.Realm, p.Subject, b.Events)
		return err
	})
	if err != nil {
		return h.fail(c, err)
	}
	return c.JSON(pushPrefsView(p.Roles, prefs))
}

// POST /push/test {lang?} — a test push to every browser of the caller,
// sent right away (like the channel test); {sent, failed}.
func (h *Handler) testPush(c *fiber.Ctx) error {
	ps := h.Notifier.Push()
	if !ps.WebEnabled() {
		return h.fail(c, errPushDisabled)
	}
	p := middleware.GetPrincipal(c)
	if ok, retry := h.limiter.Allow(c.Context(), "pushtest:"+p.Realm+"/"+p.Subject, pushTestPerMinute); !ok {
		return middleware.TooManyRequests(c, retry)
	}
	var b struct {
		Lang string `json:"lang"`
	}
	if len(c.Body()) > 0 {
		if err := parse(c, &b); err != nil {
			return h.fail(c, err)
		}
	}
	if b.Lang != "en" {
		b.Lang = "id"
	}
	subs, err := repository.ListPushSubscriptions(c.Context(), h.DB, p.Realm, p.Subject)
	if err != nil {
		return h.fail(c, err)
	}
	title, body := notification.Render(notification.EventPushTest, b.Lang, nil)
	payload, err := notification.WebPayloadFor(notification.EventPushTest, "", "", title, body).Marshal()
	if err != nil {
		return h.fail(c, err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), pushTestTimeout)
	defer cancel()
	var mu sync.Mutex
	var wg sync.WaitGroup
	sent, failed := 0, 0
	for _, s := range subs {
		wg.Add(1)
		go func(s model.PushSubscription) {
			defer wg.Done()
			err := notification.SendWebPush(ctx, h.DB, ps.Web, s.ID,
				push.Subscription{Endpoint: s.Endpoint, P256dh: s.P256dh, Auth: s.Auth}, payload)
			result := metrics.ResultSent
			switch {
			case errors.Is(err, push.ErrGone):
				result = metrics.ResultGone
			case err != nil:
				result = metrics.ResultFailed
				h.Log.Info().Err(err).Str("subscription", s.ID).Msg("test push failed")
			}
			metrics.PushSent.WithLabelValues(repository.ChannelWebPush, notification.EventPushTest, result).Inc()
			mu.Lock()
			defer mu.Unlock()
			if err == nil {
				sent++
			} else {
				failed++
			}
		}(s)
	}
	wg.Wait()
	return c.JSON(fiber.Map{"sent": sent, "failed": failed})
}

// ── Tablet FCM token ─────────────────────────────────────────────────────────

// PUT /device/push-token {token, platform} — pending or active tablet, no
// guard; the latest token wins (204).
func (h *Handler) putDevicePushToken(c *fiber.Ctx) error {
	d, err := tokenDevice(c)
	if err != nil {
		return h.fail(c, err)
	}
	var b struct {
		Token    string `json:"token"`
		Platform string `json:"platform"`
	}
	if err := parse(c, &b); err != nil {
		return h.fail(c, err)
	}
	b.Token = strings.TrimSpace(b.Token)
	if b.Token == "" || len(b.Token) > maxFCMToken || strings.IndexFunc(b.Token, func(r rune) bool {
		return r <= ' ' || r > '~'
	}) >= 0 {
		return h.fail(c, service.BadRequest("invalid_token", "token must be an FCM registration token").WithField("token"))
	}
	if b.Platform != "android" && b.Platform != "ios" {
		return h.fail(c, service.BadRequest("invalid_platform", "platform must be android or ios").WithField("platform"))
	}
	if err := repository.SetDevicePushToken(c.Context(), h.DB, d.ID, b.Token, b.Platform); err != nil {
		return h.fail(c, err)
	}
	return c.SendStatus(fiber.StatusNoContent)
}

// DELETE /device/push-token — the tablet stops receiving pushes (204).
func (h *Handler) deleteDevicePushToken(c *fiber.Ctx) error {
	d, err := tokenDevice(c)
	if err != nil {
		return h.fail(c, err)
	}
	if err := repository.ClearDevicePushToken(c.Context(), h.DB, d.ID); err != nil {
		return h.fail(c, err)
	}
	return c.SendStatus(fiber.StatusNoContent)
}

// tokenDevice is the calling tablet when it may hold a push token: registered
// and not revoked (a pending tablet registers so it hears its approval).
func tokenDevice(c *fiber.Ctx) (*model.Device, error) {
	d := middleware.GetDevice(c)
	switch {
	case d == nil:
		return nil, service.Forbidden("device_unregistered", "device not registered")
	case d.Status == "revoked":
		return nil, service.Forbidden("device_revoked", "device is not active")
	}
	return d, nil
}

// ── Tablet messages raised by web actions ────────────────────────────────────

// pushSyncKey is the request local that collects sites whose tablets must
// re-sync after the request succeeds.
const pushSyncKey = "push_sync_sites"

// markSync asks for a debounced "sync" message to the tablets of these
// sites once the request has succeeded (roster, routes, schedules changed).
func markSync(c *fiber.Ctx, siteIDs ...*string) {
	cur, _ := c.Locals(pushSyncKey).([]string)
	for _, id := range siteIDs {
		if id != nil && *id != "" && !slices.Contains(cur, *id) {
			cur = append(cur, *id)
		}
	}
	c.Locals(pushSyncKey, cur)
}

// SyncTablets stages the sync messages requested by markSync after a 2xx
// response. Best effort: the change itself is already committed.
func (h *Handler) SyncTablets() fiber.Handler {
	return func(c *fiber.Ctx) error {
		err := c.Next()
		sites, _ := c.Locals(pushSyncKey).([]string)
		if len(sites) == 0 || err != nil || c.Response().StatusCode() >= 300 || !h.Notifier.Push().FCMEnabled() {
			return err
		}
		now := service.Now()
		for _, id := range sites {
			if _, serr := h.Notifier.StageSync(c.Context(), h.DB, id, now); serr != nil {
				h.Log.Warn().Err(serr).Str("site_id", id).Msg("stage tablet sync failed")
			}
		}
		return nil
	}
}

// fmtTime renders an optional timestamp in a site's zone ("-" when unset).
func fmtTime(t *time.Time, loc *time.Location) string {
	if t == nil {
		return "-"
	}
	return t.In(loc).Format("02 Jan 15:04")
}

// reviewReason summarises why a clock-in needs review: its flags, else the
// review note (e.g. a late sync replacing an absent mark).
func reviewReason(rec *model.AttendanceRecord) string {
	if len(rec.Flags) > 0 {
		return strings.Join(rec.Flags, ", ")
	}
	if rec.ReviewNote != nil && *rec.ReviewNote != "" {
		return *rec.ReviewNote
	}
	return "review"
}

// deviceStatusText is the tablet-facing name of a device status.
func deviceStatusText(status, lang string) string {
	names := map[string][2]string{
		"pending": {"menunggu persetujuan", "waiting for approval"},
		"active":  {"aktif", "active"},
		"revoked": {"dicabut", "revoked"},
	}
	n, ok := names[status]
	if !ok {
		return status
	}
	if lang == "en" {
		return n[1]
	}
	return n[0]
}

// stageDeviceStatus tells a tablet that it was approved, revoked or moved.
func (h *Handler) stageDeviceStatus(ctx context.Context, tx pgx.Tx, d *model.Device, requestID string) (notification.Staged, error) {
	lang, siteID := "id", ""
	if d.SiteID != nil {
		siteID = *d.SiteID
		if site, err := repository.GetSite(ctx, tx, siteID); err == nil {
			lang = site.Config().Language
		}
	}
	title, body := notification.Render(push.TabletDeviceStatus, lang, map[string]string{"status": deviceStatusText(d.Status, lang)})
	return h.Notifier.StageTablets(ctx, tx, notification.TabletEvent{
		Type: push.TabletDeviceStatus, DeviceID: d.ID, SiteID: siteID, RefID: d.ID, Title: title, Body: body,
		DedupeKey: "device_status:" + d.ID + ":" + d.UpdatedAt.UTC().Format(time.RFC3339Nano), RequestID: requestID,
	})
}

// stageSosAck tells the site's tablets an SOS was acknowledged.
func (h *Handler) stageSosAck(ctx context.Context, tx pgx.Tx, a *model.SosAlert, by, requestID string) (notification.Staged, error) {
	site, err := repository.GetSite(ctx, tx, a.SiteID)
	if err != nil {
		return notification.Staged{}, err
	}
	title, body := notification.Render(push.TabletSOSAck, site.Config().Language, map[string]string{"guard": a.GuardName, "actor": by})
	return h.Notifier.StageTablets(ctx, tx, notification.TabletEvent{
		Type: push.TabletSOSAck, SiteID: a.SiteID, RefID: a.ID, Title: title, Body: body,
		DedupeKey: "sos_ack:" + a.ID, RequestID: requestID,
	})
}
