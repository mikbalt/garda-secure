package handler

import (
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5"

	"github.com/securepatrol/backend/internal/middleware"
	"github.com/securepatrol/backend/internal/model"
	"github.com/securepatrol/backend/internal/repository"
	"github.com/securepatrol/backend/internal/service"
)

// ── Routes (route editor) ────────────────────────────────────────────────────

type routeBody struct {
	SiteID      string             `json:"site_id"`
	Name        string             `json:"name"`
	Checkpoints []model.Checkpoint `json:"checkpoints"`
	Active      *bool              `json:"active"`
}

func (h *Handler) listRoutes(c *fiber.Ctx) error {
	site, err := h.siteFor(c, c.Query("site_id"))
	if err != nil {
		return h.fail(c, err)
	}
	routes, err := repository.ListRoutes(c.Context(), h.DB, site.ID, c.Query("active") == "true")
	if err != nil {
		return h.fail(c, err)
	}
	for i := range routes {
		service.DecorateQR(&routes[i], h.qr)
	}
	return c.JSON(routes)
}

func (h *Handler) routeFor(c *fiber.Ctx) (*model.Route, error) {
	r, err := repository.GetRoute(c.Context(), h.DB, c.Params("id"))
	if err != nil {
		return nil, err
	}
	if _, err := h.siteFor(c, r.SiteID); err != nil {
		return nil, err
	}
	return r, nil
}

func (h *Handler) getRoute(c *fiber.Ctx) error {
	r, err := h.routeFor(c)
	if err != nil {
		return h.fail(c, err)
	}
	service.DecorateQR(r, h.qr)
	return c.JSON(r)
}

func (h *Handler) createRoute(c *fiber.Ctx) error {
	var b routeBody
	if err := parse(c, &b); err != nil {
		return h.fail(c, err)
	}
	site, err := h.siteFor(c, b.SiteID)
	if err != nil {
		return h.fail(c, err)
	}
	r := model.Route{SiteID: site.ID, Name: b.Name, Checkpoints: b.Checkpoints, Active: b.Active == nil || *b.Active}
	if err := service.NormalizeRoute(&r, nil, h.qr); err != nil {
		return h.fail(c, err)
	}
	if err := h.checkRoutePlans(c, &r); err != nil {
		return h.fail(c, err)
	}
	out, err := repository.CreateRoute(c.Context(), h.DB, r)
	if err != nil {
		return h.fail(c, err)
	}
	return c.Status(fiber.StatusCreated).JSON(out)
}

func (h *Handler) updateRoute(c *fiber.Ctx) error {
	r, err := h.routeFor(c)
	if err != nil {
		return h.fail(c, err)
	}
	middleware.SetAuditBefore(c, r)
	var b routeBody
	if err := parse(c, &b); err != nil {
		return h.fail(c, err)
	}
	prev := *r
	prev.Checkpoints = append([]model.Checkpoint(nil), r.Checkpoints...)
	r.Name, r.Checkpoints = b.Name, b.Checkpoints
	if b.Active != nil {
		r.Active = *b.Active
	}
	if err := service.NormalizeRoute(r, &prev, h.qr); err != nil {
		return h.fail(c, err)
	}
	if err := h.checkRoutePlans(c, r); err != nil {
		return h.fail(c, err)
	}
	var out *model.Route
	err = pgx.BeginFunc(c.Context(), h.DB, func(tx pgx.Tx) error {
		var err error
		if out, err = repository.UpdateRoute(c.Context(), tx, *r); err != nil {
			return err
		}
		if out.Active == prev.Active {
			return nil
		}
		// Activated or deactivated: rebuild or cancel its scheduled patrols.
		site, err := repository.GetSite(c.Context(), tx, out.SiteID)
		if err != nil {
			return err
		}
		_, err = service.SyncRouteExpectations(c.Context(), tx, out, site)
		return err
	})
	if err != nil {
		return h.fail(c, err)
	}
	service.DecorateQR(out, h.qr)
	return c.JSON(out)
}

// ── Sessions (web) ───────────────────────────────────────────────────────────

// GET /patrol/live?site_id= — progress of every running patrol (FR-PAT-05):
// per session the checkpoints (with floor plan positions, F3), their
// state (on_time/late/overdue/pending) and the last scan time. Loaded with a
// fixed number of queries whatever the number of sessions.
func (h *Handler) livePatrols(c *fiber.Ctx) error {
	site, err := h.siteFor(c, c.Query("site_id"))
	if err != nil {
		return h.fail(c, err)
	}
	sessions, err := repository.ListActiveSessions(c.Context(), h.DB, site.ID)
	if err != nil {
		return h.fail(c, err)
	}
	progress, err := service.LoadProgressBatch(c.Context(), h.DB, sessions)
	if err != nil {
		return h.fail(c, err)
	}
	guardIDs := make([]string, 0, len(sessions))
	for _, s := range sessions {
		guardIDs = append(guardIDs, s.GuardID)
	}
	names, err := repository.GuardNames(c.Context(), h.DB, guardIDs)
	if err != nil {
		return h.fail(c, err)
	}
	out := make([]fiber.Map, 0, len(progress))
	for i := range progress {
		p := &progress[i]
		var last *time.Time
		for _, cp := range p.Checkpoints {
			if cp.ScannedAt != nil && (last == nil || cp.ScannedAt.After(*last)) {
				last = cp.ScannedAt
			}
		}
		out = append(out, fiber.Map{"progress": p, "guard_name": names[p.Session.GuardID], "last_scan_at": last})
	}
	return c.JSON(out)
}

// GET /patrol/sessions?site_id=&from=&to=
func (h *Handler) listSessions(c *fiber.Ctx) error {
	site, err := h.siteFor(c, c.Query("site_id"))
	if err != nil {
		return h.fail(c, err)
	}
	from, to, err := service.DateRange(c.Query("from"), c.Query("to"), site.Location(), 62)
	if err != nil {
		return h.fail(c, err)
	}
	rows, err := repository.SessionSummaries(c.Context(), h.DB, site.ID, from, to)
	if err != nil {
		return h.fail(c, err)
	}
	return c.JSON(rows)
}

// GET /patrol/sessions/:id — checkpoint timeline (web report or tablet).
func (h *Handler) getSession(c *fiber.Ctx) error {
	s, err := repository.GetSession(c.Context(), h.DB, c.Params("id"))
	if err != nil {
		return h.fail(c, err)
	}
	if _, err := h.siteFor(c, s.SiteID); err != nil {
		return h.fail(c, err)
	}
	p, err := service.LoadProgress(c.Context(), h.DB, *s)
	if err != nil {
		return h.fail(c, err)
	}
	if middleware.GetDevice(c) != nil {
		service.HideSignedQR(p)
	}
	return c.JSON(p)
}

// ── Sessions (tablet) ────────────────────────────────────────────────────────

// POST /patrol/sessions {id?, route_id, started_at?}
func (h *Handler) startSession(c *fiber.Ctx) error {
	var b struct {
		ID        *string    `json:"id"`
		RouteID   string     `json:"route_id"`
		StartedAt *time.Time `json:"started_at"`
	}
	if err := parse(c, &b); err != nil {
		return h.fail(c, err)
	}
	if err := optionalUUID(b.ID); err != nil {
		return h.fail(c, err)
	}
	site, err := h.deviceSite(c)
	if err != nil {
		return h.fail(c, err)
	}
	s, isNew, err := service.StartSession(c.Context(), h.DB, service.StartSessionInput{
		ID: nilIfEmpty(b.ID), RouteID: b.RouteID, At: b.StartedAt,
		Guard: middleware.GetGuard(c), Auth: middleware.GetGuardAuth(c), Site: site, DeviceID: deviceID(c),
	})
	if err != nil {
		return h.fail(c, err)
	}
	if isNew {
		// Satisfy the scheduled patrol (by event time). A failure here only
		// delays it: the worker's matching sweep retries.
		if _, err := service.MatchSession(c.Context(), h.DB, s); err != nil {
			h.Log.Warn().Err(err).Str("session_id", s.ID).Msg("matching session to scheduled patrol failed")
		}
	}
	return created(c, isNew, s)
}

// POST /patrol/sessions/:id/scans {id?, checkpoint_id | qr_code | qr_payload, method, photo_object_key?, scanned_at?, gps_lat?, gps_lng?}
func (h *Handler) recordScan(c *fiber.Ctx) error {
	var b struct {
		ID             *string    `json:"id"`
		CheckpointID   string     `json:"checkpoint_id"`
		QRCode         string     `json:"qr_code"`
		QRPayload      string     `json:"qr_payload"`
		Method         string     `json:"method"`
		PhotoObjectKey *string    `json:"photo_object_key"`
		ScannedAt      *time.Time `json:"scanned_at"`
		GPSLat         *float64   `json:"gps_lat"`
		GPSLng         *float64   `json:"gps_lng"`
	}
	if err := parse(c, &b); err != nil {
		return h.fail(c, err)
	}
	if err := optionalUUID(b.ID); err != nil {
		return h.fail(c, err)
	}
	if b.CheckpointID == "" && b.QRCode == "" && b.QRPayload == "" {
		return h.fail(c, service.BadRequest("checkpoint_required", "checkpoint_id, qr_code or qr_payload is required"))
	}
	if err := service.CheckLengths(service.S("checkpoint_id", b.CheckpointID, service.MaxIdentifier),
		service.S("qr_code", b.QRCode, service.MaxName), service.S("qr_payload", b.QRPayload, service.MaxName)); err != nil {
		return h.fail(c, err)
	}
	site, err := h.deviceSite(c)
	if err != nil {
		return h.fail(c, err)
	}
	scan, isNew, err := service.RecordScan(c.Context(), h.DB, service.ScanInput{
		ID: nilIfEmpty(b.ID), SessionID: c.Params("id"), CheckpointID: b.CheckpointID, QRCode: b.QRCode,
		QRPayload: b.QRPayload, Method: b.Method, PhotoObjectKey: nilIfEmpty(b.PhotoObjectKey), At: b.ScannedAt,
		GPSLat: b.GPSLat, GPSLng: b.GPSLng, Guard: middleware.GetGuard(c), Auth: middleware.GetGuardAuth(c),
		Site: site, DeviceID: deviceID(c), QR: h.qr,
	})
	if err != nil {
		return h.fail(c, err)
	}
	return created(c, isNew, scan)
}

// POST /patrol/sessions/:id/end {ended_at?, abandoned?}
func (h *Handler) endSession(c *fiber.Ctx) error {
	var b struct {
		EndedAt   *time.Time `json:"ended_at"`
		Abandoned bool       `json:"abandoned"`
	}
	if len(c.Body()) > 0 {
		if err := parse(c, &b); err != nil {
			return h.fail(c, err)
		}
	}
	site, err := h.deviceSite(c)
	if err != nil {
		return h.fail(c, err)
	}
	s, err := service.EndSession(c.Context(), h.DB, c.Params("id"), site, b.EndedAt, b.Abandoned)
	if err != nil {
		return h.fail(c, err)
	}
	return c.JSON(s)
}
