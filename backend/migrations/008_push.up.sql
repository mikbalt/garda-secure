-- 008 push notifications: browser (Web Push, VAPID) subscriptions of
-- dashboard users and FCM tokens of tablets. Deliveries stay in
-- notification_log (channels 'webpush' and 'fcm'; recipient = subscription
-- id or FCM token).

-- -- Audience snapshot of a dashboard user ------------------------------------
-- Realm, roles and allowed sites (token claim "sites", D-042) as of the
-- user's latest authenticated request, so the worker resolves push audiences
-- without calling Keycloak. sites NULL = every site of the realm.
-- prefs holds the user's overrides per event type ({"sos": false, ...}).
CREATE TABLE push_users (
    keycloak_realm   TEXT        NOT NULL,
    keycloak_user_id TEXT        NOT NULL,
    username         TEXT        NOT NULL DEFAULT '',
    roles            TEXT[]      NOT NULL DEFAULT '{}',
    sites            UUID[],
    prefs            JSONB       NOT NULL DEFAULT '{}'::jsonb,
    seen_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (keycloak_realm, keycloak_user_id),
    CHECK (jsonb_typeof(prefs) = 'object')
);

-- -- Browser push subscriptions (one per browser profile) --------------------
CREATE TABLE push_subscriptions (
    id               UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    keycloak_realm   TEXT        NOT NULL,
    keycloak_user_id TEXT        NOT NULL,
    kind             TEXT        NOT NULL DEFAULT 'webpush' CHECK (kind IN ('webpush')),
    endpoint         TEXT        NOT NULL UNIQUE CHECK (endpoint LIKE 'https://%' AND length(endpoint) <= 1024),
    p256dh           TEXT        NOT NULL,
    auth             TEXT        NOT NULL,
    label            TEXT        NOT NULL DEFAULT '',
    last_success_at  TIMESTAMPTZ,
    last_failure_at  TIMESTAMPTZ,
    last_error       TEXT,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    FOREIGN KEY (keycloak_realm, keycloak_user_id)
        REFERENCES push_users (keycloak_realm, keycloak_user_id) ON DELETE CASCADE
);
CREATE INDEX idx_push_subscriptions_user ON push_subscriptions(keycloak_realm, keycloak_user_id);

-- -- Tablet FCM token (latest registration wins; cleared on revoke) ----------
-- unhealthy_alerted_at debounces device_unhealthy pushes (once per 6 h).
ALTER TABLE devices
    ADD COLUMN push_token           TEXT CHECK (push_token IS NULL OR length(push_token) BETWEEN 1 AND 4096),
    ADD COLUMN push_platform        TEXT CHECK (push_platform IS NULL OR push_platform IN ('android', 'ios')),
    ADD COLUMN push_token_at        TIMESTAMPTZ,
    ADD COLUMN unhealthy_alerted_at TIMESTAMPTZ;
CREATE UNIQUE INDEX idx_devices_push_token ON devices(push_token) WHERE push_token IS NOT NULL;

-- -- SOS escalation (sos_unacknowledged): reminders already staged ----------
ALTER TABLE sos_alerts ADD COLUMN escalations SMALLINT NOT NULL DEFAULT 0 CHECK (escalations BETWEEN 0 AND 10);
CREATE INDEX idx_sos_escalation ON sos_alerts(received_at) WHERE acknowledged_at IS NULL AND escalations < 3;
