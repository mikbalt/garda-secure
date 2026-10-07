DROP TABLE IF EXISTS backups;
DROP TABLE IF EXISTS system_settings;
DROP TABLE IF EXISTS notification_log;
DROP TABLE IF EXISTS notification_rules;

ALTER TABLE sos_alerts DROP COLUMN IF EXISTS received_at, DROP COLUMN IF EXISTS device_id;
ALTER TABLE visitor_logs DROP COLUMN IF EXISTS device_id;
DROP INDEX IF EXISTS idx_defects_site;
ALTER TABLE defect_reports
    DROP CONSTRAINT IF EXISTS chk_defect_status,
    DROP COLUMN IF EXISTS device_id,
    DROP COLUMN IF EXISTS updated_at;

DROP TABLE IF EXISTS incident_amendments;
ALTER TABLE incidents
    DROP CONSTRAINT IF EXISTS chk_incident_status,
    DROP CONSTRAINT IF EXISTS chk_incident_severity,
    DROP COLUMN IF EXISTS device_id,
    DROP COLUMN IF EXISTS occurred_at;

DROP TABLE IF EXISTS sla_breaches;
DROP INDEX IF EXISTS idx_scans_session;
ALTER TABLE checkpoint_scans
    DROP CONSTRAINT IF EXISTS chk_scan_method,
    DROP COLUMN IF EXISTS received_at,
    DROP COLUMN IF EXISTS device_id,
    DROP COLUMN IF EXISTS session_id;
DROP TABLE IF EXISTS patrol_sessions;

DROP INDEX IF EXISTS idx_attendance_review;
DROP INDEX IF EXISTS uq_attendance_shift;
ALTER TABLE attendance_records
    DROP CONSTRAINT IF EXISTS chk_attendance_status,
    DROP COLUMN IF EXISTS overridden_at,
    DROP COLUMN IF EXISTS reviewed_at,
    DROP COLUMN IF EXISTS reviewed_by,
    DROP COLUMN IF EXISTS review_note,
    DROP COLUMN IF EXISTS needs_review,
    DROP COLUMN IF EXISTS device_id,
    DROP COLUMN IF EXISTS clock_out_gps_lng,
    DROP COLUMN IF EXISTS clock_out_gps_lat,
    DROP COLUMN IF EXISTS early_out_minutes,
    DROP COLUMN IF EXISTS late_minutes;

DROP INDEX IF EXISTS idx_shifts_guard_start;
DROP INDEX IF EXISTS idx_shifts_site_start;
DROP INDEX IF EXISTS uq_shifts_template_start;
ALTER TABLE shifts
    DROP CONSTRAINT IF EXISTS chk_shift_order,
    DROP COLUMN IF EXISTS template_id;
ALTER TABLE shifts ADD COLUMN repeat_rule TEXT;
DROP TABLE IF EXISTS shift_templates;

DROP TABLE IF EXISTS devices;

DROP INDEX IF EXISTS idx_guards_site;
ALTER TABLE guards DROP COLUMN IF EXISTS site_id;
