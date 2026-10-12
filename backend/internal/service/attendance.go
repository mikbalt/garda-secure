package service

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/securepatrol/backend/internal/model"
	"github.com/securepatrol/backend/internal/repository"
)

// ClockInInput is a guard clocking in from a tablet.
type ClockInInput struct {
	ID              *string    // client UUID for idempotent sync
	At              *time.Time // event time recorded on the tablet; nil = now
	Guard           *model.Guard
	Site            *model.Site
	DeviceID        *string
	SelfieObjectKey *string
	GPSLat, GPSLng  *float64
	// AfterWrite, when set, runs inside the clock-in transaction with the
	// result (new or replayed) — used to stage notifications atomically
	// with the record (transactional outbox, audit BE-R1).
	AfterWrite func(ctx context.Context, tx pgx.Tx, res *ClockInResult) error
	// DeviceLastSeen is when the tablet was last seen before this request
	// (plausibility check of backdated clock-ins).
	DeviceLastSeen *time.Time
}

// ClockInResult reports the stored record and what happened.
type ClockInResult struct {
	Record *model.AttendanceRecord
	Shift  *model.Shift
	// Created is false when the request repeated an earlier sync.
	Created bool
}

// ClockIn records a clock-in against the guard's shift whose window contains
// the event time (D-004). A clock-in for a shift already auto-flagged absent
// (offline sync arriving late) replaces the absent mark and is queued for
// supervisor review (D-007).
//
// The shift row is locked for the transaction, so a clock-in never races the
// absent auto-flag, and concurrent retries with the same client id return
// the stored record (200) instead of a 409.
func ClockIn(ctx context.Context, pool *pgxpool.Pool, in ClockInInput) (*ClockInResult, error) {
	at, err := ClientTime(in.At)
	if err != nil {
		return nil, err
	}
	if err := CheckObjectKey(in.SelfieObjectKey, in.Site.ID); err != nil {
		return nil, err
	}
	cfg := in.Site.Config()

	var res *ClockInResult
	err = pgx.BeginFunc(ctx, pool, func(tx pgx.Tx) error {
		var err error
		if res, err = clockIn(ctx, tx, in, at, cfg); err != nil {
			return err
		}
		if in.AfterWrite != nil {
			return in.AfterWrite(ctx, tx, res)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return res, nil
}

// replayedClockIn returns the record stored under the client id, if any.
func replayedClockIn(ctx context.Context, tx pgx.Tx, in ClockInInput) (*ClockInResult, error) {
	if in.ID == nil {
		return nil, nil
	}
	existing, err := repository.GetAttendance(ctx, tx, *in.ID)
	if errors.Is(err, repository.ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if err := CheckReplay(existing, false, in.Site.ID, in.Guard.ID); err != nil {
		return nil, err
	}
	return &ClockInResult{Record: existing}, nil
}

func clockIn(ctx context.Context, tx pgx.Tx, in ClockInInput, at time.Time, cfg model.SiteConfig) (*ClockInResult, error) {
	if replay, err := replayedClockIn(ctx, tx, in); err != nil || replay != nil {
		return replay, err
	}

	window := time.Duration(cfg.ClockWindowMinutes) * time.Minute
	shift, err := repository.FindShiftForClockIn(ctx, tx, in.Guard.ID, in.Site.ID, at, window)
	if errors.Is(err, repository.ErrNotFound) {
		return nil, BadRequest("no_shift", "no scheduled shift for this guard within the clock-in window")
	}
	if err != nil {
		return nil, err
	}
	// Serialise with FlagAbsentees and with identical retries, then look
	// again: a concurrent retry may have committed while we waited.
	if err := repository.LockShift(ctx, tx, shift.ID); err != nil {
		return nil, err
	}
	if replay, err := replayedClockIn(ctx, tx, in); err != nil || replay != nil {
		return replay, err
	}

	p := repository.ClockInParams{
		ID: in.ID, ShiftID: shift.ID, GuardID: in.Guard.ID, SiteID: in.Site.ID,
		DeviceID: in.DeviceID, ClockInAt: at, SelfieObjectKey: in.SelfieObjectKey,
		GPSLat: in.GPSLat, GPSLng: in.GPSLng,
	}
	p.Status, p.LateMinutes = arrivalStatus(shift.StartTime, at, cfg.LateGraceMinutes)
	p.ReceivedAt = Now()
	p.Flags, p.NeedsReview = ClockInFlags(at, p.ReceivedAt, p.Status, in.DeviceLastSeen)

	existing, err := repository.GetAttendanceByShift(ctx, tx, shift.ID)
	switch {
	case errors.Is(err, repository.ErrNotFound):
		rec, err := repository.InsertClockIn(ctx, tx, p)
		if repository.IsUniqueViolation(err) {
			return nil, Conflict("already_clocked_in", "guard already clocked in for this shift")
		}
		if err != nil {
			return nil, err
		}
		return &ClockInResult{Record: rec, Shift: shift, Created: true}, nil
	case err != nil:
		return nil, err
	case existing.Status == "absent" && existing.ClockInAt == nil && existing.OverriddenBy == nil:
		rec, err := repository.ApplyClockInToAbsent(ctx, tx, existing.ID, p,
			"Clock-in synced after the shift was auto-flagged absent")
		if err != nil {
			return nil, err
		}
		return &ClockInResult{Record: rec, Shift: shift, Created: true}, nil
	default:
		return nil, Conflict("already_clocked_in", "guard already has an attendance record for this shift")
	}
}

// ClockOutInput is a guard clocking out from a tablet.
type ClockOutInput struct {
	AttendanceID   *string // optional: makes retries of a completed clock-out idempotent
	At             *time.Time
	Guard          *model.Guard
	Site           *model.Site
	GPSLat, GPSLng *float64
}

// ClockOut closes the guard's open attendance record. Clock-out is accepted
// until shift end + clock window; leaving before end - grace counts as early.
func ClockOut(ctx context.Context, pool *pgxpool.Pool, in ClockOutInput) (*model.AttendanceRecord, bool, error) {
	at, err := ClientTime(in.At)
	if err != nil {
		return nil, false, err
	}
	cfg := in.Site.Config()

	var out *model.AttendanceRecord
	var changed bool
	err = pgx.BeginFunc(ctx, pool, func(tx pgx.Tx) error {
		var rec *model.AttendanceRecord
		if in.AttendanceID != nil {
			r, err := repository.GetAttendance(ctx, tx, *in.AttendanceID)
			if errors.Is(err, repository.ErrNotFound) {
				return NotFound("attendance_not_found", "attendance record not found")
			}
			if err != nil {
				return err
			}
			if r.GuardID != in.Guard.ID || r.SiteID != in.Site.ID {
				return Forbidden("not_your_record", "attendance record belongs to another guard")
			}
			if r.ClockOutAt != nil {
				out = r
				return nil
			}
			if r.ClockInAt == nil {
				return Conflict("not_clocked_in", "guard has not clocked in for this shift")
			}
			rec = r
		} else {
			r, err := repository.GetOpenAttendance(ctx, tx, in.Guard.ID, in.Site.ID)
			if errors.Is(err, repository.ErrNotFound) {
				return Conflict("not_clocked_in", "guard has no open clock-in at this site")
			}
			if err != nil {
				return err
			}
			rec = r
		}
		if at.Before(*rec.ClockInAt) {
			return BadRequest("before_clock_in", "clock-out time is before clock-in time")
		}

		early := 0
		if rec.ShiftID != nil {
			shift, err := repository.GetShift(ctx, tx, *rec.ShiftID)
			if err != nil {
				return err
			}
			window := time.Duration(cfg.ClockWindowMinutes) * time.Minute
			if at.After(shift.EndTime.Add(window)) {
				return Conflict("clock_out_window_closed", "clock-out window has closed; ask a supervisor to correct the record")
			}
			early = earlyOutMinutes(shift.EndTime, at, cfg.EarlyOutGraceMinutes)
		}
		r, err := repository.SetClockOut(ctx, tx, rec.ID, at, early, in.GPSLat, in.GPSLng)
		if err != nil {
			return err
		}
		out, changed = r, true
		return nil
	})
	return out, changed, err
}

// OverrideInput is an admin correction of a shift's attendance (FR-ATT-06).
type OverrideInput struct {
	ShiftID    string
	Status     string
	Reason     string
	ClockInAt  *time.Time
	ClockOutAt *time.Time
	Actor      string
}

// OverrideAttendance validates and applies an admin correction.
func OverrideAttendance(ctx context.Context, db repository.DBTX, shift *model.Shift, in OverrideInput) (*model.AttendanceRecord, error) {
	switch in.Status {
	case "present", "late", "absent":
	default:
		return nil, BadRequest("invalid_status", "status must be present, late or absent")
	}
	if strings.TrimSpace(in.Reason) == "" {
		return nil, BadRequest("reason_required", "a reason is required for an attendance override")
	}
	if in.ClockInAt != nil && in.ClockOutAt != nil && in.ClockOutAt.Before(*in.ClockInAt) {
		return nil, BadRequest("invalid_times", "clock_out_at must be after clock_in_at")
	}
	return repository.OverrideAttendance(ctx, db, repository.OverrideParams{
		ShiftID: shift.ID, GuardID: shift.GuardID, SiteID: shift.SiteID,
		Status: in.Status, ClockInAt: in.ClockInAt, ClockOutAt: in.ClockOutAt,
		Reason: strings.TrimSpace(in.Reason), Actor: in.Actor,
	})
}

// arrivalStatus classifies a clock-in: late when after start + grace; the
// late minutes are counted from the shift start.
func arrivalStatus(shiftStart, at time.Time, graceMinutes int) (string, int) {
	if at.After(shiftStart.Add(time.Duration(graceMinutes) * time.Minute)) {
		return "late", minutesBetween(shiftStart, at)
	}
	return "present", 0
}

// earlyOutMinutes returns how early a guard left, or 0 within the grace period.
func earlyOutMinutes(shiftEnd, at time.Time, graceMinutes int) int {
	if at.Before(shiftEnd.Add(-time.Duration(graceMinutes) * time.Minute)) {
		return minutesBetween(at, shiftEnd)
	}
	return 0
}
