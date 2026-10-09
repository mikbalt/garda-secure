#!/bin/sh
# Alertmanager entrypoint: render the config from the environment (values
# never appear on a command line), then start Alertmanager.
set -eu

out=/alertmanager/alertmanager.yml
umask 077
if [ -n "${ALERT_EMAIL_TO:-}" ] && [ -n "${ALERT_SMTP_HOST:-}" ]; then
  case "${ALERT_SMTP_REQUIRE_TLS:-true}" in
    true|false) ;;
    *) echo "ALERT_SMTP_REQUIRE_TLS must be true or false" >&2; exit 1 ;;
  esac
  awk -f /etc/sp/render.awk /etc/alertmanager/alertmanager.yml.tmpl > "$out"
  echo "alertmanager: e-mail alerts to ${ALERT_EMAIL_TO} via ${ALERT_SMTP_HOST}"
else
  cp /etc/alertmanager/alertmanager-noemail.yml "$out"
  echo "alertmanager: ALERT_EMAIL_TO/ALERT_SMTP_HOST not set — alerts are NOT e-mailed" >&2
fi

exec /bin/alertmanager \
  --config.file="$out" \
  --storage.path=/alertmanager \
  --web.listen-address=:9093
