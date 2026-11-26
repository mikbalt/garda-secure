-- TAQ-21: a supervisor marks a plausibility flag as reviewed. The mark lives
-- in its own table, keyed by (record kind, record id, flag), so the flagged
-- record is never written to (it may be covered by a record chain).
-- No foreign key on record_id: it points into several tables.
CREATE TABLE flag_reviews (
    id          UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    kind        TEXT        NOT NULL
                CHECK (kind IN ('attendance', 'scan', 'session', 'incident', 'sos', 'defect', 'visitor')),
    record_id   UUID        NOT NULL,
    flag        TEXT        NOT NULL,
    site_id     UUID        NOT NULL REFERENCES sites(id) ON DELETE CASCADE,
    reviewed_by TEXT        NOT NULL,
    reviewed_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    note        TEXT        NOT NULL DEFAULT '',
    UNIQUE (kind, record_id, flag)
);
CREATE INDEX idx_flag_reviews_site ON flag_reviews (site_id, reviewed_at);
