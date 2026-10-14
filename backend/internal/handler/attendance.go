package handler

import (
	"context"
	"errors"
	"strconv"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5"

	"github.com/securepatrol/backend/internal/middleware"
	"github.com/securepatrol/backend/internal/model"
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
	// The record (locked), the override and its history row (F6) change in
	// one transaction, so concurrent overrides cannot lose a before state.
	var out *model.AttendanceRecord
	err = pgx.BeginFunc(c.Context(), h.DB, func(tx pgx.Tx) error {
		before, err := repository.GetAttendanceByShift(c.Context(), tx, shift.ID)
		if err != nil && !errors.Is(err, repository.ErrNotFound) {
			return err
		}
		middleware.SetAuditBefore(c, before) // null when the shift had no record
		out, err = service.OverrideAttendance(c.Context(), tx, shift, service.OverrideInput{
			ShiftID: shift.ID, Status: b.Status, Reason: b.Reason,
			ClockInAt: b.ClockInAt, ClockOutAt: b.ClockOutAt, Actor: actor(c),
		})
		if err != nil {
			return err
		}
		_, err = repository.InsertAttendanceOverride(c.Context(), tx, before, out, *out.OverrideReason, actor(c))
		return err
	})
	if err != nil {
		return h.fail(c, err)
	}
	return c.JSON(out)
}

// GET /attendance/:id/history — override history of an attendance record,
// oldest first, with the values before and after each override (F6).
func (h *Handler) attendanceHistory(c *fiber.Ctx) error {
	rec, err := repository.GetAttendance(c.Context(), h.DB, c.Params("id"))
	if err != nil {
		return h.fail(c, err)
	}
	if _, err := h.siteFor(c, rec.SiteID); err != nil {
		return h.fail(c, err)
	}
	rows, err := repository.ListAttendanceOverrides(c.Context(), h.DB, rec.ID)
	if err != nil {
		return h.fail(c, err)
	}
	return c.JSON(fiber.Map{"attendance": rec, "overrides": rows})
}

// GET /attendance/shifts/:shiftId/history — the same, addressed by shift
// (a shift without a record answers an empty history).
func (h *Handler) shiftAttendanceHistory(c *fiber.Ctx) error {
	shift, err := h.shiftFor(c, c.Params("shiftId"))
	if err != nil {
		return h.fail(c, err)
	}
	rec, err := repository.GetAttendanceByShift(c.Context(), h.DB, shift.ID)
	if errors.Is(err, repository.ErrNotFound) {
		return c.JSON(fiber.Map{"attendance": nil, "overrides": []model.AttendanceOverride{}})
	}
	if err != nil {
		return h.fail(c, err)
	}
	rows, err := repository.ListAttendanceOverrides(c.Context(), h.DB, rec.ID)
	if err != nil {
		return h.fail(c, err)
	}
	return c.JSON(fiber.Map{"attendance": rec, "overrides": rows})
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
	var staged notification.Staged
	res, err := service.ClockIn(c.Context(), h.DB, service.ClockInInput{
		ID: nilIfEmpty(b.ID), At: b.ClockInAt, Guard: guard, Auth: middleware.GetGuardAuth(c), Site: site, DeviceID: deviceID(c),
		SelfieObjectKey: nilIfEmpty(b.SelfieObjectKey), GPSLat: b.GPSLat, GPSLng: b.GPSLng,
		DeviceLastSeen: middleware.GetDevice(c).LastSeenAt,
		// The late notification is staged in the clock-in transaction (BE-R1);
		// a replay fills it in if it is missing.
		AfterWrite: func(ctx context.Context, tx pgx.Tx, res *service.ClockInResult) error {
			staged = notification.Staged{}
			rec := res.Record
			if rec.SiteID != site.ID || (!res.Created && !replayFresh(rec.CreatedAt)) {
				return nil
			}
			if rec.NeedsReview {
				// Push only: the review queue has work (D-007, D-033).
				s, err := h.Notifier.Stage(ctx, tx, notification.Event{
					Type: notification.EventAttendanceReview, Site: site, DedupeKey: "attendance_review:" + rec.ID,
					RefID: rec.ID, RequestID: requestID(c),
					Params: map[string]string{
						"guard": guard.Name, "time": fmtTime(rec.ClockInAt, site.Location()),
						"reason": reviewReason(rec),
					},
				})
				if err != nil {
					return err
				}
				staged.IDs = append(staged.IDs, s.IDs...)
			}
			if rec.Status != "late" || rec.ShiftID == nil {
				return nil
			}
			shift := res.Shift
			if shift == nil {
				var err error
				if shift, err = repository.GetShift(ctx, tx, *rec.ShiftID); err != nil {
					return err
				}
			}
			s, err := h.Notifier.Stage(ctx, tx, notification.Event{
				Type: notification.EventLateClockIn, Site: site, DedupeKey: "late:" + rec.ID,
				RequestID: requestID(c), RefID: rec.ID,
				Params: map[string]string{
					"guard":   guard.Name,
					"minutes": strconv.Itoa(rec.LateMinutes),
					"shift":   shift.StartTime.In(site.Location()).Format("02 Jan 15:04"),
				},
			})
			staged.IDs = append(staged.IDs, s.IDs...)
			return err
		},
	})
	if err != nil {
		return h.fail(c, err)
	}
	h.Notifier.Wake(c.Context(), staged)
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
		AttendanceID: nilIfEmpty(b.AttendanceID), At: b.ClockOutAt, Guard: middleware.GetGuard(c), Auth: middleware.GetGuardAuth(c),
		Site: site, GPSLat: b.GPSLat, GPSLng: b.GPSLng,
	})
	if err != nil {
		return h.fail(c, err)
	}
	return c.JSON(rec)
}
