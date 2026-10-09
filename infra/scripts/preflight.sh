#!/bin/sh
# Pre-start checks (INF-D4, INF-S3). `make up` and scripts/deploy.sh run this
# first and stop on any FAIL. Usage: scripts/preflight.sh [env-file]
#   PREFLIGHT_ALLOW_LOCALHOST=1  accept SERVER_HOST=localhost (CI only)
set -eu
. "$(dirname -- "$0")/lib.sh"

envfile=${1:-$INFRA_DIR/.env}
fails=0
warns=0
fail() { echo "FAIL:  $*"; fails=$((fails + 1)); }
warnp() { echo "WARN:  $*"; warns=$((warns + 1)); }
ok()   { echo "ok:    $*"; }

[ -f "$envfile" ] || die "missing $envfile — run: make env SERVER_HOST=<dns-name-or-ip>"

# ── .env permissions ────────────────────────────────────────────────────────
if mode=$(stat -c %a "$envfile" 2>/dev/null); then
  case $mode in
    600|400) ok ".env mode $mode" ;;
    *) case "$(uname -s)" in
         Linux) fail ".env is mode $mode — run: chmod 600 $envfile" ;;
         *) warnp ".env mode $mode (not checked on $(uname -s))" ;;
       esac ;;
  esac
fi

load_env "$envfile"

# ── no defaults left ────────────────────────────────────────────────────────
defaults=$(grep -E '^[A-Za-z_][A-Za-z0-9_]*=.*change_me' "$envfile" | cut -d= -f1 | tr '\n' ' ' || true)
if [ -n "$defaults" ]; then fail "still set to change_me: $defaults"; else ok "no change_me values"; fi

# ── required values ─────────────────────────────────────────────────────────
for v in SERVER_HOST PUBLIC_URL KEYCLOAK_PUBLIC_URL MINIO_PUBLIC_ENDPOINT POSTGRES_DB POSTGRES_USER \
         POSTGRES_PASSWORD APP_DB_USER APP_DB_PASSWORD KC_DB_NAME KC_DB_USER KC_DB_PASSWORD \
         PG_MONITOR_PASSWORD REDIS_PASSWORD MINIO_ROOT_USER MINIO_ROOT_PASSWORD MINIO_BUCKET_PHOTOS \
         MINIO_BUCKET_BACKUPS KEYCLOAK_ADMIN KEYCLOAK_ADMIN_PASSWORD KEYCLOAK_REALMS WEB_KEYCLOAK_REALMS \
         SETTINGS_ENCRYPTION_KEY GF_SECURITY_ADMIN_PASSWORD OFFSITE_BACKUP_REPO OFFSITE_RESTIC_PASSWORD; do
  eval "val=\${$v:-}"
  [ -n "$val" ] || fail "$v is empty"
done

# ── secrets: URL-safe and long (INF-D6/S3) ──────────────────────────────────
secret_re='^[A-Za-z0-9_-]{32,}$'
check_secret() { # name value
  if ! printf '%s' "$2" | grep -Eq "$secret_re"; then
    fail "$1 must be >= 32 characters of [A-Za-z0-9_-] (use: openssl rand -hex 32)"
  fi
}
seen=""
for v in POSTGRES_PASSWORD APP_DB_PASSWORD KC_DB_PASSWORD PG_MONITOR_PASSWORD REDIS_PASSWORD \
         MINIO_ROOT_PASSWORD KEYCLOAK_ADMIN_PASSWORD GF_SECURITY_ADMIN_PASSWORD OFFSITE_RESTIC_PASSWORD \
         MINIO_APP_SECRET_KEY MINIO_OFFSITE_SECRET_KEY; do
  eval "val=\${$v:-}"
  case $v in MINIO_APP_SECRET_KEY|MINIO_OFFSITE_SECRET_KEY) [ -z "$val" ] && continue ;; esac
  [ -n "$val" ] || continue
  check_secret "$v" "$val"
  case " $seen " in *" $val "*) fail "$v reuses the value of another secret" ;; esac
  seen="$seen $val"
done
if [ "${#SETTINGS_ENCRYPTION_KEY}" -lt 32 ] || printf '%s' "$SETTINGS_ENCRYPTION_KEY" | grep -q '[[:space:]]'; then
  fail "SETTINGS_ENCRYPTION_KEY must be >= 32 characters without spaces"
fi
ok "secret format checked"

# Names embedded in URLs / SQL.
for v in MINIO_ROOT_USER MINIO_APP_ACCESS_KEY MINIO_OFFSITE_ACCESS_KEY; do
  eval "val=\${$v:-}"
  [ -z "$val" ] && continue
  printf '%s' "$val" | grep -Eq '^[A-Za-z0-9_.-]{3,64}$' || fail "$v must match [A-Za-z0-9_.-]{3,64}"
done
for v in POSTGRES_USER APP_DB_USER KC_DB_USER POSTGRES_DB KC_DB_NAME; do
  eval "val=\${$v:-}"
  printf '%s' "$val" | grep -Eq '^[a-z_][a-z0-9_]{0,62}$' || fail "$v must be a lower-case SQL identifier"
done
[ "${APP_DB_USER:-}" != "${POSTGRES_USER:-}" ] || fail "APP_DB_USER must differ from the superuser POSTGRES_USER"
[ "${KC_DB_USER:-}" != "${POSTGRES_USER:-}" ] || fail "KC_DB_USER must differ from the superuser POSTGRES_USER"
if [ -n "${MINIO_APP_ACCESS_KEY:-}" ] && [ -z "${MINIO_APP_SECRET_KEY:-}" ]; then fail "MINIO_APP_SECRET_KEY is empty"; fi
if [ -n "${MINIO_OFFSITE_ACCESS_KEY:-}" ] && [ -z "${MINIO_OFFSITE_SECRET_KEY:-}" ]; then fail "MINIO_OFFSITE_SECRET_KEY is empty"; fi
[ -n "${MINIO_APP_ACCESS_KEY:-}" ] || warnp "MINIO_APP_ACCESS_KEY empty: the API uses the MinIO root credentials (INF-S5)"
for b in "$MINIO_BUCKET_PHOTOS" "$MINIO_BUCKET_BACKUPS"; do
  printf '%s' "$b" | grep -Eq '^[a-z0-9][a-z0-9.-]{1,61}[a-z0-9]$' || fail "invalid bucket name: $b"
done

# ── one server address drives every URL (INF-D4) ────────────────────────────
host=$SERVER_HOST
if is_ipv4 "$host"; then hostkind=ip
elif is_hostname "$host"; then hostkind=dns
else fail "SERVER_HOST must be a DNS name or IPv4 address (no scheme/port): $host"; hostkind=bad
fi
if [ "$host" = localhost ] && [ "${PREFLIGHT_ALLOW_LOCALHOST:-0}" != 1 ]; then
  fail "SERVER_HOST=localhost: tablets and other PCs cannot reach the server by that name"
fi
[ "$PUBLIC_URL" = "https://$host" ] || fail "PUBLIC_URL=$PUBLIC_URL, expected https://$host"
[ "$KEYCLOAK_PUBLIC_URL" = "https://$host/auth" ] || fail "KEYCLOAK_PUBLIC_URL=$KEYCLOAK_PUBLIC_URL, expected https://$host/auth (token issuer)"
[ "$MINIO_PUBLIC_ENDPOINT" = "$host:9443" ] || fail "MINIO_PUBLIC_ENDPOINT=$MINIO_PUBLIC_ENDPOINT, expected $host:9443"
[ "${MINIO_PUBLIC_USE_SSL:-true}" = true ] || fail "MINIO_PUBLIC_USE_SSL must be true (photos are served over TLS on :9443)"
ok "URLs derive from SERVER_HOST=$host"

# ── realms ──────────────────────────────────────────────────────────────────
realms=$(printf '%s' "$KEYCLOAK_REALMS" | tr ',' ' ')
for r in $realms; do
  printf '%s' "$r" | grep -Eq '^[A-Za-z0-9][A-Za-z0-9_-]{0,62}$' || fail "invalid realm name: $r"
  [ "$r" = master ] && fail "KEYCLOAK_REALMS must not contain the master realm"
  [ -f "$INFRA_DIR/keycloak/realms/$r.json" ] || warnp "no realm file keycloak/realms/$r.json (fine if the realm was created in the console)"
done
old_ifs=$IFS; IFS=','
for item in $WEB_KEYCLOAK_REALMS; do
  id=$(printf '%s' "${item%%:*}" | tr -d ' ')
  case " $realms " in *" $id "*) ;; *) fail "WEB_KEYCLOAK_REALMS lists '$id', which is not in KEYCLOAK_REALMS" ;; esac
done
IFS=$old_ifs
[ "${KEYCLOAK_DEV_REDIRECTS:-false}" = true ] && warnp "KEYCLOAK_DEV_REDIRECTS=true allows http://localhost:5173 redirects — dev only"

# ── other values ────────────────────────────────────────────────────────────
case "${BACKUP_HOUR:-2}" in [0-9]|1[0-9]|2[0-3]) ;; *) fail "BACKUP_HOUR must be 0-23" ;; esac
case "${DB_MAX_CONNS:-20}" in ''|*[!0-9]*) fail "DB_MAX_CONNS must be a number" ;; esac
for tz in "${TZ:-}" "${BACKUP_TZ:-}"; do
  [ -z "$tz" ] && continue
  [ -e "/usr/share/zoneinfo/$tz" ] || [ ! -d /usr/share/zoneinfo ] || fail "unknown time zone: $tz"
done
for c in ${ADMIN_ALLOW_CIDRS:-}; do
  case $c in *[!0-9a-fA-F:./]*) fail "invalid ADMIN_ALLOW_CIDRS entry: $c" ;; esac
done
case "${COMPOSE_FILE:-}" in
  *docker-compose.app.yml*docker-compose.ops.yml*) ok "COMPOSE_FILE covers app + ops" ;;
  *) warnp "COMPOSE_FILE does not include docker-compose.app.yml and docker-compose.ops.yml" ;;
esac
[ -n "${ALERT_EMAIL_TO:-}" ] || warnp "ALERT_EMAIL_TO is empty: monitoring alerts are not e-mailed"
if [ -n "${ALERT_EMAIL_TO:-}" ] && [ -z "${ALERT_SMTP_HOST:-}" ]; then fail "ALERT_EMAIL_TO is set but ALERT_SMTP_HOST is empty"; fi

# ── TLS certificate (INF-D3/N5) ─────────────────────────────────────────────
certs="$INFRA_DIR/nginx/certs"
if ! command -v openssl >/dev/null; then
  warnp "openssl not installed: certificate not checked"
elif [ ! -f "$certs/cert.pem" ] || [ ! -f "$certs/key.pem" ]; then
  fail "missing nginx/certs/cert.pem or key.pem — run: make certs"
else
  if [ "$hostkind" = ip ]; then
    openssl x509 -in "$certs/cert.pem" -noout -checkip "$host" 2>/dev/null | grep -q 'does match' \
      || fail "certificate SAN does not contain IP $host — run: make certs"
  elif [ "$hostkind" = dns ]; then
    openssl x509 -in "$certs/cert.pem" -noout -checkhost "$host" 2>/dev/null | grep -q 'does match' \
      || fail "certificate SAN does not contain $host — run: make certs"
  fi
  cpub=$(openssl x509 -in "$certs/cert.pem" -noout -pubkey 2>/dev/null | openssl sha256)
  kpub=$(openssl pkey -in "$certs/key.pem" -pubout 2>/dev/null | openssl sha256 || true)
  [ "$cpub" = "$kpub" ] || fail "nginx/certs/key.pem does not belong to cert.pem"
  if ! openssl x509 -in "$certs/cert.pem" -noout -checkend 0 >/dev/null; then
    fail "certificate has expired — run: make certs-renew"
  elif ! openssl x509 -in "$certs/cert.pem" -noout -checkend $((21 * 86400)) >/dev/null; then
    warnp "certificate expires within 21 days — run: make certs-renew"
  fi
  if [ ! -f "$certs/ca.pem" ]; then
    fail "missing nginx/certs/ca.pem (CA used by the monitoring probes; for a public cert put its root CA there)"
  elif ! openssl verify -CAfile "$certs/ca.pem" -untrusted "$certs/cert.pem" "$certs/cert.pem" >/dev/null 2>&1; then
    fail "nginx/certs/cert.pem is not signed by nginx/certs/ca.pem"
  fi
  ok "certificate checked ($(openssl x509 -in "$certs/cert.pem" -noout -enddate))"
fi

# ── off-site backup target (INF-B1) ─────────────────────────────────────────
case "$OFFSITE_BACKUP_REPO" in
  /*) ;;
  *) fail "OFFSITE_BACKUP_REPO must be an absolute host path (a mounted second disk or NAS)" ;;
esac
if [ -d "$OFFSITE_BACKUP_REPO" ] && command -v df >/dev/null; then
  repo_fs=$(df -P "$OFFSITE_BACKUP_REPO" 2>/dev/null | awk 'NR==2 {print $1}')
  docker_root=$(docker info --format '{{.DockerRootDir}}' 2>/dev/null || echo /var/lib/docker)
  [ -d "$docker_root" ] || docker_root=/
  data_fs=$(df -P "$docker_root" 2>/dev/null | awk 'NR==2 {print $1}')
  if [ -n "$repo_fs" ] && [ "$repo_fs" = "$data_fs" ]; then
    warnp "OFFSITE_BACKUP_REPO ($OFFSITE_BACKUP_REPO) is on the same filesystem as the Docker data — not an off-site copy"
  fi
elif [ ! -d "$OFFSITE_BACKUP_REPO" ]; then
  warnp "OFFSITE_BACKUP_REPO $OFFSITE_BACKUP_REPO does not exist (mount the backup disk/NAS there)"
fi

# ── tools used by make keycloak-config / new-realm ──────────────────────────
command -v jq >/dev/null || fail "jq is not installed (make up / deploy run keycloak-config, which needs it): apt install jq"

echo
if [ "$fails" -gt 0 ]; then
  echo "preflight: $fails problem(s), $warns warning(s) — fix the FAIL lines above."
  exit 1
fi
echo "preflight: OK ($warns warning(s))"
