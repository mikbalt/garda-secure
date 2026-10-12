#!/bin/sh
# backup-offsite container entrypoint.
#   (no args)      install the cron schedule and run crond in the foreground
#   run            run one backup now          (make offsite-backup)
#   check [subset] restic check, optional --read-data-subset (e.g. 10%)
#   snapshots      list snapshots
#   restore <snapshot> <target>   restore into a directory (see runbook §5)
#   restic ...     any restic command against the repository
set -eu

case "${1:-cron}" in
  cron)
    schedule="${OFFSITE_SCHEDULE:-30 3 * * *}"
    case "$schedule" in
      *[!0-9\ \*,/-]*) echo "invalid OFFSITE_SCHEDULE: $schedule" >&2; exit 1 ;;
    esac
    # Jobs started by crond get this environment (restic/mc settings).
    (umask 077 && export -p > /tmp/offsite.env)
    mkdir -p /etc/crontabs
    # Daily backup; restic check (5 % of the data) on the 1st of each month.
    {
      echo "$schedule /usr/local/bin/offsite.sh backup >> /proc/1/fd/1 2>&1"
      echo "15 5 1 * * /usr/local/bin/offsite.sh check 5\\% >> /proc/1/fd/1 2>&1"
    } > /etc/crontabs/root
    echo "backup-offsite: schedule '$schedule' (TZ=${TZ:-UTC}), repository ${RESTIC_REPOSITORY:-unset}"
    exec crond -f -l 6 -c /etc/crontabs
    ;;
  run) exec /usr/local/bin/offsite.sh backup ;;
  check) exec /usr/local/bin/offsite.sh check "${2:-}" ;;
  snapshots) exec restic snapshots ;;
  restore)
    [ $# -ge 3 ] || { echo "usage: restore <snapshot|latest> <target-dir> [--include path]" >&2; exit 2; }
    snap=$2; target=$3; shift 3
    exec restic restore "$snap" --target "$target" "$@"
    ;;
  restic) shift; exec restic "$@" ;;
  *) exec "$@" ;;
esac
