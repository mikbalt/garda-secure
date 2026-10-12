-- 007 be-features (audit 2026-10: F3, F6, F7, F9, F10, WEB-3, BE-S9).
-- Independent of 006 (be-field): only touches objects from 001-005.

-- -- F10: notification channels are validated in code (notification.Channels),
-- so adding LINE/Telegram needs no schema change. Keep a shape check only.
ALTER TABLE notification_rules DROP CONSTRAINT IF EXISTS notification_rules_channel_check;
ALTER TABLE ops_alert_rules    DROP CONSTRAINT IF EXISTS ops_alert_rules_channel_check;
ALTER TABLE notification_rules
    ADD CONSTRAINT chk_notification_rules_channel_name CHECK (channel ~ '^[a-z][a-z0-9_]{1,31}$');
ALTER TABLE ops_alert_rules
    ADD CONSTRAINT chk_ops_alert_rules_channel_name CHECK (channel ~ '^[a-z][a-z0-9_]{1,31}$');

-- -- WEB-3: realm-wide banner of unacknowledged SOS alerts ---------------------
CREATE INDEX idx_sos_unacknowledged ON sos_alerts(site_id, fired_at DESC)
    WHERE acknowledged_at IS NULL;

-- -- F6: attendance override history (written in the override transaction) ---
CREATE TABLE attendance_overrides (
    id                  UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    attendance_id       UUID        NOT NULL REFERENCES attendance_records(id) ON DELETE CASCADE,
    shift_id            UUID        REFERENCES shifts(id) ON DELETE SET NULL,
    site_id             UUID        NOT NULL REFERENCES sites(id) ON DELETE CASCADE,
    before_status       TEXT,       -- NULL when the shift had no record yet
    before_clock_in_at  TIMESTAMPTZ,
    before_clock_out_at TIMESTAMPTZ,
    before_reason       TEXT,
    after_status        TEXT        NOT NULL,
    after_clock_in_at   TIMESTAMPTZ,
    after_clock_out_at  TIMESTAMPTZ,
    reason              TEXT        NOT NULL,
    actor               TEXT        NOT NULL,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_attendance_overrides_record ON attendance_overrides(attendance_id, created_at);
CREATE INDEX idx_attendance_overrides_shift  ON attendance_overrides(shift_id, created_at);

-- -- F7: effective incident values = latest amendment per field ---------------
-- Originals stay immutable (D-009); reports, filters and exports read this view.
CREATE VIEW incidents_effective AS
SELECT i.*,
       COALESCE(am.type, i.type)               AS eff_type,
       COALESCE(am.severity, i.severity)       AS eff_severity,
       COALESCE(am.description, i.description) AS eff_description,
       COALESCE(am.location, i.location)       AS eff_location,
       am.amendments > 0                       AS amended
FROM incidents i
CROSS JOIN LATERAL (
    SELECT
      (SELECT a.changes->>'type' FROM incident_amendments a
        WHERE a.incident_id = i.id AND a.changes ? 'type'
        ORDER BY a.created_at DESC, a.id DESC LIMIT 1) AS type,
      (SELECT a.changes->>'severity' FROM incident_amendments a
        WHERE a.incident_id = i.id AND a.changes ? 'severity'
        ORDER BY a.created_at DESC, a.id DESC LIMIT 1) AS severity,
      (SELECT a.changes->>'description' FROM incident_amendments a
        WHERE a.incident_id = i.id AND a.changes ? 'description'
        ORDER BY a.created_at DESC, a.id DESC LIMIT 1) AS description,
      (SELECT a.changes->>'location' FROM incident_amendments a
        WHERE a.incident_id = i.id AND a.changes ? 'location'
        ORDER BY a.created_at DESC, a.id DESC LIMIT 1) AS location,
      (SELECT count(*) FROM incident_amendments a WHERE a.incident_id = i.id) AS amendments
) am;

-- -- F9: route snapshot per patrol session ------------------------------------
-- The checkpoint list at session start; progress, reports and SLA use it so
-- editing a route does not change past sessions. NULL for older sessions
-- (they fall back to the current route).
ALTER TABLE patrol_sessions ADD COLUMN route_snapshot JSONB;

-- -- F3: site floor plans (on-prem live map, D-035) ---------------------------
CREATE TABLE site_floorplans (
    id           UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    site_id      UUID        NOT NULL REFERENCES sites(id) ON DELETE CASCADE,
    name         TEXT        NOT NULL,
    object_key   TEXT        NOT NULL UNIQUE,
    content_type TEXT        NOT NULL,
    width        INT         NOT NULL CHECK (width > 0),
    height       INT         NOT NULL CHECK (height > 0),
    size_bytes   BIGINT      NOT NULL,
    created_by   TEXT,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_floorplans_site ON site_floorplans(site_id, created_at);

-- -- BE-S9: retention job lookups ---------------------------------------------
CREATE INDEX idx_attendance_retention ON attendance_records(created_at)
    WHERE selfie_object_key IS NOT NULL OR gps_lat IS NOT NULL OR clock_out_gps_lat IS NOT NULL;
CREATE INDEX idx_scans_retention ON checkpoint_scans(scanned_at) WHERE gps_lat IS NOT NULL;
CREATE INDEX idx_visitor_created ON visitor_logs(created_at);
CREATE INDEX idx_notification_log_created ON notification_log(created_at);
