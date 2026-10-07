package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/securepatrol/backend/internal/model"
)

// ListVisitorLogs retrieves visitor logs for a site with optional date filter.
func ListVisitorLogs(ctx context.Context, pool *pgxpool.Pool, siteID, date string) ([]model.VisitorLog, error) {
	q := `SELECT id, site_id, guard_id, visitor_name, id_type, id_number, plate,
	             host_name, entry_at, exit_at, purpose, photo_object_key, created_at
	      FROM visitor_logs WHERE 1=1`
	args := []any{}
	idx := 1

	if siteID != "" {
		q += fmt.Sprintf(" AND site_id = $%d", idx)
		args = append(args, siteID)
		idx++
	}
	if date != "" {
		q += fmt.Sprintf(" AND DATE(created_at AT TIME ZONE 'UTC') = $%d", idx)
		args = append(args, date)
		idx++
	}
	_ = idx
	q += " ORDER BY created_at DESC LIMIT 200"

	rows, err := pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var logs []model.VisitorLog
	for rows.Next() {
		var v model.VisitorLog
		if err := rows.Scan(&v.ID, &v.SiteID, &v.GuardID, &v.VisitorName,
			&v.IDType, &v.IDNumber, &v.Plate, &v.HostName,
			&v.EntryAt, &v.ExitAt, &v.Purpose, &v.PhotoObjectKey, &v.CreatedAt); err != nil {
			return nil, err
		}
		logs = append(logs, v)
	}
	return logs, rows.Err()
}

// CreateVisitorLog inserts a new visitor log.
func CreateVisitorLog(ctx context.Context, pool *pgxpool.Pool, v *model.VisitorLog) (*model.VisitorLog, error) {
	var out model.VisitorLog
	err := pool.QueryRow(ctx,
		`INSERT INTO visitor_logs
		 (id, site_id, guard_id, visitor_name, id_type, id_number, plate,
		  host_name, entry_at, exit_at, purpose, photo_object_key, created_at)
		 VALUES (gen_random_uuid(), $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, NOW())
		 RETURNING id, site_id, guard_id, visitor_name, id_type, id_number, plate,
		           host_name, entry_at, exit_at, purpose, photo_object_key, created_at`,
		v.SiteID, v.GuardID, v.VisitorName, v.IDType, v.IDNumber, v.Plate,
		v.HostName, v.EntryAt, v.ExitAt, v.Purpose, v.PhotoObjectKey,
	).Scan(&out.ID, &out.SiteID, &out.GuardID, &out.VisitorName,
		&out.IDType, &out.IDNumber, &out.Plate, &out.HostName,
		&out.EntryAt, &out.ExitAt, &out.Purpose, &out.PhotoObjectKey, &out.CreatedAt)
	return &out, err
}

// UpdateVisitorExit marks exit time for a visitor log.
func UpdateVisitorExit(ctx context.Context, pool *pgxpool.Pool, id string, exitAt time.Time) (*model.VisitorLog, error) {
	var out model.VisitorLog
	err := pool.QueryRow(ctx,
		`UPDATE visitor_logs SET exit_at = $1 WHERE id = $2
		 RETURNING id, site_id, guard_id, visitor_name, id_type, id_number, plate,
		           host_name, entry_at, exit_at, purpose, photo_object_key, created_at`,
		exitAt, id,
	).Scan(&out.ID, &out.SiteID, &out.GuardID, &out.VisitorName,
		&out.IDType, &out.IDNumber, &out.Plate, &out.HostName,
		&out.EntryAt, &out.ExitAt, &out.Purpose, &out.PhotoObjectKey, &out.CreatedAt)
	return &out, err
}
