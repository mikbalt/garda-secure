-- Reverts 008. Queued push deliveries (channels webpush/fcm) stay in
-- notification_log; delete them first if the worker should not send them.
DROP INDEX IF EXISTS idx_sos_escalation;
ALTER TABLE sos_alerts DROP COLUMN IF EXISTS escalations;

DROP INDEX IF EXISTS idx_devices_push_token;
ALTER TABLE devices
    DROP COLUMN IF EXISTS unhealthy_alerted_at,
    DROP COLUMN IF EXISTS push_token_at,
    DROP COLUMN IF EXISTS push_platform,
    DROP COLUMN IF EXISTS push_token;

DROP TABLE IF EXISTS push_subscriptions;
DROP TABLE IF EXISTS push_users;
