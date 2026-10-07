package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/securepatrol/backend/internal/model"
)

const attendanceColumns = `id, shift_id, guard_id, site_id, clock_in_at, clock_out_at, status,
	late_minutes, early_out_minutes, selfie_object_key, gps_lat, gps_lng,
	clock_out_gps_lat, clock_out_gps_lng, device_id, override_reason, overridden_by,
	overridden_at, needs_review, review_note, reviewed_by, reviewed_at, created_at,
	COALESCE((SELECT g.name FROM guards g WHERE g.id = attendance_records.guard_id), '')`

func scanAttendance(row pgx.Row) (model.AttendanceRecord, error) {
	var r model.AttendanceRecord
	err := row.Scan(&r.ID, &r.ShiftID, &r.GuardID, &r.SiteID, &r.ClockInAt, &r.ClockOutAt, &r.Status,
		&r.LateMinutes, &r.EarlyOutMinutes, &r.SelfieObjectKey, &r.GPSLat, &r.GPSLng,
		&r.ClockOutGPSLat, &r.ClockOutGPSLng, &r.DeviceID, &r.OverrideReason, &r.OverriddenBy,
		&r.OverriddenAt, &r.NeedsReview, &r.ReviewNote, &r.ReviewedBy, &r.ReviewedAt, &r.CreatedAt, &r.GuardName)
	return r, err
}

func scanAttendanceRows(rows pgx.Rows) (model.AttendanceRecord, error) { return scanAttendance(rows) }

// GetAttendance retrieves an attendance record by ID.
func GetAttendance(ctx context.Context, db DBTX, id string) (*model.AttendanceRecord, error) {
	r, err := scanAttendance(db.QueryRow(ctx, `SELECT `+attendanceColumns+` FROM attendance_records WHERE id = $1`, id))
	if err != nil {
		return nil, mapNoRows(err)
	}
	return &r, nil
}

// GetAttendanceByShift returns the record of a shift, locking it when called
// inside a transaction.
func GetAttendanceByShift(ctx context.Context, db DBTX, shiftID string) (*model.AttendanceRecord, error) {
	r, err := scanAttendance(db.QueryRow(ctx,
		`SELECT `+attendanceColumns+` FROM attendance_records WHERE shift_id = $1 FOR UPDATE`, shiftID))
	if err != nil {
		return nil, mapNoRows(err)
	}
	return &r, nil
}

// GetOpenAttendance returns the guard's most recent record at a site that is
// clocked in but not yet clocked out.
func GetOpenAttendance(ctx context.Context, db DBTX, guardID, siteID string) (*model.AttendanceRecord, error) {
	r, err := scanAttendance(db.QueryRow(ctx, `
		SELECT `+attendanceColumns+` FROM attendance_records
		WHERE guard_id = $1 AND site_id = $2 AND clock_in_at IS NOT NULL AND clock_out_at IS NULL
		ORDER BY clock_in_at DESC LIMIT 1 FOR UPDATE`, guardID, siteID))
	if err != nil {
		return nil, mapNoRows(err)
	}
	return &r, nil
}

// ClockInParams carries the values of a new clock-in.
type ClockInParams struct {
	ID              *string // client-generated ID for idempotent offline sync
	ShiftID         string
	GuardID         string
	SiteID          string
	DeviceID        *string
	ClockInAt       time.Time
	Status          string
	LateMinutes     int
	SelfieObjectKey *string
	GPSLat          *float64
	GPSLng          *float64
}

// InsertClockIn creates the attendance record of a shift.
func InsertClockIn(ctx context.Context, db DBTX, p ClockInParams) (*model.AttendanceRecord, error) {
	r, err := scanAttendance(db.QueryRow(ctx, `
		INSERT INTO attendance_records
		  (id, shift_id, guard_id, site_id, device_id, clock_in_at, status, late_minutes,
		   selfie_object_key, gps_lat, gps_lng)
		VALUES (COALESCE($1::uuid, gen_random_uuid()), $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		RETURNING `+attendanceColumns,
		p.ID, p.ShiftID, p.GuardID, p.SiteID, p.DeviceID, p.ClockInAt, p.Status, p.LateMinutes,
		p.SelfieObjectKey, p.GPSLat, p.GPSLng))
	if err != nil {
		return nil, fmt.Errorf("InsertClockIn: %w", err)
	}
	return &r, nil
}

// ApplyClockInToAbsent converts an auto-flagged absent record into a clock-in
// that arrived late through offline sync, and queues it for supervisor review.
func ApplyClockInToAbsent(ctx context.Context, db DBTX, recordID string, p ClockInParams, note string) (*model.AttendanceRecord, error) {
	r, err := scanAttendance(db.QueryRow(ctx, `
		UPDATE attendance_records
		SET clock_in_at = $2, status = $3, late_minutes = $4, selfie_object_key = $5,
		    gps_lat = $6, gps_lng = $7, device_id = $8,
		    needs_review = true, review_note = $9
		WHERE id = $1
		RETURNING `+attendanceColumns,
		recordID, p.ClockInAt, p.Status, p.LateMinutes, p.SelfieObjectKey,
		p.GPSLat, p.GPSLng, p.DeviceID, note))
	if err != nil {
		return nil, mapNoRows(err)
	}
	return &r, nil
}

// SetClockOut records the clock-out of an attendance record.
func SetClockOut(ctx context.Context, db DBTX, id string, at time.Time, earlyOutMinutes int, lat, lng *float64) (*model.AttendanceRecord, error) {
	r, err := scanAttendance(db.QueryRow(ctx, `
		UPDATE attendance_records
		SET clock_out_at = $2, early_out_minutes = $3, clock_out_gps_lat = $4, clock_out_gps_lng = $5
		WHERE id = $1
		RETURNING `+attendanceColumns, id, at, earlyOutMinutes, lat, lng))
	if err != nil {
		return nil, mapNoRows(err)
	}
	return &r, nil
}

// OverrideParams carries an admin attendance correction for a shift.
type OverrideParams struct {
	ShiftID    string
	GuardID    string
	SiteID     string
	Status     string
	ClockInAt  *time.Time
	ClockOutAt *time.Time
	Reason     string
	Actor      string
}

// OverrideAttendance upserts the record of a shift with an admin-chosen status.
// Supplied clock times replace the stored ones; omitted ones are kept.
func OverrideAttendance(ctx context.Context, db DBTX, p OverrideParams) (*model.AttendanceRecord, error) {
	r, err := scanAttendance(db.QueryRow(ctx, `
		INSERT INTO attendance_records
		  (shift_id, guard_id, site_id, status, clock_in_at, clock_out_at,
		   override_reason, overridden_by, overridden_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, now())
		ON CONFLICT (shift_id) WHERE shift_id IS NOT NULL DO UPDATE
		SET status          = EXCLUDED.status,
		    clock_in_at     = COALESCE(EXCLUDED.clock_in_at, attendance_records.clock_in_at),
		    clock_out_at    = COALESCE(EXCLUDED.clock_out_at, attendance_records.clock_out_at),
		    late_minutes    = CASE WHEN EXCLUDED.status = 'late' THEN attendance_records.late_minutes ELSE 0 END,
		    override_reason = EXCLUDED.override_reason,
		    overridden_by   = EXCLUDED.overridden_by,
		    overridden_at   = now(),
		    needs_review    = false,
		    reviewed_by     = COALESCE(attendance_records.reviewed_by, EXCLUDED.overridden_by),
		    reviewed_at     = COALESCE(attendance_records.reviewed_at, now())
		RETURNING `+attendanceColumns,
		p.ShiftID, p.GuardID, p.SiteID, p.Status, p.ClockInAt, p.ClockOutAt, p.Reason, p.Actor))
	if err != nil {
		return nil, fmt.Errorf("OverrideAttendance: %w", err)
	}
	return &r, nil
}

// MarkAttendanceReviewed clears the review flag of a record.
func MarkAttendanceReviewed(ctx context.Context, db DBTX, id, actor string, note *string) (*model.AttendanceRecord, error) {
	r, err := scanAttendance(db.QueryRow(ctx, `
		UPDATE attendance_records
		SET needs_review = false, reviewed_by = $2, reviewed_at = now(),
		    review_note = COALESCE($3, review_note)
		WHERE id = $1
		RETURNING `+attendanceColumns, id, actor, note))
	if err != nil {
		return nil, mapNoRows(err)
	}
	return &r, nil
}

// ListReviewQueue returns the records of a site awaiting supervisor review.
func ListReviewQueue(ctx context.Context, db DBTX, siteID string) ([]model.AttendanceRecord, error) {
	rows, err := db.Query(ctx, `
		SELECT `+attendanceColumns+` FROM attendance_records
		WHERE site_id = $1 AND needs_review
		ORDER BY created_at`, siteID)
	if err != nil {
		return nil, fmt.Errorf("ListReviewQueue: %w", err)
	}
	return collect(rows, scanAttendanceRows)
}

// AbsentFlag identifies a record created by the absent auto-flag job.
type AbsentFlag struct {
	RecordID  string
	ShiftID   string
	GuardID   string
	GuardName string
	SiteID    string
	StartTime time.Time
}

// FlagAbsentees inserts an absent record for every shift of an active guard
// that started at least the site's absent threshold ago (within the last day)
// and has no attendance yet. Safe to run repeatedly.
func FlagAbsentees(ctx context.Context, db DBTX, defaultAbsentAfterMinutes int) ([]AbsentFlag, error) {
	rows, err := db.Query(ctx, `
		WITH due AS (
		  SELECT s.id, s.guard_id, s.site_id, s.start_time
		  FROM shifts s
		  JOIN sites st ON st.id = s.site_id
		  JOIN guards g ON g.id = s.guard_id AND g.status = 'active'
		  WHERE s.start_time > now() - interval '1 day'
		    AND s.start_time + make_interval(mins =>
		          COALESCE((st.config_json->>'absent_after_minutes')::int, $1)) <= now()
		    AND NOT EXISTS (SELECT 1 FROM attendance_records a WHERE a.shift_id = s.id)
		),
		ins AS (
		  INSERT INTO attendance_records (shift_id, guard_id, site_id, status)
		  SELECT id, guard_id, site_id, 'absent' FROM due
		  ON CONFLICT (shift_id) WHERE shift_id IS NOT NULL DO NOTHING
		  RETURNING id, shift_id, guard_id, site_id
		)
		SELECT ins.id, ins.shift_id, ins.guard_id, g.name, ins.site_id, due.start_time
		FROM ins
		JOIN due ON due.id = ins.shift_id
		JOIN guards g ON g.id = ins.guard_id`, defaultAbsentAfterMinutes)
	if err != nil {
		return nil, fmt.Errorf("FlagAbsentees: %w", err)
	}
	return collect(rows, func(r pgx.Rows) (AbsentFlag, error) {
		var f AbsentFlag
		err := r.Scan(&f.RecordID, &f.ShiftID, &f.GuardID, &f.GuardName, &f.SiteID, &f.StartTime)
		return f, err
	})
}

// DailyAttendanceRow is one scheduled shift with its attendance outcome.
type DailyAttendanceRow struct {
	ShiftID         string     `json:"shift_id"`
	GuardID         string     `json:"guard_id"`
	GuardName       string     `json:"guard_name"`
	EmployeeID      *string    `json:"employee_id,omitempty"`
	ShiftStart      time.Time  `json:"shift_start"`
	ShiftEnd        time.Time  `json:"shift_end"`
	AttendanceID    *string    `json:"attendance_id,omitempty"`
	Status          string     `json:"status"` // present | late | absent | pending
	ClockInAt       *time.Time `json:"clock_in_at,omitempty"`
	ClockOutAt      *time.Time `json:"clock_out_at,omitempty"`
	LateMinutes     int        `json:"late_minutes"`
	EarlyOutMinutes int        `json:"early_out_minutes"`
	HoursWorked     float64    `json:"hours_worked"`
	NeedsReview     bool       `json:"needs_review"`
	OverrideReason  *string    `json:"override_reason,omitempty"`
	SelfieObjectKey *string    `json:"selfie_object_key,omitempty"`
}

// DailyAttendance lists every shift of a site starting in [from, to) with its
// attendance; shifts without a record yet report status "pending".
func DailyAttendance(ctx context.Context, db DBTX, siteID string, from, to time.Time) ([]DailyAttendanceRow, error) {
	rows, err := db.Query(ctx, `
		SELECT s.id, s.guard_id, g.name, g.employee_id, s.start_time, s.end_time,
		       a.id, COALESCE(a.status, 'pending'), a.clock_in_at, a.clock_out_at,
		       COALESCE(a.late_minutes, 0), COALESCE(a.early_out_minutes, 0),
		       COALESCE(EXTRACT(EPOCH FROM (a.clock_out_at - a.clock_in_at)) / 3600, 0)::float8,
		       COALESCE(a.needs_review, false), a.override_reason, a.selfie_object_key
		FROM shifts s
		JOIN guards g ON g.id = s.guard_id
		LEFT JOIN attendance_records a ON a.shift_id = s.id
		WHERE s.site_id = $1 AND s.start_time >= $2 AND s.start_time < $3
		ORDER BY s.start_time, g.name`, siteID, from, to)
	if err != nil {
		return nil, fmt.Errorf("DailyAttendance: %w", err)
	}
	return collect(rows, func(r pgx.Rows) (DailyAttendanceRow, error) {
		var d DailyAttendanceRow
		err := r.Scan(&d.ShiftID, &d.GuardID, &d.GuardName, &d.EmployeeID, &d.ShiftStart, &d.ShiftEnd,
			&d.AttendanceID, &d.Status, &d.ClockInAt, &d.ClockOutAt, &d.LateMinutes, &d.EarlyOutMinutes,
			&d.HoursWorked, &d.NeedsReview, &d.OverrideReason, &d.SelfieObjectKey)
		return d, err
	})
}
