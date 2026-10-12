-- 004: reliability hardening (audit 2026-10: BE-R1..R6, BE-O1, F2, F14).

-- -- Notification outbox claimed from Postgres (BE-R1, BE-R2, BE-R5) ----------
-- Workers claim due rows with FOR UPDATE SKIP LOCKED; locked_until is a lease
-- so a row being sent is not claimed twice. Redis only wakes the workers.
ALTER TABLE notification_log
    ADD COLUMN lane            TEXT        NOT NULL DEFAULT 'normal' CHECK (lane IN ('sos', 'normal')),
    ADD COLUMN next_attempt_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    ADD COLUMN locked_until    TIMESTAMPTZ,
    ADD COLUMN request_id      TEXT;
UPDATE notification_log SET lane = 'sos' WHERE event_type = 'sos';
CREATE INDEX idx_notification_log_due ON notification_log(lane, next_attempt_at) WHERE status = 'queued';
CREATE INDEX idx_notification_log_ops ON notification_log(created_at DESC) WHERE site_id IS NULL;

-- -- Install-wide operations alerts (backup failed/stale, SOS undelivered ...) -
CREATE TABLE ops_alert_rules (
    id         UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    event_type TEXT        NOT NULL,
    channel    TEXT        NOT NULL CHECK (channel IN ('email', 'whatsapp')),
    recipient  TEXT        NOT NULL,
    enabled    BOOLEAN     NOT NULL DEFAULT true,
    created_by TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (event_type, channel, recipient)
);

-- -- SLA evaluation of ended sessions (F2) -----------------------------------
-- Ended sessions are evaluated once by the worker (missed checkpoints); rows
-- that ended before this migration count as already evaluated.
ALTER TABLE patrol_sessions ADD COLUMN sla_checked_at TIMESTAMPTZ;
UPDATE patrol_sessions SET sla_checked_at = now() WHERE status <> 'in_progress';
CREATE INDEX idx_sessions_sla_unchecked ON patrol_sessions(ended_at) WHERE status <> 'in_progress' AND sla_checked_at IS NULL;

-- -- Device connectivity: start of the current online period (F2) ------------
ALTER TABLE devices ADD COLUMN online_since TIMESTAMPTZ;

-- -- Backups also dump the Keycloak database (BE-R3, F15) --------------------
ALTER TABLE backups
    ADD COLUMN keycloak_object_key TEXT,
    ADD COLUMN keycloak_size_bytes BIGINT;
