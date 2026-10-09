package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/securepatrol/backend/internal/model"
)

const backupColumns = `id, object_key, size_bytes, status, trigger, error, requested_by,
	created_at, started_at, finished_at, keycloak_object_key, keycloak_size_bytes`

func scanBackup(row pgx.Row) (model.Backup, error) {
	var b model.Backup
	err := row.Scan(&b.ID, &b.ObjectKey, &b.SizeBytes, &b.Status, &b.Trigger, &b.Error,
		&b.RequestedBy, &b.CreatedAt, &b.StartedAt, &b.FinishedAt, &b.KeycloakObjectKey, &b.KeycloakSizeBytes)
	return b, err
}

func scanBackupRows(rows pgx.Rows) (model.Backup, error) { return scanBackup(rows) }

// CreateBackup records a pending backup run.
func CreateBackup(ctx context.Context, db DBTX, objectKey, trigger string, requestedBy *string) (*model.Backup, error) {
	b, err := scanBackup(db.QueryRow(ctx, `
		INSERT INTO backups (object_key, trigger, requested_by)
		VALUES ($1, $2, $3)
		RETURNING `+backupColumns, objectKey, trigger, requestedBy))
	if err != nil {
		return nil, fmt.Errorf("CreateBackup: %w", err)
	}
	return &b, nil
}

// ClaimPendingBackup marks the oldest pending backup as running and returns
// it; ErrNotFound when none is pending.
func ClaimPendingBackup(ctx context.Context, db DBTX) (*model.Backup, error) {
	b, err := scanBackup(db.QueryRow(ctx, `
		UPDATE backups SET status = 'running', started_at = now()
		WHERE id = (
		  SELECT id FROM backups WHERE status = 'pending'
		  ORDER BY created_at LIMIT 1 FOR UPDATE SKIP LOCKED)
		RETURNING `+backupColumns))
	if err != nil {
		return nil, mapNoRows(err)
	}
	return &b, nil
}

// BackupResult is the outcome of a backup run.
type BackupResult struct {
	Size              *int64
	KeycloakObjectKey *string
	KeycloakSize      *int64
	Err               *string // nil = succeeded
}

// FinishBackup stores the outcome of a backup run.
func FinishBackup(ctx context.Context, db DBTX, id string, r BackupResult) error {
	status := "succeeded"
	if r.Err != nil {
		status = "failed"
	}
	_, err := db.Exec(ctx, `
		UPDATE backups SET status = $2, size_bytes = $3, error = $4, finished_at = now(),
		       keycloak_object_key = $5, keycloak_size_bytes = $6
		WHERE id = $1`, id, status, r.Size, r.Err, r.KeycloakObjectKey, r.KeycloakSize)
	if err != nil {
		return fmt.Errorf("FinishBackup: %w", err)
	}
	return nil
}

// HasScheduledBackupSince reports whether a scheduled backup was created at or after since.
func HasScheduledBackupSince(ctx context.Context, db DBTX, since time.Time) (bool, error) {
	var exists bool
	err := db.QueryRow(ctx, `
		SELECT EXISTS (SELECT 1 FROM backups WHERE trigger = 'scheduled' AND created_at >= $1)`,
		since).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("HasScheduledBackupSince: %w", err)
	}
	return exists, nil
}

// ListBackups returns recent backups, newest first.
func ListBackups(ctx context.Context, db DBTX, limit int) ([]model.Backup, error) {
	rows, err := db.Query(ctx, `SELECT `+backupColumns+` FROM backups ORDER BY created_at DESC LIMIT $1`, limit)
	if err != nil {
		return nil, fmt.Errorf("ListBackups: %w", err)
	}
	return collect(rows, scanBackupRows)
}

// FailRunningBackups marks every run still "running" as failed. Call it
// only while holding the backup lock: then no process can be running them,
// so they were interrupted (worker crashed or was killed).
func FailRunningBackups(ctx context.Context, db DBTX, reason string) (int64, error) {
	tag, err := db.Exec(ctx, `
		UPDATE backups SET status = 'failed', error = $1, finished_at = now()
		WHERE status = 'running'`, reason)
	if err != nil {
		return 0, fmt.Errorf("FailRunningBackups: %w", err)
	}
	return tag.RowsAffected(), nil
}

// BackupHealth summarises backup history for alerting.
type BackupHealth struct {
	LastSuccess *time.Time // finish time of the newest successful backup
	FirstRun    *time.Time // creation time of the oldest backup row
}

// GetBackupHealth returns the newest success and the oldest run.
func GetBackupHealth(ctx context.Context, db DBTX) (BackupHealth, error) {
	var h BackupHealth
	err := db.QueryRow(ctx, `
		SELECT (SELECT max(finished_at) FROM backups WHERE status = 'succeeded'),
		       (SELECT min(created_at) FROM backups)`).Scan(&h.LastSuccess, &h.FirstRun)
	if err != nil {
		return h, fmt.Errorf("GetBackupHealth: %w", err)
	}
	return h, nil
}
