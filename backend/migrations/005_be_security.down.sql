-- Reverts 005. Restoring the install-wide employee_id constraint fails if
-- two sites now use the same employee ID; resolve those first.
DROP INDEX IF EXISTS uq_guards_site_employee;
ALTER TABLE guards ADD CONSTRAINT guards_employee_id_key UNIQUE (employee_id);

ALTER TABLE sos_alerts       DROP COLUMN IF EXISTS flags;
ALTER TABLE checkpoint_scans DROP COLUMN IF EXISTS flags;
ALTER TABLE incidents        DROP COLUMN IF EXISTS flags, DROP COLUMN IF EXISTS received_at;
ALTER TABLE attendance_records DROP COLUMN IF EXISTS flags, DROP COLUMN IF EXISTS received_at;

DROP TRIGGER IF EXISTS trg_audit_log_no_truncate ON audit_log;
DROP TRIGGER IF EXISTS trg_audit_log_append_only ON audit_log;
DROP FUNCTION IF EXISTS audit_log_append_only();

DROP INDEX IF EXISTS idx_audit_realm_actor;
DROP INDEX IF EXISTS idx_audit_realm;
ALTER TABLE audit_log
    DROP COLUMN IF EXISTS after_state,
    DROP COLUMN IF EXISTS before_state,
    DROP COLUMN IF EXISTS summary,
    DROP COLUMN IF EXISTS request_id,
    DROP COLUMN IF EXISTS params,
    DROP COLUMN IF EXISTS resource_type,
    DROP COLUMN IF EXISTS route,
    DROP COLUMN IF EXISTS method,
    DROP COLUMN IF EXISTS roles,
    DROP COLUMN IF EXISTS guard_id,
    DROP COLUMN IF EXISTS actor,
    DROP COLUMN IF EXISTS realm;
