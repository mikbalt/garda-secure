-- 015 TAQ-58: who exported what. One row for every chain bundle or proof pack
-- that leaves the system (an export holds personal data). The row says who,
-- which site and range, when, how big and how it ended, never the content.
-- Append-only like audit_log: the application can insert and read, not change.
-- 'scheduled' rows are the final proof pack kept by the monthly job (one per site and
-- month) and its failures (at most one a day per site); the hourly refresh of an open
-- copy is not recorded. ASCII only.

CREATE TABLE export_audit (
    id         BIGSERIAL PRIMARY KEY,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    realm      TEXT        NOT NULL,                 -- the realm that owns the site
    site_id    UUID        NOT NULL,                 -- no FK: audit rows outlive sites
    actor      TEXT        NOT NULL,                 -- user name, or 'system' (kind 'scheduled')
    actor_sub  TEXT        NOT NULL DEFAULT '',      -- Keycloak subject
    kind       TEXT        NOT NULL CHECK (kind IN ('bundle', 'proof_pack', 'scheduled')),
    range_from TIMESTAMPTZ NOT NULL,
    range_to   TIMESTAMPTZ NOT NULL,                 -- exclusive
    bytes      BIGINT      NOT NULL DEFAULT 0,
    result     TEXT        NOT NULL CHECK (result IN ('ok', 'too_large', 'failed')),
    signing    TEXT        NOT NULL DEFAULT '',      -- how the head was signed for it: signed, reused, unreachable, refused, not_configured, stored
    detail     TEXT        NOT NULL DEFAULT ''       -- a reason in plain words, no record content
);

CREATE INDEX idx_export_audit_realm ON export_audit (realm, created_at DESC);
CREATE INDEX idx_export_audit_site ON export_audit (site_id, created_at DESC);

CREATE FUNCTION export_audit_append_only() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION 'export_audit is append-only (% blocked)', TG_OP
        USING ERRCODE = 'insufficient_privilege';
END;
$$;

CREATE TRIGGER trg_export_audit_append_only
    BEFORE UPDATE OR DELETE ON export_audit
    FOR EACH ROW EXECUTE FUNCTION export_audit_append_only();

CREATE TRIGGER trg_export_audit_no_truncate
    BEFORE TRUNCATE ON export_audit
    FOR EACH STATEMENT EXECUTE FUNCTION export_audit_append_only();
