package handler

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5"

	"github.com/securepatrol/backend/internal/middleware"
	"github.com/securepatrol/backend/internal/model"
	"github.com/securepatrol/backend/internal/notification"
	"github.com/securepatrol/backend/internal/repository"
	"github.com/securepatrol/backend/internal/service"
)

var (
	severities    = []string{"low", "medium", "high", "critical"}
	workflowState = []string{"open", "in_progress", "resolved"}
)

func fmtGPS(lat, lng *float64) string {
	if lat == nil || lng == nil {
		return ""
	}
	return fmt.Sprintf("%.6f,%.6f (https://maps.google.com/?q=%.6f,%.6f)", *lat, *lng, *lat, *lng)
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// ── Incidents ────────────────────────────────────────────────────────────────

// incidentFilter parses the common list/report filters.
func (h *Handler) incidentFilter(c *fiber.Ctx) (*model.Site, repository.IncidentFilter, error) {
	site, err := h.siteFor(c, c.Query("site_id"))
	if err != nil {
		return nil, repository.IncidentFilter{}, err
	}
	f := repository.IncidentFilter{
		SiteID: site.ID, Type: c.Query("type"), Severity: c.Query("severity"), Status: c.Query("status"),
	}
	if c.Query("from") != "" || c.Query("to") != "" {
		from, to, err := service.DateRange(c.Query("from"), c.Query("to"), site.Location(), 366)
		if err != nil {
			return nil, f, err
		}
		f.From, f.To = &from, &to
	}
	return site, f, nil
}

func (h *Handler) listIncidents(c *fiber.Ctx) error {
	_, f, err := h.incidentFilter(c)
	if err != nil {
		return h.fail(c, err)
	}
	rows, err := repository.ListIncidents(c.Context(), h.DB, f)
	if err != nil {
		return h.fail(c, err)
	}
	return c.JSON(rows)
}

// GET /incidents/:id — original record, photos and amendments.
func (h *Handler) getIncident(c *fiber.Ctx) error {
	inc, err := repository.GetIncident(c.Context(), h.DB, c.Params("id"))
	if err != nil {
		return h.fail(c, err)
	}
	if _, err := h.siteFor(c, inc.SiteID); err != nil {
		return h.fail(c, err)
	}
	amendments, err := repository.ListAmendments(c.Context(), h.DB, inc.ID)
	if err != nil {
		return h.fail(c, err)
	}
	guardName := ""
	if g, err := repository.GetGuard(c.Context(), h.DB, inc.GuardID); err == nil {
		guardName = g.Name
	}
	return c.JSON(fiber.Map{
		"incident": inc, "guard_name": guardName, "amendments": amendments,
		"photos": h.photos(c.Context(), inc.PhotoObjectKeys...),
	})
}

// POST /incidents (tablet) {id?, type, severity, description?, location?, photo_object_keys[], occurred_at?}
func (h *Handler) createIncident(c *fiber.Ctx) error {
	var b struct {
		ID              *string    `json:"id"`
		Type            string     `json:"type"`
		Severity        string     `json:"severity"`
		Description     *string    `json:"description"`
		Location        *string    `json:"location"`
		PhotoObjectKeys []string   `json:"photo_object_keys"`
		OccurredAt      *time.Time `json:"occurred_at"`
	}
	if err := parse(c, &b); err != nil {
		return h.fail(c, err)
	}
	if err := optionalUUID(b.ID); err != nil {
		return h.fail(c, err)
	}
	b.Type = strings.TrimSpace(strings.ToLower(b.Type))
	if b.Type == "" {
		return h.fail(c, service.BadRequest("type_required", "type (category) is required"))
	}
	if err := service.CheckLengths(service.S("type", b.Type, service.MaxShortText),
		service.F("description", b.Description, service.MaxDescription),
		service.F("location", b.Location, service.MaxLocation)); err != nil {
		return h.fail(c, err)
	}
	if b.Severity == "" {
		b.Severity = "low"
	}
	if !slices.Contains(severities, b.Severity) {
		return h.fail(c, service.BadRequest("invalid_severity", "severity must be low, medium, high or critical"))
	}
	if b.PhotoObjectKeys == nil {
		b.PhotoObjectKeys = []string{}
	}
	if len(b.PhotoObjectKeys) > service.MaxIncidentPhotos {
		return h.fail(c, service.BadRequest("too_many_photos", "at most %d photos per incident", service.MaxIncidentPhotos))
	}
	site, err := h.deviceSite(c)
	if err != nil {
		return h.fail(c, err)
	}
	if err := service.CheckObjectKeys(b.PhotoObjectKeys, site.ID); err != nil {
		return h.fail(c, err)
	}
	at, err := service.ClientTime(b.OccurredAt)
	if err != nil {
		return h.fail(c, err)
	}
	guard := middleware.GetGuard(c)
	var inc *model.Incident
	var isNew bool
	err = h.writeWithEvents(c, func(tx pgx.Tx) ([]notification.Event, error) {
		var err error
		inc, isNew, err = repository.CreateIncident(c.Context(), tx, model.Incident{
			SiteID: site.ID, GuardID: guard.ID, DeviceID: deviceID(c), Type: b.Type, Severity: b.Severity,
			Description: nilIfEmpty(b.Description), Location: nilIfEmpty(b.Location),
			PhotoObjectKeys: b.PhotoObjectKeys, OccurredAt: at,
			Flags: append(service.TimeFlags(at, service.Now()), service.GuardFlags(middleware.GetGuardAuth(c), at)...),
		}, nilIfEmpty(b.ID))
		if err != nil {
			return nil, err
		}
		if err := service.CheckReplay(inc, isNew, site.ID, guard.ID); err != nil {
			return nil, err
		}
		if (inc.Severity != "high" && inc.Severity != "critical") || (!isNew && !replayFresh(inc.CreatedAt)) {
			return nil, nil
		}
		return []notification.Event{{
			Type: notification.EventIncidentHigh, Site: site, DedupeKey: "incident:" + inc.ID,
			Params: map[string]string{
				"guard": inc.GuardName, "severity": strings.ToUpper(inc.Severity), "type": inc.Type,
				"location": deref(inc.Location), "description": deref(inc.Description),
				"time": inc.OccurredAt.In(site.Location()).Format("02 Jan 15:04"),
			},
		}}, nil
	})
	if err != nil {
		return h.fail(c, err)
	}
	return created(c, isNew, inc)
}

// PUT /incidents/:id/status {status}
func (h *Handler) updateIncidentStatus(c *fiber.Ctx) error {
	inc, err := repository.GetIncident(c.Context(), h.DB, c.Params("id"))
	if err != nil {
		return h.fail(c, err)
	}
	if _, err := h.siteFor(c, inc.SiteID); err != nil {
		return h.fail(c, err)
	}
	var b struct {
		Status string `json:"status"`
	}
	if err := parse(c, &b); err != nil {
		return h.fail(c, err)
	}
	if !slices.Contains(workflowState, b.Status) {
		return h.fail(c, service.BadRequest("invalid_status", "status must be open, in_progress or resolved"))
	}
	out, err := repository.UpdateIncidentStatus(c.Context(), h.DB, inc.ID, b.Status)
	if err != nil {
		return h.fail(c, err)
	}
	return c.JSON(out)
}

// POST /incidents/:id/amendments {changes: {type?, severity?, description?, location?}, reason}
// The original incident stays untouched (FR-INC-05).
func (h *Handler) amendIncident(c *fiber.Ctx) error {
	inc, err := repository.GetIncident(c.Context(), h.DB, c.Params("id"))
	if err != nil {
		return h.fail(c, err)
	}
	if _, err := h.siteFor(c, inc.SiteID); err != nil {
		return h.fail(c, err)
	}
	var b struct {
		Changes map[string]string `json:"changes"`
		Reason  string            `json:"reason"`
	}
	if err := parse(c, &b); err != nil {
		return h.fail(c, err)
	}
	if strings.TrimSpace(b.Reason) == "" {
		return h.fail(c, service.BadRequest("reason_required", "a reason is required"))
	}
	if len(b.Changes) == 0 {
		return h.fail(c, service.BadRequest("changes_required", "no changes given"))
	}
	if err := service.CheckLengths(service.S("reason", b.Reason, service.MaxReason)); err != nil {
		return h.fail(c, err)
	}
	amendMax := map[string]int{"type": service.MaxShortText, "description": service.MaxDescription, "location": service.MaxLocation}
	for k, v := range b.Changes {
		if max, ok := amendMax[k]; ok {
			if err := service.CheckLengths(service.S("changes."+k, v, max)); err != nil {
				return h.fail(c, err)
			}
		}
		switch k {
		case "type", "description", "location":
		case "severity":
			if !slices.Contains(severities, v) {
				return h.fail(c, service.BadRequest("invalid_severity", "invalid severity"))
			}
		default:
			return h.fail(c, service.BadRequest("invalid_field", "field %q cannot be amended", k))
		}
	}
	raw, _ := json.Marshal(b.Changes)
	a, err := repository.CreateAmendment(c.Context(), h.DB, inc.ID, raw, strings.TrimSpace(b.Reason), actor(c))
	if err != nil {
		return h.fail(c, err)
	}
	return c.Status(fiber.StatusCreated).JSON(a)
}

// ── SOS ──────────────────────────────────────────────────────────────────────

func (h *Handler) listSos(c *fiber.Ctx) error {
	site, err := h.siteFor(c, c.Query("site_id"))
	if err != nil {
		return h.fail(c, err)
	}
	rows, err := repository.ListSosAlerts(c.Context(), h.DB, site.ID, c.Query("unack_only") == "true")
	if err != nil {
		return h.fail(c, err)
	}
	return c.JSON(rows)
}

// POST /sos (tablet) {id?, fired_at?, gps_lat?, gps_lng?} — tablets retry every
// 10 s until they get a 2xx; repeats with the same id return the same alert.
func (h *Handler) createSos(c *fiber.Ctx) error {
	var b struct {
		ID      *string    `json:"id"`
		FiredAt *time.Time `json:"fired_at"`
		GPSLat  *float64   `json:"gps_lat"`
		GPSLng  *float64   `json:"gps_lng"`
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
	at, err := service.ClientTime(b.FiredAt)
	if err != nil {
		return h.fail(c, err)
	}
	guard := middleware.GetGuard(c)
	// Retries of a known alert are never throttled; new alerts are limited
	// per tablet (RATE_LIMIT_SOS_PER_MIN) against flooding.
	if b.ID == nil || *b.ID == "" || !h.sosExists(c, *b.ID) {
		if ok, retry := h.limiter.Allow(c.Context(), "sos:"+*deviceID(c), h.Cfg.RateLimitSOS); !ok {
			return middleware.TooManyRequests(c, retry)
		}
	}
	var alert *model.SosAlert
	var isNew bool
	// The alert and its deliveries commit together; a replay (tablet retry)
	// fills in deliveries that are missing, so an SOS is never left
	// un-notified (audit BE-R1).
	err = h.writeWithEvents(c, func(tx pgx.Tx) ([]notification.Event, error) {
		var err error
		alert, isNew, err = repository.CreateSosAlert(c.Context(), tx, model.SosAlert{
			GuardID: guard.ID, SiteID: site.ID, DeviceID: deviceID(c), FiredAt: at, GPSLat: b.GPSLat, GPSLng: b.GPSLng,
			Flags: append(service.TimeFlags(at, service.Now()), service.GuardFlags(middleware.GetGuardAuth(c), at)...),
		}, nilIfEmpty(b.ID))
		if err != nil {
			return nil, err
		}
		if err := service.CheckReplay(alert, isNew, site.ID, guard.ID); err != nil {
			return nil, err
		}
		if !isNew && !replayFresh(alert.ReceivedAt) {
			return nil, nil
		}
		return []notification.Event{{
			Type: notification.EventSOS, Site: site, DedupeKey: "sos:" + alert.ID,
			Params: map[string]string{
				"guard": alert.GuardName, "time": alert.FiredAt.In(site.Location()).Format("02 Jan 15:04:05"),
				"location": fmtGPS(alert.GPSLat, alert.GPSLng),
			},
		}}, nil
	})
	if err != nil {
		return h.fail(c, err)
	}
	return created(c, isNew, alert)
}

// sosExists reports whether an SOS alert with this ID is already stored.
func (h *Handler) sosExists(c *fiber.Ctx, id string) bool {
	_, err := repository.GetSosAlert(c.Context(), h.DB, id)
	return err == nil
}

// POST /sos/:id/acknowledge — supervisors/admins on the web, or a supervisor
// guard on the site's tablet.
func (h *Handler) acknowledgeSos(c *fiber.Ctx) error {
	alert, err := repository.GetSosAlert(c.Context(), h.DB, c.Params("id"))
	if err != nil {
		return h.fail(c, err)
	}
	if _, err := h.siteFor(c, alert.SiteID); err != nil {
		return h.fail(c, err)
	}
	if g := middleware.GetGuard(c); g != nil && g.Role != "supervisor" {
		return h.fail(c, service.Forbidden("supervisor_only", "only a supervisor can acknowledge SOS alerts"))
	}
	out, err := repository.AcknowledgeSosAlert(c.Context(), h.DB, alert.ID, actor(c))
	if err != nil {
		return h.fail(c, err)
	}
	return c.JSON(out)
}

// ── Defects ──────────────────────────────────────────────────────────────────

func (h *Handler) listDefects(c *fiber.Ctx) error {
	site, err := h.siteFor(c, c.Query("site_id"))
	if err != nil {
		return h.fail(c, err)
	}
	rows, err := repository.ListDefects(c.Context(), h.DB, site.ID, c.Query("status"))
	if err != nil {
		return h.fail(c, err)
	}
	return c.JSON(rows)
}

// POST /defects (tablet) {id?, defect_type, location?, description?, photo_object_key?, reported_at?}
func (h *Handler) createDefect(c *fiber.Ctx) error {
	var b struct {
		ID             *string    `json:"id"`
		DefectType     *string    `json:"defect_type"`
		Location       *string    `json:"location"`
		Description    *string    `json:"description"`
		PhotoObjectKey *string    `json:"photo_object_key"`
		ReportedAt     *time.Time `json:"reported_at"`
	}
	if err := parse(c, &b); err != nil {
		return h.fail(c, err)
	}
	if err := optionalUUID(b.ID); err != nil {
		return h.fail(c, err)
	}
	if deref(b.DefectType) == "" {
		return h.fail(c, service.BadRequest("defect_type_required", "defect_type is required"))
	}
	if err := service.CheckLengths(service.F("defect_type", b.DefectType, service.MaxShortText),
		service.F("location", b.Location, service.MaxLocation),
		service.F("description", b.Description, service.MaxDescription)); err != nil {
		return h.fail(c, err)
	}
	site, err := h.deviceSite(c)
	if err != nil {
		return h.fail(c, err)
	}
	if err := service.CheckObjectKey(b.PhotoObjectKey, site.ID); err != nil {
		return h.fail(c, err)
	}
	at, err := service.ClientTime(b.ReportedAt)
	if err != nil {
		return h.fail(c, err)
	}
	guard := middleware.GetGuard(c)
	var d *model.DefectReport
	var isNew bool
	err = h.writeWithEvents(c, func(tx pgx.Tx) ([]notification.Event, error) {
		var err error
		d, isNew, err = repository.CreateDefect(c.Context(), tx, model.DefectReport{
			SiteID: site.ID, GuardID: guard.ID, DeviceID: deviceID(c), DefectType: b.DefectType,
			Location: nilIfEmpty(b.Location), Description: nilIfEmpty(b.Description),
			PhotoObjectKey: nilIfEmpty(b.PhotoObjectKey), CreatedAt: at,
			Flags: service.GuardFlags(middleware.GetGuardAuth(c), at),
		}, nilIfEmpty(b.ID))
		if err != nil {
			return nil, err
		}
		if err := service.CheckReplay(d, isNew, site.ID, guard.ID); err != nil {
			return nil, err
		}
		if !isNew && !replayFresh(d.UpdatedAt) {
			return nil, nil
		}
		return []notification.Event{{
			Type: notification.EventDefectOpened, Site: site, DedupeKey: "defect:" + d.ID,
			Params: map[string]string{
				"guard": d.GuardName, "defect": deref(d.DefectType), "location": deref(d.Location),
				"description": deref(d.Description),
			},
		}}, nil
	})
	if err != nil {
		return h.fail(c, err)
	}
	return created(c, isNew, d)
}

// PUT /defects/:id/status {status}
func (h *Handler) updateDefectStatus(c *fiber.Ctx) error {
	d, err := repository.GetDefect(c.Context(), h.DB, c.Params("id"))
	if err != nil {
		return h.fail(c, err)
	}
	if _, err := h.siteFor(c, d.SiteID); err != nil {
		return h.fail(c, err)
	}
	var b struct {
		Status string `json:"status"`
	}
	if err := parse(c, &b); err != nil {
		return h.fail(c, err)
	}
	if !slices.Contains(workflowState, b.Status) {
		return h.fail(c, service.BadRequest("invalid_status", "status must be open, in_progress or resolved"))
	}
	out, err := repository.UpdateDefectStatus(c.Context(), h.DB, d.ID, b.Status)
	if err != nil {
		return h.fail(c, err)
	}
	return c.JSON(out)
}

// ── Visitors ─────────────────────────────────────────────────────────────────

func (h *Handler) listVisitors(c *fiber.Ctx) error {
	site, err := h.siteFor(c, c.Query("site_id"))
	if err != nil {
		return h.fail(c, err)
	}
	from, to, err := service.DateRange(c.Query("from"), c.Query("to"), site.Location(), 92)
	if err != nil {
		return h.fail(c, err)
	}
	rows, err := repository.ListVisitors(c.Context(), h.DB, site.ID, from, to)
	if err != nil {
		return h.fail(c, err)
	}
	return c.JSON(rows)
}

// POST /visitors (tablet)
func (h *Handler) createVisitor(c *fiber.Ctx) error {
	var b struct {
		ID             *string    `json:"id"`
		VisitorName    string     `json:"visitor_name"`
		IDType         *string    `json:"id_type"`
		IDNumber       *string    `json:"id_number"`
		Plate          *string    `json:"plate"`
		HostName       *string    `json:"host_name"`
		EntryAt        *time.Time `json:"entry_at"`
		Purpose        *string    `json:"purpose"`
		PhotoObjectKey *string    `json:"photo_object_key"`
	}
	if err := parse(c, &b); err != nil {
		return h.fail(c, err)
	}
	if err := optionalUUID(b.ID); err != nil {
		return h.fail(c, err)
	}
	b.VisitorName = strings.TrimSpace(b.VisitorName)
	if b.VisitorName == "" {
		return h.fail(c, service.BadRequest("visitor_name_required", "visitor_name is required"))
	}
	if err := service.CheckLengths(service.S("visitor_name", b.VisitorName, service.MaxName),
		service.F("id_type", b.IDType, service.MaxShortText), service.F("id_number", b.IDNumber, service.MaxIdentifier),
		service.F("plate", b.Plate, service.MaxIdentifier), service.F("host_name", b.HostName, service.MaxName),
		service.F("purpose", b.Purpose, service.MaxReason)); err != nil {
		return h.fail(c, err)
	}
	site, err := h.deviceSite(c)
	if err != nil {
		return h.fail(c, err)
	}
	if err := service.CheckObjectKey(b.PhotoObjectKey, site.ID); err != nil {
		return h.fail(c, err)
	}
	entry, err := service.ClientTime(b.EntryAt)
	if err != nil {
		return h.fail(c, err)
	}
	guard := middleware.GetGuard(c)
	v, isNew, err := repository.CreateVisitor(c.Context(), h.DB, model.VisitorLog{
		SiteID: site.ID, GuardID: guard.ID, DeviceID: deviceID(c), VisitorName: b.VisitorName,
		IDType: nilIfEmpty(b.IDType), IDNumber: nilIfEmpty(b.IDNumber), Plate: nilIfEmpty(b.Plate),
		HostName: nilIfEmpty(b.HostName), EntryAt: &entry, Purpose: nilIfEmpty(b.Purpose),
		PhotoObjectKey: nilIfEmpty(b.PhotoObjectKey), Flags: service.GuardFlags(middleware.GetGuardAuth(c), entry),
	}, nilIfEmpty(b.ID))
	if err != nil {
		return h.fail(c, err)
	}
	if err := service.CheckReplay(v, isNew, site.ID, guard.ID); err != nil {
		return h.fail(c, err)
	}
	return created(c, isNew, v)
}

// POST /visitors/:id/exit {exit_at?}
func (h *Handler) visitorExit(c *fiber.Ctx) error {
	v, err := repository.GetVisitor(c.Context(), h.DB, c.Params("id"))
	if err != nil {
		return h.fail(c, err)
	}
	if _, err := h.siteFor(c, v.SiteID); err != nil {
		return h.fail(c, err)
	}
	var b struct {
		ExitAt *time.Time `json:"exit_at"`
	}
	if len(c.Body()) > 0 {
		if err := parse(c, &b); err != nil {
			return h.fail(c, err)
		}
	}
	at, err := service.ClientTime(b.ExitAt)
	if err != nil {
		return h.fail(c, err)
	}
	if v.EntryAt != nil && at.Before(*v.EntryAt) {
		return h.fail(c, service.BadRequest("exit_before_entry", "exit time is before entry time"))
	}
	out, err := repository.SetVisitorExit(c.Context(), h.DB, v.ID, at)
	if err != nil {
		return h.fail(c, err)
	}
	return c.JSON(out)
}
