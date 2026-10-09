package jobs

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/securepatrol/backend/internal/metrics"
	"github.com/securepatrol/backend/internal/model"
	"github.com/securepatrol/backend/internal/notification"
	"github.com/securepatrol/backend/internal/repository"
	"github.com/securepatrol/backend/internal/service"
)

const (
	// backupStaleAfter raises an ops alert when no backup succeeded for this
	// long (daily schedule + 2 h slack).
	backupStaleAfter = 26 * time.Hour
	// backupLockName is the advisory lock held while backups run; holding it
	// proves that no other worker is running a backup.
	backupLockName = "backups:run"
)

func (s *Scheduler) backupTimeout() time.Duration {
	if s.cfg != nil && s.cfg.BackupTimeout > 0 {
		return s.cfg.BackupTimeout
	}
	return time.Hour
}

func (s *Scheduler) backupLocation() *time.Location {
	if s.cfg != nil && s.cfg.BackupLocation != nil {
		return s.cfg.BackupLocation
	}
	if loc, err := time.LoadLocation("Asia/Jakarta"); err == nil {
		return loc
	}
	return time.UTC
}

// backupDue reports whether the daily backup of now's calendar day (in now's
// location) is due, and that day's start.
func backupDue(now time.Time, hour int) (time.Time, bool) {
	dayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	return dayStart, now.Hour() >= hour
}

// failInterruptedBackups marks backups left "running" by a crashed or killed
// worker as failed. It only does so while holding the backup lock, so a
// backup running in another worker replica is never touched.
func (s *Scheduler) failInterruptedBackups(ctx context.Context) error {
	release, ok := s.advisoryLock(ctx, backupLockName)
	if !ok {
		return nil
	}
	defer release()
	n, err := repository.FailRunningBackups(ctx, s.pool, "interrupted: the worker stopped during the backup")
	if n > 0 {
		s.log.Warn().Int64("backups", n).Msg("marked interrupted backups as failed")
	}
	return err
}

// Backups schedules the daily dump at BACKUP_HOUR in BACKUP_TZ, runs every
// pending backup (scheduled or requested from the web UI, NFR-07) and raises
// ops alerts for failed or stale backups.
func (s *Scheduler) Backups(ctx context.Context) error {
	release, ok := s.advisoryLock(ctx, backupLockName)
	if !ok {
		return nil // another worker is running backups
	}
	defer release()
	if n, err := repository.FailRunningBackups(ctx, s.pool, "interrupted: the worker stopped during the backup"); err != nil {
		return err
	} else if n > 0 {
		s.log.Warn().Int64("backups", n).Msg("marked interrupted backups as failed")
	}

	now := service.Now().In(s.backupLocation())
	if dayStart, due := backupDue(now, s.cfg.BackupHour); due {
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

	var runErr error
	for ctx.Err() == nil {
		b, err := repository.ClaimPendingBackup(ctx, s.pool)
		if errors.Is(err, repository.ErrNotFound) {
			break
		}
		if err != nil {
			return err
		}
		if err := s.runBackup(ctx, b); err != nil && runErr == nil {
			runErr = err
		}
	}
	if err := s.checkBackupHealth(ctx); err != nil {
		s.log.Error().Err(err).Msg("backup health check failed")
	}
	return runErr
}

// runBackup dumps the application database (and Keycloak's when configured)
// and records the outcome. A failure raises an ops alert.
func (s *Scheduler) runBackup(ctx context.Context, b *model.Backup) error {
	var res repository.BackupResult
	var errs []string
	size, err := s.dump(ctx, s.cfg.DatabaseURL, b.ObjectKey)
	if err != nil {
		errs = append(errs, "database: "+err.Error())
	} else {
		res.Size = &size
	}
	if s.cfg.BackupKeycloakDatabaseURL != "" {
		key := keycloakObjectKey(b.ObjectKey)
		kSize, err := s.dump(ctx, s.cfg.BackupKeycloakDatabaseURL, key)
		if err != nil {
			errs = append(errs, "keycloak: "+err.Error())
		} else {
			res.KeycloakObjectKey, res.KeycloakSize = &key, &kSize
		}
	}
	if len(errs) > 0 {
		m := strings.Join(errs, "; ")
		res.Err = &m
	}

	// Record the outcome even when ctx was cancelled (shutdown).
	fctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := repository.FinishBackup(fctx, s.pool, b.ID, res); err != nil {
		return err
	}
	if res.Err == nil {
		s.log.Info().Str("backup", b.ID).Int64("bytes", size).Msg("backup stored")
		return nil
	}
	s.log.Error().Str("backup", b.ID).Str("error", *res.Err).Msg("backup failed")
	if s.notifier != nil {
		_, perr := s.notifier.Publish(fctx, notification.Event{
			Type: notification.EventOpsBackupFailed, DedupeKey: "ops:backup_failed:" + b.ID,
			Params: map[string]string{
				"backup": b.ObjectKey, "trigger": b.Trigger, "error": *res.Err,
				"time": time.Now().In(s.backupLocation()).Format("2006-01-02 15:04 MST"),
			},
		})
		if perr != nil {
			s.log.Error().Err(perr).Str("backup", b.ID).Msg("could not raise ops alert for failed backup")
		}
	}
	return fmt.Errorf("backup %s: %s", b.ID, *res.Err)
}

// checkBackupHealth updates the last-success metric and raises a daily ops
// alert when no backup succeeded for backupStaleAfter.
func (s *Scheduler) checkBackupHealth(ctx context.Context) error {
	h, err := repository.GetBackupHealth(ctx, s.pool)
	if err != nil {
		return err
	}
	if h.LastSuccess != nil {
		metrics.BackupLastSuccess.Set(float64(h.LastSuccess.Unix()))
	}
	now := service.Now()
	since := h.LastSuccess
	if since == nil {
		since = h.FirstRun // never succeeded: stale once the first run is old
	}
	if since == nil || now.Sub(*since) < backupStaleAfter || s.notifier == nil {
		return nil
	}
	loc := s.backupLocation()
	last := "never"
	if h.LastSuccess != nil {
		last = h.LastSuccess.In(loc).Format("2006-01-02 15:04 MST")
	}
	_, err = s.notifier.Publish(ctx, notification.Event{
		Type:      notification.EventOpsBackupStale,
		DedupeKey: "ops:backup_stale:" + now.In(loc).Format("2006-01-02"),
		Params:    map[string]string{"last": last, "hour": fmt.Sprintf("%02d", s.cfg.BackupHour)},
	})
	return err
}

// keycloakObjectKey names the Keycloak dump of a backup run.
func keycloakObjectKey(appKey string) string {
	name := appKey
	if i := strings.LastIndex(name, "/"); i >= 0 {
		name = name[i+1:]
	}
	return "keycloak/" + strings.Replace(name, "securepatrol-", "keycloak-", 1)
}

// dump streams `pg_dump -Fc` of one database straight into MinIO. The
// password goes to pg_dump through PGPASSWORD, never on its command line.
func (s *Scheduler) dump(ctx context.Context, databaseURL, key string) (int64, error) {
	dbArg, password, err := pgDumpTarget(databaseURL)
	if err != nil {
		return 0, err
	}
	ctx, cancel := context.WithTimeout(ctx, s.backupTimeout())
	defer cancel()
	// #nosec G204 -- fixed binary; the connection string comes from our own config.
	cmd := exec.CommandContext(ctx, "pg_dump", "--format=custom", "--no-owner", "--no-privileges",
		"--dbname="+dbArg)
	cmd.Env = os.Environ()
	if password != "" {
		cmd.Env = append(cmd.Env, "PGPASSWORD="+password)
	}
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
		if ctx.Err() != nil {
			return 0, fmt.Errorf("pg_dump: %w (timeout %s)", ctx.Err(), s.backupTimeout())
		}
		return 0, fmt.Errorf("pg_dump: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	return size, nil
}

// pgDumpTarget splits a connection string into a --dbname value without the
// password and the password itself (for PGPASSWORD). It accepts URLs
// (postgres://user:pass@host/db?...) and key=value strings.
func pgDumpTarget(dsn string) (dbname, password string, err error) {
	dsn = strings.TrimSpace(dsn)
	if strings.HasPrefix(dsn, "postgres://") || strings.HasPrefix(dsn, "postgresql://") {
		u, err := url.Parse(dsn)
		if err != nil {
			return "", "", errors.New("database URL is not a valid URL (escape special characters in the password)")
		}
		if u.User != nil {
			password, _ = u.User.Password()
			u.User = url.User(u.User.Username())
		}
		q := u.Query()
		if p := q.Get("password"); p != "" {
			password = p
			q.Del("password")
			u.RawQuery = q.Encode()
		}
		return u.String(), password, nil
	}
	// key=value form: drop the password=... pair (values may be quoted).
	var kept []string
	for _, f := range splitKeyValues(dsn) {
		if k, v, ok := strings.Cut(f, "="); ok && strings.TrimSpace(k) == "password" {
			password = strings.Trim(strings.TrimSpace(v), "'")
			continue
		}
		kept = append(kept, f)
	}
	return strings.Join(kept, " "), password, nil
}

// splitKeyValues splits "a=1 b='x y'" on spaces outside single quotes.
func splitKeyValues(s string) []string {
	var out []string
	var cur strings.Builder
	quoted := false
	for _, r := range s {
		switch {
		case r == '\'':
			quoted = !quoted
			cur.WriteRune(r)
		case r == ' ' && !quoted:
			if cur.Len() > 0 {
				out = append(out, cur.String())
				cur.Reset()
			}
		default:
			cur.WriteRune(r)
		}
	}
	if cur.Len() > 0 {
		out = append(out, cur.String())
	}
	return out
}
