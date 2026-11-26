#!/bin/sh
# Role split of D-071 on a data volume that predates it (postgres/init only
# runs on an empty volume): creates sp_runtime and sp_signer if missing, sets
# their passwords from .env and applies the grants. Idempotent. Run from infra/
# with the stack's postgres running, BEFORE restarting the API on a version
# that uses SP_RUNTIME_DB_PASSWORD:
#   sh scripts/db-roles.sh
# Passwords go through the environment (psql \getenv), never into argv or SQL text.
set -eu
cd "$(dirname "$0")/.."
[ -f .env ] || { echo "db-roles: infra/.env not found" >&2; exit 1; }
set -a; . ./.env; set +a
: "${SP_RUNTIME_DB_PASSWORD:?}" "${SP_SIGNER_DB_PASSWORD:?}" "${APP_DB_USER:?}" "${POSTGRES_DB:?}" "${POSTGRES_USER:?}"

dc="docker compose"
$dc exec -T -e SP_RUNTIME_DB_PASSWORD -e SP_SIGNER_DB_PASSWORD -e POSTGRES_DB postgres \
  psql -v ON_ERROR_STOP=1 -U "$POSTGRES_USER" -d "$POSTGRES_DB" <<'SQL'
\getenv app_db POSTGRES_DB
\getenv rt_pw SP_RUNTIME_DB_PASSWORD
\getenv sg_pw SP_SIGNER_DB_PASSWORD
SELECT NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'sp_runtime') AS mk_rt \gset
\if :mk_rt
CREATE ROLE sp_runtime LOGIN PASSWORD :'rt_pw';
\else
ALTER ROLE sp_runtime LOGIN PASSWORD :'rt_pw';
\endif
SELECT NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'sp_signer') AS mk_sg \gset
\if :mk_sg
CREATE ROLE sp_signer LOGIN PASSWORD :'sg_pw' CONNECTION LIMIT 5;
\else
ALTER ROLE sp_signer LOGIN PASSWORD :'sg_pw' CONNECTION LIMIT 5;
\endif
GRANT CONNECT ON DATABASE :"app_db" TO sp_runtime, sp_signer;
SQL

# The grants must run as the table owner (default privileges are per role).
# Migration 013 created the function; before it ran there is nothing to grant yet
# (013 applies the grants itself when the API starts).
if $dc exec -T postgres psql -U "$APP_DB_USER" -d "$POSTGRES_DB" -Atc \
     "SELECT to_regproc('garda_apply_role_grants') IS NOT NULL" | grep -q t; then
  $dc exec -T postgres psql -v ON_ERROR_STOP=1 -U "$APP_DB_USER" -d "$POSTGRES_DB" -c 'SELECT garda_apply_role_grants()'
fi
echo "db-roles: sp_runtime and sp_signer are ready"
