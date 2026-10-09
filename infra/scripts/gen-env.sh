#!/bin/sh
# Create infra/.env from .env.example with fresh random secrets.
#   scripts/gen-env.sh <SERVER_HOST> [output-file]
# - SERVER_HOST: DNS name (preferred) or IPv4 address of the server
# - every "change_me" secret becomes `openssl rand -hex 32` (URL-safe)
# - least-privilege MinIO users are enabled (MINIO_APP_* / MINIO_OFFSITE_*)
# Refuses to overwrite an existing file. The result is chmod 600.
set -eu
. "$(dirname -- "$0")/lib.sh"

host=${1:-}
out=${2:-$INFRA_DIR/.env}
[ -n "$host" ] || die "usage: $0 <SERVER_HOST> [output-file]"
is_ipv4 "$host" || is_hostname "$host" || die "SERVER_HOST must be a DNS name or an IPv4 address: $host"
[ -e "$out" ] && die "$out already exists — edit it, or move it away first"
command -v openssl >/dev/null || die "openssl is required"

rand() { openssl rand -hex 32; }

umask 077
tmp="$out.tmp.$$"
cr=$(printf '\r')
while IFS= read -r line || [ -n "$line" ]; do
  line=${line%"$cr"}
  case $line in
    SERVER_HOST=*) echo "SERVER_HOST=$host" ;;
    *=change_me) echo "${line%%=*}=$(rand)" ;;
    MINIO_APP_ACCESS_KEY=) echo "MINIO_APP_ACCESS_KEY=securepatrol-app" ;;
    MINIO_APP_SECRET_KEY=) echo "MINIO_APP_SECRET_KEY=$(rand)" ;;
    MINIO_OFFSITE_ACCESS_KEY=) echo "MINIO_OFFSITE_ACCESS_KEY=securepatrol-offsite" ;;
    MINIO_OFFSITE_SECRET_KEY=) echo "MINIO_OFFSITE_SECRET_KEY=$(rand)" ;;
    *) printf '%s\n' "$line" ;;
  esac
done < "$INFRA_DIR/.env.example" > "$tmp"
mv "$tmp" "$out"
chmod 600 "$out"

info "wrote $out (mode 600) for SERVER_HOST=$host"
echo "    Next: review it (SMTP, ALERT_*, OFFSITE_BACKUP_REPO, realms), then"
echo "    make certs && make up"
echo "    Store a copy of the secrets in your password manager: without"
echo "    SETTINGS_ENCRYPTION_KEY and OFFSITE_RESTIC_PASSWORD backups are useless."
