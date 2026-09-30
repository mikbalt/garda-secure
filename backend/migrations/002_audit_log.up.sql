CREATE TABLE audit_log (
    id          UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id     TEXT,
    action      TEXT        NOT NULL,
    resource_id TEXT,
    ip          TEXT,
    user_agent  TEXT,
    status_code INT,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_audit_user   ON audit_log(user_id, created_at);
CREATE INDEX idx_audit_action ON audit_log(action, created_at);
