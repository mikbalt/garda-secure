-- 014 TAQ-18: what the verification export needs. chain_record_canonical(kind, id)
-- returns the exact text that chain_record_hash hashes (after the "<kind>:"
-- prefix), so a third party can recompute record_hash from the export. It
-- repeats the column lists of chain_record_hash (011) without touching it; the
-- integration test TestExportCanonicalMatchesRecordHash fails if the two ever
-- differ. Also an index so a date range finds its seq range without a scan.
-- ASCII only.

CREATE FUNCTION chain_record_canonical(p_kind TEXT, p_id UUID) RETURNS TEXT
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
    RETURN v::text;   -- NULL when the record does not exist
END $$;

CREATE INDEX idx_record_chain_site_time ON record_chain(site_id, created_at);
