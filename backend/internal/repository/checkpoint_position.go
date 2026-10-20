package repository

import (
	"context"
	"fmt"
	"time"
)

// LearnCheckpointPositions sets the GPS position of route checkpoints to the
// median of their QR scans since `since` (D-059). A checkpoint is learned once
// it has at least minScans scans with a GPS fix by at least minGuards guards;
// manual entries and scans whose signed QR failed verification are ignored.
// Positions are rounded to 6 decimals (~0.1 m) and a route is only rewritten
// when a position changed. The checkpoints JSON is rebuilt from the row being
// updated, so a concurrent route edit is not overwritten. Returns the number
// of routes updated.
func LearnCheckpointPositions(ctx context.Context, db DBTX, since time.Time, minScans, minGuards int) (int64, error) {
	tag, err := db.Exec(ctx, `
		WITH pos AS (
			SELECT route_id, checkpoint_id,
			       round(percentile_cont(0.5) WITHIN GROUP (ORDER BY gps_lat)::numeric, 6) AS lat,
			       round(percentile_cont(0.5) WITHIN GROUP (ORDER BY gps_lng)::numeric, 6) AS lng
			FROM checkpoint_scans
			WHERE gps_lat IS NOT NULL AND gps_lng IS NOT NULL AND scanned_at >= $1
			  AND method = 'qr' AND NOT ('qr_invalid' = ANY(flags))
			GROUP BY route_id, checkpoint_id
			HAVING count(*) >= $2 AND count(DISTINCT guard_id) >= $3
		)
		UPDATE routes r SET checkpoints = (
			SELECT jsonb_agg(CASE WHEN p.lat IS NULL THEN e.cp
			                      ELSE e.cp || jsonb_build_object('lat', p.lat, 'lng', p.lng) END
			                 ORDER BY e.ord)
			FROM jsonb_array_elements(r.checkpoints) WITH ORDINALITY AS e(cp, ord)
			LEFT JOIN pos p ON p.route_id = r.id AND p.checkpoint_id = e.cp->>'id')
		WHERE EXISTS (
			SELECT 1 FROM jsonb_array_elements(r.checkpoints) AS e(cp)
			JOIN pos p ON p.route_id = r.id AND p.checkpoint_id = e.cp->>'id'
			WHERE (e.cp->>'lat')::numeric IS DISTINCT FROM p.lat
			   OR (e.cp->>'lng')::numeric IS DISTINCT FROM p.lng)`,
		since, minScans, minGuards)
	if err != nil {
		return 0, fmt.Errorf("LearnCheckpointPositions: %w", err)
	}
	return tag.RowsAffected(), nil
}
