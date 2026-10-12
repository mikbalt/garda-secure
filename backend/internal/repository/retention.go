package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// KeyedRow is a row ID with object keys to delete before the row is purged.
type KeyedRow struct {
	ID   string
	Keys []string
}

func scanKeyedRows(rows pgx.Rows, err error, what string) ([]KeyedRow, error) {
	if err != nil {
		return nil, fmt.Errorf("%s: %w", what, err)
	}
	return collect(rows, func(r pgx.Rows) (KeyedRow, error) {
		var k KeyedRow
		var key *string
		err := r.Scan(&k.ID, &key)
		if key != nil && *key != "" {
			k.Keys = []string{*key}
		}
		return k, err
	})
}

// ExpiredVisitors returns visitor logs created before cutoff, oldest first.
func ExpiredVisitors(ctx context.Context, db DBTX, cutoff time.Time, limit int) ([]KeyedRow, error) {
	rows, err := db.Query(ctx, `
		SELECT id, photo_object_key FROM visitor_logs
		WHERE created_at < $1 ORDER BY created_at LIMIT $2`, cutoff, limit)
	return scanKeyedRows(rows, err, "ExpiredVisitors")
}

// DeleteVisitor removes a visitor log.
func DeleteVisitor(ctx context.Context, db DBTX, id string) error {
	if _, err := db.Exec(ctx, `DELETE FROM visitor_logs WHERE id = $1`, id); err != nil {
		return fmt.Errorf("DeleteVisitor: %w", err)
	}
	return nil
}

// ExpiredSelfies returns attendance records created before cutoff that still
// reference a selfie.
func ExpiredSelfies(ctx context.Context, db DBTX, cutoff time.Time, limit int) ([]KeyedRow, error) {
	rows, err := db.Query(ctx, `
		SELECT id, selfie_object_key FROM attendance_records
		WHERE selfie_object_key IS NOT NULL AND created_at < $1 ORDER BY created_at LIMIT $2`, cutoff, limit)
	return scanKeyedRows(rows, err, "ExpiredSelfies")
}

// ClearSelfie removes the selfie reference of an attendance record.
func ClearSelfie(ctx context.Context, db DBTX, id string) error {
	if _, err := db.Exec(ctx, `UPDATE attendance_records SET selfie_object_key = NULL WHERE id = $1`, id); err != nil {
		return fmt.Errorf("ClearSelfie: %w", err)
	}
	return nil
}

// ClearAttendanceGPS nulls the clock-in/out positions of up to limit records
// created before cutoff; it returns how many were cleared.
func ClearAttendanceGPS(ctx context.Context, db DBTX, cutoff time.Time, limit int) (int64, error) {
	tag, err := db.Exec(ctx, `
		UPDATE attendance_records
		SET gps_lat = NULL, gps_lng = NULL, clock_out_gps_lat = NULL, clock_out_gps_lng = NULL
		WHERE id IN (SELECT id FROM attendance_records
		             WHERE created_at < $1 AND (gps_lat IS NOT NULL OR clock_out_gps_lat IS NOT NULL
		                                        OR gps_lng IS NOT NULL OR clock_out_gps_lng IS NOT NULL)
		             LIMIT $2)`, cutoff, limit)
	if err != nil {
		return 0, fmt.Errorf("ClearAttendanceGPS: %w", err)
	}
	return tag.RowsAffected(), nil
}

// ClearScanGPS nulls the positions of up to limit checkpoint scans made
// before cutoff.
func ClearScanGPS(ctx context.Context, db DBTX, cutoff time.Time, limit int) (int64, error) {
	tag, err := db.Exec(ctx, `
		UPDATE checkpoint_scans SET gps_lat = NULL, gps_lng = NULL
		WHERE id IN (SELECT id FROM checkpoint_scans
		             WHERE scanned_at < $1 AND (gps_lat IS NOT NULL OR gps_lng IS NOT NULL) LIMIT $2)`, cutoff, limit)
	if err != nil {
		return 0, fmt.Errorf("ClearScanGPS: %w", err)
	}
	return tag.RowsAffected(), nil
}

// PurgedRecipient replaces recipients of expired deliveries.
const PurgedRecipient = "[purged]"

// PurgeNotificationLog clears subject, body, recipient and dedupe key of up
// to limit finished deliveries created before cutoff (the row itself stays
// for statistics).
func PurgeNotificationLog(ctx context.Context, db DBTX, cutoff time.Time, limit int) (int64, error) {
	tag, err := db.Exec(ctx, `
		UPDATE notification_log
		SET subject = event_type, body = '', recipient = $3, dedupe_key = NULL, last_error = NULL
		WHERE id IN (SELECT id FROM notification_log
		             WHERE created_at < $1 AND recipient <> $3 AND status <> 'queued' LIMIT $2)`,
		cutoff, limit, PurgedRecipient)
	if err != nil {
		return 0, fmt.Errorf("PurgeNotificationLog: %w", err)
	}
	return tag.RowsAffected(), nil
}

// ExpiredIncidentPhotos returns incidents that occurred before cutoff and
// still reference photos.
func ExpiredIncidentPhotos(ctx context.Context, db DBTX, cutoff time.Time, limit int) ([]KeyedRow, error) {
	rows, err := db.Query(ctx, `
		SELECT id, photo_object_keys FROM incidents
		WHERE occurred_at < $1 AND photo_object_keys <> '[]'::jsonb ORDER BY occurred_at LIMIT $2`, cutoff, limit)
	if err != nil {
		return nil, fmt.Errorf("ExpiredIncidentPhotos: %w", err)
	}
	return collect(rows, func(r pgx.Rows) (KeyedRow, error) {
		var k KeyedRow
		err := r.Scan(&k.ID, &k.Keys)
		return k, err
	})
}

// ClearIncidentPhotos removes the photo references of an incident
// (retention of evidence photos, only when configured).
func ClearIncidentPhotos(ctx context.Context, db DBTX, id string) error {
	if _, err := db.Exec(ctx, `UPDATE incidents SET photo_object_keys = '[]'::jsonb WHERE id = $1`, id); err != nil {
		return fmt.Errorf("ClearIncidentPhotos: %w", err)
	}
	return nil
}

// ExpiredDefectPhotos returns defect reports created before cutoff with a photo.
func ExpiredDefectPhotos(ctx context.Context, db DBTX, cutoff time.Time, limit int) ([]KeyedRow, error) {
	rows, err := db.Query(ctx, `
		SELECT id, photo_object_key FROM defect_reports
		WHERE photo_object_key IS NOT NULL AND created_at < $1 ORDER BY created_at LIMIT $2`, cutoff, limit)
	return scanKeyedRows(rows, err, "ExpiredDefectPhotos")
}

// ClearDefectPhoto removes the photo reference of a defect report.
func ClearDefectPhoto(ctx context.Context, db DBTX, id string) error {
	if _, err := db.Exec(ctx, `UPDATE defect_reports SET photo_object_key = NULL WHERE id = $1`, id); err != nil {
		return fmt.Errorf("ClearDefectPhoto: %w", err)
	}
	return nil
}

// ExpiredScanPhotos returns checkpoint scans made before cutoff with a photo.
func ExpiredScanPhotos(ctx context.Context, db DBTX, cutoff time.Time, limit int) ([]KeyedRow, error) {
	rows, err := db.Query(ctx, `
		SELECT id, photo_object_key FROM checkpoint_scans
		WHERE photo_object_key IS NOT NULL AND scanned_at < $1 ORDER BY scanned_at LIMIT $2`, cutoff, limit)
	return scanKeyedRows(rows, err, "ExpiredScanPhotos")
}

// ClearScanPhoto removes the photo reference of a checkpoint scan.
func ClearScanPhoto(ctx context.Context, db DBTX, id string) error {
	if _, err := db.Exec(ctx, `UPDATE checkpoint_scans SET photo_object_key = NULL WHERE id = $1`, id); err != nil {
		return fmt.Errorf("ClearScanPhoto: %w", err)
	}
	return nil
}

// PlainVisitorIDNumbers returns visitor logs whose ID number is not yet
// encrypted (rows from before BE-S9).
func PlainVisitorIDNumbers(ctx context.Context, db DBTX, encPrefix string, limit int) ([][2]string, error) {
	rows, err := db.Query(ctx, `
		SELECT id, id_number FROM visitor_logs
		WHERE id_number IS NOT NULL AND id_number <> '' AND left(id_number, length($1)) <> $1
		LIMIT $2`, encPrefix, limit)
	if err != nil {
		return nil, fmt.Errorf("PlainVisitorIDNumbers: %w", err)
	}
	return collect(rows, func(r pgx.Rows) ([2]string, error) {
		var v [2]string
		err := r.Scan(&v[0], &v[1])
		return v, err
	})
}

// SetVisitorIDNumber replaces a plaintext ID number with its encrypted form
// (only if it is still the given plaintext).
func SetVisitorIDNumber(ctx context.Context, db DBTX, id, plain, enc string) error {
	if _, err := db.Exec(ctx, `UPDATE visitor_logs SET id_number = $3 WHERE id = $1 AND id_number = $2`, id, plain, enc); err != nil {
		return fmt.Errorf("SetVisitorIDNumber: %w", err)
	}
	return nil
}
