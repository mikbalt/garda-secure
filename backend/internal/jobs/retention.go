package jobs

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/securepatrol/backend/internal/metrics"
	"github.com/securepatrol/backend/internal/pii"
	"github.com/securepatrol/backend/internal/repository"
	"github.com/securepatrol/backend/internal/service"
)

// Retention batch sizes: one run handles at most retentionBatches batches per
// kind, so a large backlog drains over several hourly runs.
const (
	retentionBatch   = 500
	retentionBatches = 20
)

// Retention applies the data retention periods (BE-S9, UU PDP 27/2022;
// defaults in config.Retention, documented in the runbook):
//   - visitor logs and their photos are deleted;
//   - clock-in selfies are deleted (object) and their key cleared;
//   - attendance and checkpoint-scan GPS positions are cleared;
//   - notification_log subjects, bodies and recipients are cleared;
//   - incident/defect/checkpoint photos only when RETENTION_EVIDENCE_PHOTO_DAYS > 0.
//
// It also encrypts visitor ID numbers stored before encryption existed.
// Objects are deleted with every version (the photos bucket is versioned).
// Items are isolated: a failing object delete leaves its row for the next
// run and does not stop the others. audit_log is append-only and never
// purged in v1.
func (s *Scheduler) Retention(ctx context.Context) error {
	r := s.cfg.Retention
	now := service.Now()
	cut := func(days int) time.Time { return now.AddDate(0, 0, -days) }
	var errs []error
	step := func(name string, fn func() error) {
		if err := fn(); err != nil {
			s.log.Error().Err(err).Str("step", name).Msg("retention step failed; continuing")
			errs = append(errs, fmt.Errorf("%s: %w", name, err))
		}
	}
	step("encrypt visitor id numbers", func() error { return s.encryptVisitorIDs(ctx) })
	if r.VisitorDays > 0 {
		step("visitor logs", func() error {
			return s.purgeKeyed(ctx, "visitor_log", "visitor_photo", func() ([]repository.KeyedRow, error) {
				return repository.ExpiredVisitors(ctx, s.pool, cut(r.VisitorDays), retentionBatch)
			}, func(id string) error { return repository.DeleteVisitor(ctx, s.pool, id) })
		})
	}
	if r.SelfieDays > 0 {
		step("selfies", func() error {
			return s.purgeKeyed(ctx, "", "selfie", func() ([]repository.KeyedRow, error) {
				return repository.ExpiredSelfies(ctx, s.pool, cut(r.SelfieDays), retentionBatch)
			}, func(id string) error { return repository.ClearSelfie(ctx, s.pool, id) })
		})
	}
	if r.GPSDays > 0 {
		step("attendance gps", func() error {
			return s.clearBatches("attendance_gps", func() (int64, error) {
				return repository.ClearAttendanceGPS(ctx, s.pool, cut(r.GPSDays), retentionBatch*10)
			})
		})
		step("scan gps", func() error {
			return s.clearBatches("scan_gps", func() (int64, error) {
				return repository.ClearScanGPS(ctx, s.pool, cut(r.GPSDays), retentionBatch*10)
			})
		})
	}
	if r.NotificationDays > 0 {
		step("notification log", func() error {
			return s.clearBatches("notification_log", func() (int64, error) {
				return repository.PurgeNotificationLog(ctx, s.pool, cut(r.NotificationDays), retentionBatch*10)
			})
		})
	}
	if r.EvidencePhotoDays > 0 {
		c := cut(r.EvidencePhotoDays)
		step("incident photos", func() error {
			return s.purgeKeyed(ctx, "", "incident_photo", func() ([]repository.KeyedRow, error) {
				return repository.ExpiredIncidentPhotos(ctx, s.pool, c, retentionBatch)
			}, func(id string) error { return repository.ClearIncidentPhotos(ctx, s.pool, id) })
		})
		step("defect photos", func() error {
			return s.purgeKeyed(ctx, "", "defect_photo", func() ([]repository.KeyedRow, error) {
				return repository.ExpiredDefectPhotos(ctx, s.pool, c, retentionBatch)
			}, func(id string) error { return repository.ClearDefectPhoto(ctx, s.pool, id) })
		})
		step("checkpoint photos", func() error {
			return s.purgeKeyed(ctx, "", "checkpoint_photo", func() ([]repository.KeyedRow, error) {
				return repository.ExpiredScanPhotos(ctx, s.pool, c, retentionBatch)
			}, func(id string) error { return repository.ClearScanPhoto(ctx, s.pool, id) })
		})
	}
	return errors.Join(errs...)
}

// purgeKeyed deletes the objects of each listed row, then applies done to
// the row. A row whose objects cannot be deleted is skipped (retried next
// run). rowKind/objKind label the metrics ("" = not counted).
func (s *Scheduler) purgeKeyed(ctx context.Context, rowKind, objKind string, list func() ([]repository.KeyedRow, error), done func(id string) error) error {
	skip := map[string]bool{}
	failed := 0
	for b := 0; b < retentionBatches && ctx.Err() == nil; b++ {
		rows, err := list()
		if err != nil {
			return err
		}
		progress := false
		for _, row := range rows {
			if skip[row.ID] {
				continue
			}
			ok := true
			for _, key := range row.Keys {
				if err := s.removeObject(ctx, key); err != nil {
					s.log.Warn().Err(err).Str("key", key).Msg("retention: object delete failed; row kept for the next run")
					count(objKind, "failed", 1)
					ok = false
					break
				}
				count(objKind, "done", 1)
			}
			if ok {
				if err := done(row.ID); err != nil {
					s.log.Warn().Err(err).Str("id", row.ID).Msg("retention: row update failed")
					count(rowKind, "failed", 1)
					ok = false
				} else {
					count(rowKind, "done", 1)
					progress = true
				}
			}
			if !ok {
				skip[row.ID] = true
				failed++
			}
		}
		if len(rows) < retentionBatch || !progress {
			break
		}
	}
	if failed > 0 {
		return fmt.Errorf("%d item(s) failed", failed)
	}
	return nil
}

// clearBatches runs an UPDATE batch until it touches fewer rows than a batch.
func (s *Scheduler) clearBatches(kind string, batch func() (int64, error)) error {
	for b := 0; b < retentionBatches; b++ {
		n, err := batch()
		if err != nil {
			count(kind, "failed", 1)
			return err
		}
		count(kind, "done", int(n))
		if n < retentionBatch*10 {
			return nil
		}
	}
	return nil
}

func (s *Scheduler) removeObject(ctx context.Context, key string) error {
	if s.stg == nil {
		return errors.New("object storage not configured")
	}
	return s.stg.RemoveAllVersions(ctx, s.cfg.MinioBucketPhotos, key)
}

// encryptVisitorIDs encrypts visitor ID numbers stored in plaintext before
// BE-S9 (new rows are encrypted by the API).
func (s *Scheduler) encryptVisitorIDs(ctx context.Context) error {
	c, err := pii.For(s.cfg.SettingsEncryptionKey)
	if err != nil {
		return err
	}
	for b := 0; b < retentionBatches && ctx.Err() == nil; b++ {
		rows, err := repository.PlainVisitorIDNumbers(ctx, s.pool, pii.Prefix, retentionBatch)
		if err != nil {
			return err
		}
		for _, r := range rows {
			enc, err := c.Encrypt(r[1])
			if err == nil {
				err = repository.SetVisitorIDNumber(ctx, s.pool, r[0], r[1], enc)
			}
			if err != nil {
				count("visitor_id_encrypted", "failed", 1)
				return err
			}
			count("visitor_id_encrypted", "done", 1)
		}
		if len(rows) < retentionBatch {
			return nil
		}
	}
	return nil
}

func count(kind, result string, n int) {
	if kind == "" || n <= 0 {
		return
	}
	metrics.RetentionItems.WithLabelValues(kind, result).Add(float64(n))
}
