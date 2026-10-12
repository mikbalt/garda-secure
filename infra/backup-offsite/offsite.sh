#!/bin/sh
# Off-site backup run (INF-B1/B2/B5/B6). Called by crond inside the
# backup-offsite container:
#   offsite.sh backup        mirror buckets -> staging, restic backup, forget/prune
#   offsite.sh check [5%]    verify repository integrity (+ read a data subset)
#
# Writes Prometheus textfile metrics to /status/securepatrol_offsite.prom
# (read by node-exporter) so a failed or missing run raises an alert.
set -eu

STAGING=/cache/staging
STATUS_DIR=/status
METRICS="$STATUS_DIR/securepatrol_offsite.prom"
LOCK=/tmp/offsite.lock

# crond may not pass the container environment to jobs: entrypoint.sh saves it.
# shellcheck disable=SC1091
[ -f /tmp/offsite.env ] && . /tmp/offsite.env

log() { echo "$(date -u '+%Y-%m-%dT%H:%M:%SZ') offsite: $*"; }

# Keep the existing value of a metric (so a failed run keeps the last success).
metric_value() {
  [ -f "$METRICS" ] || { echo 0; return; }
  v=$(awk -v m="$1" '$1 == m { print $2 }' "$METRICS" | tail -n 1)
  echo "${v:-0}"
}

write_metrics() { # $1 job, $2 result (1 ok / 0 failed)
  now=$(date +%s)
  last_backup_ok=$(metric_value sp_offsite_backup_last_success_timestamp_seconds)
  last_check_ok=$(metric_value sp_offsite_check_last_success_timestamp_seconds)
  backup_result=$(metric_value sp_offsite_backup_last_result)
  check_result=$(metric_value sp_offsite_check_last_result)
  if [ "$1" = backup ]; then
    backup_result=$2
    [ "$2" = 1 ] && last_backup_ok=$now
  else
    check_result=$2
    [ "$2" = 1 ] && last_check_ok=$now
  fi
  mkdir -p "$STATUS_DIR"
  tmp="$METRICS.$$"
  cat > "$tmp" <<EOF
# HELP sp_offsite_backup_last_success_timestamp_seconds Unix time of the last successful off-site backup.
# TYPE sp_offsite_backup_last_success_timestamp_seconds gauge
sp_offsite_backup_last_success_timestamp_seconds $last_backup_ok
# HELP sp_offsite_backup_last_result 1 if the last off-site backup run succeeded, else 0.
# TYPE sp_offsite_backup_last_result gauge
sp_offsite_backup_last_result $backup_result
# HELP sp_offsite_check_last_success_timestamp_seconds Unix time of the last successful restic check.
# TYPE sp_offsite_check_last_success_timestamp_seconds gauge
sp_offsite_check_last_success_timestamp_seconds $last_check_ok
# HELP sp_offsite_check_last_result 1 if the last restic check succeeded, else 0.
# TYPE sp_offsite_check_last_result gauge
sp_offsite_check_last_result $check_result
EOF
  chmod 644 "$tmp"   # node-exporter (nobody) reads it
  mv "$tmp" "$METRICS"
}

ensure_repo() {
  if ! restic cat config >/dev/null 2>&1; then
    log "initialising restic repository at $RESTIC_REPOSITORY"
    restic init
  fi
}

do_backup() {
  ensure_repo
  mkdir -p "$STAGING/config"
  # 1. Buckets -> staging (incremental; --remove keeps the mirror exact).
  for b in "$MINIO_BUCKET_BACKUPS" "$MINIO_BUCKET_PHOTOS"; do
    log "mirroring bucket $b"
    mc mirror --quiet --overwrite --remove "sp/$b" "$STAGING/$b"
  done
  # 2. Config bundle: .env (contains SETTINGS_ENCRYPTION_KEY), certs + CA key,
  #    Keycloak realm files.
  rm -rf "$STAGING/config"
  mkdir -p "$STAGING/config"
  cp /config/infra.env "$STAGING/config/infra.env"
  cp -R /config/certs "$STAGING/config/certs"
  cp -R /config/realms "$STAGING/config/realms"
  chmod -R go-rwx "$STAGING/config"
  # 3. Snapshot + retention.
  log "restic backup"
  restic backup --host "${SERVER_HOST:-securepatrol}" --tag securepatrol "$STAGING"
  log "restic forget/prune (30 daily, 12 monthly)"
  restic forget --host "${SERVER_HOST:-securepatrol}" --tag securepatrol \
    --keep-daily 30 --keep-monthly 12 --prune
}

do_check() {
  ensure_repo
  if [ -n "${1:-}" ]; then
    restic check --read-data-subset="$1"
  else
    restic check
  fi
}

job=${1:-backup}
exec 9>"$LOCK"
if ! flock -n 9; then
  log "another run is in progress; skipping $job"
  exit 0
fi

case "$job" in
  backup)
    if do_backup; then write_metrics backup 1; log "backup OK"; else write_metrics backup 0; log "backup FAILED"; exit 1; fi
    ;;
  check)
    if do_check "${2:-}"; then write_metrics check 1; log "check OK"; else write_metrics check 0; log "check FAILED"; exit 1; fi
    ;;
  *) echo "usage: offsite.sh backup|check [subset]" >&2; exit 2 ;;
esac
