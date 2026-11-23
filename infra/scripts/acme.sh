#!/bin/sh
# Public (Let's Encrypt / ACME) certificate for nginx, via certbot's webroot
# challenge on port 80 (docs/runbook.md §6). Idempotent; safe to run from cron.
#   scripts/acme.sh issue      first certificate for ACME_DOMAIN (needs ACME_EMAIL)
#   scripts/acme.sh renew      renew when due (<30 days left); does nothing otherwise
#   scripts/acme.sh renew -f   force a renewal now (renewal test)
# After every issue/renewal the new chain is installed as nginx/certs/cert.pem +
# key.pem (the same files `make certs` writes; key keeps mode 0600), checked with
# the preflight rules and loaded with `nginx -t && nginx -s reload`. A failed
# check leaves the previous certificate in place.
# ca.pem (the monitoring probes verify against it) is NOT replaced unless
# ACME_ROOT_PEM points to the issuing root (e.g. ISRG Root X1, isrgrootx1.pem).
# Settings (environment or infra/.env):
#   ACME_DOMAIN     default SERVER_HOST — must resolve publicly to this server, port 80 open
#   ACME_EMAIL      expiry notices (issue only)
#   ACME_SERVER     default Let's Encrypt production. Staging for a dry run:
#                   https://acme-staging-v02.api.letsencrypt.org/directory
#   ACME_CA_BUNDLE  PEM that certbot trusts for ACME_SERVER (private/test ACME servers only)
#   ACME_ROOT_PEM   issuing root to install as nginx/certs/ca.pem
#   ACME_DOCKER_ARGS extra `docker run` arguments (e.g. --network for a test ACME server)
#   CERTBOT_IMAGE   default certbot/certbot
# State lives in infra/acme (webroot, certbot config + keys; mode 0700) — back it up
# with the rest of infra/ (the off-site backup already includes .env and nginx/certs).
set -eu
. "$(dirname -- "$0")/lib.sh"
[ -f "$INFRA_DIR/.env" ] && load_env "$INFRA_DIR/.env"

mode=${1:-}
force=""
[ "${2:-}" = "-f" ] && force="--force-renewal"
case "$mode" in issue|renew) ;; *) die "usage: $0 issue|renew [-f]" ;; esac
domain=${ACME_DOMAIN:-${SERVER_HOST:-}}
is_hostname "$domain" && ! is_ipv4 "$domain" && [ "$domain" != localhost ] \
  || die "ACME_DOMAIN must be a public DNS name (Let's Encrypt does not issue for $domain)"
[ "$mode" = renew ] || [ -n "${ACME_EMAIL:-}" ] || die "set ACME_EMAIL for the first issue"
command -v docker >/dev/null || die "docker is required"

acme="$INFRA_DIR/acme"
certs="$INFRA_DIR/nginx/certs"
umask 077
mkdir -p "$acme/webroot" "$acme/config"
chmod 755 "$acme" "$acme/webroot"        # nginx (non-root worker) reads the challenge files
server=${ACME_SERVER:-https://acme-v02.api.letsencrypt.org/directory}

mounts="-v $acme/config:/etc/letsencrypt -v $acme/webroot:/var/www/certbot"
envs=""
if [ -n "${ACME_CA_BUNDLE:-}" ]; then
  [ -f "$ACME_CA_BUNDLE" ] || die "ACME_CA_BUNDLE not found: $ACME_CA_BUNDLE"
  mounts="$mounts -v $ACME_CA_BUNDLE:/acme-ca.pem:ro"
  envs="-e REQUESTS_CA_BUNDLE=/acme-ca.pem"
fi
# shellcheck disable=SC2086
certbot() { docker run --rm $mounts $envs ${ACME_DOCKER_ARGS:-} "${CERTBOT_IMAGE:-certbot/certbot}" "$@"; }

if [ "$mode" = issue ]; then
  info "requesting a certificate for $domain from $server"
  certbot certonly --webroot -w /var/www/certbot -d "$domain" --server "$server" \
    --email "$ACME_EMAIL" --agree-tos --no-eff-email --non-interactive --keep-until-expiring
else
  info "certbot renew ($server)"
  certbot renew --non-interactive --no-random-sleep-on-renew $force
fi

live="$acme/config/live/$domain"
[ -f "$live/fullchain.pem" ] || die "no certificate in $live — run: $0 issue"
# Same checks as scripts/preflight.sh, before anything is replaced.
openssl x509 -in "$live/fullchain.pem" -noout -checkhost "$domain" | grep -q 'does match' || die "certificate does not cover $domain"
[ "$(openssl x509 -in "$live/fullchain.pem" -noout -pubkey | openssl sha256)" = "$(openssl pkey -in "$live/privkey.pem" -pubout | openssl sha256)" ] \
  || die "certificate and key do not belong together"
openssl x509 -in "$live/fullchain.pem" -noout -checkend $((7 * 86400)) >/dev/null || die "certificate expires within 7 days"

if cmp -s "$live/fullchain.pem" "$certs/cert.pem"; then
  info "nginx/certs/cert.pem is already current ($(openssl x509 -in "$certs/cert.pem" -noout -enddate))"
  exit 0
fi
if [ -n "${ACME_ROOT_PEM:-}" ]; then
  [ -f "$ACME_ROOT_PEM" ] || die "ACME_ROOT_PEM not found: $ACME_ROOT_PEM"
  cp "$ACME_ROOT_PEM" "$certs/ca.pem.new" && mv "$certs/ca.pem.new" "$certs/ca.pem"
fi
[ ! -f "$certs/ca.pem" ] || openssl verify -CAfile "$certs/ca.pem" -untrusted "$live/fullchain.pem" "$live/fullchain.pem" >/dev/null 2>&1 \
  || die "chain does not verify against nginx/certs/ca.pem — pass ACME_ROOT_PEM=<issuing root>"
mkdir -p "$certs"
cp "$live/fullchain.pem" "$certs/cert.pem.new"; cp "$live/privkey.pem" "$certs/key.pem.new"
chmod 600 "$certs/key.pem.new"
mv "$certs/cert.pem.new" "$certs/cert.pem"; mv "$certs/key.pem.new" "$certs/key.pem"
info "installed $(openssl x509 -in "$certs/cert.pem" -noout -subject -enddate | tr '\n' ' ')"
if compose ps --status running --services 2>/dev/null | grep -qx nginx; then
  compose exec -T nginx nginx -t && compose exec -T nginx nginx -s reload
  info "nginx reloaded"
else
  warn "nginx is not running — start it to serve the new certificate"
fi
