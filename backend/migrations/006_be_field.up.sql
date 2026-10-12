-- 006 be-field (D-035): server-verified guard sessions, signed checkpoint QR
-- codes, patrol schedules and "patrol not started" expectations.

-- -- Guard sessions: PIN brute-force protection ------------------------------
-- 5 consecutive wrong PINs lock the guard for 15 minutes (counted on the
-- server across all tablets; reset on success or when an admin sets a PIN).
ALTER TABLE guards
    ADD COLUMN pin_failed_attempts INT         NOT NULL DEFAULT 0,
    ADD COLUMN pin_locked_until    TIMESTAMPTZ;

-- -- Plausibility flags on the remaining field records -----------------------
-- e.g. guard_unverified (no valid guard token when the record was made).
ALTER TABLE patrol_sessions ADD COLUMN flags TEXT[] NOT NULL DEFAULT '{}';
ALTER TABLE defect_reports  ADD COLUMN flags TEXT[] NOT NULL DEFAULT '{}';
ALTER TABLE visitor_logs    ADD COLUMN flags TEXT[] NOT NULL DEFAULT '{}';

-- -- Signed checkpoint QR codes ------------------------------------------------
-- Checkpoints are stored in routes.checkpoints (JSONB). Each element gains the
-- keys qr_version (int) and qr_signed (bool), written by the API. A checkpoint
-- without them is a legacy (unsigned) code, so existing rows need no change.

-- -- Patrol schedules ----------------------------------------------------------
-- A route can have several windows. every_minutes NULL = one patrol at
-- start_time; otherwise patrols are due every every_minutes from start_time
-- until (excluding) end_time. end_time <= start_time crosses midnight
-- (equal = 24 h). Weekdays (ISO 1..7) refer to the day the window starts,
-- in the site's time zone.
CREATE TABLE route_schedules (
    id            UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    route_id      UUID        NOT NULL REFERENCES routes(id) ON DELETE CASCADE,
    weekdays      SMALLINT[]  NOT NULL,
    start_time    TIME        NOT NULL,
    end_time      TIME,
    every_minutes INT,
    grace_minutes INT         NOT NULL DEFAULT 15,
    active        BOOLEAN     NOT NULL DEFAULT true,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (cardinality(weekdays) > 0 AND weekdays <@ ARRAY[1,2,3,4,5,6,7]::SMALLINT[]),
    CHECK (every_minutes IS NULL OR (every_minutes BETWEEN 15 AND 720 AND end_time IS NOT NULL)),
    CHECK (grace_minutes BETWEEN 0 AND 180)
);
CREATE INDEX idx_route_schedules_route ON route_schedules(route_id);

-- One row per scheduled patrol, materialised 48 h ahead by the worker.
-- A session of the route started in [due_at - grace, grace_until] marks it
-- started; after grace_until without one it is missed (patrol_not_started
-- notification). late_start = started after grace_until (missed, then a late
-- patrol arrived). One session satisfies at most one expectation.
CREATE TABLE patrol_expectations (
    id          UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    route_id    UUID        NOT NULL REFERENCES routes(id) ON DELETE CASCADE,
    site_id     UUID        NOT NULL REFERENCES sites(id) ON DELETE CASCADE,
    schedule_id UUID        REFERENCES route_schedules(id) ON DELETE SET NULL,
    due_at      TIMESTAMPTZ NOT NULL,
    grace_until TIMESTAMPTZ NOT NULL,
    status      TEXT        NOT NULL DEFAULT 'pending'
                            CHECK (status IN ('pending', 'started', 'missed', 'cancelled')),
    session_id  UUID        REFERENCES patrol_sessions(id) ON DELETE SET NULL,
    late_start  BOOLEAN     NOT NULL DEFAULT false,
    notified_at TIMESTAMPTZ,
    matched_at  TIMESTAMPTZ,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (route_id, due_at),
    CHECK (grace_until >= due_at)
);
CREATE UNIQUE INDEX uq_patrol_expectations_session ON patrol_expectations(session_id) WHERE session_id IS NOT NULL;
CREATE INDEX idx_patrol_expectations_site_due ON patrol_expectations(site_id, due_at);
CREATE INDEX idx_patrol_expectations_overdue ON patrol_expectations(grace_until) WHERE status = 'pending';

-- Matching sessions to expectations looks sessions up by route and start.
CREATE INDEX idx_sessions_route_started ON patrol_sessions(route_id, started_at);
