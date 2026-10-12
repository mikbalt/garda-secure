package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// ActiveSos is one unacknowledged SOS alert for the realm-wide web banner
// (WEB-3): small, with the site and guard names resolved.
type ActiveSos struct {
	ID         string    `json:"id"`
	SiteID     string    `json:"site_id"`
	SiteName   string    `json:"site_name"`
	GuardID    string    `json:"guard_id"`
	GuardName  string    `json:"guard_name"`
	FiredAt    time.Time `json:"fired_at"`
	ReceivedAt time.Time `json:"received_at"`
	GPSLat     *float64  `json:"gps_lat,omitempty"`
	GPSLng     *float64  `json:"gps_lng,omitempty"`
}

// ListActiveSos returns the unacknowledged SOS alerts of the given sites,
// newest first, at most limit rows, plus the total number.
func ListActiveSos(ctx context.Context, db DBTX, siteIDs []string, limit int) ([]ActiveSos, int, error) {
	var total int
	if err := db.QueryRow(ctx, `
		SELECT count(*) FROM sos_alerts WHERE acknowledged_at IS NULL AND site_id = ANY($1::uuid[])`,
		siteIDs).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("ListActiveSos count: %w", err)
	}
	rows, err := db.Query(ctx, `
		SELECT a.id, a.site_id, s.name, a.guard_id, COALESCE(g.name, ''), a.fired_at, a.received_at,
		       a.gps_lat, a.gps_lng
		FROM sos_alerts a
		JOIN sites s ON s.id = a.site_id
		LEFT JOIN guards g ON g.id = a.guard_id
		WHERE a.acknowledged_at IS NULL AND a.site_id = ANY($1::uuid[])
		ORDER BY a.fired_at DESC
		LIMIT $2`, siteIDs, limit)
	if err != nil {
		return nil, 0, fmt.Errorf("ListActiveSos: %w", err)
	}
	out, err := collect(rows, func(r pgx.Rows) (ActiveSos, error) {
		var a ActiveSos
		err := r.Scan(&a.ID, &a.SiteID, &a.SiteName, &a.GuardID, &a.GuardName, &a.FiredAt, &a.ReceivedAt,
			&a.GPSLat, &a.GPSLng)
		return a, err
	})
	return out, total, err
}
