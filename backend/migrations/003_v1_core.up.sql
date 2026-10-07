-- v1 core: device auth, shift templates, patrol sessions, amendments,
-- notifications, settings and backups.

-- -- Guards belong to one site ------------------------------------------------
ALTER TABLE guards ADD COLUMN site_id UUID REFERENCES sites(id) ON DELETE SET NULL;
CREATE INDEX idx_guards_site ON guards(site_id);

-- -- Devices (shared tablets, one site each) ----------------------------------
CREATE TABLE devices (
    id               UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    keycloak_realm   TEXT        NOT NULL,
    keycloak_user_id TEXT        NOT NULL,
    name             TEXT        NOT NULL,
    site_id          UUID        REFERENCES sites(id) ON DELETE SET NULL,
    status           TEXT        NOT NULL DEFAULT 'pending'
                                 CHECK (status IN ('pending', 'active', 'revoked')),
    approved_by      TEXT,
    approved_at      TIMESTAMPTZ,
    last_seen_at     TIMESTAMPTZ,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (keycloak_realm, keycloak_user_id)
);

-- -- Shift templates; concrete shifts are pre-generated from them -------------
CREATE TABLE shift_templates (
    id          UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    site_id     UUID        NOT NULL REFERENCES sites(id)  ON DELETE CASCADE,
    guard_id    UUID        NOT NULL REFERENCES guards(id) ON DELETE CASCADE,
    start_time  TIME        NOT NULL,
    end_time    TIME        NOT NULL,           -- end <= start means the shift ends the next day
    weekdays    SMALLINT[]  NOT NULL,           -- ISO weekdays: 1 = Monday ... 7 = Sunday
    valid_from  DATE        NOT NULL,
    valid_until DATE,
    active      BOOLEAN     NOT NULL DEFAULT true,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (start_time <> end_time),
    CHECK (cardinality(weekdays) > 0 AND weekdays <@ ARRAY[1,2,3,4,5,6,7]::SMALLINT[]),
    CHECK (valid_until IS NULL OR valid_until >= valid_from)
);
CREATE INDEX idx_shift_templates_site ON shift_templates(site_id);

ALTER TABLE shifts DROP COLUMN repeat_rule;
ALTER TABLE shifts ADD COLUMN template_id UUID REFERENCES shift_templates(id) ON DELETE SET NULL;
ALTER TABLE shifts ADD CONSTRAINT chk_shift_order CHECK (end_time > start_time);
CREATE UNIQUE INDEX uq_shifts_template_start ON shifts(template_id, start_time) WHERE template_id IS NOT NULL;
CREATE INDEX idx_shifts_site_start  ON shifts(site_id, start_time);
CREATE INDEX idx_shifts_guard_start ON shifts(guard_id, start_time);

-- -- Attendance ---------------------------------------------------------------
-- status is the arrival outcome (present | late | absent); early departure is
-- tracked separately in early_out_minutes so a late guard who also leaves
-- early keeps both labels.
UPDATE attendance_records SET status = 'present' WHERE status IN ('early_out', 'pending');
ALTER TABLE attendance_records
    ADD COLUMN late_minutes      INT         NOT NULL DEFAULT 0,
    ADD COLUMN early_out_minutes INT         NOT NULL DEFAULT 0,
    ADD COLUMN clock_out_gps_lat DOUBLE PRECISION,
    ADD COLUMN clock_out_gps_lng DOUBLE PRECISION,
    ADD COLUMN device_id         UUID        REFERENCES devices(id) ON DELETE SET NULL,
    ADD COLUMN needs_review      BOOLEAN     NOT NULL DEFAULT false,
    ADD COLUMN review_note       TEXT,
    ADD COLUMN reviewed_by       TEXT,
    ADD COLUMN reviewed_at       TIMESTAMPTZ,
    ADD COLUMN overridden_at     TIMESTAMPTZ;
ALTER TABLE attendance_records
    ADD CONSTRAINT chk_attendance_status CHECK (status IN ('present', 'late', 'absent'));
CREATE UNIQUE INDEX uq_attendance_shift ON attendance_records(shift_id) WHERE shift_id IS NOT NULL;
CREATE INDEX idx_attendance_review ON attendance_records(site_id) WHERE needs_review;

-- -- Patrol sessions & SLA breaches -------------------------------------------
CREATE TABLE patrol_sessions (
    id         UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    route_id   UUID        NOT NULL REFERENCES routes(id),
    site_id    UUID        NOT NULL REFERENCES sites(id),
    guard_id   UUID        NOT NULL REFERENCES guards(id),
    device_id  UUID        REFERENCES devices(id) ON DELETE SET NULL,
    started_at TIMESTAMPTZ NOT NULL,
    ended_at   TIMESTAMPTZ,
    status     TEXT        NOT NULL DEFAULT 'in_progress'
                           CHECK (status IN ('in_progress', 'completed', 'abandoned')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_sessions_site_started ON patrol_sessions(site_id, started_at);
CREATE INDEX idx_sessions_active ON patrol_sessions(status) WHERE status = 'in_progress';

ALTER TABLE checkpoint_scans
    ADD COLUMN session_id  UUID        REFERENCES patrol_sessions(id) ON DELETE CASCADE,
    ADD COLUMN device_id   UUID        REFERENCES devices(id) ON DELETE SET NULL,
    ADD COLUMN received_at TIMESTAMPTZ NOT NULL DEFAULT now();
ALTER TABLE checkpoint_scans
    ADD CONSTRAINT chk_scan_method CHECK (method IN ('qr', 'manual'));
CREATE INDEX idx_scans_session ON checkpoint_scans(session_id, scanned_at);

CREATE TABLE sla_breaches (
    id            UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    session_id    UUID        NOT NULL REFERENCES patrol_sessions(id) ON DELETE CASCADE,
    checkpoint_id TEXT        NOT NULL,
    due_at        TIMESTAMPTZ NOT NULL,
    detected_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (session_id, checkpoint_id)
);

-- -- Incidents: immutable originals + amendments ------------------------------
ALTER TABLE incidents
    ADD COLUMN occurred_at TIMESTAMPTZ,
    ADD COLUMN device_id   UUID REFERENCES devices(id) ON DELETE SET NULL;
UPDATE incidents SET occurred_at = created_at WHERE occurred_at IS NULL;
ALTER TABLE incidents ALTER COLUMN occurred_at SET NOT NULL;
ALTER TABLE incidents
    ADD CONSTRAINT chk_incident_severity CHECK (severity IN ('low', 'medium', 'high', 'critical')),
    ADD CONSTRAINT chk_incident_status   CHECK (status IN ('open', 'in_progress', 'resolved'));

CREATE TABLE incident_amendments (
    id          UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    incident_id UUID        NOT NULL REFERENCES incidents(id) ON DELETE CASCADE,
    changes     JSONB       NOT NULL,
    reason      TEXT        NOT NULL,
    amended_by  TEXT        NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_amendments_incident ON incident_amendments(incident_id, created_at);

ALTER TABLE defect_reports
    ADD COLUMN updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    ADD COLUMN device_id  UUID REFERENCES devices(id) ON DELETE SET NULL;
ALTER TABLE defect_reports
    ADD CONSTRAINT chk_defect_status CHECK (status IN ('open', 'in_progress', 'resolved'));
CREATE INDEX idx_defects_site ON defect_reports(site_id, created_at);

ALTER TABLE visitor_logs ADD COLUMN device_id UUID REFERENCES devices(id) ON DELETE SET NULL;

ALTER TABLE sos_alerts
    ADD COLUMN device_id   UUID        REFERENCES devices(id) ON DELETE SET NULL,
    ADD COLUMN received_at TIMESTAMPTZ NOT NULL DEFAULT now();

-- -- Notifications ------------------------------------------------------------
CREATE TABLE notification_rules (
    id         UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    site_id    UUID        NOT NULL REFERENCES sites(id) ON DELETE CASCADE,
    event_type TEXT        NOT NULL,
    channel    TEXT        NOT NULL CHECK (channel IN ('email', 'whatsapp')),
    recipient  TEXT        NOT NULL,
    enabled    BOOLEAN     NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (site_id, event_type, channel, recipient)
);

CREATE TABLE notification_log (
    id         UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    site_id    UUID        REFERENCES sites(id) ON DELETE SET NULL,
    event_type TEXT        NOT NULL,
    channel    TEXT        NOT NULL,
    recipient  TEXT        NOT NULL,
    subject    TEXT        NOT NULL DEFAULT '',
    body       TEXT        NOT NULL,
    status     TEXT        NOT NULL DEFAULT 'queued' CHECK (status IN ('queued', 'sent', 'failed')),
    attempts   INT         NOT NULL DEFAULT 0,
    last_error TEXT,
    dedupe_key TEXT        UNIQUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    sent_at    TIMESTAMPTZ
);
CREATE INDEX idx_notification_log_site ON notification_log(site_id, created_at DESC);
CREATE INDEX idx_notification_log_status ON notification_log(status, created_at DESC);

-- -- System settings (channel credentials etc.; secrets encrypted by the API) -
CREATE TABLE system_settings (
    key        TEXT        PRIMARY KEY,
    value      JSONB       NOT NULL,
    updated_by TEXT,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- -- Database backups ---------------------------------------------------------
CREATE TABLE backups (
    id           UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    object_key   TEXT        NOT NULL,
    size_bytes   BIGINT,
    status       TEXT        NOT NULL DEFAULT 'pending'
                             CHECK (status IN ('pending', 'running', 'succeeded', 'failed')),
    trigger      TEXT        NOT NULL CHECK (trigger IN ('scheduled', 'manual')),
    error        TEXT,
    requested_by TEXT,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    started_at   TIMESTAMPTZ,
    finished_at  TIMESTAMPTZ
);
CREATE INDEX idx_backups_created ON backups(created_at DESC);
