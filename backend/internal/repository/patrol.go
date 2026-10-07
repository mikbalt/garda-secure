package repository

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/securepatrol/backend/internal/model"
)

// RouteReport holds patrol completion stats for a route on a given date.
type RouteReport struct {
	RouteID            string               `json:"route_id"`
	Date               string               `json:"date"`
	TotalCheckpoints   int                  `json:"total_checkpoints"`
	ScannedCheckpoints int                  `json:"scanned_checkpoints"`
	CompletionPct      float64              `json:"completion_pct"`
	MissedCheckpoints  []model.Checkpoint   `json:"missed_checkpoints"`
	Scans              []model.CheckpointScan `json:"scans"`
}

// ListRoutes returns all routes for a site.
func ListRoutes(ctx context.Context, pool *pgxpool.Pool, siteID string) ([]model.Route, error) {
	query := `SELECT id, site_id, name, checkpoints, active, created_at FROM routes WHERE 1=1`
	args := []any{}
	if siteID != "" {
		args = append(args, siteID)
		query += fmt.Sprintf(" AND site_id = $%d", len(args))
	}
	query += " ORDER BY name"

	rows, err := pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("ListRoutes: %w", err)
	}
	defer rows.Close()

	var routes []model.Route
	for rows.Next() {
		var r model.Route
		if err := rows.Scan(&r.ID, &r.SiteID, &r.Name, &r.Checkpoints, &r.Active, &r.CreatedAt); err != nil {
			return nil, fmt.Errorf("ListRoutes scan: %w", err)
		}
		routes = append(routes, r)
	}
	return routes, rows.Err()
}

// GetRoute retrieves a single route by ID.
func GetRoute(ctx context.Context, pool *pgxpool.Pool, id string) (*model.Route, error) {
	var r model.Route
	err := pool.QueryRow(ctx,
		`SELECT id, site_id, name, checkpoints, active, created_at FROM routes WHERE id = $1`,
		id,
	).Scan(&r.ID, &r.SiteID, &r.Name, &r.Checkpoints, &r.Active, &r.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("GetRoute: %w", err)
	}
	return &r, nil
}

// CreateRoute inserts a new patrol route.
func CreateRoute(ctx context.Context, pool *pgxpool.Pool, r model.Route) (*model.Route, error) {
	err := pool.QueryRow(ctx,
		`INSERT INTO routes (site_id, name, checkpoints, active)
		 VALUES ($1, $2, $3, $4)
		 RETURNING id, site_id, name, checkpoints, active, created_at`,
		r.SiteID, r.Name, r.Checkpoints, r.Active,
	).Scan(&r.ID, &r.SiteID, &r.Name, &r.Checkpoints, &r.Active, &r.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("CreateRoute: %w", err)
	}
	return &r, nil
}

// RecordScan inserts a checkpoint scan record.
func RecordScan(ctx context.Context, pool *pgxpool.Pool, scan model.CheckpointScan) (*model.CheckpointScan, error) {
	err := pool.QueryRow(ctx,
		`INSERT INTO checkpoint_scans (route_id, checkpoint_id, guard_id, scanned_at, method, photo_object_key, gps_lat, gps_lng)
		 VALUES ($1, $2, $3, now(), $4, $5, $6, $7)
		 RETURNING id, route_id, checkpoint_id, guard_id, scanned_at, method, photo_object_key, gps_lat, gps_lng`,
		scan.RouteID, scan.CheckpointID, scan.GuardID, scan.Method,
		scan.PhotoObjectKey, scan.GPSLat, scan.GPSLng,
	).Scan(&scan.ID, &scan.RouteID, &scan.CheckpointID, &scan.GuardID,
		&scan.ScannedAt, &scan.Method, &scan.PhotoObjectKey, &scan.GPSLat, &scan.GPSLng)
	if err != nil {
		return nil, fmt.Errorf("RecordScan: %w", err)
	}
	return &scan, nil
}

// ListScans returns scans for a route on a given date.
func ListScans(ctx context.Context, pool *pgxpool.Pool, routeID, date string) ([]model.CheckpointScan, error) {
	rows, err := pool.Query(ctx,
		`SELECT id, route_id, checkpoint_id, guard_id, scanned_at, method, photo_object_key, gps_lat, gps_lng
		 FROM checkpoint_scans
		 WHERE route_id = $1 AND scanned_at::date = $2::date
		 ORDER BY scanned_at`,
		routeID, date,
	)
	if err != nil {
		return nil, fmt.Errorf("ListScans: %w", err)
	}
	defer rows.Close()

	var scans []model.CheckpointScan
	for rows.Next() {
		var s model.CheckpointScan
		if err := rows.Scan(&s.ID, &s.RouteID, &s.CheckpointID, &s.GuardID,
			&s.ScannedAt, &s.Method, &s.PhotoObjectKey, &s.GPSLat, &s.GPSLng); err != nil {
			return nil, fmt.Errorf("ListScans scan: %w", err)
		}
		scans = append(scans, s)
	}
	return scans, rows.Err()
}

// GetRouteReport returns completion stats for a route on a date.
func GetRouteReport(ctx context.Context, pool *pgxpool.Pool, routeID, date string) (*RouteReport, error) {
	route, err := GetRoute(ctx, pool, routeID)
	if err != nil {
		return nil, err
	}

	// Parse checkpoints from JSONB.
	var checkpoints []model.Checkpoint
	if err := json.Unmarshal(route.Checkpoints, &checkpoints); err != nil {
		return nil, fmt.Errorf("parse checkpoints: %w", err)
	}

	scans, err := ListScans(ctx, pool, routeID, date)
	if err != nil {
		return nil, err
	}

	// Index scanned checkpoint IDs.
	scanned := map[string]bool{}
	for _, s := range scans {
		scanned[s.CheckpointID] = true
	}

	missed := []model.Checkpoint{}
	for _, cp := range checkpoints {
		if !scanned[cp.ID] {
			missed = append(missed, cp)
		}
	}

	total := len(checkpoints)
	done := len(checkpoints) - len(missed)
	var pct float64
	if total > 0 {
		pct = float64(done) / float64(total) * 100
	}

	return &RouteReport{
		RouteID:            routeID,
		Date:               date,
		TotalCheckpoints:   total,
		ScannedCheckpoints: done,
		CompletionPct:      pct,
		MissedCheckpoints:  missed,
		Scans:              scans,
	}, nil
}
