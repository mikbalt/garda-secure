-- 013 TAQ-56: signed chain heads, keyring, and the owner/runtime role split.
-- The signer service (own container, own key) signs each site's chain head and
-- stores it in signed_heads, so a past rewritten and re-chained afterwards no
-- longer matches. ASCII only.

-- Public keys of the signer, append-only. kind:
--   genesis    first key of the install (epoch 1); trusted by pinning its key_id
--   rotation   new key of the same epoch; signed by the new key AND by the
--              previous key (the handover)
--   new_epoch  start of epoch n+1 after loss or compromise; signed by the new
--              key only, so clients must re-pin it out of band
-- Signed payload: see signer.KeyPayload.
CREATE TABLE keyring (
    id           BIGSERIAL   PRIMARY KEY,
    key_id       TEXT        NOT NULL UNIQUE CHECK (key_id ~ '^[0-9a-f]{64}$'),   -- hex sha256 of the public key
    epoch        INT         NOT NULL CHECK (epoch >= 1),
    kind         TEXT        NOT NULL CHECK (kind IN ('genesis', 'rotation', 'new_epoch')),
    public_key   BYTEA       NOT NULL CHECK (octet_length(public_key) = 32),
    prev_key_id  TEXT        REFERENCES keyring(key_id),
    reason       TEXT        NOT NULL DEFAULT '',
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    new_key_sig  BYTEA       NOT NULL CHECK (octet_length(new_key_sig) = 64),
    prev_key_sig BYTEA       CHECK (prev_key_sig IS NULL OR octet_length(prev_key_sig) = 64),
    CHECK ((kind = 'genesis'   AND epoch = 1 AND prev_key_id IS NULL     AND prev_key_sig IS NULL)
        OR (kind = 'rotation'  AND prev_key_id IS NOT NULL                AND prev_key_sig IS NOT NULL)
        OR (kind = 'new_epoch' AND epoch > 1                              AND prev_key_sig IS NULL))
);

-- Heads signed by the signer. Append-only; one signature per (site, seq, key).
CREATE TABLE signed_heads (
    id         BIGSERIAL   PRIMARY KEY,
    site_id    UUID        NOT NULL,            -- no FK: evidence outlives site rows (uuid = canonical lowercase)
    seq        BIGINT      NOT NULL CHECK (seq >= 1),
    entry_hash BYTEA       NOT NULL CHECK (octet_length(entry_hash) = 32),
    signed_at  TIMESTAMPTZ NOT NULL,
    reason     TEXT        NOT NULL CHECK (reason IN ('hourly', 'day_end', 'on_demand')),
    key_id     TEXT        NOT NULL REFERENCES keyring(key_id),
    signature  BYTEA       NOT NULL CHECK (octet_length(signature) = 64),
    UNIQUE (site_id, seq, key_id)
);
CREATE INDEX idx_signed_heads_site ON signed_heads(site_id, seq DESC);

CREATE FUNCTION signer_tables_append_only() RETURNS trigger
    LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION '% is append-only (% blocked)', TG_TABLE_NAME, TG_OP
        USING ERRCODE = 'insufficient_privilege';
END;
$$;
CREATE TRIGGER trg_keyring_append_only BEFORE UPDATE OR DELETE ON keyring
    FOR EACH ROW EXECUTE FUNCTION signer_tables_append_only();
CREATE TRIGGER trg_keyring_no_truncate BEFORE TRUNCATE ON keyring
    FOR EACH STATEMENT EXECUTE FUNCTION signer_tables_append_only();
CREATE TRIGGER trg_signed_heads_append_only BEFORE UPDATE OR DELETE ON signed_heads
    FOR EACH ROW EXECUTE FUNCTION signer_tables_append_only();
CREATE TRIGGER trg_signed_heads_no_truncate BEFORE TRUNCATE ON signed_heads
    FOR EACH STATEMENT EXECUTE FUNCTION signer_tables_append_only();

-- Backstop for the signer's own checks: a head never goes below one already
-- signed, and a signed seq never gets a second hash. Serialised per site.
CREATE FUNCTION signed_heads_guard() RETURNS trigger
    LANGUAGE plpgsql AS $$
BEGIN
    PERFORM pg_advisory_xact_lock(hashtextextended('signed_heads:' || NEW.site_id::text, 0));
    IF EXISTS (SELECT 1 FROM signed_heads s WHERE s.site_id = NEW.site_id AND s.seq > NEW.seq) THEN
        RAISE EXCEPTION 'signed_heads: seq % is lower than an already signed head', NEW.seq
            USING ERRCODE = 'check_violation';
    END IF;
    IF EXISTS (SELECT 1 FROM signed_heads s WHERE s.site_id = NEW.site_id AND s.seq = NEW.seq
                  AND s.entry_hash <> NEW.entry_hash) THEN
        RAISE EXCEPTION 'signed_heads: seq % is already signed with a different hash', NEW.seq
            USING ERRCODE = 'check_violation';
    END IF;
    RETURN NEW;
END;
$$;
CREATE TRIGGER trg_signed_heads_guard BEFORE INSERT ON signed_heads
    FOR EACH ROW EXECUTE FUNCTION signed_heads_guard();

-- Does the chain still link from p_from_seq (inclusive) to its head? Checks
-- contiguous seq, prev_hash links, entry_hash content and the head row. Only
-- reads chain tables, so the signer role can run it. NULL seq = intact.
CREATE FUNCTION chain_links_break(p_site UUID, p_from_seq BIGINT) RETURNS TABLE (seq BIGINT, reason TEXT)
    LANGUAGE plpgsql STABLE AS $$
DECLARE
    r        record;
    h        record;
    v_expect BIGINT := p_from_seq;
    v_prev   BYTEA;
BEGIN
    IF p_from_seq <= 1 THEN
        v_expect := 1;
        v_prev := chain_genesis(p_site);
    ELSE
        SELECT entry_hash INTO v_prev FROM record_chain WHERE site_id = p_site AND record_chain.seq = p_from_seq - 1;
        IF v_prev IS NULL THEN
            RETURN QUERY SELECT p_from_seq - 1, 'entry missing'::text;
            RETURN;
        END IF;
        v_expect := p_from_seq;
    END IF;
    FOR r IN SELECT * FROM record_chain c WHERE c.site_id = p_site AND c.seq >= v_expect ORDER BY c.seq LOOP
        IF r.seq <> v_expect THEN
            RETURN QUERY SELECT v_expect, 'entry missing'::text;
            RETURN;
        END IF;
        IF r.prev_hash <> v_prev THEN
            RETURN QUERY SELECT r.seq, 'prev_hash does not link to the previous entry'::text;
            RETURN;
        END IF;
        IF r.entry_hash <> chain_entry_hash(r.site_id, r.seq, r.kind, r.record_id, r.record_hash,
                                            r.prev_hash, r.hash_version, r.origin, r.created_at) THEN
            RETURN QUERY SELECT r.seq, 'entry content does not match its hash'::text;
            RETURN;
        END IF;
        v_prev   := r.entry_hash;
        v_expect := v_expect + 1;
    END LOOP;
    SELECT * INTO h FROM record_chain_heads WHERE site_id = p_site;
    IF NOT FOUND OR h.last_seq <> v_expect - 1 OR h.last_hash <> v_prev THEN
        RETURN QUERY SELECT v_expect, 'chain head does not match the last entry'::text;
    END IF;
END $$;

-- ---------------------------------------------------------------------------
-- Role split (D-071). The role that runs migrations owns the tables (APP_DB_USER);
-- the API and worker run as sp_runtime, the signer as sp_signer. Both are
-- created by infra/postgres/init (fresh volume) or infra/scripts/db-roles.sh.
-- Where a role does not exist (plain dev databases), its grants are skipped
-- and the connecting role simply owns everything, as before.
--
-- chain_append stays executable by the runtime role: the Go repository calls it
-- in the transaction of every record write. It runs as the owner (SECURITY
-- DEFINER), so sp_runtime needs no write access to the chain tables and cannot
-- disable their triggers (that needs table ownership). It can still append an
-- entry that reflects a false change; that is visible as a new entry, and the
-- past stays anchored by signed_heads.
-- ---------------------------------------------------------------------------
ALTER FUNCTION chain_append(TEXT, UUID, TEXT) SECURITY DEFINER SET search_path = public, pg_temp;
ALTER FUNCTION chain_first_break(UUID)        SECURITY DEFINER SET search_path = public, pg_temp;

REVOKE EXECUTE ON FUNCTION chain_append(TEXT, UUID, TEXT) FROM PUBLIC;
REVOKE EXECUTE ON FUNCTION chain_entry_hash(UUID, BIGINT, TEXT, UUID, BYTEA, BYTEA, SMALLINT, TEXT, TIMESTAMPTZ) FROM PUBLIC;
REVOKE EXECUTE ON FUNCTION chain_links_break(UUID, BIGINT) FROM PUBLIC;

-- Idempotent: also run by infra/scripts/db-roles.sh on volumes that predate the roles.
CREATE FUNCTION garda_apply_role_grants() RETURNS void
    LANGUAGE plpgsql AS $$
BEGIN
    IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'sp_runtime') THEN
        GRANT USAGE ON SCHEMA public TO sp_runtime;
        GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA public TO sp_runtime;
        GRANT USAGE, SELECT, UPDATE ON ALL SEQUENCES IN SCHEMA public TO sp_runtime;
        -- The chain and signer tables are written by chain_append / the signer only.
        REVOKE INSERT, UPDATE, DELETE, TRUNCATE ON record_chain, record_chain_heads, signed_heads, keyring FROM sp_runtime;
        REVOKE UPDATE ON SEQUENCE signed_heads_id_seq, keyring_id_seq FROM sp_runtime;
        REVOKE INSERT, UPDATE, DELETE, TRUNCATE ON schema_migrations FROM sp_runtime;
        GRANT EXECUTE ON FUNCTION chain_append(TEXT, UUID, TEXT) TO sp_runtime;
        -- Tables and sequences added by later migrations (run as this role).
        EXECUTE format('ALTER DEFAULT PRIVILEGES FOR ROLE %I IN SCHEMA public GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO sp_runtime', current_user);
        EXECUTE format('ALTER DEFAULT PRIVILEGES FOR ROLE %I IN SCHEMA public GRANT USAGE, SELECT, UPDATE ON SEQUENCES TO sp_runtime', current_user);
    END IF;
    IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'sp_signer') THEN
        GRANT USAGE ON SCHEMA public TO sp_signer;
        GRANT SELECT ON record_chain, record_chain_heads TO sp_signer;
        GRANT SELECT (id, timezone) ON sites TO sp_signer;       -- day end is the site's
        GRANT SELECT, INSERT ON signed_heads, keyring TO sp_signer;
        GRANT USAGE ON SEQUENCE signed_heads_id_seq, keyring_id_seq TO sp_signer;
        GRANT EXECUTE ON FUNCTION chain_entry_hash(UUID, BIGINT, TEXT, UUID, BYTEA, BYTEA, SMALLINT, TEXT, TIMESTAMPTZ) TO sp_signer;
        GRANT EXECUTE ON FUNCTION chain_links_break(UUID, BIGINT) TO sp_signer;
    END IF;
END $$;
REVOKE EXECUTE ON FUNCTION garda_apply_role_grants() FROM PUBLIC;

SELECT garda_apply_role_grants();
