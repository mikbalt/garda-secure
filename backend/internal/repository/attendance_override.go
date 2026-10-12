package repository

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/securepatrol/backend/internal/model"
)

const overrideColumns = `id, attendance_id, shift_id, site_id, before_status, before_clock_in_at,
	before_clock_out_at, before_reason, after_status, after_clock_in_at, after_clock_out_at,
	reason, actor, created_at`

func scanOverride(r pgx.Row) (model.AttendanceOverride, error) {
	var o model.AttendanceOverride
	err := r.Scan(&o.ID, &o.AttendanceID, &o.ShiftID, &o.SiteID, &o.BeforeStatus, &o.BeforeClockInAt,
		&o.BeforeClockOutAt, &o.BeforeReason, &o.AfterStatus, &o.AfterClockInAt, &o.AfterClockOutAt,
		&o.Reason, &o.Actor, &o.CreatedAt)
	return o, err
}

// InsertAttendanceOverride records an override with its before/after values;
// call it in the override's transaction (F6).
func InsertAttendanceOverride(ctx context.Context, db DBTX, before, after *model.AttendanceRecord, reason, actor string) (*model.AttendanceOverride, error) {
	o := model.AttendanceOverride{
		AttendanceID: after.ID, ShiftID: after.ShiftID, SiteID: after.SiteID,
		AfterStatus: after.Status, AfterClockInAt: after.ClockInAt, AfterClockOutAt: after.ClockOutAt,
		Reason: reason, Actor: actor,
	}
	if before != nil {
		st := before.Status
		o.BeforeStatus, o.BeforeClockInAt, o.BeforeClockOutAt, o.BeforeReason =
			&st, before.ClockInAt, before.ClockOutAt, before.OverrideReason
	}
	out, err := scanOverride(db.QueryRow(ctx, `
		INSERT INTO attendance_overrides
		  (attendance_id, shift_id, site_id, before_status, before_clock_in_at, before_clock_out_at,
		   before_reason, after_status, after_clock_in_at, after_clock_out_at, reason, actor)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		RETURNING `+overrideColumns,
		o.AttendanceID, o.ShiftID, o.SiteID, o.BeforeStatus, o.BeforeClockInAt, o.BeforeClockOutAt,
		o.BeforeReason, o.AfterStatus, o.AfterClockInAt, o.AfterClockOutAt, o.Reason, o.Actor))
	if err != nil {
		return nil, fmt.Errorf("InsertAttendanceOverride: %w", err)
	}
	return &out, nil
}

// ListAttendanceOverrides returns the override history of an attendance
// record, oldest first.
func ListAttendanceOverrides(ctx context.Context, db DBTX, attendanceID string) ([]model.AttendanceOverride, error) {
	rows, err := db.Query(ctx, `
		SELECT `+overrideColumns+` FROM attendance_overrides
		WHERE attendance_id = $1 ORDER BY created_at, id`, attendanceID)
	if err != nil {
		return nil, fmt.Errorf("ListAttendanceOverrides: %w", err)
	}
	return collect(rows, func(r pgx.Rows) (model.AttendanceOverride, error) { return scanOverride(r) })
}
