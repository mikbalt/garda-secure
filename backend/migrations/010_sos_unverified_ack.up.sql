-- A tablet acknowledgement by a supervisor whose PIN the server did not verify
-- (no valid guard token, D-065). It quiets the site's tablets, but the alert
-- stays unacknowledged (acknowledged_at IS NULL): the web banner and the
-- sos_unacknowledged reminders go on until a verified or web acknowledgement.
ALTER TABLE sos_alerts
    ADD COLUMN unverified_ack_by TEXT,
    ADD COLUMN unverified_ack_at TIMESTAMPTZ;
