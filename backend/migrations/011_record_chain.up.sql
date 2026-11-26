-- 011 TAQ-17: hash-chained record log. Every attendance record, checkpoint
-- scan, incident and incident amendment is chained to the previous record of
-- the same site, so an edit or a deletion outside the application is found by
-- chain_first_break(). audit_log stays as it is (append-only, 005).
-- ASCII only.

-- One row per site: the chain head. The row lock serialises writers per site.
CREATE TABLE record_chain_heads (
    site_id   UUID   PRIMARY KEY,   -- no FK: evidence outlives site rows
    last_seq  BIGINT NOT NULL,
    last_hash BYTEA  NOT NULL
);

CREATE TABLE record_chain (
    site_id      UUID        NOT NULL,
    seq          BIGINT      NOT NULL,
    kind         TEXT        NOT NULL
                             CHECK (kind IN ('attendance', 'scan', 'incident', 'incident_amendment')),
    record_id    UUID        NOT NULL,
    record_hash  BYTEA       NOT NULL,   -- sha256 of the record's canonical form
    prev_hash    BYTEA       NOT NULL,   -- entry_hash of seq-1, or the site genesis value
    entry_hash   BYTEA       NOT NULL,   -- also covers origin and created_at
    hash_version SMALLINT    NOT NULL DEFAULT 1,
    origin       TEXT        NOT NULL DEFAULT 'live' CHECK (origin IN ('live', 'backfill')),
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (site_id, seq)
);
CREATE INDEX idx_record_chain_record ON record_chain(kind, record_id, seq DESC);

-- Like audit_log: the application may only append.
CREATE FUNCTION record_chain_append_only() RETURNS trigger
    LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION 'record_chain is append-only (% blocked)', TG_OP
        USING ERRCODE = 'insufficient_privilege';
END;
$$;
CREATE TRIGGER trg_record_chain_append_only
    BEFORE UPDATE OR DELETE ON record_chain
    FOR EACH ROW EXECUTE FUNCTION record_chain_append_only();
CREATE TRIGGER trg_record_chain_no_truncate
    BEFORE TRUNCATE ON record_chain
    FOR EACH STATEMENT EXECUTE FUNCTION record_chain_append_only();

-- The head may only move forward by one entry; it cannot be deleted or reset.
CREATE FUNCTION record_chain_heads_guard() RETURNS trigger
    LANGUAGE plpgsql AS $$
BEGIN
    IF TG_OP = 'UPDATE' AND NEW.site_id = OLD.site_id AND NEW.last_seq = OLD.last_seq + 1 THEN
        RETURN NEW;
    END IF;
    RAISE EXCEPTION 'record_chain_heads may only advance by one entry (% blocked)', TG_OP
        USING ERRCODE = 'insufficient_privilege';
END;
$$;
CREATE TRIGGER trg_record_chain_heads_guard
    BEFORE UPDATE OR DELETE ON record_chain_heads
    FOR EACH ROW EXECUTE FUNCTION record_chain_heads_guard();
CREATE TRIGGER trg_record_chain_heads_no_truncate
    BEFORE TRUNCATE ON record_chain_heads
    FOR EACH STATEMENT EXECUTE FUNCTION record_chain_heads_guard();

-- Genesis value: per site, so chains cannot be spliced into each other.
CREATE FUNCTION chain_genesis(p_site UUID) RETURNS BYTEA
    LANGUAGE sql IMMUTABLE AS
$$ SELECT sha256(convert_to('garda-record-chain:v1:' || p_site::text, 'UTF8')) $$;

-- The site a record belongs to (NULL when the record does not exist).
CREATE FUNCTION chain_record_site(p_kind TEXT, p_id UUID) RETURNS UUID
    LANGUAGE sql STABLE AS
$$ SELECT CASE p_kind
    WHEN 'attendance' THEN (SELECT site_id FROM attendance_records WHERE id = p_id)
    WHEN 'scan'       THEN (SELECT r.site_id FROM checkpoint_scans s JOIN routes r ON r.id = s.route_id WHERE s.id = p_id)
    WHEN 'incident'   THEN (SELECT site_id FROM incidents WHERE id = p_id)
    WHEN 'incident_amendment' THEN (SELECT i.site_id FROM incident_amendments a JOIN incidents i ON i.id = a.incident_id WHERE a.id = p_id)
  END $$;

-- Canonical form (hash_version 1): the JSON object of the record's listed
-- columns, as jsonb text (keys sorted, no insignificant whitespace), with
-- timestamps in UTC; hashed as sha256('<kind>:' || that text). Columns added
-- later are not covered until a new hash_version lists them. The function
-- settings pin the time zone and float format so the text never depends on
-- the session. NULL when the record does not exist.
CREATE FUNCTION chain_record_hash(p_kind TEXT, p_id UUID) RETURNS BYTEA
    LANGUAGE plpgsql STABLE
    SET TimeZone = 'UTC' SET extra_float_digits = 1 AS $$
DECLARE
    v JSONB;
BEGIN
    IF p_kind = 'attendance' THEN
        SELECT to_jsonb(t) INTO v FROM (
            SELECT id, shift_id, guard_id, site_id, clock_in_at, clock_out_at, status, late_minutes,
                   early_out_minutes, selfie_object_key, gps_lat, gps_lng, clock_out_gps_lat,
                   clock_out_gps_lng, device_id, override_reason, overridden_by, overridden_at,
                   needs_review, review_note, reviewed_by, reviewed_at, created_at, received_at, flags
            FROM attendance_records WHERE id = p_id) t;
    ELSIF p_kind = 'scan' THEN
        SELECT to_jsonb(t) INTO v FROM (
            SELECT id, session_id, route_id, checkpoint_id, guard_id, device_id, scanned_at,
                   received_at, method, photo_object_key, gps_lat, gps_lng, flags
            FROM checkpoint_scans WHERE id = p_id) t;
    ELSIF p_kind = 'incident' THEN
        SELECT to_jsonb(t) INTO v FROM (
            SELECT id, site_id, guard_id, device_id, type, severity, description, photo_object_keys,
                   location, status, occurred_at, received_at, flags, gps_lat, gps_lng,
                   created_at, updated_at
            FROM incidents WHERE id = p_id) t;
    ELSIF p_kind = 'incident_amendment' THEN
        SELECT to_jsonb(t) INTO v FROM (
            SELECT id, incident_id, changes, reason, amended_by, created_at
            FROM incident_amendments WHERE id = p_id) t;
    ELSE
        RAISE EXCEPTION 'unknown chain record kind %', p_kind;
    END IF;
    IF v IS NULL THEN
        RETURN NULL;
    END IF;
    RETURN sha256(convert_to(p_kind || ':' || v::text, 'UTF8'));
END $$;

-- Entry hash: sha256 of version, site, seq, kind, record id, record_hash,
-- prev_hash, origin and created_at (UTC, microseconds), joined with '|'.
CREATE FUNCTION chain_entry_hash(p_site UUID, p_seq BIGINT, p_kind TEXT, p_id UUID,
                                 p_record BYTEA, p_prev BYTEA, p_version SMALLINT,
                                 p_origin TEXT, p_created TIMESTAMPTZ) RETURNS BYTEA
    LANGUAGE sql IMMUTABLE AS
$$ SELECT sha256(convert_to(p_version || '|' || p_site || '|' || p_seq || '|' || p_kind || '|' || p_id
                            || '|' || encode(p_record, 'hex') || '|' || encode(p_prev, 'hex')
                            || '|' || p_origin
                            || '|' || to_char(p_created AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS.US'), 'UTF8')) $$;

-- Append the record's current state to its site's chain; returns the new seq.
-- Call it in the transaction that wrote the record. The head row lock makes
-- concurrent writers of one site take turns (order = lock order), so seq has
-- no gaps and no two entries share a predecessor.
CREATE FUNCTION chain_append(p_kind TEXT, p_id UUID, p_origin TEXT DEFAULT 'live') RETURNS BIGINT
    LANGUAGE plpgsql AS $$
DECLARE
    v_site UUID := chain_record_site(p_kind, p_id);
    v_seq  BIGINT;
    v_prev BYTEA;
    v_rec  BYTEA;
    v_hash BYTEA;
    v_now  TIMESTAMPTZ := clock_timestamp();
BEGIN
    IF v_site IS NULL THEN
        RAISE EXCEPTION 'chain_append: % % not found', p_kind, p_id;
    END IF;
    INSERT INTO record_chain_heads (site_id, last_seq, last_hash)
    VALUES (v_site, 0, chain_genesis(v_site))
    ON CONFLICT (site_id) DO NOTHING;
    SELECT last_seq + 1, last_hash INTO v_seq, v_prev
    FROM record_chain_heads WHERE site_id = v_site FOR UPDATE;
    -- Read the record only after the lock: a writer that waited sees the
    -- committed state of its predecessor.
    v_rec  := chain_record_hash(p_kind, p_id);
    v_hash := chain_entry_hash(v_site, v_seq, p_kind, p_id, v_rec, v_prev, 1::smallint, p_origin, v_now);
    INSERT INTO record_chain (site_id, seq, kind, record_id, record_hash, prev_hash, entry_hash, origin, created_at)
    VALUES (v_site, v_seq, p_kind, p_id, v_rec, v_prev, v_hash, p_origin, v_now);
    UPDATE record_chain_heads SET last_seq = v_seq, last_hash = v_hash WHERE site_id = v_site;
    RETURN v_seq;
END $$;

-- Records of a site that have no chain entry.
CREATE FUNCTION chain_unchained_records(p_site UUID) RETURNS TABLE (kind TEXT, record_id UUID)
    LANGUAGE sql STABLE AS
$$ SELECT u.kind, u.id FROM (
       SELECT 'attendance' AS kind, id FROM attendance_records WHERE site_id = p_site
       UNION ALL
       SELECT 'scan', s.id FROM checkpoint_scans s JOIN routes r ON r.id = s.route_id WHERE r.site_id = p_site
       UNION ALL
       SELECT 'incident', id FROM incidents WHERE site_id = p_site
       UNION ALL
       SELECT 'incident_amendment', a.id FROM incident_amendments a JOIN incidents i ON i.id = a.incident_id
        WHERE i.site_id = p_site
   ) u
   WHERE NOT EXISTS (SELECT 1 FROM record_chain c
                     WHERE c.site_id = p_site AND c.kind = u.kind AND c.record_id = u.id) $$;

-- First broken link of a site's chain, or no row when it is intact. Checks,
-- in seq order: no gap (a deleted entry), prev_hash links, entry_hash matches
-- its content (an edited entry), and that the latest entry of each record
-- still matches the live record (an edited or deleted record). After the last
-- entry it checks the chain head (dropped newest entries).
CREATE FUNCTION chain_first_break(p_site UUID) RETURNS TABLE (seq BIGINT, reason TEXT)
    LANGUAGE plpgsql STABLE AS $$
DECLARE
    r        record;
    v_expect BIGINT := 1;
    v_prev   BYTEA  := chain_genesis(p_site);
    v_live   BYTEA;
    h        record;
BEGIN
    FOR r IN SELECT * FROM record_chain c WHERE c.site_id = p_site ORDER BY c.seq LOOP
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
        IF NOT EXISTS (SELECT 1 FROM record_chain n
                       WHERE n.kind = r.kind AND n.record_id = r.record_id AND n.seq > r.seq
                         AND n.site_id = r.site_id) THEN
            v_live := chain_record_hash(r.kind, r.record_id);
            IF v_live IS NULL THEN
                RETURN QUERY SELECT r.seq, 'record deleted'::text;
                RETURN;
            ELSIF v_live <> r.record_hash THEN
                RETURN QUERY SELECT r.seq, 'record modified'::text;
                RETURN;
            END IF;
        END IF;
        v_prev   := r.entry_hash;
        v_expect := v_expect + 1;
    END LOOP;
    -- The head must exist once the chain has entries, and match its last one.
    SELECT * INTO h FROM record_chain_heads WHERE site_id = p_site;
    IF NOT FOUND THEN
        IF v_expect > 1 THEN
            RETURN QUERY SELECT v_expect, 'chain head missing'::text;
            RETURN;
        END IF;
    ELSIF h.last_seq <> v_expect - 1 OR h.last_hash <> v_prev THEN
        RETURN QUERY SELECT v_expect, 'chain head does not match the last entry'::text;
        RETURN;
    END IF;
    -- Records with no chain entry at all (rows inserted outside the application,
    -- or whose entries were all deleted).
    IF EXISTS (SELECT 1 FROM chain_unchained_records(p_site)) THEN
        RETURN QUERY SELECT v_expect, 'record has no chain entry'::text;
    END IF;
END $$;

-- Existing rows: chain them once, per site, oldest first (ties by kind, id).
-- These entries vouch for the state at migration time, not for the original
-- write, hence origin = 'backfill'. Read-only on the record tables.
DO $$
DECLARE
    r record;
BEGIN
    FOR r IN
        SELECT kind, id FROM (
            SELECT 'attendance' AS kind, id, site_id, created_at AS ts FROM attendance_records
            UNION ALL
            SELECT 'scan', s.id, rt.site_id, s.received_at
              FROM checkpoint_scans s JOIN routes rt ON rt.id = s.route_id
            UNION ALL
            SELECT 'incident', id, site_id, created_at FROM incidents
            UNION ALL
            SELECT 'incident_amendment', a.id, i.site_id, a.created_at
              FROM incident_amendments a JOIN incidents i ON i.id = a.incident_id
        ) x ORDER BY site_id, ts, kind, id
    LOOP
        PERFORM chain_append(r.kind, r.id, 'backfill');
    END LOOP;
END $$;
