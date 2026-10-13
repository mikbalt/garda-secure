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
#     - user-profile attribute `sites` (multi-valued, admin-only) and the
#       securepatrol-web mapper "allowed sites" -> access-token claim `sites`
#       (per-site scoping, D-035 / F4)
#     - confidential client securepatrol-admin-api (KEYCLOAK_ADMIN_CLIENT_ID)
#       whose service account holds realm-management view-users, query-users,
#       manage-users, view-realm; its secret is written to
#       infra/secrets/keycloak-admin-clients ("realm=secret", read by the API
#       through KEYCLOAK_ADMIN_CLIENT_SECRETS_FILE; never printed) — F1
#     - "Full scope allowed" off on securepatrol-web / -tablet / -admin-api
#       (deprecated in Keycloak 26.8): tokens carry only the roles the API
#       honours — web: admin, supervisor, hr, client-readonly, platform-admin;
#       tablet: device (+ offline_access via its scope); admin-api: the four
#       realm-management roles above. The tablet client keeps the default
#       client scope `basic` (claim `sub`, required by the API).
#   master realm: brute-force protection on.
# Needs docker compose (keycloak running) and jq on the host.
# Auth: KEYCLOAK_AUTOMATION_CLIENT_ID/SECRET (service account) when set, else
# the bootstrap admin KC_BOOTSTRAP_ADMIN_USERNAME/PASSWORD — see
# docs/runbook.md §2 (master realm OTP).
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
  # KC_BOOTSTRAP_ADMIN_USERNAME / _PASSWORD are already in the container env.
  compose exec -T keycloak sh -c \
    '/opt/keycloak/bin/kcadm.sh config credentials --config '"$CFG"' --server http://127.0.0.1:8080/auth --realm master --user "$KC_BOOTSTRAP_ADMIN_USERNAME" --password "$KC_BOOTSTRAP_ADMIN_PASSWORD"' >/dev/null
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

ADMIN_CLIENT=${KEYCLOAK_ADMIN_CLIENT_ID:-securepatrol-admin-api}
SECRETS_DIR=$INFRA_DIR/secrets
SECRETS_FILE=$SECRETS_DIR/keycloak-admin-clients
# Roles each client's tokens may carry (the API ignores all others, D-032).
WEB_ROLES="platform-admin admin supervisor hr client-readonly"
TABLET_ROLES="device"
ADMIN_API_ROLES="view-users query-users manage-users view-realm"

# limit_scope REALM CLIENT_UUID CONTAINER ROLE... — switch "Full scope
# allowed" off and map the roles into the client's scope. CONTAINER is
# "realm" or the UUID of the client that owns the roles. Roles that do not
# exist (yet) are skipped — run again after creating them; mappings added by
# hand are kept.
limit_scope() {
  ls_r=$1 ls_id=$2 ls_container=$3
  shift 3
  ls_want=$(printf '%s\n' "$@" | jq -R . | jq -sc .)
  if [ "$ls_container" = realm ]; then
    ls_path="clients/$ls_id/scope-mappings/realm"
    ls_all=$(kc get roles -r "$ls_r")
  else
    ls_path="clients/$ls_id/scope-mappings/clients/$ls_container"
    ls_all=$(kc get "clients/$ls_container/roles" -r "$ls_r")
  fi
  ls_have=$(kc get "$ls_path" -r "$ls_r")
  ls_add=$(jq -cn --argjson all "$ls_all" --argjson have "$ls_have" --argjson want "$ls_want" \
    '[$all[] | select(.name as $n | any($want[]; . == $n) and (any($have[]; .name == $n) | not)) | {id, name}]')
  if [ "$ls_add" != "[]" ]; then
    printf '%s' "$ls_add" | kc create "$ls_path" -r "$ls_r" -f - >/dev/null
  fi
  kc update "clients/$ls_id" -r "$ls_r" -s fullScopeAllowed=false >/dev/null
}

# ensure_default_scope REALM CLIENT_UUID SCOPE — e.g. `basic` (claim sub),
# which a client imported with an explicit scope list may lack.
ensure_default_scope() {
  if ! kc get "clients/$2/default-client-scopes" -r "$1" | jq -e --arg s "$3" 'any(.[]; .name == $s)' >/dev/null; then
    eds_id=$(kc get client-scopes -r "$1" | jq -r --arg s "$3" 'first(.[] | select(.name == $s) | .id) // empty')
    if [ -z "$eds_id" ]; then
      warn "  client scope $3 does not exist in $1"
      return 0
    fi
    kc update "clients/$2/default-client-scopes/$eds_id" -r "$1" -n -b '{}' >/dev/null
    info "  default client scope $3 added"
  fi
}

configure_sites() { # realm, web client id — allowed sites attribute + claim (F4)
  r=$1 cid=$2
  profile=$(kc get users/profile -r "$r")
  if ! printf '%s' "$profile" | jq -e 'any(.attributes[]; .name == "sites")' >/dev/null; then
    printf '%s' "$profile" | jq -c '.attributes += [{
        "name": "sites", "displayName": "Allowed sites", "multivalued": true,
        "permissions": {"view": ["admin"], "edit": ["admin"]},
        "validations": {"length": {"max": 36}}}]' \
      | kc update users/profile -r "$r" -f - >/dev/null
    info "  user profile: attribute sites added"
  fi
  [ -n "$cid" ] || return 0
  if ! kc get "clients/$cid/protocol-mappers/models" -r "$r" | jq -e 'any(.[]; .name == "allowed sites")' >/dev/null; then
    printf '%s' '{"name":"allowed sites","protocol":"openid-connect","protocolMapper":"oidc-usermodel-attribute-mapper",
      "config":{"user.attribute":"sites","claim.name":"sites","jsonType.label":"String","multivalued":"true",
      "access.token.claim":"true","id.token.claim":"false","userinfo.token.claim":"false"}}' \
      | kc create "clients/$cid/protocol-mappers/models" -r "$r" -f - >/dev/null
    info "  securepatrol-web: claim sites mapped from the user attribute"
  fi
}

configure_admin_client() { # realm — confidential client for the API's user management (F1)
  r=$1
  id=$(kc get clients -r "$r" -q "clientId=$ADMIN_CLIENT" --fields id | jq -r '.[0].id // empty')
  if [ -z "$id" ]; then
    kc create clients -r "$r" -s "clientId=$ADMIN_CLIENT" -s enabled=true -s publicClient=false \
      -s serviceAccountsEnabled=true -s standardFlowEnabled=false -s implicitFlowEnabled=false \
      -s directAccessGrantsEnabled=false -s 'description=SecurePatrol API: user and tablet-account management' >/dev/null
    id=$(kc get clients -r "$r" -q "clientId=$ADMIN_CLIENT" --fields id | jq -r '.[0].id // empty')
    info "  client $ADMIN_CLIENT created"
  else
    kc update "clients/$id" -r "$r" -s enabled=true -s publicClient=false -s serviceAccountsEnabled=true \
      -s standardFlowEnabled=false -s implicitFlowEnabled=false -s directAccessGrantsEnabled=false >/dev/null
  fi
  [ -n "$id" ] || die "client $ADMIN_CLIENT could not be created in $r"
  sa=$(kc get "clients/$id/service-account-user" -r "$r" | jq -r '.username')
  kc add-roles -r "$r" --uusername "$sa" --cclientid realm-management \
    --rolename view-users --rolename query-users --rolename manage-users --rolename view-realm >/dev/null
  rm_id=$(kc get clients -r "$r" -q clientId=realm-management --fields id | jq -r '.[0].id // empty')
  [ -n "$rm_id" ] || die "client realm-management not found in $r"
  # shellcheck disable=SC2086 # word splitting of the role list is intended
  limit_scope "$r" "$id" "$rm_id" $ADMIN_API_ROLES
  secret=$(kc get "clients/$id/client-secret" -r "$r" | jq -r '.value // empty')
  if [ -z "$secret" ]; then
    secret=$(kc create "clients/$id/client-secret" -r "$r" -o | jq -r '.value // empty')
  fi
  [ -n "$secret" ] || die "no secret for client $ADMIN_CLIENT in $r"
  # The directory is private to the operator; the file itself is readable so
  # the API container (non-root user) can read its bind-mounted copy.
  mkdir -p "$SECRETS_DIR" && chmod 700 "$SECRETS_DIR"
  tmp=$(mktemp "$SECRETS_DIR/.kc-admin.XXXXXX")
  { [ -f "$SECRETS_FILE" ] && grep -v "^$r=" "$SECRETS_FILE" || true; printf '%s=%s\n' "$r" "$secret"; } > "$tmp"
  chmod 644 "$tmp" && mv "$tmp" "$SECRETS_FILE"
  info "  $ADMIN_CLIENT: service account roles set, secret stored in secrets/keycloak-admin-clients"
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
  configure_sites "$r" "$cid"
  configure_admin_client "$r"
  # shellcheck disable=SC2086 # word splitting of the role lists is intended
  if [ -n "$cid" ]; then
    limit_scope "$r" "$cid" realm $WEB_ROLES
  fi
  tid=$(kc get clients -r "$r" -q clientId=securepatrol-tablet --fields id | jq -r '.[0].id // empty')
  if [ -n "$tid" ]; then
    ensure_default_scope "$r" "$tid" basic
    # shellcheck disable=SC2086
    limit_scope "$r" "$tid" realm $TABLET_ROLES
  else
    warn "  client securepatrol-tablet not found in $r"
  fi
  info "  token role scope: web = $WEB_ROLES; tablet = $TABLET_ROLES; $ADMIN_CLIENT = $ADMIN_API_ROLES"
done

info "realm master: brute-force protection"
kc update realms/master -s bruteForceProtected=true -s failureFactor=5 -s waitIncrementSeconds=60 \
  -s maxFailureWaitSeconds=900 >/dev/null
info "keycloak-config done"
