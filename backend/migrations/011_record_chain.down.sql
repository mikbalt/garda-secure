DROP FUNCTION chain_first_break(UUID);
DROP FUNCTION chain_append(TEXT, UUID, TEXT);
DROP FUNCTION chain_entry_hash(UUID, BIGINT, TEXT, UUID, BYTEA, BYTEA, SMALLINT);
DROP FUNCTION chain_record_hash(TEXT, UUID);
DROP FUNCTION chain_record_site(TEXT, UUID);
DROP FUNCTION chain_genesis(UUID);
DROP TABLE record_chain;
DROP TABLE record_chain_heads;
DROP FUNCTION record_chain_append_only();
