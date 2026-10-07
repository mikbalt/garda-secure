#!/bin/sh
# Runs once, on first start of an empty data volume: Keycloak gets its own
# database so app migrations and backups never mix with Keycloak's tables.
set -eu
psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" --dbname "$POSTGRES_DB" <<-SQL
  SELECT 'CREATE DATABASE ${KC_DB_NAME:-keycloak} OWNER ${POSTGRES_USER}'
  WHERE NOT EXISTS (SELECT FROM pg_database WHERE datname = '${KC_DB_NAME:-keycloak}')\gexec
SQL
