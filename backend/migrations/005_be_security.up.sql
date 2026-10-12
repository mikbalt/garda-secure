-- 005 be-security hardening: richer, append-only audit log (BE-S8),
-- receipt times and fraud flags on field records (BE-S1), employee IDs
-- unique per site instead of install-wide (BE-S10).

-- -- Audit log context ---------------------------------------------------------
ALTER TABLE audit_log
    ADD COLUMN realm         TEXT,
    ADD COLUMN actor         TEXT,
    ADD COLUMN guard_id      UUID,       -- no FK: audit rows outlive guards
    ADD COLUMN roles         TEXT[]      NOT NULL DEFAULT '{}',
    ADD COLUMN method        TEXT,
    ADD COLUMN route         TEXT,
    ADD COLUMN resource_type TEXT,
    ADD COLUMN params        JSONB,
    ADD COLUMN request_id    TEXT,
    ADD COLUMN summary       JSONB,      -- redacted request body (web writes)
    ADD COLUMN before_state  JSONB,
    ADD COLUMN after_state   JSONB;

-- Old rows stored "realm/actor[ guard:<id>]" and "METHOD /route".
UPDATE audit_log
SET realm  = split_part(user_id, '/', 1),
    actor  = split_part(split_part(user_id, '/', 2), ' ', 1),
    method = split_part(action, ' ', 1),
    route  = split_part(action, ' ', 2)
WHERE user_id LIKE '%/%';

CREATE INDEX idx_audit_realm ON audit_log(realm, created_at DESC);
CREATE INDEX idx_audit_realm_actor ON audit_log(realm, actor, created_at DESC);

-- The application may only append to the audit log.
CREATE FUNCTION audit_log_append_only() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION 'audit_log is append-only (% blocked)', TG_OP
        USING ERRCODE = 'insufficient_privilege';
END;
$$;

CREATE TRIGGER trg_audit_log_append_only
    BEFORE UPDATE OR DELETE ON audit_log
    FOR EACH ROW EXECUTE FUNCTION audit_log_append_only();

CREATE TRIGGER trg_audit_log_no_truncate
    BEFORE TRUNCATE ON audit_log
    FOR EACH STATEMENT EXECUTE FUNCTION audit_log_append_only();

-- -- Receipt times and plausibility flags ------------------------------------
-- received_at = server time the record reached the API (client event times
-- may be backdated by offline sync); flags = plausibility findings for
-- supervisors (e.g. late_sync, clock_ahead, gps_far, too_fast).
ALTER TABLE attendance_records
    ADD COLUMN received_at TIMESTAMPTZ,
    ADD COLUMN flags       TEXT[]      NOT NULL DEFAULT '{}';
UPDATE attendance_records SET received_at = created_at WHERE clock_in_at IS NOT NULL;

ALTER TABLE incidents
    ADD COLUMN received_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    ADD COLUMN flags       TEXT[]      NOT NULL DEFAULT '{}';
UPDATE incidents SET received_at = created_at;

ALTER TABLE checkpoint_scans ADD COLUMN flags TEXT[] NOT NULL DEFAULT '{}';
ALTER TABLE sos_alerts       ADD COLUMN flags TEXT[] NOT NULL DEFAULT '{}';

-- -- Employee IDs: unique per site (the API also checks the realm) ------------
ALTER TABLE guards DROP CONSTRAINT IF EXISTS guards_employee_id_key;
CREATE UNIQUE INDEX uq_guards_site_employee ON guards(site_id, employee_id)
    WHERE employee_id IS NOT NULL;
