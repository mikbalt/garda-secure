-- Site soft delete (issue #34): a deactivated site keeps all history but
-- disappears from lists, selectors and background jobs. Reversible.
ALTER TABLE sites ADD COLUMN active BOOLEAN NOT NULL DEFAULT true;
CREATE INDEX idx_sites_active ON sites (active);
