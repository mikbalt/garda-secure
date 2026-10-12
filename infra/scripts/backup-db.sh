#!/bin/sh
# Dump the app and Keycloak databases to infra/backups/<label>/ (INF-M4).
# Used before every deploy (migrations cannot be undone otherwise) and by
# `make backup-db`. Runs pg_dump inside the postgres container over the local
# socket — no password on any command line. Keeps the newest 10 sets.
#   scripts/backup-db.sh [label]
set -eu
. "$(dirname -- "$0")/lib.sh"
load_env "$INFRA_DIR/.env"

label=${1:-manual}
case "$label" in *[!A-Za-z0-9._-]*) die "label may only contain [A-Za-z0-9._-]" ;; esac

if ! compose ps --status running --services 2>/dev/null | grep -qx postgres; then
  warn "postgres is not running — nothing to back up (first install?)"
  exit 0
fi

root="$INFRA_DIR/backups"
dest="$root/$(date -u +%Y%m%dT%H%M%SZ)-$label"
umask 077
mkdir -p "$dest"
for db in "$POSTGRES_DB" "$KC_DB_NAME"; do
  info "pg_dump $db -> $dest/$db.dump"
  compose exec -T postgres pg_dump -U "$POSTGRES_USER" --format=custom --no-owner --no-privileges "$db" > "$dest/$db.dump.part"
  mv "$dest/$db.dump.part" "$dest/$db.dump"
  [ -s "$dest/$db.dump" ] || die "empty dump for $db"
done
echo "$dest"

# Retention: newest 10 dump sets.
# shellcheck disable=SC2012
ls -1d "$root"/*/ 2>/dev/null | sort -r | tail -n +11 | while IFS= read -r old; do
  rm -rf "$old"
done
