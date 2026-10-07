package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/securepatrol/backend/internal/model"
)

// ListShifts returns shifts for a given site and optional date range.
func ListShifts(ctx context.Context, pool *pgxpool.Pool, siteID, guardID, date string) ([]model.Shift, error) {
	q := `SELECT id, guard_id, site_id, start_time, end_time, repeat_rule, created_at
	      FROM shifts WHERE 1=1`
	args := []any{}
	idx := 1

	if siteID != "" {
		q += fmt.Sprintf(" AND site_id = $%d", idx)
		args = append(args, siteID)
		idx++
	}
	if guardID != "" {
		q += fmt.Sprintf(" AND guard_id = $%d", idx)
		args = append(args, guardID)
		idx++
	}
	if date != "" {
		q += fmt.Sprintf(" AND DATE(start_time AT TIME ZONE 'UTC') = $%d", idx)
		args = append(args, date)
		idx++
	}
	_ = idx

	q += " ORDER BY start_time ASC"

	rows, err := pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var shifts []model.Shift
	for rows.Next() {
		var s model.Shift
		if err := rows.Scan(&s.ID, &s.GuardID, &s.SiteID, &s.StartTime, &s.EndTime, &s.RepeatRule, &s.CreatedAt); err != nil {
			return nil, err
		}
		shifts = append(shifts, s)
	}
	return shifts, rows.Err()
}

// GetShift retrieves a shift by ID.
func GetShift(ctx context.Context, pool *pgxpool.Pool, id string) (*model.Shift, error) {
	row := pool.QueryRow(ctx,
		`SELECT id, guard_id, site_id, start_time, end_time, repeat_rule, created_at
		 FROM shifts WHERE id = $1`, id)
	var s model.Shift
	if err := row.Scan(&s.ID, &s.GuardID, &s.SiteID, &s.StartTime, &s.EndTime, &s.RepeatRule, &s.CreatedAt); err != nil {
		return nil, err
	}
	return &s, nil
}

// CreateShift inserts a new shift and returns it with the generated ID.
func CreateShift(ctx context.Context, pool *pgxpool.Pool, guardID, siteID string, start, end time.Time, repeatRule *string) (*model.Shift, error) {
	var s model.Shift
	err := pool.QueryRow(ctx,
		`INSERT INTO shifts (id, guard_id, site_id, start_time, end_time, repeat_rule, created_at)
		 VALUES (gen_random_uuid(), $1, $2, $3, $4, $5, NOW())
		 RETURNING id, guard_id, site_id, start_time, end_time, repeat_rule, created_at`,
		guardID, siteID, start, end, repeatRule,
	).Scan(&s.ID, &s.GuardID, &s.SiteID, &s.StartTime, &s.EndTime, &s.RepeatRule, &s.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &s, nil
}

// UpdateShift updates mutable shift fields.
func UpdateShift(ctx context.Context, pool *pgxpool.Pool, id string, fields map[string]any) (*model.Shift, error) {
	// Build a safe SET clause from allowed fields.
	allowed := map[string]bool{"start_time": true, "end_time": true, "repeat_rule": true}
	set, args, idx := "", []any{}, 1
	for k, v := range fields {
		if !allowed[k] {
			continue
		}
		if set != "" {
			set += ", "
		}
		set += fmt.Sprintf("%s = $%d", k, idx)
		args = append(args, v)
		idx++
	}
	if set == "" {
		return GetShift(ctx, pool, id)
	}
	args = append(args, id)
	var s model.Shift
	err := pool.QueryRow(ctx,
		fmt.Sprintf(`UPDATE shifts SET %s WHERE id = $%d
		             RETURNING id, guard_id, site_id, start_time, end_time, repeat_rule, created_at`, set, idx),
		args...,
	).Scan(&s.ID, &s.GuardID, &s.SiteID, &s.StartTime, &s.EndTime, &s.RepeatRule, &s.CreatedAt)
	return &s, err
}

// DeleteShift deletes a shift by ID.
func DeleteShift(ctx context.Context, pool *pgxpool.Pool, id string) error {
	_, err := pool.Exec(ctx, `DELETE FROM shifts WHERE id = $1`, id)
	return err
}

// TodayShiftForGuard finds the currently-active (or upcoming-within-1h) shift for a guard.
func TodayShiftForGuard(ctx context.Context, pool *pgxpool.Pool, guardID string) (*model.Shift, error) {
	now := time.Now().UTC()
	row := pool.QueryRow(ctx,
		`SELECT id, guard_id, site_id, start_time, end_time, repeat_rule, created_at
		 FROM shifts
		 WHERE guard_id = $1
		   AND start_time <= $2 + INTERVAL '1 hour'
		   AND end_time   >= $2
		 ORDER BY start_time ASC
		 LIMIT 1`,
		guardID, now,
	)
	var s model.Shift
	if err := row.Scan(&s.ID, &s.GuardID, &s.SiteID, &s.StartTime, &s.EndTime, &s.RepeatRule, &s.CreatedAt); err != nil {
		return nil, err
	}
	return &s, nil
}
