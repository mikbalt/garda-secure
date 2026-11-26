#!/bin/sh
# Runs once, on the first start of an empty data volume (INF-S4):
#  - APP_DB_USER owns only POSTGRES_DB (the app database) — the API runs its
#    migrations as this role, the worker dumps it with pg_dump;
#  - KC_DB_USER owns only KC_DB_NAME (Keycloak gets its own database, D-019);
#  - sp_runtime (API/worker) and sp_signer (signer service) are not owners (D-071);
#  - sp_monitor (pg_monitor) is used by postgres_exporter;
#  - POSTGRES_USER stays the superuser for admin/restore only.
# Nobody else may connect to either database. Passwords are read into psql
# variables (quoted by psql), never interpolated into SQL by the shell.
# Existing volumes: see docs/runbook.md §1 "Database roles".
set -eu

# SP_RUNTIME_DB_PASSWORD / SP_SIGNER_DB_PASSWORD: D-071 role split (see below).
: "${APP_DB_USER:?}" "${APP_DB_PASSWORD:?}" "${KC_DB_USER:?}" "${KC_DB_PASSWORD:?}" "${PG_MONITOR_PASSWORD:?}"
: "${SP_RUNTIME_DB_PASSWORD:?}" "${SP_SIGNER_DB_PASSWORD:?}"
KC_DB_NAME="${KC_DB_NAME:-keycloak}"

export KC_DB_NAME
# \getenv (psql 15+) reads the values from the environment: no secrets in argv.
psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" --dbname "$POSTGRES_DB" <<'SQL'
\getenv app_db POSTGRES_DB
\getenv app_user APP_DB_USER
\getenv app_pw APP_DB_PASSWORD
\getenv kc_db KC_DB_NAME
\getenv kc_user KC_DB_USER
\getenv kc_pw KC_DB_PASSWORD
\getenv mon_pw PG_MONITOR_PASSWORD
\getenv rt_pw SP_RUNTIME_DB_PASSWORD
\getenv sg_pw SP_SIGNER_DB_PASSWORD
CREATE ROLE :"app_user" LOGIN PASSWORD :'app_pw';
CREATE ROLE :"kc_user" LOGIN PASSWORD :'kc_pw';
CREATE ROLE sp_monitor LOGIN PASSWORD :'mon_pw' CONNECTION LIMIT 5;
GRANT pg_monitor TO sp_monitor;
-- Owner/runtime split (D-071): APP_DB_USER owns the tables and runs migrations;
-- the API and worker connect as sp_runtime (table DML, no ownership, no
-- DDL), the signer as sp_signer. Migration 013 grants their privileges.
CREATE ROLE sp_runtime LOGIN PASSWORD :'rt_pw';
CREATE ROLE sp_signer LOGIN PASSWORD :'sg_pw' CONNECTION LIMIT 5;

-- App database: owned by the app role (it creates the trusted extensions
-- pgcrypto/uuid-ossp and all tables through migrations).
ALTER DATABASE :"app_db" OWNER TO :"app_user";
REVOKE ALL ON DATABASE :"app_db" FROM PUBLIC;
GRANT CONNECT, TEMPORARY ON DATABASE :"app_db" TO :"app_user";
GRANT CONNECT ON DATABASE :"app_db" TO sp_monitor, sp_runtime, sp_signer;

-- Keycloak database.
CREATE DATABASE :"kc_db" OWNER :"kc_user";
REVOKE ALL ON DATABASE :"kc_db" FROM PUBLIC;
GRANT CONNECT, TEMPORARY ON DATABASE :"kc_db" TO :"kc_user";
GRANT CONNECT ON DATABASE :"kc_db" TO sp_monitor;
SQL

# PostgreSQL 15+: the public schema belongs to pg_database_owner, so the new
# owners can create objects in it. Remove the default CREATE for PUBLIC anyway.
for db in "$POSTGRES_DB" "$KC_DB_NAME"; do
  psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" --dbname "$db" \
    -c 'REVOKE CREATE ON SCHEMA public FROM PUBLIC;'
done

echo "securepatrol: roles ${APP_DB_USER}, ${KC_DB_USER}, sp_runtime, sp_signer, sp_monitor created"
