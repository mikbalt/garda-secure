// Package repository provides data-access functions for attendance records.
package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/securepatrol/backend/internal/model"
)

// ClockInRequest holds validated input for the clock-in operation.
type ClockInRequest struct {
	GuardID         string
	SiteID          string
	ShiftID         *string
	GPSLat          *float64
	GPSLng          *float64
	SelfieObjectKey *string
	Status          string // "present" or "late"
}

// ListByDate returns all attendance records for a site on a given calendar date.
// date should be in "YYYY-MM-DD" format (UTC).
func ListByDate(ctx context.Context, pool *pgxpool.Pool, siteID, date string) ([]model.AttendanceRecord, error) {
	rows, err := pool.Query(ctx, `
		SELECT id, shift_id, guard_id, site_id, clock_in_at, clock_out_at,
		       status, selfie_object_key, gps_lat, gps_lng,
		       override_reason, overridden_by, created_at
		FROM attendance_records
		WHERE site_id = $1
		  AND created_at::date = $2::date
		ORDER BY created_at`,
		siteID, date,
	)
	if err != nil {
		return nil, fmt.Errorf("ListByDate query: %w", err)
	}
	defer rows.Close()

	return scanAttendanceRows(rows)
}

// ListByDateStatus returns records for a site on a date filtered by status.
func ListByDateStatus(ctx context.Context, pool *pgxpool.Pool, siteID, date, status string) ([]model.AttendanceRecord, error) {
	rows, err := pool.Query(ctx, `
		SELECT id, shift_id, guard_id, site_id, clock_in_at, clock_out_at,
		       status, selfie_object_key, gps_lat, gps_lng,
		       override_reason, overridden_by, created_at
		FROM attendance_records
		WHERE site_id = $1
		  AND created_at::date = $2::date
		  AND status = $3
		ORDER BY created_at`,
		siteID, date, status,
	)
	if err != nil {
		return nil, fmt.Errorf("ListByDateStatus query: %w", err)
	}
	defer rows.Close()

	return scanAttendanceRows(rows)
}

// ClockIn inserts a new attendance record (clock-in event).
func ClockIn(ctx context.Context, pool *pgxpool.Pool, req ClockInRequest) (*model.AttendanceRecord, error) {
	now := time.Now().UTC()
	var rec model.AttendanceRecord
	err := pool.QueryRow(ctx, `
		INSERT INTO attendance_records
		  (shift_id, guard_id, site_id, clock_in_at, status, selfie_object_key, gps_lat, gps_lng)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING id, shift_id, guard_id, site_id, clock_in_at, clock_out_at,
		          status, selfie_object_key, gps_lat, gps_lng,
		          override_reason, overridden_by, created_at`,
		req.ShiftID, req.GuardID, req.SiteID, now, req.Status,
		req.SelfieObjectKey, req.GPSLat, req.GPSLng,
	).Scan(
		&rec.ID, &rec.ShiftID, &rec.GuardID, &rec.SiteID,
		&rec.ClockInAt, &rec.ClockOutAt,
		&rec.Status, &rec.SelfieObjectKey,
		&rec.GPSLat, &rec.GPSLng,
		&rec.OverrideReason, &rec.OverriddenBy, &rec.CreatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("ClockIn insert: %w", err)
	}
	return &rec, nil
}

// ClockOut sets clock_out_at on an existing attendance record.
func ClockOut(ctx context.Context, pool *pgxpool.Pool, id string, status string, gpsLat, gpsLng *float64) (*model.AttendanceRecord, error) {
	now := time.Now().UTC()
	var rec model.AttendanceRecord
	err := pool.QueryRow(ctx, `
		UPDATE attendance_records
		SET clock_out_at = $1, status = $2, gps_lat = COALESCE($3, gps_lat), gps_lng = COALESCE($4, gps_lng)
		WHERE id = $5
		RETURNING id, shift_id, guard_id, site_id, clock_in_at, clock_out_at,
		          status, selfie_object_key, gps_lat, gps_lng,
		          override_reason, overridden_by, created_at`,
		now, status, gpsLat, gpsLng, id,
	).Scan(
		&rec.ID, &rec.ShiftID, &rec.GuardID, &rec.SiteID,
		&rec.ClockInAt, &rec.ClockOutAt,
		&rec.Status, &rec.SelfieObjectKey,
		&rec.GPSLat, &rec.GPSLng,
		&rec.OverrideReason, &rec.OverriddenBy, &rec.CreatedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("attendance record not found: %s", id)
		}
		return nil, fmt.Errorf("ClockOut update: %w", err)
	}
	return &rec, nil
}

// GetByID retrieves a single attendance record.
func GetAttendanceByID(ctx context.Context, pool *pgxpool.Pool, id string) (*model.AttendanceRecord, error) {
	var rec model.AttendanceRecord
	err := pool.QueryRow(ctx, `
		SELECT id, shift_id, guard_id, site_id, clock_in_at, clock_out_at,
		       status, selfie_object_key, gps_lat, gps_lng,
		       override_reason, overridden_by, created_at
		FROM attendance_records WHERE id = $1`, id,
	).Scan(
		&rec.ID, &rec.ShiftID, &rec.GuardID, &rec.SiteID,
		&rec.ClockInAt, &rec.ClockOutAt,
		&rec.Status, &rec.SelfieObjectKey,
		&rec.GPSLat, &rec.GPSLng,
		&rec.OverrideReason, &rec.OverriddenBy, &rec.CreatedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("GetAttendanceByID: %w", err)
	}
	return &rec, nil
}

// GetTodayClockIn returns the existing clock-in record for a guard at a site today (nil if none).
func GetTodayClockIn(ctx context.Context, pool *pgxpool.Pool, guardID, siteID, date string) (*model.AttendanceRecord, error) {
	var rec model.AttendanceRecord
	err := pool.QueryRow(ctx, `
		SELECT id, shift_id, guard_id, site_id, clock_in_at, clock_out_at,
		       status, selfie_object_key, gps_lat, gps_lng,
		       override_reason, overridden_by, created_at
		FROM attendance_records
		WHERE guard_id = $1 AND site_id = $2 AND created_at::date = $3::date
		LIMIT 1`,
		guardID, siteID, date,
	).Scan(
		&rec.ID, &rec.ShiftID, &rec.GuardID, &rec.SiteID,
		&rec.ClockInAt, &rec.ClockOutAt,
		&rec.Status, &rec.SelfieObjectKey,
		&rec.GPSLat, &rec.GPSLng,
		&rec.OverrideReason, &rec.OverriddenBy, &rec.CreatedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("GetTodayClockIn: %w", err)
	}
	return &rec, nil
}

// InsertAbsent creates an absent record for a guard that has no attendance on the given date.
func InsertAbsent(ctx context.Context, pool *pgxpool.Pool, guardID, siteID, shiftID, date string) error {
	_, err := pool.Exec(ctx, `
		INSERT INTO attendance_records (shift_id, guard_id, site_id, status, created_at)
		VALUES ($1, $2, $3, 'absent', $4::date)
		ON CONFLICT DO NOTHING`,
		shiftID, guardID, siteID, date,
	)
	if err != nil {
		return fmt.Errorf("InsertAbsent: %w", err)
	}
	return nil
}

// DailySummaryResult holds counts by status for a given site/date.
type DailySummaryResult struct {
	Present  int `json:"present"`
	Absent   int `json:"absent"`
	Late     int `json:"late"`
	EarlyOut int `json:"early_out"`
	Pending  int `json:"pending"`
}

// DailySummary returns attendance counts grouped by status for a site on a date.
func DailySummary(ctx context.Context, pool *pgxpool.Pool, siteID, date string) (*DailySummaryResult, error) {
	rows, err := pool.Query(ctx, `
		SELECT status, COUNT(*) AS cnt
		FROM attendance_records
		WHERE site_id = $1 AND created_at::date = $2::date
		GROUP BY status`,
		siteID, date,
	)
	if err != nil {
		return nil, fmt.Errorf("DailySummary query: %w", err)
	}
	defer rows.Close()

	result := &DailySummaryResult{}
	for rows.Next() {
		var status string
		var cnt int
		if err := rows.Scan(&status, &cnt); err != nil {
			return nil, fmt.Errorf("DailySummary scan: %w", err)
		}
		switch status {
		case "present":
			result.Present = cnt
		case "absent":
			result.Absent = cnt
		case "late":
			result.Late = cnt
		case "early_out":
			result.EarlyOut = cnt
		case "pending":
			result.Pending = cnt
		}
	}
	return result, rows.Err()
}

// MonthlyRecapRow holds per-guard summary for a month.
type MonthlyRecapRow struct {
	GuardID  string `json:"guard_id"`
	Name     string `json:"name"`
	Present  int    `json:"present"`
	Absent   int    `json:"absent"`
	Late     int    `json:"late"`
	EarlyOut int    `json:"early_out"`
}

// MonthlyRecap returns a per-guard attendance summary for a month (YYYY-MM).
func MonthlyRecap(ctx context.Context, pool *pgxpool.Pool, siteID, month string) ([]MonthlyRecapRow, error) {
	rows, err := pool.Query(ctx, `
		SELECT ar.guard_id, g.name,
		       COUNT(*) FILTER (WHERE ar.status = 'present')   AS present,
		       COUNT(*) FILTER (WHERE ar.status = 'absent')    AS absent,
		       COUNT(*) FILTER (WHERE ar.status = 'late')      AS late,
		       COUNT(*) FILTER (WHERE ar.status = 'early_out') AS early_out
		FROM attendance_records ar
		JOIN guards g ON g.id = ar.guard_id
		WHERE ar.site_id = $1
		  AND to_char(ar.created_at, 'YYYY-MM') = $2
		GROUP BY ar.guard_id, g.name
		ORDER BY g.name`,
		siteID, month,
	)
	if err != nil {
		return nil, fmt.Errorf("MonthlyRecap query: %w", err)
	}
	defer rows.Close()

	var recap []MonthlyRecapRow
	for rows.Next() {
		var r MonthlyRecapRow
		if err := rows.Scan(&r.GuardID, &r.Name, &r.Present, &r.Absent, &r.Late, &r.EarlyOut); err != nil {
			return nil, fmt.Errorf("MonthlyRecap scan: %w", err)
		}
		recap = append(recap, r)
	}
	return recap, rows.Err()
}

// GetShiftForGuardOnDate retrieves the shift (if any) for a guard at a site on a given date.
func GetShiftForGuardOnDate(ctx context.Context, pool *pgxpool.Pool, guardID, siteID, date string) (*model.Shift, error) {
	var s model.Shift
	err := pool.QueryRow(ctx, `
		SELECT id, guard_id, site_id, start_time, end_time, repeat_rule, created_at
		FROM shifts
		WHERE guard_id = $1 AND site_id = $2 AND start_time::date = $3::date
		LIMIT 1`,
		guardID, siteID, date,
	).Scan(&s.ID, &s.GuardID, &s.SiteID, &s.StartTime, &s.EndTime, &s.RepeatRule, &s.CreatedAt)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("GetShiftForGuardOnDate: %w", err)
	}
	return &s, nil
}

// GetShiftsForSiteOnDate returns all shifts for a site on a given date.
func GetShiftsForSiteOnDate(ctx context.Context, pool *pgxpool.Pool, siteID, date string) ([]model.Shift, error) {
	rows, err := pool.Query(ctx, `
		SELECT id, guard_id, site_id, start_time, end_time, repeat_rule, created_at
		FROM shifts
		WHERE site_id = $1 AND start_time::date = $2::date`,
		siteID, date,
	)
	if err != nil {
		return nil, fmt.Errorf("GetShiftsForSiteOnDate: %w", err)
	}
	defer rows.Close()

	var shifts []model.Shift
	for rows.Next() {
		var s model.Shift
		if err := rows.Scan(&s.ID, &s.GuardID, &s.SiteID, &s.StartTime, &s.EndTime, &s.RepeatRule, &s.CreatedAt); err != nil {
			return nil, fmt.Errorf("GetShiftsForSiteOnDate scan: %w", err)
		}
		shifts = append(shifts, s)
	}
	return shifts, rows.Err()
}

// scanAttendanceRows is a shared scan helper.
func scanAttendanceRows(rows pgx.Rows) ([]model.AttendanceRecord, error) {
	var records []model.AttendanceRecord
	for rows.Next() {
		var r model.AttendanceRecord
		if err := rows.Scan(
			&r.ID, &r.ShiftID, &r.GuardID, &r.SiteID,
			&r.ClockInAt, &r.ClockOutAt,
			&r.Status, &r.SelfieObjectKey,
			&r.GPSLat, &r.GPSLng,
			&r.OverrideReason, &r.OverriddenBy, &r.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan attendance row: %w", err)
		}
		records = append(records, r)
	}
	return records, rows.Err()
}

// GetShiftByID retrieves a shift by its ID.
func GetShiftByID(ctx context.Context, pool *pgxpool.Pool, id string) (*model.Shift, error) {
	var s model.Shift
	err := pool.QueryRow(ctx,
		`SELECT id, guard_id, site_id, start_time, end_time, repeat_rule, created_at FROM shifts WHERE id = $1`,
		id,
	).Scan(&s.ID, &s.GuardID, &s.SiteID, &s.StartTime, &s.EndTime, &s.RepeatRule, &s.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("GetShiftByID: %w", err)
	}
	return &s, nil
}
