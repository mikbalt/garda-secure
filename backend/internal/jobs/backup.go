package jobs

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/securepatrol/backend/internal/repository"
	"github.com/securepatrol/backend/internal/service"
)

// Backups schedules the daily dump at BACKUP_HOUR (server local time) and
// runs any pending backup, scheduled or requested from the web UI (NFR-07).
func (s *Scheduler) Backups(ctx context.Context) error {
	if err := repository.FailStaleBackups(ctx, s.pool); err != nil {
		return err
	}
	now := time.Now()
	if now.Hour() >= s.cfg.BackupHour {
		dayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
		done, err := repository.HasScheduledBackupSince(ctx, s.pool, dayStart)
		if err != nil {
			return err
		}
		if !done {
			if _, err := repository.CreateBackup(ctx, s.pool, service.BackupObjectKey(now), "scheduled", nil); err != nil {
				return err
			}
		}
	}

	for {
		b, err := repository.ClaimPendingBackup(ctx, s.pool)
		if errors.Is(err, repository.ErrNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		size, runErr := s.dump(ctx, b.ObjectKey)
		var msg *string
		if runErr != nil {
			m := runErr.Error()
			msg = &m
			s.log.Error().Err(runErr).Str("backup", b.ID).Msg("backup failed")
		} else {
			s.log.Info().Str("backup", b.ID).Int64("bytes", size).Msg("backup stored")
		}
		var sizePtr *int64
		if runErr == nil {
			sizePtr = &size
		}
		if err := repository.FinishBackup(context.Background(), s.pool, b.ID, sizePtr, msg); err != nil {
			return err
		}
	}
}

// dump streams `pg_dump -Fc` straight into MinIO.
func (s *Scheduler) dump(ctx context.Context, key string) (int64, error) {
	ctx, cancel := context.WithTimeout(ctx, time.Hour)
	defer cancel()
	// #nosec G204 -- fixed binary; the connection string comes from our own config.
	cmd := exec.CommandContext(ctx, "pg_dump", "--format=custom", "--no-owner", "--no-privileges",
		"--dbname="+s.cfg.DatabaseURL)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return 0, err
	}
	if err := cmd.Start(); err != nil {
		return 0, fmt.Errorf("start pg_dump: %w", err)
	}
	size, putErr := s.stg.Put(ctx, s.cfg.MinioBucketBackups, key, stdout, -1, "application/octet-stream")
	if putErr != nil {
		cancel() // pg_dump would otherwise block writing to the abandoned pipe
		_ = cmd.Wait()
		return 0, putErr
	}
	if err := cmd.Wait(); err != nil {
		return 0, fmt.Errorf("pg_dump: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	return size, nil
}
