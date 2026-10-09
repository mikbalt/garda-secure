// Package handler implements the HTTP API.
package handler

import (
	"bytes"
	"context"
	"encoding/csv"
	"errors"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	"github.com/rs/zerolog"

	"github.com/securepatrol/backend/internal/config"
	"github.com/securepatrol/backend/internal/middleware"
	"github.com/securepatrol/backend/internal/model"
	"github.com/securepatrol/backend/internal/notification"
	"github.com/securepatrol/backend/internal/repository"
	"github.com/securepatrol/backend/internal/service"
	"github.com/securepatrol/backend/internal/settings"
	"github.com/securepatrol/backend/internal/storage"
)

// Handler holds the dependencies shared by all endpoints.
type Handler struct {
	DB       *pgxpool.Pool
	Redis    *redis.Client
	Storage  *storage.Client
	Notifier *notification.Notifier
	Settings *settings.Store
	Cfg      *config.Config
	Log      zerolog.Logger

	limiter *middleware.Limiter // set by Mount
}

// Role groups used in route registration.
var (
	rolesWeb    = []string{middleware.RoleAdmin, middleware.RoleSupervisor, middleware.RoleHR, middleware.RoleClientReadonly}
	rolesOps    = []string{middleware.RoleAdmin, middleware.RoleSupervisor}
	rolesOpsCli = []string{middleware.RoleAdmin, middleware.RoleSupervisor, middleware.RoleClientReadonly}
	rolesStaff  = []string{middleware.RoleAdmin, middleware.RoleSupervisor, middleware.RoleHR}
	rolesAdmin  = []string{middleware.RoleAdmin}
)

// Register mounts every /api/v1 route. The group must already run the
// middleware chain set up by Mount.
func (h *Handler) Register(api fiber.Router) {
	web := func(roles ...string) fiber.Handler { return middleware.RequireRoles(roles...) }
	device := middleware.RequireActiveDevice()
	field := []fiber.Handler{middleware.RequireGuard(h.DB)}
	// webOrField admits web roles, or an active tablet acting for a guard.
	webOrField := func(roles ...string) fiber.Handler {
		return middleware.RequireRolesOrGuard(h.DB, roles...)
	}
	isDevice := middleware.RequireRoles(middleware.RoleDevice)

	api.Get("/me", h.me)

	// Tablet bootstrap. Middleware is attached per route, not on the group:
	// Fiber group middleware matches by plain prefix, so "/device" would
	// also catch "/devices".
	dev := api.Group("/device")
	dev.Post("/register", isDevice, h.registerDevice)
	dev.Get("/me", isDevice, h.deviceMe)
	dev.Get("/roster", device, h.deviceRoster)
	dev.Get("/routes", device, h.deviceRoutes)
	dev.Get("/shifts", append(field, h.deviceShifts)...)
	dev.Get("/status", append(field, h.deviceStatus)...)
	dev.Get("/sos", device, h.deviceSos)

	sites := api.Group("/sites")
	sites.Get("/", web(rolesWeb...), h.listSites)
	sites.Get("/:id", web(rolesWeb...), h.getSite)
	sites.Post("/", web(rolesAdmin...), h.createSite)
	sites.Put("/:id", web(rolesAdmin...), h.updateSite)

	guards := api.Group("/guards")
	guards.Get("/", web(rolesStaff...), h.listGuards)
	guards.Get("/:id", web(rolesStaff...), h.getGuard)
	guards.Post("/", web(rolesAdmin...), h.createGuard)
	guards.Put("/:id", web(rolesAdmin...), h.updateGuard)
	guards.Put("/:id/pin", web(rolesAdmin...), h.setGuardPin)

	devices := api.Group("/devices", web(rolesAdmin...))
	devices.Get("/", h.listDevices)
	devices.Put("/:id", h.updateDevice)

	tpl := api.Group("/shift-templates")
	tpl.Get("/", web(rolesStaff...), h.listTemplates)
	tpl.Post("/", web(rolesAdmin...), h.createTemplate)
	tpl.Put("/:id", web(rolesAdmin...), h.updateTemplate)
	tpl.Delete("/:id", web(rolesAdmin...), h.deleteTemplate)

	shifts := api.Group("/shifts")
	shifts.Get("/", web(rolesStaff...), h.listShifts)
	shifts.Post("/", web(rolesAdmin...), h.createShift)
	shifts.Put("/:id", web(rolesAdmin...), h.updateShift)
	shifts.Delete("/:id", web(rolesAdmin...), h.deleteShift)

	att := api.Group("/attendance")
	att.Get("/daily", web(rolesWeb...), h.dailyAttendance)
	att.Get("/review", web(rolesOps...), h.reviewQueue)
	att.Post("/:id/review", web(rolesOps...), h.markReviewed)
	att.Put("/shifts/:shiftId/override", web(rolesAdmin...), h.overrideAttendance)
	att.Post("/clock-in", append(field, h.clockIn)...)
	att.Post("/clock-out", append(field, h.clockOut)...)

	pat := api.Group("/patrol")
	pat.Get("/routes", web(rolesOpsCli...), h.listRoutes)
	pat.Get("/routes/:id", web(rolesOpsCli...), h.getRoute)
	pat.Post("/routes", web(rolesAdmin...), h.createRoute)
	pat.Put("/routes/:id", web(rolesAdmin...), h.updateRoute)
	pat.Get("/live", web(rolesOpsCli...), h.livePatrols)
	pat.Get("/sessions", web(rolesOpsCli...), h.listSessions)
	pat.Get("/sessions/:id", webOrField(rolesOpsCli...), h.getSession)
	pat.Post("/sessions", append(field, h.startSession)...)
	pat.Post("/sessions/:id/scans", append(field, h.recordScan)...)
	pat.Post("/sessions/:id/end", append(field, h.endSession)...)

	inc := api.Group("/incidents")
	inc.Get("/", web(rolesOpsCli...), h.listIncidents)
	inc.Get("/:id", web(rolesOpsCli...), h.getIncident)
	inc.Post("/", append(field, h.createIncident)...)
	inc.Put("/:id/status", web(rolesOps...), h.updateIncidentStatus)
	inc.Post("/:id/amendments", web(rolesOps...), h.amendIncident)

	sos := api.Group("/sos")
	sos.Get("/", web(rolesOps...), h.listSos)
	sos.Post("/", append(field, h.createSos)...)
	sos.Post("/:id/acknowledge", webOrField(rolesOps...), h.acknowledgeSos)

	def := api.Group("/defects")
	def.Get("/", web(rolesOps...), h.listDefects)
	def.Post("/", append(field, h.createDefect)...)
	def.Put("/:id/status", web(rolesOps...), h.updateDefectStatus)

	vis := api.Group("/visitors")
	vis.Get("/", web(rolesOps...), h.listVisitors)
	vis.Post("/", append(field, h.createVisitor)...)
	vis.Post("/:id/exit", append(field, h.visitorExit)...)

	api.Post("/media", append(field, h.uploadMedia)...)
	api.Get("/media/url", web(rolesWeb...), h.mediaURL)

	rep := api.Group("/reports")
	rep.Get("/attendance/daily", web(rolesWeb...), h.reportDailyAttendance)
	rep.Get("/attendance/monthly", web(rolesWeb...), h.reportMonthlyRecap)
	rep.Get("/patrol/sessions", web(rolesOpsCli...), h.reportPatrolSessions)
	rep.Get("/incidents", web(rolesOpsCli...), h.reportIncidents)
	rep.Get("/guards/kpi", web(rolesStaff...), h.reportGuardKPI)

	api.Get("/dashboard/summary", web(rolesWeb...), h.dashboardSummary)

	notif := api.Group("/notifications", web(rolesAdmin...))
	notif.Get("/event-types", h.eventTypes)
	notif.Get("/rules", h.listRules)
	notif.Post("/rules", h.createRule)
	notif.Put("/rules/:id", h.updateRule)
	notif.Delete("/rules/:id", h.deleteRule)
	notif.Get("/log", h.listNotificationLog)
	notif.Post("/log/:id/retry", h.retryNotification)

	// Install-wide settings and backups: platform-admin only (BE-S3).
	platform := middleware.RequirePlatformAdmin(h.Cfg.PlatformAdminRealms)
	set := api.Group("/settings", platform)
	set.Get("/channels", h.getChannels)
	set.Put("/channels", h.putChannels)
	set.Post("/channels/test", h.testChannel)

	bk := api.Group("/backups", platform)
	bk.Get("/", h.listBackups)
	bk.Post("/", h.createBackup)

	api.Get("/audit", web(rolesAdmin...), h.listAudit)
}

// ── Error handling ───────────────────────────────────────────────────────────

// fail converts an error into a JSON response.
func (h *Handler) fail(c *fiber.Ctx, err error) error {
	var se *service.Error
	switch {
	case errors.As(err, &se):
		body := fiber.Map{"error": se.Msg}
		if se.Code != "" {
			body["code"] = se.Code
		}
		if se.Field != "" {
			body["field"] = se.Field
		}
		return c.Status(se.Status).JSON(body)
	case errors.Is(err, repository.ErrNotFound), repository.IsInvalidInput(err):
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "not found", "code": "not_found"})
	case repository.IsUniqueViolation(err):
		return c.Status(fiber.StatusConflict).JSON(fiber.Map{"error": "already exists", "code": "duplicate"})
	case repository.IsForeignKeyViolation(err):
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "referenced record does not exist", "code": "invalid_reference"})
	case repository.IsCheckViolation(err):
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid value", "code": "invalid_value"})
	}
	h.Log.Error().Err(err).Str("path", c.Path()).Str("method", c.Method()).Msg("request failed")
	return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "internal error", "code": "internal"})
}

// parse decodes a JSON body.
func parse(c *fiber.Ctx, v any) error {
	if err := c.BodyParser(v); err != nil {
		return service.BadRequest("invalid_body", "invalid JSON body")
	}
	return nil
}

// optionalUUID validates a client-supplied ID.
func optionalUUID(id *string) error {
	if id == nil || *id == "" {
		return nil
	}
	u, err := uuid.Parse(*id)
	if err != nil {
		return service.BadRequest("invalid_id", "id must be a UUID")
	}
	s := u.String()
	*id = s
	return nil
}

func nilIfEmpty(id *string) *string {
	if id == nil || *id == "" {
		return nil
	}
	return id
}

// created responds 201 for new rows and 200 for idempotent repeats.
func created(c *fiber.Ctx, isNew bool, v any) error {
	if isNew {
		return c.Status(fiber.StatusCreated).JSON(v)
	}
	return c.JSON(v)
}

// ── Access control helpers ───────────────────────────────────────────────────

// siteFor loads a site and checks the caller may access it: tablets only their
// own site, web users only sites of their realm (others look like 404).
func (h *Handler) siteFor(c *fiber.Ctx, siteID string) (*model.Site, error) {
	if siteID == "" {
		return nil, service.BadRequest("site_id_required", "site_id is required")
	}
	site, err := repository.GetSite(c.Context(), h.DB, siteID)
	if err != nil {
		return nil, err
	}
	if d := middleware.GetDevice(c); d != nil {
		if d.SiteID == nil || *d.SiteID != site.ID {
			return nil, service.Forbidden("other_site", "device is not assigned to this site")
		}
		return site, nil
	}
	p := middleware.GetPrincipal(c)
	if p == nil || site.KeycloakRealm == nil || *site.KeycloakRealm != p.Realm {
		return nil, repository.ErrNotFound
	}
	return site, nil
}

// deviceSite returns the site of the calling tablet.
func (h *Handler) deviceSite(c *fiber.Ctx) (*model.Site, error) {
	d := middleware.GetDevice(c)
	if d == nil || d.SiteID == nil {
		return nil, service.Forbidden("device_inactive", "device has no site")
	}
	return repository.GetSite(c.Context(), h.DB, *d.SiteID)
}

func deviceID(c *fiber.Ctx) *string {
	if d := middleware.GetDevice(c); d != nil {
		id := d.ID
		return &id
	}
	return nil
}

// actor is the identity recorded in audit-style columns.
func actor(c *fiber.Ctx) string {
	if g := middleware.GetGuard(c); g != nil {
		return "guard:" + g.ID
	}
	if p := middleware.GetPrincipal(c); p != nil {
		return p.Realm + "/" + p.Actor()
	}
	return "unknown"
}

// ── Output helpers ───────────────────────────────────────────────────────────

// presignTTL bounds how long photo links in API responses stay valid.
const presignTTL = 15 * time.Minute

// Photo is a stored image reference with a short-lived URL.
type Photo struct {
	Key      string `json:"key"`
	URL      string `json:"url,omitempty"`
	Uploaded bool   `json:"uploaded"` // false = referenced by a record but not uploaded yet
}

// photos resolves object keys to pre-signed URLs, flagging pending uploads.
func (h *Handler) photos(ctx context.Context, keys ...string) []Photo {
	out := []Photo{}
	for _, k := range keys {
		if k == "" {
			continue
		}
		p := Photo{Key: k}
		ok, err := h.Storage.Exists(ctx, h.Cfg.MinioBucketPhotos, k)
		if err == nil && ok {
			p.Uploaded = true
			p.URL, _ = h.Storage.PresignGet(ctx, h.Cfg.MinioBucketPhotos, k, presignTTL)
		}
		out = append(out, p)
	}
	return out
}

// wantsCSV reports whether ?format=csv was requested.
func wantsCSV(c *fiber.Ctx) bool { return c.Query("format") == "csv" }

// sendCSV writes a UTF-8 CSV with BOM so Excel opens it correctly.
func sendCSV(c *fiber.Ctx, filename string, header []string, rows [][]string) error {
	var buf bytes.Buffer
	buf.Write([]byte{0xEF, 0xBB, 0xBF}) // UTF-8 BOM
	w := csv.NewWriter(&buf)
	_ = w.Write(header)
	for _, r := range rows {
		for i, cell := range r {
			r[i] = csvSafe(cell)
		}
		_ = w.Write(r)
	}
	w.Flush()
	c.Set(fiber.HeaderContentType, "text/csv; charset=utf-8")
	c.Set(fiber.HeaderContentDisposition, `attachment; filename="`+filename+`"`)
	return c.Send(buf.Bytes())
}

// csvSafe stops spreadsheet apps from evaluating user-entered text as a
// formula (CSV injection).
func csvSafe(s string) string {
	if s != "" && strings.ContainsRune("=+-@\t\r", rune(s[0])) {
		return "'" + s
	}
	return s
}

// publish sends a notification event, logging (not failing) on error.
func (h *Handler) publish(ctx context.Context, ev notification.Event) {
	if _, err := h.Notifier.Publish(ctx, ev); err != nil {
		h.Log.Error().Err(err).Str("event", ev.Type).Msg("publish notification failed")
	}
}
