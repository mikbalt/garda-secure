-- Reverts 007. Restoring the channel CHECKs fails if rules use channels other
-- than email/whatsapp; delete those rules first. Floor plan objects in the
-- photos bucket are not removed.
DROP INDEX IF EXISTS idx_notification_log_created;
DROP INDEX IF EXISTS idx_visitor_created;
DROP INDEX IF EXISTS idx_scans_retention;
DROP INDEX IF EXISTS idx_attendance_retention;

DROP TABLE IF EXISTS site_floorplans;
ALTER TABLE patrol_sessions DROP COLUMN IF EXISTS route_snapshot;
DROP VIEW IF EXISTS incidents_effective;
DROP TABLE IF EXISTS attendance_overrides;
DROP INDEX IF EXISTS idx_sos_unacknowledged;

ALTER TABLE ops_alert_rules    DROP CONSTRAINT IF EXISTS chk_ops_alert_rules_channel_name;
ALTER TABLE notification_rules DROP CONSTRAINT IF EXISTS chk_notification_rules_channel_name;
ALTER TABLE ops_alert_rules
    ADD CONSTRAINT ops_alert_rules_channel_check CHECK (channel IN ('email', 'whatsapp'));
ALTER TABLE notification_rules
    ADD CONSTRAINT notification_rules_channel_check CHECK (channel IN ('email', 'whatsapp'));
