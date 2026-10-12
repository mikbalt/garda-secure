-- Reverts 006. Signed QR metadata inside routes.checkpoints (qr_version,
-- qr_signed) is left in place; older code ignores unknown JSON keys and keeps
-- matching the stored qr_code text.
DROP INDEX IF EXISTS idx_sessions_route_started;
DROP TABLE IF EXISTS patrol_expectations;
DROP TABLE IF EXISTS route_schedules;

ALTER TABLE visitor_logs    DROP COLUMN IF EXISTS flags;
ALTER TABLE defect_reports  DROP COLUMN IF EXISTS flags;
ALTER TABLE patrol_sessions DROP COLUMN IF EXISTS flags;

ALTER TABLE guards
    DROP COLUMN IF EXISTS pin_locked_until,
    DROP COLUMN IF EXISTS pin_failed_attempts;
