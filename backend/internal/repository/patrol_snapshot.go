package repository

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/securepatrol/backend/internal/model"
)

// sessionRouteColumns selects a session's route with the checkpoints of the
// session's snapshot (taken at start, F9); sessions from before the
// snapshot existed fall back to the current checkpoints.
const sessionRouteColumns = `r.id, r.site_id, r.name, COALESCE(ps.route_snapshot, r.checkpoints), r.active, r.created_at`

// GetSessionRoute returns the route a session is evaluated against.
func GetSessionRoute(ctx context.Context, db DBTX, sessionID string) (*model.Route, error) {
	r, err := scanRoute(db.QueryRow(ctx, `
		SELECT `+sessionRouteColumns+`
		FROM patrol_sessions ps JOIN routes r ON r.id = ps.route_id
		WHERE ps.id = $1`, sessionID))
	if err != nil {
		return nil, mapNoRows(err)
	}
	return &r, nil
}

// ListSessionRoutes returns the evaluation route of each session, keyed by
// session ID (one query for many sessions).
func ListSessionRoutes(ctx context.Context, db DBTX, sessionIDs []string) (map[string]model.Route, error) {
	rows, err := db.Query(ctx, `
		SELECT ps.id, `+sessionRouteColumns+`
		FROM patrol_sessions ps JOIN routes r ON r.id = ps.route_id
		WHERE ps.id = ANY($1::uuid[])`, sessionIDs)
	if err != nil {
		return nil, fmt.Errorf("ListSessionRoutes: %w", err)
	}
	out := map[string]model.Route{}
	_, err = collect(rows, func(row pgx.Rows) (struct{}, error) {
		var id string
		var r model.Route
		err := row.Scan(&id, &r.ID, &r.SiteID, &r.Name, &r.Checkpoints, &r.Active, &r.CreatedAt)
		if r.Checkpoints == nil {
			r.Checkpoints = []model.Checkpoint{}
		}
		out[id] = r
		return struct{}{}, err
	})
	return out, err
}

// ListScansForSessions returns the scans of many sessions in time order,
// keyed by session ID.
func ListScansForSessions(ctx context.Context, db DBTX, sessionIDs []string) (map[string][]model.CheckpointScan, error) {
	rows, err := db.Query(ctx, `
		SELECT `+scanColumns+` FROM checkpoint_scans
		WHERE session_id = ANY($1::uuid[]) ORDER BY session_id, scanned_at`, sessionIDs)
	if err != nil {
		return nil, fmt.Errorf("ListScansForSessions: %w", err)
	}
	scans, err := collect(rows, scanScanRows)
	if err != nil {
		return nil, err
	}
	out := map[string][]model.CheckpointScan{}
	for _, s := range scans {
		if s.SessionID != nil {
			out[*s.SessionID] = append(out[*s.SessionID], s)
		}
	}
	return out, nil
}
