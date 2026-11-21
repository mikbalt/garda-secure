-- Reverts 010. Unverified tablet acknowledgements are forgotten; the alerts
-- they belong to are unacknowledged either way.
ALTER TABLE sos_alerts
    DROP COLUMN IF EXISTS unverified_ack_by,
    DROP COLUMN IF EXISTS unverified_ack_at;
