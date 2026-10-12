package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// SetDeviceHealth stores the latest health beacon of a tablet.
func SetDeviceHealth(ctx context.Context, db DBTX, deviceID string, health []byte, at time.Time) error {
	tag, err := db.Exec(ctx, `UPDATE devices SET health = $2, health_at = $3 WHERE id = $1`, deviceID, health, at)
	if err != nil {
		return fmt.Errorf("SetDeviceHealth: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// DeviceHealth is a stored health beacon.
type DeviceHealth struct {
	Health json.RawMessage
	At     time.Time
}

// ListDeviceHealth returns the latest beacon of each device that sent one.
func ListDeviceHealth(ctx context.Context, db DBTX, ids []string) (map[string]DeviceHealth, error) {
	rows, err := db.Query(ctx, `
		SELECT id, health, health_at FROM devices
		WHERE id = ANY($1::uuid[]) AND health IS NOT NULL AND health_at IS NOT NULL`, ids)
	if err != nil {
		return nil, fmt.Errorf("ListDeviceHealth: %w", err)
	}
	out := map[string]DeviceHealth{}
	_, err = collect(rows, func(r pgx.Rows) (struct{}, error) {
		var id string
		var h DeviceHealth
		err := r.Scan(&id, &h.Health, &h.At)
		out[id] = h
		return struct{}{}, err
	})
	return out, err
}
