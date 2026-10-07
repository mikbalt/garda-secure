package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/securepatrol/backend/internal/model"
)

const backupColumns = `id, object_key, size_bytes, status, trigger, error, requested_by,
	created_at, started_at, finished_at`

func scanBackup(row pgx.Row) (model.Backup, error) {
	var b model.Backup
	err := row.Scan(&b.ID, &b.ObjectKey, &b.SizeBytes, &b.Status, &b.Trigger, &b.Error,
		&b.RequestedBy, &b.CreatedAt, &b.StartedAt, &b.FinishedAt)
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

// FinishBackup stores the outcome of a backup run.
func FinishBackup(ctx context.Context, db DBTX, id string, size *int64, errMsg *string) error {
	status := "succeeded"
	if errMsg != nil {
		status = "failed"
	}
	_, err := db.Exec(ctx, `
		UPDATE backups SET status = $2, size_bytes = $3, error = $4, finished_at = now()
		WHERE id = $1`, id, status, size, errMsg)
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

// FailStaleBackups marks runs stuck in "running" (e.g. worker crashed) as failed.
func FailStaleBackups(ctx context.Context, db DBTX) error {
	_, err := db.Exec(ctx, `
		UPDATE backups SET status = 'failed', error = 'interrupted', finished_at = now()
		WHERE status = 'running' AND started_at < now() - interval '2 hours'`)
	return err
}
