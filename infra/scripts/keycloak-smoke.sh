#!/bin/sh
# End-to-end Keycloak smoke test: run by the CI stack test and after every
# Keycloak upgrade (docs/runbook.md §2). It creates temporary users in the
# realm and deletes them again on exit; nothing else is changed.
#   scripts/keycloak-smoke.sh [realm]        default: first of KEYCLOAK_REALMS
# Checks, the way browsers, tablets and the API see Keycloak:
#   1. discovery: issuer = KEYCLOAK_PUBLIC_URL/realms/<realm> (hostname v2)
#   2. the API's admin client (secrets/keycloak-admin-clients) gets a token on
#      the internal URL whose issuer is the public URL, and manages users
#      through the Admin REST API exactly like the API does (D-041)
#   3. tablet: device account (names + @device.invalid e-mail) password grant
#      with offline_access -> access token typ Bearer, azp securepatrol-tablet,
#      sub, roles = device only; offline refresh token; refresh works;
#      GET /api/v1/me accepts it
#   4. web: client-readonly user limited to one site, authorization code +
#      PKCE through the login page -> typ, azp securepatrol-web, roles, claim
#      sites; GET /api/v1/me shows the site restriction (D-042)
#   5. staff (supervisor): after the password the login asks to set up OTP
# Needs the stack running (make up), keycloak-config done, curl/jq/openssl.
#   KEYCLOAK_SMOKE_INTERNAL_URL  Keycloak without nginx (default
#                                http://127.0.0.1:8081/auth, the host port)
set -eu
. "$(dirname -- "$0")/lib.sh"

load_env "$INFRA_DIR/.env"
for t in curl jq openssl; do command -v "$t" >/dev/null || die "$t is required"; done

realm=${1:-${KEYCLOAK_REALMS%%,*}}
base=$PUBLIC_URL
issuer="$KEYCLOAK_PUBLIC_URL/realms/$realm"
internal=${KEYCLOAK_SMOKE_INTERNAL_URL:-http://127.0.0.1:8081/auth}
admin_client=${KEYCLOAK_ADMIN_CLIENT_ID:-securepatrol-admin-api}
ca="$INFRA_DIR/nginx/certs/ca.pem"
tmp=$(mktemp -d)
created=""
tag=$(openssl rand -hex 4)

ok()   { echo "ok:    $*"; }
fail() { echo "FAIL:  $*" >&2; exit 1; }

# pub ARGS... — request through nginx (TLS verified against the private CA)
pub() {
  if [ -f "$ca" ]; then curl -sS --cacert "$ca" "$@"; else curl -sS "$@"; fi
}

# claims JWT — the decoded payload
claims() {
  c_p=$(printf '%s' "$1" | cut -d. -f2 | tr '_-' '/+')
  case $((${#c_p} % 4)) in 2) c_p="$c_p==" ;; 3) c_p="$c_p=" ;; esac
  printf '%s' "$c_p" | openssl base64 -d -A
}

# expect DESCRIPTION JQ-FILTER JSON — $iss, $site, $realm are available
expect() {
  if printf '%s' "$3" | jq -e --arg iss "$issuer" --arg site "${site:-}" --arg realm "$realm" "$2" >/dev/null; then
    ok "$1"
  else
    printf '%s\n' "$3" | jq -S . >&2 2>/dev/null || printf '%s\n' "$3" >&2
    fail "$1 ($2)"
  fi
}

# adm METHOD PATH [BODY-FILE] — Admin REST call of the API's admin client
adm() {
  if [ $# -ge 3 ]; then
    curl -sS -f -X "$1" -H "@$tmp/admin-auth" -H 'Content-Type: application/json' \
      --data-binary "@$3" "$internal/admin/realms/$realm$2"
  else
    curl -sS -f -X "$1" -H "@$tmp/admin-auth" "$internal/admin/realms/$realm$2"
  fi
}

cleanup() {
  for u in $created; do
    adm DELETE "/users/$u" >/dev/null 2>&1 || echo "WARN:  could not delete temporary user $u" >&2
  done
  rm -rf "$tmp"
}
trap cleanup EXIT

# create_user USERNAME ROLE PASSWORD JSON — prints the new user's id
create_user() {
  printf '%s' "$4" > "$tmp/user.json"
  adm POST /users "$tmp/user.json" >/dev/null
  cu_id=$(adm GET "/users?username=$1&exact=true" | jq -r '.[0].id // empty')
  [ -n "$cu_id" ] || fail "user $1 was not created"
  created="$created $cu_id"
  adm GET "/roles/$2" | jq -c '[{id, name}]' > "$tmp/roles.json"
  adm POST "/users/$cu_id/role-mappings/realm" "$tmp/roles.json" >/dev/null
  jq -cn --arg pw "$3" '{type: "password", value: $pw, temporary: false}' > "$tmp/pw.json"
  adm PUT "/users/$cu_id/reset-password" "$tmp/pw.json" >/dev/null
  printf '%s' "$cu_id"
}

# ── 1. discovery ────────────────────────────────────────────────────────────
disc=$(pub -f "$issuer/.well-known/openid-configuration")
expect "discovery issuer and endpoints on $KEYCLOAK_PUBLIC_URL" \
  '.issuer == $iss and (.token_endpoint | startswith($iss + "/")) and (.authorization_endpoint | startswith($iss + "/")) and (.jwks_uri | startswith($iss + "/"))' "$disc"
token_ep=$(printf '%s' "$disc" | jq -r .token_endpoint)
auth_ep=$(printf '%s' "$disc" | jq -r .authorization_endpoint)

# ── 2. the API's admin client ───────────────────────────────────────────────
secrets_file="$INFRA_DIR/secrets/keycloak-admin-clients"
[ -f "$secrets_file" ] || fail "missing $secrets_file — run make keycloak-config"
umask 077
sed -n "s/^$realm=//p" "$secrets_file" | head -n 1 | tr -d '\n' > "$tmp/secret"
[ -s "$tmp/secret" ] || fail "no secret for $realm in secrets/keycloak-admin-clients — run make keycloak-config"
atok=$(curl -sS -f --data-urlencode grant_type=client_credentials --data-urlencode "client_id=$admin_client" \
  --data-urlencode "client_secret@$tmp/secret" "$internal/realms/$realm/protocol/openid-connect/token" | jq -er .access_token) \
  || fail "$admin_client: client-credentials grant on $internal failed"
printf 'Authorization: Bearer %s\n' "$atok" > "$tmp/admin-auth"
expect "$admin_client token (internal URL) carries the public issuer" \
  '.typ == "Bearer" and .azp == "'"$admin_client"'" and .iss == $iss' "$(claims "$atok")"
adm GET "/users?max=1" >/dev/null || fail "$admin_client cannot list users"
adm GET "/roles/device/users?max=1" >/dev/null || fail "$admin_client cannot list role members"
ok "$admin_client: Admin REST user + role queries"

# ── 3. tablet: device account, password grant + offline token ─────────────
dev_user="device-smoke-$tag"
dev_pw="Sm$(openssl rand -hex 12)9"
create_user "$dev_user" device "$dev_pw" "$(jq -cn --arg u "$dev_user" \
  '{username: $u, firstName: "Smoke", lastName: "Tablet", email: ($u + "@device.invalid"),
    emailVerified: true, enabled: true, requiredActions: []}')" >/dev/null
ok "device account $dev_user created like the API does (D-041)"

printf '%s' "$dev_pw" > "$tmp/devpw"
resp=$(pub -X POST --data-urlencode grant_type=password --data-urlencode client_id=securepatrol-tablet \
  --data-urlencode "username=$dev_user" --data-urlencode "password@$tmp/devpw" \
  --data-urlencode "scope=openid offline_access" "$token_ep")
at=$(printf '%s' "$resp" | jq -r '.access_token // empty')
rt=$(printf '%s' "$resp" | jq -r '.refresh_token // empty')
[ -n "$at" ] || fail "tablet password grant: $(printf '%s' "$resp" | jq -c '{error, error_description}' 2>/dev/null)"
expect "tablet access token: typ, azp, sub, issuer, roles = device only" \
  '.typ == "Bearer" and .azp == "securepatrol-tablet" and .iss == $iss and (.sub | type == "string" and length > 0)
   and (.roles | index("device")) and ((.roles - ["device", "offline_access"]) == [])' "$(claims "$at")"
expect "tablet refresh token is an offline token" '.typ == "Offline"' "$(claims "$rt")"
printf '%s' "$rt" > "$tmp/rt"
at2=$(pub -f -X POST --data-urlencode grant_type=refresh_token --data-urlencode client_id=securepatrol-tablet \
  --data-urlencode "refresh_token@$tmp/rt" "$token_ep" | jq -er .access_token) || fail "tablet refresh with the offline token"
ok "tablet offline refresh"
printf 'Authorization: Bearer %s\n' "$at2" > "$tmp/auth"
expect "API accepts the tablet token (GET /api/v1/me)" \
  '.client_id == "securepatrol-tablet" and .roles == ["device"] and .realm == $realm' \
  "$(pub -f -H "@$tmp/auth" "$base/api/v1/me")"

# ── 4./5. web: authorization code + PKCE through the login page ─────────────
on_keycloak() { case $1 in "$issuer"/*) return 0 ;; esac; return 1; }

# web_login USERNAME PASSWORD — posts the login form, follows Keycloak's own
# redirects (e.g. to a required action), leaves the last page in $tmp/page
# and prints the redirect URL back to the app (empty when Keycloak shows a
# page, e.g. OTP setup).
web_login() {
  jar="$tmp/cookies-$1"
  verifier=$(openssl rand -hex 32)
  challenge=$(printf '%s' "$verifier" | openssl dgst -sha256 -binary | openssl base64 -A | tr '+/' '-_' | tr -d '=')
  printf '%s' "$verifier" > "$tmp/verifier"
  pub -f -c "$jar" -b "$jar" -o "$tmp/login.html" -G "$auth_ep" \
    --data-urlencode client_id=securepatrol-web --data-urlencode response_type=code \
    --data-urlencode scope=openid --data-urlencode "redirect_uri=$base/" \
    --data-urlencode "code_challenge=$challenge" --data-urlencode code_challenge_method=S256 \
    --data-urlencode "state=$(openssl rand -hex 8)" --data-urlencode "nonce=$(openssl rand -hex 8)"
  action=$(tr -d '\n' < "$tmp/login.html" | grep -o 'action="[^"]*login-actions/authenticate[^"]*"' | head -n 1 \
    | sed 's/^action="//; s/"$//; s/&amp;/\&/g')
  case $action in
    "$issuer"/login-actions/authenticate*) ;;
    *) fail "login form of securepatrol-web not found or not on $issuer (got '${action}')" ;;
  esac
  printf '%s' "$2" > "$tmp/webpw"
  wl_loc=$(pub -c "$jar" -b "$jar" -o "$tmp/page" -w '%{redirect_url}' \
    --data-urlencode "username=$1" --data-urlencode "password@$tmp/webpw" "$action")
  wl_hops=0
  while [ -n "$wl_loc" ] && on_keycloak "$wl_loc" && [ "$wl_hops" -lt 5 ]; do
    wl_loc=$(pub -c "$jar" -b "$jar" -o "$tmp/page" -w '%{redirect_url}' "$wl_loc")
    wl_hops=$((wl_hops + 1))
  done
  printf '%s' "$wl_loc"
}

site=$(openssl rand -hex 16 | sed 's/^\(.\{8\}\)\(.\{4\}\)\(.\{4\}\)\(.\{4\}\)/\1-\2-\3-\4-/')
web_user="smoke-viewer-$tag"
web_pw="Sm$(openssl rand -hex 12)9"
create_user "$web_user" client-readonly "$web_pw" "$(jq -cn --arg u "$web_user" --arg s "$site" \
  '{username: $u, firstName: "Smoke", lastName: "Viewer", email: ($u + "@example.invalid"),
    emailVerified: true, enabled: true, requiredActions: [], attributes: {sites: [$s]}}')" >/dev/null
loc=$(web_login "$web_user" "$web_pw")
case $loc in
  "$base/"*) ;;
  *) fail "web login of $web_user did not redirect back to $base/ (page: $(grep -o '<title>[^<]*' "$tmp/page" | head -n 1))" ;;
esac
code=$(printf '%s' "$loc" | sed -n 's/.*[?&#]code=\([^&]*\).*/\1/p')
[ -n "$code" ] || fail "no authorization code in the redirect"
printf '%s' "$code" > "$tmp/code"
wt=$(pub -f -X POST --data-urlencode grant_type=authorization_code --data-urlencode client_id=securepatrol-web \
  --data-urlencode "code@$tmp/code" --data-urlencode "redirect_uri=$base/" \
  --data-urlencode "code_verifier@$tmp/verifier" "$token_ep" | jq -er .access_token) || fail "web code exchange (PKCE)"
expect "web access token: typ, azp, sub, roles = client-readonly, claim sites" \
  '.typ == "Bearer" and .azp == "securepatrol-web" and .iss == $iss and (.sub | length > 0)
   and .roles == ["client-readonly"] and .sites == [$site]' "$(claims "$wt")"
printf 'Authorization: Bearer %s\n' "$wt" > "$tmp/auth"
expect "API accepts the web token and applies the site restriction" \
  '.client_id == "securepatrol-web" and .roles == ["client-readonly"] and .sites == [$site] and .sites_restricted == true' \
  "$(pub -f -H "@$tmp/auth" "$base/api/v1/me")"

staff_user="smoke-supervisor-$tag"
staff_pw="Sm$(openssl rand -hex 12)9"
create_user "$staff_user" supervisor "$staff_pw" "$(jq -cn --arg u "$staff_user" \
  '{username: $u, firstName: "Smoke", lastName: "Supervisor", email: ($u + "@example.invalid"),
    emailVerified: true, enabled: true, requiredActions: []}')" >/dev/null
loc=$(web_login "$staff_user" "$staff_pw")
[ -z "$loc" ] || fail "supervisor $staff_user logged in without OTP (redirect to $loc)"
grep -Eq 'kc-totp-settings|name="totp"' "$tmp/page" || fail "supervisor login did not ask to set up OTP"
ok "staff login (browser-mfa) asks for OTP setup"

info "keycloak smoke test passed for realm $realm"
