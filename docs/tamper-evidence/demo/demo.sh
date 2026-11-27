#!/usr/bin/env bash
# Tamper-evidence demo (TAQ-23). Seeds a demo site with a month of records, exports a bundle, checks it
# with garda-verify (VALID), edits ONE record directly in the database as the owner role (the check
# fails), reverts the edit, then shows a tampered bundle file failing. Read README.md first.
#
# Required environment (see README.md):
#   DEMO_API     API base incl. /api/v1, e.g. http://localhost:8080/api/v1
#   DEMO_TOKEN   bearer token of an admin or supervisor of DEMO_REALM
#   DEMO_REALM   Keycloak realm that owns the demo site (the token's realm)
#   DEMO_DB_URL  database URL of the OWNER role (the one that runs migrations)
#   DEMO_PIN     signer key fingerprint (64 hex): the one you hold, not the one the server shows
#   DEMO_SIGN_URL  signer base URL that accepts POST /sign?site_id= (internal network only)
# Optional: PSQL (psql command, default "psql"), GARDA_VERIFY (path; default: garda-verify on PATH,
#   else built from backend/cmd/garda-verify), DEMO_PURGE=1 (disposable database only, see README).
set -euo pipefail

: "${DEMO_API:?}" "${DEMO_TOKEN:?}" "${DEMO_REALM:?}" "${DEMO_DB_URL:?}" "${DEMO_PIN:?}" "${DEMO_SIGN_URL:?}"
PSQL=${PSQL:-psql}
here=$(cd "$(dirname "$0")" && pwd)
work=$(mktemp -d "${TMPDIR:-/tmp}/garda-demo.XXXXXX")
site=""; att=""; orig=""; edited=0

q() { $PSQL "$DEMO_DB_URL" -v ON_ERROR_STOP=1 -X -q -At "$@"; }
step() { printf '\n\033[1m== %s\033[0m\n' "$*"; }
say() { printf '   %s\n' "$*"; }

cleanup() {
  rc=$?
  # Never leave the demo record edited: the signer and every later check would see a broken site.
  if [ "$edited" = 1 ] && [ -n "$att" ]; then
    q -c "UPDATE attendance_records SET status = '$orig' WHERE id = '$att'" >/dev/null 2>&1 \
      && say "(cleanup) the edited record was put back to '$orig'" \
      || say "(cleanup) COULD NOT revert record $att to '$orig': do it by hand before anything else"
  fi
  if [ "${DEMO_PURGE:-}" = 1 ] && [ -n "$site" ]; then purge; fi
  rm -rf "$work"
  exit $rc
}
trap cleanup EXIT

purge() {
  # Disposable database only: the chain is append-only on purpose, deleting it needs the owner to
  # switch the guards off, and a real signer keeps its own memory of what it signed.
  q <<SQL >/dev/null
ALTER TABLE record_chain DISABLE TRIGGER USER;
ALTER TABLE record_chain_heads DISABLE TRIGGER USER;
ALTER TABLE signed_heads DISABLE TRIGGER USER;
DELETE FROM signed_heads WHERE site_id = '$site';
DELETE FROM record_chain WHERE site_id = '$site';
DELETE FROM record_chain_heads WHERE site_id = '$site';
ALTER TABLE record_chain ENABLE TRIGGER USER;
ALTER TABLE record_chain_heads ENABLE TRIGGER USER;
ALTER TABLE signed_heads ENABLE TRIGGER USER;
DELETE FROM checkpoint_scans WHERE route_id IN (SELECT id FROM routes WHERE site_id = '$site');
DELETE FROM patrol_sessions WHERE site_id = '$site';
DELETE FROM routes WHERE site_id = '$site';
DELETE FROM attendance_records WHERE site_id = '$site';
DELETE FROM incidents WHERE site_id = '$site';
DELETE FROM shifts WHERE site_id = '$site';
DELETE FROM guards WHERE site_id = '$site';
DELETE FROM sites WHERE id = '$site';
SQL
  say "(cleanup) DEMO_PURGE: the demo site and its chain were deleted from this disposable database"
}

# garda-verify, run the way a client would: exit code and text are shown as they are.
verify() { # verify <label> <bundle> [extra args]
  local label=$1 bundle=$2; shift 2
  echo "   \$ garda-verify --pin <fingerprint> $* $(basename "$bundle")"
  set +e
  "$VERIFY" --pin "$DEMO_PIN" "$@" "$bundle" 2>&1 | sed 's/^/   | /'
  vrc=${PIPESTATUS[0]}
  set -e
  echo "   exit code: $vrc   ($label)"
}

export_bundle() { # export_bundle <file>
  local code
  code=$(curl -sS -o "$1" -D "$work/headers" -w '%{http_code}' -H "Authorization: Bearer $DEMO_TOKEN" \
    "$DEMO_API/sites/$site/chain/export?from=$from&to=$to")
  [ "$code" = 200 ] || { echo "export failed: HTTP $code: $(head -c 300 "$1")"; exit 1; }
  say "bundle: $(basename "$1"), $(wc -c <"$1" | tr -d ' ') bytes, $(grep -i '^x-bundle-sha256' "$work/headers" | tr -d '\r')"
}

step "0. Checks"
if [ -n "${GARDA_VERIFY:-}" ]; then VERIFY=$GARDA_VERIFY
elif command -v garda-verify >/dev/null 2>&1; then VERIFY=$(command -v garda-verify)
else
  say "garda-verify not found: building it from the repository"
  (cd "$here/../../../backend" && go build -o "$work/garda-verify" ./cmd/garda-verify)
  VERIFY=$work/garda-verify
fi
for c in curl python3; do command -v "$c" >/dev/null || { echo "$c is required"; exit 1; }; done
[[ "$DEMO_PIN" =~ ^[0-9a-f]{64}$ ]] || { echo "DEMO_PIN must be 64 lowercase hex characters"; exit 1; }
q -c "SELECT chain_record_canonical('incident', gen_random_uuid())" >/dev/null \
  || { echo "database has no chain_record_canonical (migration 014): this stack is too old"; exit 1; }
say "garda-verify: $VERIFY"
say "database reachable, chain export migration present"

step "1. Seed: a demo site with 30 days of attendance, patrol scans and incidents"
name="GARDA DEMO $(date -u +%Y%m%d-%H%M%S)"
today=$(q -c "SELECT (now() AT TIME ZONE 'Asia/Jakarta')::date")
from=$(q -c "SELECT '$today'::date - 31"); to=$today
site=$(q -v "name=$name" -v "realm=$DEMO_REALM" <<'SQL'
BEGIN;
INSERT INTO sites (name, keycloak_realm, timezone) VALUES (:'name', :'realm', 'Asia/Jakarta') RETURNING id AS site \gset
SELECT set_config('demo.site', :'site', false) \gset
DO $$
DECLARE
  s uuid := current_setting('demo.site')::uuid;
  g uuid; r uuid; sess uuid; sh uuid; a uuid; sc uuid; inc uuid; d int; t timestamptz;
BEGIN
  INSERT INTO guards (name, role, site_id) VALUES ('Demo Satpam', 'guard', s) RETURNING id INTO g;
  INSERT INTO routes (site_id, name) VALUES (s, 'Demo round') RETURNING id INTO r;
  INSERT INTO patrol_sessions (route_id, site_id, guard_id, started_at) VALUES (r, s, g, now()) RETURNING id INTO sess;
  FOR d IN 1..30 LOOP
    t := date_trunc('day', now()) - make_interval(days => 31 - d) + interval '1 hour';
    INSERT INTO shifts (guard_id, site_id, start_time, end_time) VALUES (g, s, t, t + interval '8 hours') RETURNING id INTO sh;
    INSERT INTO attendance_records (shift_id, guard_id, site_id, clock_in_at, status)
      VALUES (sh, g, s, t + make_interval(mins => d % 7), CASE WHEN d % 9 = 0 THEN 'late' ELSE 'present' END) RETURNING id INTO a;
    PERFORM chain_append('attendance', a);
    INSERT INTO checkpoint_scans (route_id, session_id, checkpoint_id, guard_id, scanned_at, method)
      VALUES (r, sess, 'cp1', g, t + interval '30 minutes', 'qr') RETURNING id INTO sc;
    PERFORM chain_append('scan', sc);
    INSERT INTO checkpoint_scans (route_id, session_id, checkpoint_id, guard_id, scanned_at, method)
      VALUES (r, sess, 'cp2', g, t + interval '60 minutes', 'qr') RETURNING id INTO sc;
    PERFORM chain_append('scan', sc);
    IF d % 10 = 0 THEN
      INSERT INTO incidents (site_id, guard_id, type, severity, description, occurred_at)
        VALUES (s, g, 'suspicious_person', 'low', 'Demo incident day ' || d, t + interval '2 hours') RETURNING id INTO inc;
      PERFORM chain_append('incident', inc);
    END IF;
  END LOOP;
END $$;
COMMIT;
SELECT :'site';
SQL
)
site=$(printf '%s' "$site" | tail -n 1)
[[ "$site" =~ ^[0-9a-f-]{36}$ ]] || { echo "seeding failed (site id: '$site')"; exit 1; }
n=$(q -c "SELECT count(*) FROM record_chain WHERE site_id = '$site'")
say "site '$name' ($site): $n chain entries, one per record, written as the application would"
say "range checked below: $from .. $to (site time zone Asia/Jakarta)"

step "2. The signer signs the newest entry (the app cannot do this: other process, other key, other volume)"
curl -fsS -X POST "$DEMO_SIGN_URL/sign?site_id=$site" | sed "s/^/   | /"

step "3. A client exports the bundle and checks it on their own machine with the fingerprint they hold"
export_bundle "$work/bundle-1.zip"
verify "expected VALID: 0" "$work/bundle-1.zip" --no-range-check
[ "$vrc" = 0 ] || { echo "demo stopped: the clean bundle did not verify"; exit 1; }
say "(--no-range-check because the entries were made today: a check of the dates needs a signed head made after the"
say " last day of the range, so a strict 'garda-verify --expect-from/--expect-to' can only pass from tomorrow on. README.)"

step "4. Someone edits ONE attendance record directly in the database, as the owner role, bypassing the app"
att=$(q -c "SELECT id FROM attendance_records WHERE site_id = '$site' AND status = 'present' ORDER BY clock_in_at OFFSET 14 LIMIT 1")
day=$(q -c "SELECT (clock_in_at AT TIME ZONE 'Asia/Jakarta')::date FROM attendance_records WHERE id = '$att'")
orig=present; edited=1
say "SQL: UPDATE attendance_records SET status = 'absent' WHERE id = '$att';   -- the guard's $day shift"
q -c "UPDATE attendance_records SET status = 'absent' WHERE id = '$att'"
say "the dashboard now shows 'absent'. The database accepted it. Now a client checks again:"
export_bundle "$work/bundle-2.zip"
verify "expected BROKEN: 1" "$work/bundle-2.zip" --no-range-check
[ "$vrc" = 1 ] || { echo "demo stopped: the edit was NOT caught (exit $vrc)"; exit 1; }
say "online check (the server asked, so it trusts the server; with the pin it is the same engine):"
curl -fsS -H "Authorization: Bearer $DEMO_TOKEN" "$DEMO_API/sites/$site/chain/verify?from=$from&to=$to&pin=$DEMO_PIN" \
  | python3 -I -c 'import json,sys; r=json.load(sys.stdin); print("   | status:", r["status"], "| valid_pinned:", r.get("valid_pinned"), "| first_break:", r.get("first_break"))'

step "5. The edit is put back; the same check passes again (so this demo leaves no broken site behind)"
q -c "UPDATE attendance_records SET status = 'present' WHERE id = '$att'"; edited=0
export_bundle "$work/bundle-3.zip"
verify "expected VALID: 0" "$work/bundle-3.zip" --no-range-check

step "6. A tampered bundle file: a similar edit, made in the exported file instead of the database"
python3 -I - "$work/bundle-3.zip" "$work/tampered-a.zip" "$work/tampered-b.zip" <<'PY'
import hashlib, json, sys, zipfile
src, out_a, out_b = sys.argv[1:4]
z = zipfile.ZipFile(src); files = {n: z.read(n) for n in z.namelist()}
lines = files["entries.jsonl"].decode().splitlines(); hit = False
for i, l in enumerate(lines):
    e = json.loads(l)
    if not hit and e["kind"] == "attendance" and e.get("record_state") == "latest" and '"status": "present"' in e["record"]:
        e["record"] = e["record"].replace('"status": "present"', '"status": "absent"', 1)
        lines[i] = json.dumps(e, separators=(",", ":")); hit = True
assert hit, "no attendance record found to change"
files["entries.jsonl"] = ("\n".join(lines) + "\n").encode()
def write(path, fl):
    with zipfile.ZipFile(path, "w", zipfile.ZIP_DEFLATED) as o:
        for n, b in fl.items(): o.writestr(n, b)
write(out_a, files)                                   # A: only entries.jsonl changed
m = json.loads(files["manifest.json"]); m["files"]["entries.jsonl"] = hashlib.sha256(files["entries.jsonl"]).hexdigest()
files["manifest.json"] = json.dumps(m, indent=2).encode()
write(out_b, files)                                   # B: the forger also fixes the file hash in the manifest
PY
say "A: one record changed inside entries.jsonl, nothing else touched"
verify "expected: 3 (bundle changed)" "$work/tampered-a.zip" --no-range-check
say "B: the forger also corrects the file hash in the manifest, as a careful one would"
verify "expected: 1 (record does not match its chain entry)" "$work/tampered-b.zip" --no-range-check

step "Done"
say "demo site '$name' stays in the database: the chain and the signed heads are append-only on purpose."
say "Run again as often as you like: every run makes a new site. Temporary files: removed."
