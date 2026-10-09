package handler

import (
	"errors"
	"strconv"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/securepatrol/backend/internal/middleware"
	"github.com/securepatrol/backend/internal/notification"
	"github.com/securepatrol/backend/internal/repository"
	"github.com/securepatrol/backend/internal/service"
)

// GET /attendance/daily?site_id=&date=YYYY-MM-DD — every scheduled shift of the
// site-local day with its attendance outcome (FR-REP-01 data).
func (h *Handler) dailyAttendance(c *fiber.Ctx) error {
	site, err := h.siteFor(c, c.Query("site_id"))
	if err != nil {
		return h.fail(c, err)
	}
	from, to, err := service.DayRange(c.Query("date"), site.Location())
	if err != nil {
		return h.fail(c, err)
	}
	rows, err := repository.DailyAttendance(c.Context(), h.DB, site.ID, from, to)
	if err != nil {
		return h.fail(c, err)
	}
	return c.JSON(fiber.Map{"site_id": site.ID, "date": from.In(site.Location()).Format("2006-01-02"), "rows": rows})
}

// GET /attendance/review?site_id= — records awaiting supervisor review (FR-OFF-02).
func (h *Handler) reviewQueue(c *fiber.Ctx) error {
	site, err := h.siteFor(c, c.Query("site_id"))
	if err != nil {
		return h.fail(c, err)
	}
	rows, err := repository.ListReviewQueue(c.Context(), h.DB, site.ID)
	if err != nil {
		return h.fail(c, err)
	}
	return c.JSON(rows)
}

// POST /attendance/:id/review {note}
func (h *Handler) markReviewed(c *fiber.Ctx) error {
	rec, err := repository.GetAttendance(c.Context(), h.DB, c.Params("id"))
	if err != nil {
		return h.fail(c, err)
	}
	if _, err := h.siteFor(c, rec.SiteID); err != nil {
		return h.fail(c, err)
	}
	var b struct {
		Note *string `json:"note"`
	}
	if len(c.Body()) > 0 {
		if err := parse(c, &b); err != nil {
			return h.fail(c, err)
		}
	}
	if err := service.CheckLengths(service.F("note", b.Note, service.MaxReason)); err != nil {
		return h.fail(c, err)
	}
	out, err := repository.MarkAttendanceReviewed(c.Context(), h.DB, rec.ID, actor(c), nilIfEmpty(b.Note))
	if err != nil {
		return h.fail(c, err)
	}
	return c.JSON(out)
}

// PUT /attendance/shifts/:shiftId/override {status, reason, clock_in_at?, clock_out_at?}
func (h *Handler) overrideAttendance(c *fiber.Ctx) error {
	shift, err := h.shiftFor(c, c.Params("shiftId"))
	if err != nil {
		return h.fail(c, err)
	}
	before, err := repository.GetAttendanceByShift(c.Context(), h.DB, shift.ID)
	if err != nil && !errors.Is(err, repository.ErrNotFound) {
		return h.fail(c, err)
	}
	middleware.SetAuditBefore(c, before) // null when the shift had no record
	var b struct {
		Status     string     `json:"status"`
		Reason     string     `json:"reason"`
		ClockInAt  *time.Time `json:"clock_in_at"`
		ClockOutAt *time.Time `json:"clock_out_at"`
	}
	if err := parse(c, &b); err != nil {
		return h.fail(c, err)
	}
	if err := service.CheckLengths(service.S("reason", b.Reason, service.MaxReason)); err != nil {
		return h.fail(c, err)
	}
	out, err := service.OverrideAttendance(c.Context(), h.DB, shift, service.OverrideInput{
		ShiftID: shift.ID, Status: b.Status, Reason: b.Reason,
		ClockInAt: b.ClockInAt, ClockOutAt: b.ClockOutAt, Actor: actor(c),
	})
	if err != nil {
		return h.fail(c, err)
	}
	return c.JSON(out)
}

// POST /attendance/clock-in {id?, clock_in_at?, selfie_object_key?, gps_lat?, gps_lng?}
func (h *Handler) clockIn(c *fiber.Ctx) error {
	var b struct {
		ID              *string    `json:"id"`
		ClockInAt       *time.Time `json:"clock_in_at"`
		SelfieObjectKey *string    `json:"selfie_object_key"`
		GPSLat          *float64   `json:"gps_lat"`
		GPSLng          *float64   `json:"gps_lng"`
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
	guard := middleware.GetGuard(c)
	res, err := service.ClockIn(c.Context(), h.DB, service.ClockInInput{
		ID: nilIfEmpty(b.ID), At: b.ClockInAt, Guard: guard, Site: site, DeviceID: deviceID(c),
		SelfieObjectKey: nilIfEmpty(b.SelfieObjectKey), GPSLat: b.GPSLat, GPSLng: b.GPSLng,
		DeviceLastSeen: middleware.GetDevice(c).LastSeenAt,
	})
	if err != nil {
		return h.fail(c, err)
	}
	if res.Created && res.Record.Status == "late" && res.Shift != nil {
		h.publish(c.Context(), notification.Event{
			Type: notification.EventLateClockIn, Site: site, DedupeKey: "late:" + res.Record.ID,
			Params: map[string]string{
				"guard":   guard.Name,
				"minutes": strconv.Itoa(res.Record.LateMinutes),
				"shift":   res.Shift.StartTime.In(site.Location()).Format("02 Jan 15:04"),
			},
		})
	}
	return created(c, res.Created, res.Record)
}

// POST /attendance/clock-out {attendance_id?, clock_out_at?, gps_lat?, gps_lng?}
func (h *Handler) clockOut(c *fiber.Ctx) error {
	var b struct {
		AttendanceID *string    `json:"attendance_id"`
		ClockOutAt   *time.Time `json:"clock_out_at"`
		GPSLat       *float64   `json:"gps_lat"`
		GPSLng       *float64   `json:"gps_lng"`
	}
	if err := parse(c, &b); err != nil {
		return h.fail(c, err)
	}
	if err := optionalUUID(b.AttendanceID); err != nil {
		return h.fail(c, err)
	}
	site, err := h.deviceSite(c)
	if err != nil {
		return h.fail(c, err)
	}
	rec, _, err := service.ClockOut(c.Context(), h.DB, service.ClockOutInput{
		AttendanceID: nilIfEmpty(b.AttendanceID), At: b.ClockOutAt, Guard: middleware.GetGuard(c),
		Site: site, GPSLat: b.GPSLat, GPSLng: b.GPSLng,
	})
	if err != nil {
		return h.fail(c, err)
	}
	return c.JSON(rec)
}
