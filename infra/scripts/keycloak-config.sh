#!/bin/sh
# Bring the RUNNING Keycloak in line with infra/.env — idempotent, safe to run
# after every start/deploy (`make keycloak-config`; `make up` and deploy.sh
# call it). Realm JSON files are imported only once, so this is how existing
# realms receive changes (INF-D4, INF-S2, INF-N2):
#   every realm in KEYCLOAK_REALMS (or the realms given as arguments):
#     - securepatrol-web redirect URIs = ${PUBLIC_URL}/* (+ Vite dev URL only
#       when KEYCLOAK_DEV_REDIRECTS=true), web origins "+"
#     - OTP enforced for admin / platform-admin / hr / supervisor: composite
#       role `mfa-required` + browser flow `browser-mfa` (cookie | username/
#       password -> OTP if the user has mfa-required; first login enrols OTP)
#     - brute-force protection on, CONFIGURE_TOTP required action enabled
#   master realm: brute-force protection on.
# Needs docker compose (keycloak running) and jq on the host.
# Auth: KEYCLOAK_AUTOMATION_CLIENT_ID/SECRET (service account) when set, else
# the KEYCLOAK_ADMIN user — see docs/runbook.md §2 (master realm OTP).
set -eu
. "$(dirname -- "$0")/lib.sh"

load_env "$INFRA_DIR/.env"
command -v jq >/dev/null || die "jq is required (apt install jq)"

MFA_ROLE=mfa-required
MFA_ROLES="admin platform-admin hr supervisor"
FLOW=browser-mfa
CFG=/tmp/kcadm-securepatrol.config

kc() { compose exec -T keycloak /opt/keycloak/bin/kcadm.sh "$@" --config "$CFG"; }
kcq() { kc "$@" 2>/dev/null; }

info "logging in to Keycloak (admin CLI inside the container)"
if [ -n "${KEYCLOAK_AUTOMATION_CLIENT_ID:-}" ]; then
  # The secret goes through stdin, not the host command line.
  printf '%s\n' "${KEYCLOAK_AUTOMATION_CLIENT_SECRET:?KEYCLOAK_AUTOMATION_CLIENT_SECRET is empty}" | \
    compose exec -T keycloak sh -c \
    'read -r s; /opt/keycloak/bin/kcadm.sh config credentials --config '"$CFG"' --server http://127.0.0.1:8080/auth --realm master --client "$1" --secret "$s"' \
    sh "$KEYCLOAK_AUTOMATION_CLIENT_ID" >/dev/null
else
  # KEYCLOAK_ADMIN / KEYCLOAK_ADMIN_PASSWORD are already in the container env.
  compose exec -T keycloak sh -c \
    '/opt/keycloak/bin/kcadm.sh config credentials --config '"$CFG"' --server http://127.0.0.1:8080/auth --realm master --user "$KEYCLOAK_ADMIN" --password "$KEYCLOAK_ADMIN_PASSWORD"' >/dev/null
fi
trap 'compose exec -T keycloak rm -f "$CFG" >/dev/null 2>&1 || true' EXIT

redirects=$(jq -cn --arg u "$PUBLIC_URL/*" --arg dev "${KEYCLOAK_DEV_REDIRECTS:-false}" \
  '[$u] + (if $dev == "true" then ["http://localhost:5173/*"] else [] end)')

# exec_json REALM — all executions of the flow (nested ones included) as JSON
exec_json() { kc get "authentication/flows/$FLOW/executions" -r "$1"; }

set_requirement() { # realm, json-of-execution, requirement
  body=$(printf '%s' "$2" | jq -c --arg r "$3" '{id, requirement: $r} + (if has("priority") then {priority} else {} end)')
  # With -f, kcadm sends the body as-is (no GET/merge of the execution list).
  printf '%s' "$body" | kc update "authentication/flows/$FLOW/executions" -r "$1" -f - >/dev/null
}

configure_mfa() { # realm
  r=$1
  kcq get "roles/$MFA_ROLE" -r "$r" >/dev/null \
    || kc create roles -r "$r" -s "name=$MFA_ROLE" \
         -s "description=Must use OTP for web login (composite of admin/platform-admin/hr/supervisor)" >/dev/null
  for role in $MFA_ROLES; do
    if kcq get "roles/$role" -r "$r" >/dev/null; then
      kc add-roles -r "$r" --rname "$role" --rolename "$MFA_ROLE" >/dev/null
    fi
  done

  if ! kc get authentication/flows -r "$r" | jq -e --arg f "$FLOW" 'any(.[]; .alias == $f)' >/dev/null; then
    info "  creating browser flow $FLOW"
    kc create authentication/flows -r "$r" -s "alias=$FLOW" -s providerId=basic-flow -s topLevel=true \
      -s builtIn=false -s "description=Browser login; OTP required for users with role $MFA_ROLE" >/dev/null
    kc create "authentication/flows/$FLOW/executions/execution" -r "$r" -s provider=auth-cookie >/dev/null
    kc create "authentication/flows/$FLOW/executions/flow" -r "$r" -s "alias=$FLOW-forms" \
      -s type=basic-flow -s provider=registration-page-form -s "description=Username, password, OTP" >/dev/null
    kc create "authentication/flows/$FLOW-forms/executions/execution" -r "$r" -s provider=auth-username-password-form >/dev/null
    kc create "authentication/flows/$FLOW-forms/executions/flow" -r "$r" -s "alias=$FLOW-otp" \
      -s type=basic-flow -s provider=registration-page-form -s "description=OTP when the user has $MFA_ROLE" >/dev/null
    kc create "authentication/flows/$FLOW-otp/executions/execution" -r "$r" -s provider=conditional-user-role >/dev/null
    kc create "authentication/flows/$FLOW-otp/executions/execution" -r "$r" -s provider=auth-otp-form >/dev/null

    execs=$(exec_json "$r")
    pick() { printf '%s' "$execs" | jq -c "$1" ; }
    set_requirement "$r" "$(pick 'first(.[] | select(.providerId == "auth-cookie"))')" ALTERNATIVE
    set_requirement "$r" "$(pick "first(.[] | select(.authenticationFlow == true and .displayName == \"$FLOW-forms\"))")" ALTERNATIVE
    set_requirement "$r" "$(pick 'first(.[] | select(.providerId == "auth-username-password-form"))')" REQUIRED
    set_requirement "$r" "$(pick "first(.[] | select(.authenticationFlow == true and .displayName == \"$FLOW-otp\"))")" CONDITIONAL
    set_requirement "$r" "$(pick 'first(.[] | select(.providerId == "conditional-user-role"))')" REQUIRED
    set_requirement "$r" "$(pick 'first(.[] | select(.providerId == "auth-otp-form"))')" REQUIRED
    cond_id=$(pick 'first(.[] | select(.providerId == "conditional-user-role")) | .id' | tr -d '"')
    kc create "authentication/executions/$cond_id/config" -r "$r" -s "alias=$FLOW-role" \
      -s "config.condUserRole=$MFA_ROLE" >/dev/null
  fi

  kc update authentication/required-actions/CONFIGURE_TOTP -r "$r" -s enabled=true >/dev/null
  kc update "realms/$r" -s "browserFlow=$FLOW" -s bruteForceProtected=true >/dev/null
}

if [ $# -gt 0 ]; then realms=$*; else realms=$(printf '%s' "$KEYCLOAK_REALMS" | tr ',' ' '); fi
for r in $realms; do
  info "realm $r"
  if ! kcq get "realms/$r" --fields realm >/dev/null; then
    warn "realm $r does not exist in Keycloak (import infra/keycloak/realms/$r.json or create it) — skipped"
    continue
  fi
  cid=$(kc get clients -r "$r" -q clientId=securepatrol-web --fields id | jq -r '.[0].id // empty')
  if [ -n "$cid" ]; then
    kc update "clients/$cid" -r "$r" -s "redirectUris=$redirects" -s 'webOrigins=["+"]' \
      -s 'attributes."post.logout.redirect.uris"=+' >/dev/null
    info "  securepatrol-web redirect URIs: $redirects"
  else
    warn "  client securepatrol-web not found in $r"
  fi
  configure_mfa "$r"
  info "  OTP enforced for: $MFA_ROLES (via role $MFA_ROLE, flow $FLOW)"
done

info "realm master: brute-force protection"
kc update realms/master -s bruteForceProtected=true -s failureFactor=5 -s waitIncrementSeconds=60 \
  -s maxFailureWaitSeconds=900 >/dev/null
info "keycloak-config done"
