#!/bin/sh
# Generate the realm import file for a new client company (D-005).
#   ./new-realm.sh securepatrol-client3 "PT Contoh Tiga" [--dev]
# Redirect URIs of the web client come from PUBLIC_URL in infra/.env
# (https://<SERVER_HOST>/*). --dev also allows the Vite dev server
# (http://localhost:5173/*) — never for a production realm.
# Values are inserted with jq, so names with quotes/slashes stay valid JSON.
#
# Then: add the realm to KEYCLOAK_REALMS / WEB_KEYCLOAK_REALMS in infra/.env
# and run `make up` (Keycloak imports files from realms/ only for realms that
# do not exist yet; `make keycloak-config` then enforces OTP and redirects).
set -eu
. "$(dirname -- "$0")/../scripts/lib.sh"

realm=${1:-}
display=${2:-$realm}
dev=false
[ "${3:-}" = "--dev" ] && dev=true
[ -n "$realm" ] || die "usage: $0 <realm-name> [\"Display name\"] [--dev]"
printf '%s' "$realm" | grep -Eq '^[a-z0-9][a-z0-9-]{1,62}$' || die "realm name must match [a-z0-9][a-z0-9-]+"
[ "$realm" != master ] || die "master is reserved"
command -v jq >/dev/null || die "jq is required (apt install jq)"

[ -f "$INFRA_DIR/.env" ] && load_env "$INFRA_DIR/.env"
public_url=${PUBLIC_URL:-}
case "$public_url" in
  https://*) ;;
  *) die "PUBLIC_URL must be set to https://<SERVER_HOST> in infra/.env (got '${public_url}')" ;;
esac

dir="$INFRA_DIR/keycloak"
out="$dir/realms/$realm.json"
[ -e "$out" ] && die "$out already exists"

jq --arg realm "$realm" --arg display "$display" --arg url "$public_url" --argjson dev "$dev" '
  .realm = $realm
  | .displayName = $display
  | .clients |= map(
      if .clientId == "securepatrol-web" then
        .redirectUris = ([$url + "/*"] + (if $dev then ["http://localhost:5173/*"] else [] end))
        | .webOrigins = ["+"]
      else . end)
' "$dir/realm-template.json" > "$out.tmp"
mv "$out.tmp" "$out"
echo "wrote $out (redirect URIs: $public_url/*$([ "$dev" = true ] && echo ', http://localhost:5173/*'))"
