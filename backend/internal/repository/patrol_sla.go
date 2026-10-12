package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/securepatrol/backend/internal/model"
)

// DeviceContact describes a tablet's connectivity: OnlineSince is the start
// of its current online period (requests less than 3 minutes apart) and
// LastSeen its latest request (written at most once a minute).
type DeviceContact struct {
	OnlineSince *time.Time
	LastSeen    *time.Time
}

// GetDeviceContact returns the connectivity of a device.
func GetDeviceContact(ctx context.Context, db DBTX, deviceID string) (*DeviceContact, error) {
	var c DeviceContact
	err := db.QueryRow(ctx, `SELECT online_since, last_seen_at FROM devices WHERE id = $1`, deviceID).
		Scan(&c.OnlineSince, &c.LastSeen)
	if err != nil {
		return nil, mapNoRows(err)
	}
	return &c, nil
}

// ListUncheckedEndedSessions returns ended sessions whose SLA has not been
// evaluated yet (oldest first).
func ListUncheckedEndedSessions(ctx context.Context, db DBTX, limit int) ([]model.PatrolSession, error) {
	rows, err := db.Query(ctx, `
		SELECT `+sessionColumns+` FROM patrol_sessions
		WHERE status <> 'in_progress' AND sla_checked_at IS NULL
		ORDER BY ended_at
		LIMIT $1`, limit)
	if err != nil {
		return nil, fmt.Errorf("ListUncheckedEndedSessions: %w", err)
	}
	return collect(rows, scanSessionRows)
}

// MarkSessionSLAChecked records that an ended session's SLA was evaluated.
func MarkSessionSLAChecked(ctx context.Context, db DBTX, id string) error {
	if _, err := db.Exec(ctx, `UPDATE patrol_sessions SET sla_checked_at = now() WHERE id = $1`, id); err != nil {
		return fmt.Errorf("MarkSessionSLAChecked: %w", err)
	}
	return nil
}

// ListStaleSessionIDs returns in-progress sessions started before now-maxAge.
func ListStaleSessionIDs(ctx context.Context, db DBTX, maxAge time.Duration) ([]string, error) {
	rows, err := db.Query(ctx, `
		SELECT id FROM patrol_sessions
		WHERE status = 'in_progress' AND started_at < now() - $1::interval
		ORDER BY started_at`, maxAge)
	if err != nil {
		return nil, fmt.Errorf("ListStaleSessionIDs: %w", err)
	}
	return collect(rows, func(r pgx.Rows) (string, error) {
		var id string
		err := r.Scan(&id)
		return id, err
	})
}
