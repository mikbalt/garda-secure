package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/securepatrol/backend/internal/model"
)

// OpsAlertRule routes one install-wide operations event (backup failed,
// SOS undelivered, ...) to one recipient. Unlike notification_rules it has
// no site: ops events concern the whole installation.
type OpsAlertRule struct {
	ID        string    `json:"id"`
	EventType string    `json:"event_type"`
	Channel   string    `json:"channel"` // email | whatsapp
	Recipient string    `json:"recipient"`
	Enabled   bool      `json:"enabled"`
	CreatedBy *string   `json:"created_by,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

const opsRuleColumns = `id, event_type, channel, recipient, enabled, created_by, created_at`

func scanOpsRule(row pgx.Row) (OpsAlertRule, error) {
	var r OpsAlertRule
	err := row.Scan(&r.ID, &r.EventType, &r.Channel, &r.Recipient, &r.Enabled, &r.CreatedBy, &r.CreatedAt)
	return r, err
}

func scanOpsRuleRows(rows pgx.Rows) (OpsAlertRule, error) { return scanOpsRule(rows) }

// ListOpsAlertRules returns every ops alert rule.
func ListOpsAlertRules(ctx context.Context, db DBTX) ([]OpsAlertRule, error) {
	rows, err := db.Query(ctx, `SELECT `+opsRuleColumns+` FROM ops_alert_rules ORDER BY event_type, channel, recipient`)
	if err != nil {
		return nil, fmt.Errorf("ListOpsAlertRules: %w", err)
	}
	return collect(rows, scanOpsRuleRows)
}

// ListOpsRecipients returns the enabled ops rules of one event type.
func ListOpsRecipients(ctx context.Context, db DBTX, eventType string) ([]OpsAlertRule, error) {
	rows, err := db.Query(ctx, `
		SELECT `+opsRuleColumns+` FROM ops_alert_rules WHERE event_type = $1 AND enabled`, eventType)
	if err != nil {
		return nil, fmt.Errorf("ListOpsRecipients: %w", err)
	}
	return collect(rows, scanOpsRuleRows)
}

// GetOpsAlertRule retrieves an ops rule by ID.
func GetOpsAlertRule(ctx context.Context, db DBTX, id string) (*OpsAlertRule, error) {
	r, err := scanOpsRule(db.QueryRow(ctx, `SELECT `+opsRuleColumns+` FROM ops_alert_rules WHERE id = $1`, id))
	if err != nil {
		return nil, mapNoRows(err)
	}
	return &r, nil
}

// CreateOpsAlertRule inserts an ops rule.
func CreateOpsAlertRule(ctx context.Context, db DBTX, r OpsAlertRule) (*OpsAlertRule, error) {
	out, err := scanOpsRule(db.QueryRow(ctx, `
		INSERT INTO ops_alert_rules (event_type, channel, recipient, enabled, created_by)
		VALUES ($1, $2, $3, true, $4)
		RETURNING `+opsRuleColumns, r.EventType, r.Channel, r.Recipient, r.CreatedBy))
	if err != nil {
		return nil, fmt.Errorf("CreateOpsAlertRule: %w", err)
	}
	return &out, nil
}

// SetOpsAlertRuleEnabled toggles an ops rule.
func SetOpsAlertRuleEnabled(ctx context.Context, db DBTX, id string, enabled bool) (*OpsAlertRule, error) {
	r, err := scanOpsRule(db.QueryRow(ctx, `
		UPDATE ops_alert_rules SET enabled = $2 WHERE id = $1 RETURNING `+opsRuleColumns, id, enabled))
	if err != nil {
		return nil, mapNoRows(err)
	}
	return &r, nil
}

// DeleteOpsAlertRule removes an ops rule.
func DeleteOpsAlertRule(ctx context.Context, db DBTX, id string) error {
	tag, err := db.Exec(ctx, `DELETE FROM ops_alert_rules WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("DeleteOpsAlertRule: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// ListOpsNotificationLog returns recent install-wide deliveries (rows without
// a site), optionally by status.
func ListOpsNotificationLog(ctx context.Context, db DBTX, status string, limit int) ([]model.NotificationLog, error) {
	rows, err := db.Query(ctx, `
		SELECT `+logColumns+` FROM notification_log
		WHERE site_id IS NULL AND ($1 = '' OR status = $1)
		ORDER BY created_at DESC
		LIMIT $2`, status, limit)
	if err != nil {
		return nil, fmt.Errorf("ListOpsNotificationLog: %w", err)
	}
	return collect(rows, scanLogRows)
}
