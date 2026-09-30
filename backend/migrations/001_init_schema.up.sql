CREATE EXTENSION IF NOT EXISTS "pgcrypto";
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

CREATE TABLE guards (
    id               UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    keycloak_user_id TEXT        UNIQUE,
    name             TEXT        NOT NULL,
    employee_id      TEXT        UNIQUE,
    role             TEXT        NOT NULL,
    pin_hash         TEXT,
    status           TEXT        NOT NULL DEFAULT 'active',
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE sites (
    id             UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    name           TEXT        NOT NULL,
    client_id      TEXT,
    keycloak_realm TEXT,
    address        TEXT,
    timezone       TEXT        NOT NULL DEFAULT 'UTC',
    config_json    JSONB       NOT NULL DEFAULT '{}',
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE shifts (
    id          UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    guard_id    UUID        NOT NULL REFERENCES guards(id)  ON DELETE CASCADE,
    site_id     UUID        NOT NULL REFERENCES sites(id)   ON DELETE CASCADE,
    start_time  TIMESTAMPTZ NOT NULL,
    end_time    TIMESTAMPTZ NOT NULL,
    repeat_rule TEXT,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE attendance_records (
    id               UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    shift_id         UUID        REFERENCES shifts(id),
    guard_id         UUID        NOT NULL REFERENCES guards(id),
    site_id          UUID        NOT NULL REFERENCES sites(id),
    clock_in_at      TIMESTAMPTZ,
    clock_out_at     TIMESTAMPTZ,
    status           TEXT        NOT NULL DEFAULT 'pending',
    selfie_object_key TEXT,
    gps_lat          DOUBLE PRECISION,
    gps_lng          DOUBLE PRECISION,
    override_reason  TEXT,
    overridden_by    TEXT,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE routes (
    id          UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    site_id     UUID        NOT NULL REFERENCES sites(id) ON DELETE CASCADE,
    name        TEXT        NOT NULL,
    checkpoints JSONB       NOT NULL DEFAULT '[]',
    active      BOOLEAN     NOT NULL DEFAULT true,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE checkpoint_scans (
    id               UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    route_id         UUID        NOT NULL REFERENCES routes(id),
    checkpoint_id    TEXT        NOT NULL,
    guard_id         UUID        NOT NULL REFERENCES guards(id),
    scanned_at       TIMESTAMPTZ NOT NULL,
    method           TEXT        NOT NULL DEFAULT 'qr',
    photo_object_key TEXT,
    gps_lat          DOUBLE PRECISION,
    gps_lng          DOUBLE PRECISION
);

CREATE TABLE incidents (
    id               UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    site_id          UUID        NOT NULL REFERENCES sites(id),
    guard_id         UUID        NOT NULL REFERENCES guards(id),
    type             TEXT        NOT NULL,
    severity         TEXT        NOT NULL DEFAULT 'low',
    description      TEXT,
    photo_object_keys JSONB      NOT NULL DEFAULT '[]',
    location         TEXT,
    status           TEXT        NOT NULL DEFAULT 'open',
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE defect_reports (
    id               UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    site_id          UUID        NOT NULL REFERENCES sites(id),
    guard_id         UUID        NOT NULL REFERENCES guards(id),
    defect_type      TEXT,
    location         TEXT,
    description      TEXT,
    photo_object_key TEXT,
    status           TEXT        NOT NULL DEFAULT 'open',
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE visitor_logs (
    id               UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    site_id          UUID        NOT NULL REFERENCES sites(id),
    guard_id         UUID        NOT NULL REFERENCES guards(id),
    visitor_name     TEXT        NOT NULL,
    id_type          TEXT,
    id_number        TEXT,
    plate            TEXT,
    host_name        TEXT,
    entry_at         TIMESTAMPTZ,
    exit_at          TIMESTAMPTZ,
    purpose          TEXT,
    photo_object_key TEXT,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE sos_alerts (
    id               UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    guard_id         UUID        NOT NULL REFERENCES guards(id),
    site_id          UUID        NOT NULL REFERENCES sites(id),
    fired_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    gps_lat          DOUBLE PRECISION,
    gps_lng          DOUBLE PRECISION,
    acknowledged_by  TEXT,
    acknowledged_at  TIMESTAMPTZ
);

-- Indexes for common query patterns
CREATE INDEX idx_attendance_guard_site ON attendance_records(guard_id, site_id, created_at);
CREATE INDEX idx_attendance_status     ON attendance_records(status, site_id);
CREATE INDEX idx_scans_guard           ON checkpoint_scans(guard_id, scanned_at);
CREATE INDEX idx_scans_route           ON checkpoint_scans(route_id, scanned_at);
CREATE INDEX idx_incidents_site        ON incidents(site_id, created_at);
CREATE INDEX idx_incidents_status      ON incidents(status, severity);
CREATE INDEX idx_sos_site              ON sos_alerts(site_id, fired_at);
CREATE INDEX idx_visitor_site          ON visitor_logs(site_id, created_at);
