-- Roles keep no grants on the dropped objects; default privileges and the
-- runtime grants on older tables are left (harmless, re-applied by 013 up).
DROP FUNCTION IF EXISTS garda_apply_role_grants();
DROP FUNCTION IF EXISTS chain_links_break(UUID, BIGINT);
ALTER FUNCTION chain_append(TEXT, UUID, TEXT) SECURITY INVOKER RESET search_path;
ALTER FUNCTION chain_first_break(UUID) SECURITY INVOKER RESET search_path;
GRANT EXECUTE ON FUNCTION chain_append(TEXT, UUID, TEXT) TO PUBLIC;
GRANT EXECUTE ON FUNCTION chain_entry_hash(UUID, BIGINT, TEXT, UUID, BYTEA, BYTEA, SMALLINT, TEXT, TIMESTAMPTZ) TO PUBLIC;
DROP TABLE IF EXISTS signed_heads;
DROP TABLE IF EXISTS keyring;
DROP FUNCTION IF EXISTS signed_heads_guard();
DROP FUNCTION IF EXISTS signer_tables_append_only();
