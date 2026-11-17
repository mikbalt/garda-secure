-- Reverts 009. Deactivated sites become active again.
DROP INDEX IF EXISTS idx_sites_active;
ALTER TABLE sites DROP COLUMN IF EXISTS active;
