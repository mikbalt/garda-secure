package repository

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/securepatrol/backend/internal/model"
)

// ── Rules ────────────────────────────────────────────────────────────────────

const ruleColumns = `id, site_id, event_type, channel, recipient, enabled, created_at`

func scanRule(row pgx.Row) (model.NotificationRule, error) {
	var r model.NotificationRule
	err := row.Scan(&r.ID, &r.SiteID, &r.EventType, &r.Channel, &r.Recipient, &r.Enabled, &r.CreatedAt)
	return r, err
}

func scanRuleRows(rows pgx.Rows) (model.NotificationRule, error) { return scanRule(rows) }

// ListNotificationRules returns the rules of a site.
func ListNotificationRules(ctx context.Context, db DBTX, siteID string) ([]model.NotificationRule, error) {
	rows, err := db.Query(ctx, `
		SELECT `+ruleColumns+` FROM notification_rules
		WHERE site_id = $1 ORDER BY event_type, channel, recipient`, siteID)
	if err != nil {
		return nil, fmt.Errorf("ListNotificationRules: %w", err)
	}
	return collect(rows, scanRuleRows)
}

// ListRecipients returns the enabled rules for one event at a site.
func ListRecipients(ctx context.Context, db DBTX, siteID, eventType string) ([]model.NotificationRule, error) {
	rows, err := db.Query(ctx, `
		SELECT `+ruleColumns+` FROM notification_rules
		WHERE site_id = $1 AND event_type = $2 AND enabled`, siteID, eventType)
	if err != nil {
		return nil, fmt.Errorf("ListRecipients: %w", err)
	}
	return collect(rows, scanRuleRows)
}

// GetNotificationRule retrieves a rule by ID.
func GetNotificationRule(ctx context.Context, db DBTX, id string) (*model.NotificationRule, error) {
	r, err := scanRule(db.QueryRow(ctx, `SELECT `+ruleColumns+` FROM notification_rules WHERE id = $1`, id))
	if err != nil {
		return nil, mapNoRows(err)
	}
	return &r, nil
}

// CreateNotificationRule inserts a rule.
func CreateNotificationRule(ctx context.Context, db DBTX, r model.NotificationRule) (*model.NotificationRule, error) {
	out, err := scanRule(db.QueryRow(ctx, `
		INSERT INTO notification_rules (site_id, event_type, channel, recipient, enabled)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING `+ruleColumns, r.SiteID, r.EventType, r.Channel, r.Recipient, r.Enabled))
	if err != nil {
		return nil, fmt.Errorf("CreateNotificationRule: %w", err)
	}
	return &out, nil
}

// SetNotificationRuleEnabled toggles a rule.
func SetNotificationRuleEnabled(ctx context.Context, db DBTX, id string, enabled bool) (*model.NotificationRule, error) {
	r, err := scanRule(db.QueryRow(ctx, `
		UPDATE notification_rules SET enabled = $2 WHERE id = $1
		RETURNING `+ruleColumns, id, enabled))
	if err != nil {
		return nil, mapNoRows(err)
	}
	return &r, nil
}

// DeleteNotificationRule removes a rule.
func DeleteNotificationRule(ctx context.Context, db DBTX, id string) error {
	tag, err := db.Exec(ctx, `DELETE FROM notification_rules WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("DeleteNotificationRule: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// ── Delivery log ─────────────────────────────────────────────────────────────

const logColumns = `id, site_id, event_type, channel, recipient, subject, body, status,
	attempts, last_error, created_at, sent_at`

func scanLog(row pgx.Row) (model.NotificationLog, error) {
	var l model.NotificationLog
	err := row.Scan(&l.ID, &l.SiteID, &l.EventType, &l.Channel, &l.Recipient, &l.Subject, &l.Body,
		&l.Status, &l.Attempts, &l.LastError, &l.CreatedAt, &l.SentAt)
	return l, err
}

func scanLogRows(rows pgx.Rows) (model.NotificationLog, error) { return scanLog(rows) }

// InsertNotificationLog queues one delivery. When dedupeKey is set and was
// already used, nothing is inserted and ("", nil) is returned.
func InsertNotificationLog(ctx context.Context, db DBTX, l model.NotificationLog, dedupeKey *string) (string, error) {
	var id string
	err := db.QueryRow(ctx, `
		INSERT INTO notification_log (site_id, event_type, channel, recipient, subject, body, dedupe_key)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (dedupe_key) DO NOTHING
		RETURNING id`,
		l.SiteID, l.EventType, l.Channel, l.Recipient, l.Subject, l.Body, dedupeKey,
	).Scan(&id)
	if err != nil {
		if mapNoRows(err) == ErrNotFound {
			return "", nil
		}
		return "", fmt.Errorf("InsertNotificationLog: %w", err)
	}
	return id, nil
}

// GetNotificationLog retrieves a delivery record by ID.
func GetNotificationLog(ctx context.Context, db DBTX, id string) (*model.NotificationLog, error) {
	l, err := scanLog(db.QueryRow(ctx, `SELECT `+logColumns+` FROM notification_log WHERE id = $1`, id))
	if err != nil {
		return nil, mapNoRows(err)
	}
	return &l, nil
}

// ListNotificationLog returns recent deliveries of a set of sites, optionally
// by status.
func ListNotificationLog(ctx context.Context, db DBTX, siteIDs []string, status string, limit int) ([]model.NotificationLog, error) {
	rows, err := db.Query(ctx, `
		SELECT `+logColumns+` FROM notification_log
		WHERE site_id = ANY($1) AND ($2 = '' OR status = $2)
		ORDER BY created_at DESC
		LIMIT $3`, siteIDs, status, limit)
	if err != nil {
		return nil, fmt.Errorf("ListNotificationLog: %w", err)
	}
	return collect(rows, scanLogRows)
}

// MarkNotificationAttempt records the outcome of one delivery attempt.
func MarkNotificationAttempt(ctx context.Context, db DBTX, id, status string, lastErr *string) error {
	_, err := db.Exec(ctx, `
		UPDATE notification_log
		SET status = $2, attempts = attempts + 1, last_error = $3,
		    sent_at = CASE WHEN $2 = 'sent' THEN now() ELSE sent_at END
		WHERE id = $1`, id, status, lastErr)
	if err != nil {
		return fmt.Errorf("MarkNotificationAttempt: %w", err)
	}
	return nil
}

// RequeueNotification resets a failed delivery so it can be retried.
func RequeueNotification(ctx context.Context, db DBTX, id string) error {
	tag, err := db.Exec(ctx, `
		UPDATE notification_log SET status = 'queued', last_error = NULL
		WHERE id = $1 AND status = 'failed'`, id)
	if err != nil {
		return fmt.Errorf("RequeueNotification: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// ListQueuedNotificationIDs returns deliveries still queued after `olderThan`
// seconds; the worker re-enqueues them in case Redis lost them.
func ListQueuedNotificationIDs(ctx context.Context, db DBTX, olderThanSeconds int) ([]string, error) {
	rows, err := db.Query(ctx, `
		SELECT id FROM notification_log
		WHERE status = 'queued' AND created_at < now() - make_interval(secs => $1::int)
		  AND created_at > now() - interval '1 day'
		ORDER BY created_at LIMIT 500`, olderThanSeconds)
	if err != nil {
		return nil, fmt.Errorf("ListQueuedNotificationIDs: %w", err)
	}
	return collect(rows, func(r pgx.Rows) (string, error) {
		var id string
		err := r.Scan(&id)
		return id, err
	})
}

// ── Settings ─────────────────────────────────────────────────────────────────

// GetSetting loads a system setting value; ErrNotFound when unset.
func GetSetting(ctx context.Context, db DBTX, key string) ([]byte, error) {
	var v []byte
	if err := db.QueryRow(ctx, `SELECT value FROM system_settings WHERE key = $1`, key).Scan(&v); err != nil {
		return nil, mapNoRows(err)
	}
	return v, nil
}

// PutSetting upserts a system setting value.
func PutSetting(ctx context.Context, db DBTX, key string, value []byte, actor string) error {
	_, err := db.Exec(ctx, `
		INSERT INTO system_settings (key, value, updated_by, updated_at)
		VALUES ($1, $2, $3, now())
		ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, updated_by = EXCLUDED.updated_by, updated_at = now()`,
		key, value, actor)
	if err != nil {
		return fmt.Errorf("PutSetting: %w", err)
	}
	return nil
}
