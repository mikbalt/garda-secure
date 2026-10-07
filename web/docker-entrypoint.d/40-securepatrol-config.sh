#!/bin/sh
# Writes the runtime configuration of the SecurePatrol web app to /config.js.
# The official nginx image runs every executable script in /docker-entrypoint.d/
# before starting nginx, so the same image works for every deployment.
#
# Environment:
#   API_URL             API base incl. /api/v1        (default /api/v1)
#   KEYCLOAK_URL        Keycloak base URL              (default /auth)
#   KEYCLOAK_CLIENT_ID  public web client              (default securepatrol-web)
#   KEYCLOAK_REALMS     comma list "realm:Label,..."   (default securepatrol)
set -eu

OUT="${SP_CONFIG_PATH:-/usr/share/nginx/html/config.js}"

# JSON-escape a value (backslash, double quote, and "<" so it is safe inside HTML).
esc() {
  printf '%s' "$1" | sed -e 's/\\/\\\\/g' -e 's/"/\\"/g' -e 's/</\\u003c/g'
}

trim() {
  printf '%s' "$1" | sed -e 's/^[[:space:]]*//' -e 's/[[:space:]]*$//'
}

realms=""
old_ifs=$IFS
IFS=','
for item in ${KEYCLOAK_REALMS:-securepatrol}; do
  item=$(trim "$item")
  [ -z "$item" ] && continue
  id=$(trim "${item%%:*}")
  case "$item" in
    *:*) label=$(trim "${item#*:}") ;;
    *) label=$id ;;
  esac
  [ -z "$label" ] && label=$id
  entry="{\"id\":\"$(esc "$id")\",\"label\":\"$(esc "$label")\"}"
  if [ -z "$realms" ]; then realms=$entry; else realms="$realms,$entry"; fi
done
IFS=$old_ifs

cat > "$OUT" <<EOF
// Generated at container start by /docker-entrypoint.d/40-securepatrol-config.sh
window.__SP_CONFIG__ = {
  "apiUrl": "$(esc "${API_URL:-/api/v1}")",
  "keycloakUrl": "$(esc "${KEYCLOAK_URL:-/auth}")",
  "keycloakClientId": "$(esc "${KEYCLOAK_CLIENT_ID:-securepatrol-web}")",
  "realms": [$realms]
};
EOF

echo "securepatrol: wrote $OUT"
