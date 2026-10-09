#!/bin/sh
# Restore one database from a pg_dump custom-format file (rollback after a
# failed deploy, or disaster recovery — docs/runbook.md §5/§7).
#   scripts/restore-db.sh app      <file.dump>   app database (POSTGRES_DB)
#   scripts/restore-db.sh keycloak <file.dump>   Keycloak database (KC_DB_NAME)
# Stops the services using the database, restores with --clean as the owning
# role (objects stay owned by APP_DB_USER / KC_DB_USER), starts them again.
set -eu
. "$(dirname -- "$0")/lib.sh"
load_env "$INFRA_DIR/.env"

which=${1:-}
file=${2:-}
[ -f "$file" ] || die "usage: $0 app|keycloak <file.dump>"
case "$which" in
  app) db=$POSTGRES_DB; owner=$APP_DB_USER; services="api notification-worker" ;;
  keycloak) db=$KC_DB_NAME; owner=$KC_DB_USER; services="keycloak" ;;
  *) die "first argument must be app or keycloak" ;;
esac

printf 'Restore %s into database "%s" (current data is REPLACED). Type the database name to continue: ' "$file" "$db"
read -r answer
[ "$answer" = "$db" ] || die "aborted"

info "stopping $services"
# shellcheck disable=SC2086
compose stop $services
info "restoring $db as role $owner"
compose exec -T postgres pg_restore -U "$POSTGRES_USER" --dbname "$db" --clean --if-exists \
  --no-owner --no-privileges --role "$owner" --single-transaction < "$file"
info "starting $services"
# shellcheck disable=SC2086
compose start $services
info "restore of $db finished"
