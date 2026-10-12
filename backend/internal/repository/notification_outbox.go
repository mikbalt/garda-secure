package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/securepatrol/backend/internal/model"
)

// Delivery is a notification_log row claimed by a worker.
type Delivery struct {
	model.NotificationLog
	Lane      string
	RequestID *string
}

// ClaimDelivery takes the oldest due delivery of a lane and leases it to the
// caller for `lease`, so no other worker (replica) sends it meanwhile. A
// worker that dies mid-send loses the lease and the row is sent again
// (at-least-once). ErrNotFound when nothing is due.
func ClaimDelivery(ctx context.Context, db DBTX, lane string, lease time.Duration) (*Delivery, error) {
	var d Delivery
	err := db.QueryRow(ctx, `
		UPDATE notification_log SET locked_until = now() + make_interval(secs => $2)
		WHERE id = (
		  SELECT id FROM notification_log
		  WHERE status = 'queued' AND lane = $1 AND next_attempt_at <= now()
		    AND (locked_until IS NULL OR locked_until < now())
		  ORDER BY next_attempt_at, created_at
		  LIMIT 1
		  FOR UPDATE SKIP LOCKED)
		RETURNING `+logColumns+`, lane, request_id`, lane, lease.Seconds(),
	).Scan(&d.ID, &d.SiteID, &d.EventType, &d.Channel, &d.Recipient, &d.Subject, &d.Body,
		&d.Status, &d.Attempts, &d.LastError, &d.CreatedAt, &d.SentAt, &d.Lane, &d.RequestID)
	if err != nil {
		return nil, mapNoRows(err)
	}
	return &d, nil
}

// MarkDeliverySent records a successful attempt and releases the lease.
func MarkDeliverySent(ctx context.Context, db DBTX, id string) error {
	_, err := db.Exec(ctx, `
		UPDATE notification_log
		SET status = 'sent', attempts = attempts + 1, last_error = NULL, sent_at = now(), locked_until = NULL
		WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("MarkDeliverySent: %w", err)
	}
	return nil
}

// MarkDeliveryRetry records a failed attempt and schedules the next one.
func MarkDeliveryRetry(ctx context.Context, db DBTX, id, lastErr string, next time.Time) error {
	_, err := db.Exec(ctx, `
		UPDATE notification_log
		SET attempts = attempts + 1, last_error = $2, next_attempt_at = $3, locked_until = NULL
		WHERE id = $1`, id, lastErr, next)
	if err != nil {
		return fmt.Errorf("MarkDeliveryRetry: %w", err)
	}
	return nil
}

// MarkDeliveryFailed records the last failed attempt; the delivery gives up.
func MarkDeliveryFailed(ctx context.Context, db DBTX, id, lastErr string) error {
	_, err := db.Exec(ctx, `
		UPDATE notification_log
		SET status = 'failed', attempts = attempts + 1, last_error = $2, locked_until = NULL
		WHERE id = $1`, id, lastErr)
	if err != nil {
		return fmt.Errorf("MarkDeliveryFailed: %w", err)
	}
	return nil
}

// CountQueuedByLane returns the number of queued deliveries per lane.
func CountQueuedByLane(ctx context.Context, db DBTX) (map[string]int, error) {
	rows, err := db.Query(ctx, `
		SELECT lane, COUNT(*) FROM notification_log WHERE status = 'queued' GROUP BY lane`)
	if err != nil {
		return nil, fmt.Errorf("CountQueuedByLane: %w", err)
	}
	type laneCount struct {
		lane string
		n    int
	}
	counts, err := collect(rows, func(r pgx.Rows) (laneCount, error) {
		var c laneCount
		err := r.Scan(&c.lane, &c.n)
		return c, err
	})
	if err != nil {
		return nil, err
	}
	out := map[string]int{}
	for _, c := range counts {
		out[c.lane] = c.n
	}
	return out, nil
}

// CountUnacknowledgedSos returns the number of unacknowledged SOS alerts of
// every site.
func CountUnacknowledgedSos(ctx context.Context, db DBTX) (int, error) {
	var n int
	if err := db.QueryRow(ctx, `SELECT COUNT(*) FROM sos_alerts WHERE acknowledged_at IS NULL`).Scan(&n); err != nil {
		return 0, fmt.Errorf("CountUnacknowledgedSos: %w", err)
	}
	return n, nil
}
