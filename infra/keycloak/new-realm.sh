#!/bin/sh
# Generate the realm import file for a new client company (D-005).
#   ./new-realm.sh securepatrol-client3 "PT Contoh Tiga"
# Then add the realm to KEYCLOAK_REALMS / WEB_KEYCLOAK_REALMS in infra/.env.
# Keycloak imports files from realms/ only when the realm does not exist yet;
# for a running stack restart keycloak or import it in the admin console.
set -eu
realm="$1"
display="${2:-$1}"
dir="$(cd "$(dirname "$0")" && pwd)"
out="$dir/realms/$realm.json"
[ -e "$out" ] && { echo "$out already exists" >&2; exit 1; }
sed -e "s/__REALM__/$realm/g" -e "s/__DISPLAY__/$display/g" "$dir/realm-template.json" > "$out"
echo "wrote $out"
