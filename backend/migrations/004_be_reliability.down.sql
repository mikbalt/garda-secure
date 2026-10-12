ALTER TABLE backups
    DROP COLUMN IF EXISTS keycloak_size_bytes,
    DROP COLUMN IF EXISTS keycloak_object_key;

ALTER TABLE devices DROP COLUMN IF EXISTS online_since;

DROP INDEX IF EXISTS idx_sessions_sla_unchecked;
ALTER TABLE patrol_sessions DROP COLUMN IF EXISTS sla_checked_at;

DROP TABLE IF EXISTS ops_alert_rules;

DROP INDEX IF EXISTS idx_notification_log_ops;
DROP INDEX IF EXISTS idx_notification_log_due;
ALTER TABLE notification_log
    DROP COLUMN IF EXISTS request_id,
    DROP COLUMN IF EXISTS locked_until,
    DROP COLUMN IF EXISTS next_attempt_at,
    DROP COLUMN IF EXISTS lane;
