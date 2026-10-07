package handler

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"

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
	inc, isNew, err := repository.CreateIncident(c.Context(), h.DB, model.Incident{
		SiteID: site.ID, GuardID: guard.ID, DeviceID: deviceID(c), Type: b.Type, Severity: b.Severity,
		Description: nilIfEmpty(b.Description), Location: nilIfEmpty(b.Location),
		PhotoObjectKeys: b.PhotoObjectKeys, OccurredAt: at,
	}, nilIfEmpty(b.ID))
	if err != nil {
		return h.fail(c, err)
	}
	if isNew && (inc.Severity == "high" || inc.Severity == "critical") {
		h.publish(c.Context(), notification.Event{
			Type: notification.EventIncidentHigh, Site: site, DedupeKey: "incident:" + inc.ID,
			Params: map[string]string{
				"guard": guard.Name, "severity": strings.ToUpper(inc.Severity), "type": inc.Type,
				"location": deref(inc.Location), "description": deref(inc.Description),
				"time": inc.OccurredAt.In(site.Location()).Format("02 Jan 15:04"),
			},
		})
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
	for k, v := range b.Changes {
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
	alert, isNew, err := repository.CreateSosAlert(c.Context(), h.DB, model.SosAlert{
		GuardID: guard.ID, SiteID: site.ID, DeviceID: deviceID(c), FiredAt: at, GPSLat: b.GPSLat, GPSLng: b.GPSLng,
	}, nilIfEmpty(b.ID))
	if err != nil {
		return h.fail(c, err)
	}
	if isNew {
		h.publish(c.Context(), notification.Event{
			Type: notification.EventSOS, Site: site, DedupeKey: "sos:" + alert.ID,
			Params: map[string]string{
				"guard": guard.Name, "time": alert.FiredAt.In(site.Location()).Format("02 Jan 15:04:05"),
				"location": fmtGPS(alert.GPSLat, alert.GPSLng),
			},
		})
	}
	return created(c, isNew, alert)
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
	d, isNew, err := repository.CreateDefect(c.Context(), h.DB, model.DefectReport{
		SiteID: site.ID, GuardID: guard.ID, DeviceID: deviceID(c), DefectType: b.DefectType,
		Location: nilIfEmpty(b.Location), Description: nilIfEmpty(b.Description),
		PhotoObjectKey: nilIfEmpty(b.PhotoObjectKey), CreatedAt: at,
	}, nilIfEmpty(b.ID))
	if err != nil {
		return h.fail(c, err)
	}
	if isNew {
		h.publish(c.Context(), notification.Event{
			Type: notification.EventDefectOpened, Site: site, DedupeKey: "defect:" + d.ID,
			Params: map[string]string{
				"guard": guard.Name, "defect": deref(d.DefectType), "location": deref(d.Location),
				"description": deref(d.Description),
			},
		})
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
	v, isNew, err := repository.CreateVisitor(c.Context(), h.DB, model.VisitorLog{
		SiteID: site.ID, GuardID: middleware.GetGuard(c).ID, DeviceID: deviceID(c), VisitorName: b.VisitorName,
		IDType: nilIfEmpty(b.IDType), IDNumber: nilIfEmpty(b.IDNumber), Plate: nilIfEmpty(b.Plate),
		HostName: nilIfEmpty(b.HostName), EntryAt: &entry, Purpose: nilIfEmpty(b.Purpose),
		PhotoObjectKey: nilIfEmpty(b.PhotoObjectKey),
	}, nilIfEmpty(b.ID))
	if err != nil {
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

// ── Media upload ─────────────────────────────────────────────────────────────

// MaxPhotoBytes caps a single photo upload.
const MaxPhotoBytes = 10 << 20

var imageExt = map[string]string{"image/jpeg": "jpg", "image/png": "png", "image/webp": "webp"}

// POST /media (tablet, multipart) fields: file, kind, id? — returns the object
// key to reference from records. Re-uploading the same id overwrites, so
// retries are safe.
func (h *Handler) uploadMedia(c *fiber.Ctx) error {
	site, err := h.deviceSite(c)
	if err != nil {
		return h.fail(c, err)
	}
	kind := c.FormValue("kind")
	if !slices.Contains(service.MediaKinds, kind) {
		return h.fail(c, service.BadRequest("invalid_kind", "kind must be one of %s", strings.Join(service.MediaKinds, ", ")))
	}
	id := c.FormValue("id")
	if id == "" {
		id = uuid.NewString()
	} else if err := optionalUUID(&id); err != nil {
		return h.fail(c, err)
	}
	fh, err := c.FormFile("file")
	if err != nil {
		return h.fail(c, service.BadRequest("file_required", "multipart field 'file' is required"))
	}
	if fh.Size > MaxPhotoBytes {
		return h.fail(c, service.BadRequest("file_too_large", "photo exceeds %d MB", MaxPhotoBytes>>20))
	}
	f, err := fh.Open()
	if err != nil {
		return h.fail(c, err)
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, MaxPhotoBytes+1))
	if err != nil {
		return h.fail(c, err)
	}
	ct := http.DetectContentType(data)
	ext, ok := imageExt[ct]
	if !ok {
		return h.fail(c, service.BadRequest("invalid_file_type", "only JPEG, PNG or WebP images are accepted"))
	}
	key := service.ObjectKey(site.ID, kind, id, ext)
	if _, err := h.Storage.Put(c.Context(), h.Cfg.MinioBucketPhotos, key, bytes.NewReader(data), int64(len(data)), ct); err != nil {
		return h.fail(c, err)
	}
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{"object_key": key, "content_type": ct, "size": len(data)})
}
