package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/securepatrol/backend/internal/model"
)

// ── Routes ───────────────────────────────────────────────────────────────────

const routeColumns = `id, site_id, name, checkpoints, active, created_at`

func scanRoute(row pgx.Row) (model.Route, error) {
	var r model.Route
	err := row.Scan(&r.ID, &r.SiteID, &r.Name, &r.Checkpoints, &r.Active, &r.CreatedAt)
	if r.Checkpoints == nil {
		r.Checkpoints = []model.Checkpoint{}
	}
	return r, err
}

func scanRouteRows(rows pgx.Rows) (model.Route, error) { return scanRoute(rows) }

// ListRoutes returns the routes of a site, optionally only active ones.
func ListRoutes(ctx context.Context, db DBTX, siteID string, activeOnly bool) ([]model.Route, error) {
	rows, err := db.Query(ctx, `
		SELECT `+routeColumns+` FROM routes
		WHERE site_id = $1 AND (NOT $2 OR active)
		ORDER BY name`, siteID, activeOnly)
	if err != nil {
		return nil, fmt.Errorf("ListRoutes: %w", err)
	}
	return collect(rows, scanRouteRows)
}

// GetRoute retrieves a route by ID.
func GetRoute(ctx context.Context, db DBTX, id string) (*model.Route, error) {
	r, err := scanRoute(db.QueryRow(ctx, `SELECT `+routeColumns+` FROM routes WHERE id = $1`, id))
	if err != nil {
		return nil, mapNoRows(err)
	}
	return &r, nil
}

// CreateRoute inserts a patrol route.
func CreateRoute(ctx context.Context, db DBTX, r model.Route) (*model.Route, error) {
	out, err := scanRoute(db.QueryRow(ctx, `
		INSERT INTO routes (site_id, name, checkpoints, active)
		VALUES ($1, $2, $3, $4)
		RETURNING `+routeColumns, r.SiteID, r.Name, r.Checkpoints, r.Active))
	if err != nil {
		return nil, fmt.Errorf("CreateRoute: %w", err)
	}
	return &out, nil
}

// UpdateRoute replaces name, checkpoints and active flag of a route.
func UpdateRoute(ctx context.Context, db DBTX, r model.Route) (*model.Route, error) {
	out, err := scanRoute(db.QueryRow(ctx, `
		UPDATE routes SET name = $2, checkpoints = $3, active = $4
		WHERE id = $1
		RETURNING `+routeColumns, r.ID, r.Name, r.Checkpoints, r.Active))
	if err != nil {
		return nil, mapNoRows(err)
	}
	return &out, nil
}

// ── Sessions ─────────────────────────────────────────────────────────────────

const sessionColumns = `id, route_id, site_id, guard_id, device_id, started_at, ended_at, status, created_at, flags`

func scanSession(row pgx.Row) (model.PatrolSession, error) {
	var s model.PatrolSession
	err := row.Scan(&s.ID, &s.RouteID, &s.SiteID, &s.GuardID, &s.DeviceID,
		&s.StartedAt, &s.EndedAt, &s.Status, &s.CreatedAt, &s.Flags)
	if s.Flags == nil {
		s.Flags = []string{}
	}
	return s, err
}

func scanSessionRows(rows pgx.Rows) (model.PatrolSession, error) { return scanSession(rows) }

// CreateSession starts a patrol session. With a client-supplied ID the call is
// idempotent: a retry returns the existing session and created=false.
func CreateSession(ctx context.Context, db DBTX, s model.PatrolSession, clientID *string) (*model.PatrolSession, bool, error) {
	out, err := scanSession(db.QueryRow(ctx, `
		INSERT INTO patrol_sessions (id, route_id, site_id, guard_id, device_id, started_at, flags, route_snapshot)
		VALUES (COALESCE($1::uuid, gen_random_uuid()), $2, $3, $4, $5, $6, $7,
		        (SELECT checkpoints FROM routes WHERE id = $2)) -- F9: checkpoints as of the start
		ON CONFLICT (id) DO NOTHING
		RETURNING `+sessionColumns,
		clientID, s.RouteID, s.SiteID, s.GuardID, s.DeviceID, s.StartedAt, flagsOrEmpty(s.Flags)))
	if err == nil {
		return &out, true, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) || clientID == nil {
		return nil, false, fmt.Errorf("CreateSession: %w", err)
	}
	existing, err := GetSession(ctx, db, *clientID)
	return existing, false, err
}

// GetSession retrieves a session by ID.
func GetSession(ctx context.Context, db DBTX, id string) (*model.PatrolSession, error) {
	s, err := scanSession(db.QueryRow(ctx, `SELECT `+sessionColumns+` FROM patrol_sessions WHERE id = $1`, id))
	if err != nil {
		return nil, mapNoRows(err)
	}
	return &s, nil
}

// EndSession closes a session that is still in progress.
func EndSession(ctx context.Context, db DBTX, id string, at time.Time, status string) (*model.PatrolSession, error) {
	s, err := scanSession(db.QueryRow(ctx, `
		UPDATE patrol_sessions SET ended_at = $2, status = $3
		WHERE id = $1 AND status = 'in_progress'
		RETURNING `+sessionColumns, id, at, status))
	if err != nil {
		return nil, mapNoRows(err)
	}
	return &s, nil
}

// ListSessions returns the sessions of a site started in [from, to).
func ListSessions(ctx context.Context, db DBTX, siteID string, from, to time.Time) ([]model.PatrolSession, error) {
	rows, err := db.Query(ctx, `
		SELECT `+sessionColumns+` FROM patrol_sessions
		WHERE site_id = $1 AND started_at >= $2 AND started_at < $3
		ORDER BY started_at DESC`, siteID, from, to)
	if err != nil {
		return nil, fmt.Errorf("ListSessions: %w", err)
	}
	return collect(rows, scanSessionRows)
}

// ListActiveSessions returns the in-progress sessions of a site, or of every
// site when siteID is empty.
func ListActiveSessions(ctx context.Context, db DBTX, siteID string) ([]model.PatrolSession, error) {
	rows, err := db.Query(ctx, `
		SELECT `+sessionColumns+` FROM patrol_sessions
		WHERE status = 'in_progress' AND ($1 = '' OR site_id::text = $1)
		ORDER BY started_at`, siteID)
	if err != nil {
		return nil, fmt.Errorf("ListActiveSessions: %w", err)
	}
	return collect(rows, scanSessionRows)
}

// AbandonStaleSessions closes sessions left open longer than maxAge.
func AbandonStaleSessions(ctx context.Context, db DBTX, maxAge time.Duration) (int64, error) {
	tag, err := db.Exec(ctx, `
		UPDATE patrol_sessions SET status = 'abandoned', ended_at = now()
		WHERE status = 'in_progress' AND started_at < now() - $1::interval`, maxAge)
	if err != nil {
		return 0, fmt.Errorf("AbandonStaleSessions: %w", err)
	}
	return tag.RowsAffected(), nil
}

// ── Scans ────────────────────────────────────────────────────────────────────

const scanColumns = `id, session_id, route_id, checkpoint_id, guard_id, device_id, scanned_at,
	received_at, method, photo_object_key, gps_lat, gps_lng, flags`

func scanScan(row pgx.Row) (model.CheckpointScan, error) {
	var s model.CheckpointScan
	err := row.Scan(&s.ID, &s.SessionID, &s.RouteID, &s.CheckpointID, &s.GuardID, &s.DeviceID,
		&s.ScannedAt, &s.ReceivedAt, &s.Method, &s.PhotoObjectKey, &s.GPSLat, &s.GPSLng, &s.Flags)
	if s.Flags == nil {
		s.Flags = []string{}
	}
	return s, err
}

func scanScanRows(rows pgx.Rows) (model.CheckpointScan, error) { return scanScan(rows) }

// InsertScan records a checkpoint scan; idempotent on a client-supplied ID.
func InsertScan(ctx context.Context, db DBTX, s model.CheckpointScan, clientID *string) (*model.CheckpointScan, bool, error) {
	out, err := scanScan(db.QueryRow(ctx, `
		INSERT INTO checkpoint_scans
		  (id, session_id, route_id, checkpoint_id, guard_id, device_id, scanned_at, method,
		   photo_object_key, gps_lat, gps_lng, flags)
		VALUES (COALESCE($1::uuid, gen_random_uuid()), $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		ON CONFLICT (id) DO NOTHING
		RETURNING `+scanColumns,
		clientID, s.SessionID, s.RouteID, s.CheckpointID, s.GuardID, s.DeviceID, s.ScannedAt,
		s.Method, s.PhotoObjectKey, s.GPSLat, s.GPSLng, flagsOrEmpty(s.Flags)))
	if err == nil {
		return &out, true, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) || clientID == nil {
		return nil, false, fmt.Errorf("InsertScan: %w", err)
	}
	existing, err := scanScan(db.QueryRow(ctx, `SELECT `+scanColumns+` FROM checkpoint_scans WHERE id = $1`, *clientID))
	if err != nil {
		return nil, false, mapNoRows(err)
	}
	return &existing, false, nil
}

// ListSessionScans returns the scans of a session in time order.
func ListSessionScans(ctx context.Context, db DBTX, sessionID string) ([]model.CheckpointScan, error) {
	rows, err := db.Query(ctx,
		`SELECT `+scanColumns+` FROM checkpoint_scans WHERE session_id = $1 ORDER BY scanned_at`, sessionID)
	if err != nil {
		return nil, fmt.Errorf("ListSessionScans: %w", err)
	}
	return collect(rows, scanScanRows)
}

// ── SLA breaches ─────────────────────────────────────────────────────────────

// InsertSLABreach records a breach once per (session, checkpoint); returns
// true only the first time so callers notify exactly once.
func InsertSLABreach(ctx context.Context, db DBTX, sessionID, checkpointID string, dueAt time.Time) (bool, error) {
	tag, err := db.Exec(ctx, `
		INSERT INTO sla_breaches (session_id, checkpoint_id, due_at)
		VALUES ($1, $2, $3)
		ON CONFLICT (session_id, checkpoint_id) DO NOTHING`, sessionID, checkpointID, dueAt)
	if err != nil {
		return false, fmt.Errorf("InsertSLABreach: %w", err)
	}
	return tag.RowsAffected() > 0, nil
}

// GetActiveSessionForGuard returns the guard's in-progress session at a site.
func GetActiveSessionForGuard(ctx context.Context, db DBTX, guardID, siteID string) (*model.PatrolSession, error) {
	s, err := scanSession(db.QueryRow(ctx, `
		SELECT `+sessionColumns+` FROM patrol_sessions
		WHERE guard_id = $1 AND site_id = $2 AND status = 'in_progress'
		ORDER BY started_at DESC LIMIT 1`, guardID, siteID))
	if err != nil {
		return nil, mapNoRows(err)
	}
	return &s, nil
}
