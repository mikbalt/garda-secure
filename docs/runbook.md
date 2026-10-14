# SecurePatrol — Operations Runbook

For the single operator running the on-premise stack (PRD NFR-01, NFR-07, NFR-09).
All commands run on the server from `infra/` unless noted. Audit IDs (INF-…, BE-…)
refer to [docs/audit-2026-10.md](audit-2026-10.md).

> `infra/.env` is a Docker Compose env file, **not** a shell script (values contain
> spaces and `<`). Never `source` it; the scripts in `infra/scripts/` read it safely.

## 1. First install

Requirements: Linux server, Docker 24+ with Compose **v2.24+** (the app overlay uses
`!override`), `make`, `openssl`, `jq`, `git`. A **second disk or NAS share** for the
off-site backup (§5).

```bash
git clone <repo> /opt/securepatrol && cd /opt/securepatrol/infra
make env SERVER_HOST=patrol.example.co.id   # or the server's LAN IP, e.g. 192.168.10.5
#   -> infra/.env with random secrets (chmod 600), incl. SETTINGS_ENCRYPTION_KEY.
#      Review it: SMTP_*, ALERT_*, OFFSITE_BACKUP_REPO, KEYCLOAK_REALMS /
#      WEB_KEYCLOAK_REALMS, PLATFORM_ADMIN_REALMS.
#   Copy the whole file into your password manager (it is also in the off-site backup).
sudo mkdir -p /mnt/securepatrol-offsite      # mount the backup disk/NAS here (fstab)
make certs                                   # private CA + server certificate for SERVER_HOST
make up                                      # preflight, build/start everything, configure Keycloak
make ps                                      # every service "healthy" (minio-init "exited (0)")
```

* **One address:** `SERVER_HOST` (DNS name or IPv4) is the only place the server's
  address is configured. `PUBLIC_URL`, `KEYCLOAK_PUBLIC_URL` (token issuer),
  `MINIO_PUBLIC_ENDPOINT` (photo links), the certificate SAN and the Keycloak redirect
  URIs all derive from it. Changing it later: edit `SERVER_HOST`, `make certs`
  (a new CA if the name changes — devices must trust it again), `make up`.
* **`make preflight`** (run by `make up` and every deploy) refuses to start with
  `change_me` values, secrets shorter than 32 characters or not URL-safe
  (`openssl rand -hex 32`), reused secrets, URLs that do not match `SERVER_HOST`,
  a certificate whose SAN/key/CA does not match, an expired certificate or a
  world-readable `.env`.
* `COMPOSE_FILE` in `.env` lists `docker-compose.yml` (infrastructure),
  `docker-compose.app.yml` (api, worker, web, TLS nginx) and `docker-compose.ops.yml`
  (monitoring, off-site backup), so plain `docker compose …` commands act on the
  whole stack.
* With `VERSION=dev` (default) `make up` builds the api/worker, web and
  backup-offsite images locally; with a release version it only pulls (§7).
* Install the **CA certificate** on every tablet and admin PC (§6) — otherwise the
  browser blocks the login page and photo links (`:9443`).

Local development (no TLS, Keycloak on :8081): `make env SERVER_HOST=localhost`,
set `KEYCLOAK_PUBLIC_URL=http://localhost:8081/auth` and `KEYCLOAK_DEV_REDIRECTS=true`
in `.env`, then `make up-infra` and `make keycloak-config` (allows the Vite dev server
`http://localhost:5173` as redirect URI). Never set `KEYCLOAK_DEV_REDIRECTS` on a server.

### Database roles (INF-S4)

The Postgres init script creates, on the **first start of an empty volume**:
`securepatrol_app` (`APP_DB_USER`; owns only the app DB; used by api/worker),
`keycloak` (`KC_DB_USER`; owns only the Keycloak DB) and `sp_monitor` (`pg_monitor`,
for the exporter). `POSTGRES_USER` (default `postgres`) is the superuser — admin and
restore only.

* The app role owns the app database and (PostgreSQL 15+) its `public` schema, so the
  API runs all migrations as that role on start: extensions (`pgcrypto`, `uuid-ossp`),
  tables, functions and triggers.
* The `audit_log` append-only trigger (migration 005) rejects UPDATE, DELETE and
  TRUNCATE for **every** role, the app role included (§10).
* The worker dumps the Keycloak database with the `KC_DB_USER` credentials
  (`BACKUP_KEYCLOAK_DATABASE_URL`, set in `docker-compose.app.yml`; §5).

A volume created before this change still has the old single superuser. Migrate it
once: with the **old** `.env`, `make backup-db`; then `docker compose down`, remove the
volume (`docker volume rm infra_pgdata`), switch to the new `.env`, `docker compose up -d
postgres`, and restore both dumps with `scripts/restore-db.sh app …/securepatrol.dump`
and `scripts/restore-db.sh keycloak …/keycloak.dump`; finally `make up`.

## 2. Keycloak: users, roles, MFA

Admin console: `https://<SERVER_HOST>/auth/admin/` (bootstrap user
`KC_BOOTSTRAP_ADMIN_USERNAME`, later your personal admin — see *Master realm
hardening*) — reachable
only from `ADMIN_ALLOW_CIDRS` (default: private LAN ranges + localhost; INF-N2). From
elsewhere use an SSH tunnel: `ssh -L 8081:127.0.0.1:8081 server`, then
`http://localhost:8081/auth/admin/` (the console redirects to the public URL for login;
add your VPN range to `ADMIN_ALLOW_CIDRS` if needed, then `docker compose up -d nginx`).

Each client company is a realm (`securepatrol-client1`, …). Routine user work is
done **in the web app** (*Administration → Users*, *Tablet accounts*; NFR-09, see
"User management from the web" below); the console is only needed for the
operator (`platform-admin`) and exceptional cases. By hand, in each realm:

1. **Users → Add user** for every person; set a temporary password under *Credentials*.
2. **Role mapping → Assign role**: `admin`, `supervisor`, `hr` or `client-readonly`.
3. **Platform admins** (the operator / security-company staff): role `platform-admin`,
   see below.
4. Guards do **not** need Keycloak accounts — they are created in the web app
   (*Guards*) and unlock tablets with a PIN.

Roles only count in tokens of the matching client: web roles (`admin`, `supervisor`,
`hr`, `client-readonly`, `platform-admin`) in tokens of `securepatrol-web`
(`KEYCLOAK_WEB_CLIENT`), the `device` role in tokens of `securepatrol-tablet`
(`KEYCLOAK_TABLET_CLIENT`) — §10.

### The operator: role `platform-admin`

Notification channels (SMTP/WhatsApp credentials), ops alerts and backups are
install-wide, so a client company's `admin` cannot change them. Only users with the
realm role **`platform-admin`** can open *Settings → Channels* and *Settings → Backups*
and manage ops-alert recipients (API: `/settings/*`, `/backups/*`,
`/notifications/ops-rules`).

1. Pick the realm the operator logs in to (ideally a dedicated operator realm
   listed in `KEYCLOAK_REALMS`, otherwise the first client realm).
2. If the realm was imported before this role existed, create it:
   *Realm roles → Create role* → name `platform-admin`, then run
   `make keycloak-config` (adds it to `mfa-required`). Realm files are only imported
   when a realm is created; `infra/keycloak/realms/*.json` and the realm template
   contain the role for new installs.
3. *Users → <operator> → Role mapping → Assign role* → `platform-admin` (plus
   `admin` if the operator also manages that realm's sites). OTP is enforced
   automatically (below).
4. Set `PLATFORM_ADMIN_REALMS=<that realm>` in `.env` so a `platform-admin`
   role created by mistake in a client realm has no effect, then `make up`.
   Empty means "any trusted realm".

Never give `platform-admin` to client staff.

### MFA is enforced (INF-S2, PRD FR-AUTH-02)

`make keycloak-config` makes `admin`, `platform-admin`, `hr` and `supervisor`
composites of the role `mfa-required` and binds the browser flow `browser-mfa`
(cookie, or username + password → OTP when the user has `mfa-required`). Such users
must set up an authenticator app at their next login. Lost phone: *Users → user →
Credentials → delete the OTP credential*; the user enrols again at the next login.
`client-readonly` and `device` accounts are not asked for OTP.

### Keeping realms in sync — `make keycloak-config`

Realm JSON files are imported **only when the realm does not exist yet**. Everything
that must also reach existing realms is applied by `scripts/keycloak-config.sh`
(idempotent; `make up` and every deploy run it): web-client redirect URIs from
`PUBLIC_URL`, the MFA flow/roles, brute-force protection (client realms + master),
the `sites` user-profile attribute + mapper, the `securepatrol-admin-api` client, and
the **token role scope**: *Full scope allowed* is off on `securepatrol-web`,
`securepatrol-tablet` and `securepatrol-admin-api` (the switch is deprecated since
Keycloak 26.8, which logs a warning for every token of a client that still has it), so
web tokens carry only `admin`, `supervisor`, `hr`, `client-readonly`,
`platform-admin`, tablet tokens only `device` (+ `offline_access`), and the admin
client only its four `realm-management` roles. A realm role the app should see in
tokens must be added to the client's *Client scopes → Dedicated scope → Scope* tab
(the script does it for the roles above once they exist — re-run it after creating
`platform-admin`). The tablet client also keeps the default client scope `basic`
(claim `sub`, required by the API).
Run it by hand after changing `SERVER_HOST`, `KEYCLOAK_DEV_REDIRECTS` or a realm list:
`make keycloak-config` (or `sh scripts/keycloak-config.sh <realm>`). Other realm
changes (token lifetimes, password policy, new realm roles) are made in the console or
with `docker compose exec keycloak /opt/keycloak/bin/kcadm.sh …`.

### User management from the web (F1)

The API manages users through the Keycloak Admin REST API with one confidential
client per realm, **`securepatrol-admin-api`** (`KEYCLOAK_ADMIN_CLIENT_ID`), whose
service account holds only the `realm-management` roles `view-users`,
`query-users`, `manage-users` and `view-realm`. `make keycloak-config` creates or
repairs it in every realm and writes the client secrets to
**`infra/secrets/keycloak-admin-clients`** (`realm=secret` lines; directory 0700,
file 0644 so the API container user can read its bind-mounted copy; never
printed, git-ignored). The API reads it as the compose secret
`/run/secrets/keycloak_admin_clients` (`KEYCLOAK_ADMIN_CLIENT_SECRETS_FILE`; or
`KEYCLOAK_ADMIN_CLIENT_SECRETS=realm=secret,...`) via `KEYCLOAK_INTERNAL_URL`
(`http://keycloak:8080/auth`) and re-reads the file when a realm's secret is
missing or rejected — no restart needed after `make keycloak-config`.
`make preflight` creates the file empty on first use; until it is filled the
user pages answer 503 `kc_unavailable`.

* **Rotate a secret:** console → realm → *Clients → securepatrol-admin-api →
  Credentials → Regenerate*, then `make keycloak-config` (rewrites the file).
* **Backups:** the file is derived from Keycloak (regenerated by
  `make keycloak-config`); it need not be in the config bundle.
* Admins can create people with roles `admin`, `supervisor`, `hr`,
  `client-readonly` (never `device`; `platform-admin` only by a platform admin),
  limit them to sites, reset passwords (temporary password shown once, or an
  e-mail link — needs the realm's SMTP settings under *Realm settings → Email*),
  remove a lost authenticator (*Reset OTP*) and end sessions. Nobody can remove
  their own admin role or disable themselves.
* **Tablet accounts** (*Tablet accounts* page): `device-<name>` users with only
  the `device` role and a random 24-character password shown once — type it on
  the tablet's first login. Their e-mail is `<username>@device.invalid`
  (Keycloak's VERIFY_PROFILE refuses the password grant while a profile is
  incomplete).
  Disable the account *and* revoke the device in *Devices* when a tablet is lost.

### Site scoping (F4, D-035)

A user limited to sites gets the multi-valued user attribute **`sites`** (site
UUIDs), mapped into the access token as claim `sites` by the `securepatrol-web`
mapper *allowed sites*. `make keycloak-config` declares the attribute in the
realm's user profile (admin-only view/edit, so users cannot change it in the
account console) and creates the mapper. Set it from the web user page; empty =
the whole realm. Changes apply at the user's next token refresh (≤ 15 min).

### Master realm hardening

The master realm is the Keycloak super-admin; it is never used by the app.

1. Brute-force protection is switched on by `make keycloak-config`.
2. **Automation client first** (so the script keeps working once the admin has OTP):
   master realm → *Clients → Create* `securepatrol-automation`, *Client authentication*
   ON, *Service accounts* ON, standard flow and direct access OFF → *Service account
   roles → Assign role* `admin` → *Credentials*: copy the secret into `.env`
   (`KEYCLOAK_AUTOMATION_CLIENT_ID=securepatrol-automation`,
   `KEYCLOAK_AUTOMATION_CLIENT_SECRET=…`), run `make keycloak-config` to verify.
3. **Personal admin with OTP:** Keycloak 26 creates the bootstrap user
   (`KC_BOOTSTRAP_ADMIN_USERNAME`) as a *temporary* admin (console banner) and only on
   the very first start. Create a personal admin in the master realm (*Users → Add
   user*, role `admin`, *Required user actions → Configure OTP*), log in with it, scan
   the QR code, keep a second, sealed break-glass admin with OTP, then delete the
   temporary bootstrap admin. Installations upgraded from Keycloak 24 keep their old
   `admin` user (not marked temporary): give it OTP or replace it the same way. The
   `KC_BOOTSTRAP_ADMIN_*` values in `.env` are then no longer valid logins; that is
   fine once the automation client (step 2) is configured.
4. **Lost all admin access:** stop Keycloak and create a new temporary admin with the
   same configuration as the service:
   `docker compose stop keycloak && docker compose run --rm keycloak bootstrap-admin user --username tmp-admin --password:env KC_BOOTSTRAP_ADMIN_PASSWORD --no-prompt && docker compose up -d keycloak`
   (or `bootstrap-admin service --client-id tmp-automation --client-secret:env …`), log
   in, repair, delete it again.

### Adding a client company (new realm)

```bash
cd infra/keycloak && ./new-realm.sh securepatrol-client3 "PT Contoh"
```
The file gets the redirect URI `https://<SERVER_HOST>/*` (JSON-safe, via jq). Add the
realm to `KEYCLOAK_REALMS` and `WEB_KEYCLOAK_REALMS` in `.env`, then `make up`
(Keycloak imports new realm files on start, existing realms are untouched;
`keycloak-config` then sets redirect URIs and enforces MFA).

### Keycloak 26 settings (INF-D8, D-034)

Keycloak (`quay.io/keycloak/keycloak:26.x`, pinned by tag and digest) runs `start`
(production mode) on plain HTTP behind nginx under `/auth`:

| Setting | Value | Why |
|---|---|---|
| `KC_HOSTNAME` | `KEYCLOAK_PUBLIC_URL` (`https://<SERVER_HOST>/auth`) | Hostname v2, full URL. It is the token issuer for **every** request — also for the API's back-channel calls to `http://keycloak:8080/auth` (JWKS, admin client). `KC_HOSTNAME_BACKCHANNEL_DYNAMIC` stays off; the admin console uses the same URL. |
| `KC_HTTP_ENABLED` / `KC_HTTP_RELATIVE_PATH` | `true` / `/auth` | TLS ends at nginx; nginx proxies `/auth/` unchanged. |
| `KC_PROXY_HEADERS` | `xforwarded` | nginx overwrites `X-Forwarded-For/Proto/Host/Port` and drops a client-supplied `X-Forwarded-Prefix`. |
| `KC_PROXY_TRUSTED_ADDRESSES` | `SP_DOCKER_SUBNET`, `127.0.0.1` | Forwarded headers count only from the stack network (nginx). |
| `KC_HEALTH_ENABLED` / `KC_METRICS_ENABLED` | `true` | Served on the **management port 9000**, path `/auth/health/ready`, `/auth/metrics` (the management port inherits `/auth`). Port 9000 is neither published nor proxied; the container healthcheck and Prometheus (`keycloak:9000`) use it. Port 8080 answers 404 for both. |
| `KC_BOOTSTRAP_ADMIN_USERNAME` / `_PASSWORD` | `.env` | Temporary master admin, created only on the very first start (Keycloak 24: `KEYCLOAK_ADMIN*`; preflight rejects the old names). |

Keycloak opens its ports while it is still migrating or importing; only
`/health/ready` says it serves requests (the healthcheck uses it, `start_period` 3 min).
The stock image re-augments itself for the build options (`KC_DB`, health, metrics)
on every start (~30 s); no custom image is built.

Token contract the API, web app and tablets rely on (checked by
`scripts/keycloak-smoke.sh` in CI): access tokens with `typ: Bearer`, `azp` = the
client, `sub` (client scope `basic`), realm roles in the flat claim `roles` (client
mapper *realm roles (flat)*, limited by the client's role scope — §2 above), the web
claim `sites`, issuer `KEYCLOAK_PUBLIC_URL/realms/<realm>`. Tablets use the password
grant with `scope=openid offline_access` (direct access grants only on
`securepatrol-tablet`); Keycloak 26 answers wrong credentials with HTTP 400 (24: 401)
— the tablet app treats both as "bad credentials". Offline refresh tokens now carry an
`exp` (= offline session idle, 30 days).

### Upgrading Keycloak (24 → 26, and every later 26.x release)

Every Keycloak release migrates its database schema **automatically and one way** on
its first start; an older Keycloak cannot open a migrated database. Test on a copy
first (§5 restore drill on a spare machine with a production dump), then:

1. **Announce a short logins freeze** (5–10 min). Tablet offline tokens survive
   (offline sessions live in the database); from 24, web users have to log in again
   (Keycloak 24 kept online sessions in memory — from 26 on they are persisted and
   survive later upgrades). Tablets keep working offline anyway.
2. **Back up:** `make backup-db` (the deploy script does it too: `pre-<version>-…`).
   Note the folder — it holds `keycloak.dump`, the rollback point.
3. **`.env` (24 → 26 only):** rename the bootstrap admin variables —
   `sed -i 's/^KEYCLOAK_ADMIN=/KC_BOOTSTRAP_ADMIN_USERNAME=/; s/^KEYCLOAK_ADMIN_PASSWORD=/KC_BOOTSTRAP_ADMIN_PASSWORD=/' .env`
   (`make preflight` fails until this is done). Nothing else in `.env` changes.
4. **Deploy** the release that contains the new image (`scripts/deploy.sh vX.Y.Z` or
   `make up`). The first start takes longer (schema migration; with very large
   session tables Keycloak may log `CREATE INDEX` statements and create them in the
   background). `docker compose logs -f keycloak` shows the progress; the deploy
   waits until the healthcheck is green and then runs `make keycloak-config`, which
   switches *Full scope allowed* off, adds the role scope mappings and the tablet's
   `basic` scope.
5. **Check:**
   * `sh scripts/keycloak-smoke.sh <realm>` for every realm — discovery issuer, the
     API's admin client, a tablet password grant + offline refresh, a web login with
     PKCE incl. the `sites` claim, and the OTP prompt for staff (it creates temporary
     users and deletes them again);
   * a real web login with OTP, *Administration → Users* lists users, one tablet
     syncs (no re-login needed);
   * Grafana / Prometheus: target `keycloak` up (`keycloak:9000`);
   * `docker compose logs keycloak | grep -i -E 'warn|error'` — no warnings about
     unknown options or *Full scope allowed*.
6. **Rollback** (only if the checks fail and cannot be fixed forward) — the old image
   plus the pre-upgrade dump, never the old image on the migrated database:
   1. `git checkout --detach refs/tags/<previous>` (its compose file has the old image);
      for 24 rename the two `.env` variables back to `KEYCLOAK_ADMIN*`;
   2. `RESTORE_FRESH=1 RESTORE_CONFIRM=keycloak sh scripts/restore-db.sh keycloak backups/<time>-pre-…/keycloak.dump`
      (stops Keycloak, empties the schema so no table of the newer version survives,
      restores in one transaction, starts the old image);
   3. `scripts/deploy.sh <previous>` (or `make up`), then `make keycloak-config`.

   Password changes, OTP enrolments, new users and sessions since the upgrade are lost.

PostgreSQL majors (16 → 17) likewise need a planned dump/restore migration, and the
API image's `pg_dump` (`postgresql16-client`) must be upgraded with the server.

## 3. Tablets (shared per site — D-001)

1. In the site's realm create a user like `tablet-gudang-a-1`, set a strong password,
   assign realm role **`device`** (nothing else).
2. Install the app (APK from the CI release, signed with the release key; with a
   private CA it must be a build that bundles `securepatrol-ca.crt`, see *Kiosk/MDM*
   below), then log in once with that account; it registers itself and shows
   "waiting for approval". The server address must be `https://`.
3. Web app → *Settings → Devices*: approve the tablet and pick its site.
4. Web app → *Guards*: make sure each guard is assigned to the site and has a PIN.
   The tablet syncs the roster and guards unlock it with their PIN (works offline).
5. Lost/stolen tablet: *Settings → Devices → Revoke* (takes effect immediately), and
   disable the Keycloak user. On its next contact the tablet deletes the guard list
   (PIN hashes), routes and guard tokens; unsent records stay until they are sent or
   discarded. A tablet that has not synced its guard list for **7 days** refuses PIN
   unlocks by itself (SOS still works).

**Verified guards (D-035).** When the tablet is online, unlocking also checks the
PIN on the server (`POST /device/guard-sessions`) and the tablet sends the returned
guard token with every request. Records made without a valid token (offline unlock)
are flagged `guard_unverified`; unverified clock-ins land in the attendance review
queue. Five wrong PINs in a row lock that guard's server PIN check for 15 minutes
(on every tablet); setting a new PIN in *Guards* unlocks immediately. Failed and
locked attempts are in the audit log (`resource_type=device.guard-sessions`).

**Checkpoint QR codes (D-035).** New checkpoints get signed codes
(`SP1:<checkpoint>:<signature>`); the server verifies every scan and tablets never
receive the codes, so a code cannot be scanned without the printed sticker. Old
plain codes keep working but their scans are flagged `qr_unsigned`. To switch a
route to signed codes — or after a sticker was copied or lost — use *Regenerate QR
codes* on the route (`POST /patrol/routes/:id/regenerate-qr`) and **reprint the
whole sheet**: every previously printed code of that route is then flagged
`qr_invalid`. Changing `QR_SIGNING_SECRET` (or `SETTINGS_ENCRYPTION_KEY` while it
is unset) invalidates all signed stickers of all routes — regenerate and reprint.

**Patrol schedules (D-035).** Give a route schedule windows (e.g. day 08:00–16:00
every 60 min, night 22:00–06:00 every 120 min, grace 15 min). The worker plans the
patrols 48 h ahead; a patrol not started within its grace period raises
`patrol_not_started` (add a notification rule for it) — also when the tablet is
offline, because nobody may be patrolling. A session synced later still counts
(late starts are marked `late_start`). *Reports → Patrol compliance* shows
expected / on time / late / missed per route and day.

**On the tablet (app behaviour an operator should know).**
* *Protected actions:* *Sign out device*, *Change server* and *Remove from outbox*
  need a supervisor PIN (a roster guard with role `supervisor`) or the tablet
  account's password. Sign-out and server change are refused while records are still
  queued — send them first (or have a supervisor remove them one by one).
* *SOS without PIN:* on the lock screen a guard taps their name and holds **SOS**;
  the alert is sent at once (flagged `guard_unverified`).
* *Outbox:* items the server rejects are shown as "need attention" on the home
  screen with the reason; items failing with server errors are parked after 50
  attempts / 3 days so they cannot block the queue; a pending/revoked device pauses
  sending without losing anything.
* *Clock:* event times use the server's clock (offset from every response); the
  home screen warns when the tablet clock is more than 2 minutes off. Times are
  shown in the site's time zone.
* *Local data* (guard list, routes, queued records) is encrypted with a key held in
  the Android Keystore-backed secure storage; the app is excluded from Android
  backups and device transfer, and blocks screenshots/recent-apps previews.
* *Health beacon:* every 15 min the tablet posts app version, queue depth, failed
  items, last sync, clock offset, repeated PIN failures and its last errors to
  `POST /api/v1/device/telemetry` (the endpoint must exist on the server; until then
  the tablet retries every 6 h).
* *Push notifications (optional):* an APK built with the Firebase values
  (`SP_FCM_*` secrets, mobile/README.md) registers its FCM token with the server and
  reacts at once to an SOS from another post, approval/revocation/move and roster
  changes; without them, or while push is unhealthy, it polls as before (SOS every
  15 s, 30 s on the lock screen; with healthy push every 60 s — SOS never depends on
  push alone). Android 13+ asks for the notification permission right after the
  tablet registers — allow it. *About this tablet* shows the push status (Active /
  Not configured / Permission denied / Token not sent yet) with *Ask for permission
  again*; the health beacon reports it as `push`. Server side (Firebase service
  account, sending): see runbook: *Push notifications*.

### Kiosk / MDM setup (dedicated device)

A post tablet should run SecurePatrol and nothing else. Preferred: **Android
Enterprise dedicated device** (a.k.a. COSU) through an EMM — e.g. Google Android
Management API, Microsoft Intune "Android Enterprise dedicated devices", Samsung
Knox Manage, Scalefusion, Hexnode, ManageEngine MDM. Steps:

1. **Factory-reset and enrol** the tablet as *fully managed / dedicated* (QR code
   or `afw#<emm>` at the welcome screen). Do not add a personal Google account.
2. **Install the app** as a private/managed app: upload the release APK
   (`securepatrol-tablet-<version>+<build>.apk` from the CI release, package
   `id.co.securepatrol.tablet`) to the EMM; set it to install automatically and
   **auto-update** (managed Google Play private app, or the EMM's own app
   repository for APK updates). Updates keep the local data as long as the signing
   key stays the same — keep the release keystore safe (CI secrets).
   *Upgrading from the pre-hardening build (package `id.co.securepatrol.securepatrol`):
   it is a different app; let its outbox empty first, then uninstall it.*
3. **Lock-task (kiosk) mode:** single-app kiosk with SecurePatrol as the only
   allowed app (Android Management API: `kioskCustomLauncherEnabled` off,
   `applications[].installType: KIOSK`; Intune: *Single app, full screen*). Allow the
   status bar only for system notifications if your EMM offers it; keep the camera
   and location permissions **granted by policy** (`permissionGrants` /
   *Auto grant*) so no dialog interrupts a guard.
4. **Disable Settings access:** block *Settings*, the notification shade quick
   settings, factory reset from the UI, USB debugging/developer options, installing
   unknown apps, adding users/accounts, and safe-mode boot. Leave Wi-Fi configured
   by policy (or a managed APN for SIM tablets) and screen timeout long.
5. **Managed configuration / pre-set server:** the app does not read EMM managed
   configurations (yet). Build the APK with the installation's defaults instead —
   `flutter build apk --release --dart-define=SP_SERVER=https://patrol.example.co.id
   --dart-define=SP_REALM=securepatrol-client1` (the first-run screen then only
   needs *Continue*). One build per installation is normal anyway because of the CA.
6. **Private CA:** copy `infra/nginx/certs/securepatrol-ca.crt` to
   `mobile/assets/certs/securepatrol-ca.crt` before the build (git-ignored; in CI
   write it from a secret). The app's HTTPS (Dart) ignores CAs installed in Android
   settings, and release builds trust only system CAs at the Android level. Without
   the file only public certificates (e.g. Let's Encrypt) work.
7. **Time:** enable automatic date/time and time zone by policy.

**No EMM (small sites): screen pinning fallback.** *Settings → Security → App
pinning* (or *Pin windows*) → on, "Ask for PIN before unpinning" → on with a device
PIN only the supervisor knows; open SecurePatrol → Overview → app icon → *Pin*.
Also: remove other apps or hide them, disable developer options, set a device PIN
(the app stays usable while pinned), and turn on auto date/time. This prevents
casual use but is weaker than lock-task mode (anyone with the device PIN can unpin).

## 4. Notifications

*Settings → Channels* (`platform-admin` only): SMTP and WhatsApp credentials
(stored encrypted with `SETTINGS_ENCRYPTION_KEY`). Use **Send test** after saving; a
failure shows a short reason (`auth_failed`, `timeout`, `destination_not_allowed`, …)
— the full error is in the API log (`make logs`, message "test notification failed").

Guard rails (protect the stored secrets from being sent elsewhere):
- Changing the SMTP host, port or user, or the WhatsApp API URL or phone
  number ID, requires typing the password/token again.
- SMTP ports: 25, 465, 587 or 2525. A host entered in the UI must resolve to a
  public address; an on-prem relay on a private network must be allow-listed
  with `SMTP_ALLOWED_PRIVATE_NETS=10.0.5.20` (IPs or CIDRs, comma-separated).
  The relay configured in `.env` (`SMTP_HOST`) is always trusted. The check is
  repeated when connecting, so DNS changes cannot bypass it.
- The WhatsApp API URL must start with `https://graph.facebook.com/`; for an
  on-prem gateway set `WHATSAPP_API_URL_ALLOWLIST=https://wa-gw.example.local/`.

WhatsApp Business Cloud API only delivers business-initiated messages through an
**approved template**. Create one in Meta Business Manager with a single body
variable, e.g. `SecurePatrol: {{1}}`, and set its name/language in Settings.

*Settings → Notification rules*: per site, which events go to which e-mail/phone.
*Settings → Notification log*: failed deliveries show the error and can be retried
(one more attempt).

How delivery works: every notification is written to `notification_log` in the
same database transaction as the SOS/incident/... that caused it. The worker
claims due rows straight from Postgres (several sender goroutines,
`NOTIFY_WORKERS`, default 4); one sender serves only SOS so an SOS never waits
behind a slow mail server. Redis only wakes the senders up faster — with Redis
down, notifications still go out within ~2 s. Every send has a 30 s timeout.
SOS deliveries retry 10 s, 20 s, 30 s, then every minute for 30 minutes before
they are marked `failed`; other events retry after 30 s, 2, 10 and 30 minutes.

Operator alerts come from two independent paths — configure both before go-live:

* **Ops alerts from the app** (install-wide, not per site, sent through the channels
  above): a `platform-admin` configures recipients via
  `/api/v1/notifications/ops-rules` (web UI to follow) for
  `ops_backup_failed`, `ops_backup_stale` (no successful backup for 26 h),
  `ops_sos_undelivered` (an SOS delivery gave up, or an SOS fired at a site with
  no SOS rule) and `ops_job_failing` (a scheduled job failed 5 times in a row).
  Configure at least one e-mail **and** one WhatsApp recipient for
  `ops_sos_undelivered` and the backup alerts.
* **Alertmanager** (§8) for the platform itself — worker down, SOS unacknowledged,
  notifications failing, backups, disk, certificates — e-mailed via `ALERT_*` in
  `.env`; it also works when the app's own channels are broken.

## 5. Backups and restore (NFR-07, INF-B1–B6)

**Targets:** RPO **24 h** (at most one day of data lost), RTO **4 h** (service back on
replacement hardware within four hours, given the off-site disk and the secrets).

| Layer | What | Where | Kept | Alert |
|---|---|---|---|---|
| Daily dump (worker, `BACKUP_HOUR` in `BACKUP_TZ`, also *Settings → Backups → Back up now*) | app DB + Keycloak DB (`pg_dump -Fc`) | MinIO bucket `MINIO_BUCKET_BACKUPS` (versioned): `postgres/securepatrol-<UTC time>.dump`, `keycloak/keycloak-<UTC time>.dump` | `BACKUP_RETENTION_DAYS` (35 d), old versions +7 d | `BackupStale` > 26 h; ops alerts `ops_backup_failed`, `ops_backup_stale` |
| Off-site (`backup-offsite`, `OFFSITE_SCHEDULE`, default 03:30) | backups bucket, **photos** bucket, config bundle (`.env`, `nginx/certs` incl. CA key, realm files) | restic repo on `OFFSITE_BACKUP_REPO` (encrypted with `OFFSITE_RESTIC_PASSWORD`) | 30 daily + 12 monthly; `restic check` monthly | `OffsiteBackupStale`, `OffsiteBackupFailed`, `OffsiteCheckFailed` |
| Pre-deploy (`scripts/deploy.sh`, `make backup-db`) | app DB + Keycloak DB | `infra/backups/<time>-<label>/` | newest 10 | — |

Daily dump details:
* `BACKUP_TZ` (default `Asia/Jakarta`) is independent of the container `TZ`.
* The Keycloak database is included because `docker-compose.app.yml` sets
  `BACKUP_KEYCLOAK_DATABASE_URL` for the worker (`KC_DB_USER`, the Keycloak DB owner).
  The dump is listed as `keycloak_object_key` in *Settings → Backups*. Outside this
  Compose stack, set it yourself (`postgres://<user>:<password>@postgres:5432/keycloak?sslmode=disable`)
  or run `make backup-db` after user changes.
* Each dump is limited by `BACKUP_TIMEOUT` (default `1h`). The database password is
  passed to `pg_dump` via `PGPASSWORD`, never on the command line. The image's
  `pg_dump` matches the PostgreSQL 16 server.
* A failed backup raises the ops alert `ops_backup_failed`; no successful backup for
  26 h raises `ops_backup_stale` (once a day) and the `BackupStale` alert. Backups left
  "running" by a crashed worker are marked failed automatically when the worker
  restarts.
* *Back up now* and the backup list need role `platform-admin`.

Off-site and secrets:
* `OFFSITE_BACKUP_REPO` must be a different physical disk or a NAS (preflight warns if
  it is on the Docker data disk). For a copy outside the building, rotate two USB disks
  weekly (each its own restic repo) or sync the repo to a second site.
* Without `OFFSITE_RESTIC_PASSWORD` the off-site copy is unreadable, and without
  `SETTINGS_ENCRYPTION_KEY` saved SMTP/WhatsApp secrets are — keep `.env` in the
  password manager.
* Manual runs: `make offsite-backup`, `make offsite-check`, `make offsite-snapshots`.

**Encryption at rest (INF-B6):** put `/var/lib/docker` (database, photos, dumps) and
the backup disk on LUKS-encrypted volumes (`cryptsetup luksFormat`, unlock at boot
with a key file on the root disk or a passphrase via console/Clevis-Tang). restic
encrypts its repository itself; `.env` and `nginx/certs/*-key.pem` are mode 600.

### Restore one database (e.g. bad data, failed migration)

```bash
cd infra
# newest dump from the backups bucket (or infra/backups/<time>-pre-…/):
docker compose run --rm --no-deps --entrypoint sh minio-init \
  -c 'mc ls sp/$MINIO_BUCKET_BACKUPS/postgres/ | tail -3'      # Keycloak: …/keycloak/
docker compose run --rm --no-deps --entrypoint sh -v "$PWD/backups:/out" minio-init \
  -c 'mc cp sp/$MINIO_BUCKET_BACKUPS/postgres/<file>.dump /out/restore.dump'
sh scripts/restore-db.sh app backups/restore.dump        # asks for confirmation
# Keycloak: sh scripts/restore-db.sh keycloak <keycloak dump>
```
`restore-db.sh` stops the services using that database (api + worker, or Keycloak),
restores it as the owning role (`--clean --single-transaction`) and starts them again.
A dump taken **before an upgrade** (Keycloak 26 → 24, or an app version with newer
migrations) goes back with `RESTORE_FRESH=1`: the schema is emptied in the same
transaction, so no table of the newer version survives — and start the matching older
image (§2 *Upgrading Keycloak*, §7). A Keycloak dump is always opened by the same or a
newer Keycloak (newer ones migrate it again on start), never by an older one.
Practise the restore on a spare machine at least once before go-live (drill below).

### Full restore drill (disaster recovery, ≤ 4 h)

Practise this on a spare machine before go-live and then **every quarter** (record the
date, duration and problems in the operations log).

1. **Server:** install Docker, `make`, `jq`, `openssl`, `git`; clone the repo and check
   out the deployed tag (`git checkout --detach refs/tags/vX.Y.Z`).
2. **Off-site disk:** mount it, e.g. at `/mnt/securepatrol-offsite`.
3. **Config bundle** (needs `OFFSITE_RESTIC_PASSWORD` from the password manager):
   ```bash
   export RESTIC_PASSWORD='…'   # typed, not saved in history: use `read -rs RESTIC_PASSWORD`
   docker run --rm -e RESTIC_PASSWORD -v /mnt/securepatrol-offsite:/repo:ro -v /tmp/sp-restore:/restore \
     restic/restic:0.19.1 -r /repo restore latest --target /restore
   cd /opt/securepatrol/infra
   install -m 600 /tmp/sp-restore/cache/staging/config/infra.env .env
   mkdir -p nginx/certs && cp -a /tmp/sp-restore/cache/staging/config/certs/. nginx/certs/
   ```
   If the server gets a new address, change `SERVER_HOST` and run `make certs`
   (devices must then trust the new CA).
4. **Databases:** `docker compose up -d postgres`, wait until healthy, then
   ```bash
   B=/tmp/sp-restore/cache/staging/$(grep ^MINIO_BUCKET_BACKUPS= .env | cut -d= -f2)
   ls -t $B/postgres/ | head -3            # newest app dump
   ls -t $B/keycloak/ | head -3            # newest Keycloak dump (same run)
   RESTORE_CONFIRM=securepatrol sh scripts/restore-db.sh app $B/postgres/<newest>.dump
   RESTORE_CONFIRM=keycloak sh scripts/restore-db.sh keycloak $B/keycloak/<newest>.dump
   ```
   (If no Keycloak dump exists, use the newest pre-deploy dump or re-create users from
   the realm files.)
5. **Photos and dump history:**
   ```bash
   docker compose up -d minio minio-init
   P=$(grep ^MINIO_BUCKET_PHOTOS= .env | cut -d= -f2)
   docker compose run --rm --no-deps --entrypoint sh -v /tmp/sp-restore/cache/staging:/restore:ro \
     minio-init -c 'mc mirror --overwrite /restore/$MINIO_BUCKET_PHOTOS sp/$MINIO_BUCKET_PHOTOS && mc mirror --overwrite /restore/$MINIO_BUCKET_BACKUPS sp/$MINIO_BUCKET_BACKUPS'
   ```
6. **Start everything:** `make up` (or `infra/scripts/deploy.sh vX.Y.Z` for registry
   images). Check: web login with OTP, a tablet syncs, photos of last week open,
   *Settings → Backups* shows history, Grafana shows all targets up.
7. `shred -u` / `rm -rf /tmp/sp-restore` when done — it contains secrets.

**Test-restore without disaster:** run steps 1–6 on a spare VM against the real
off-site disk (read-only mount); compare row counts of a few tables
(`docker compose exec postgres psql -U postgres -d securepatrol -c 'select count(*) from incidents'`)
with production. `make offsite-check` verifies 10 % of the stored data.

## 6. Certificates (INF-D3, INF-N5)

`make certs` creates a **private CA** (`nginx/certs/ca.pem`, 10 years, name-constrained
to `SERVER_HOST` + localhost, so it cannot be used to impersonate other websites) and a
server certificate (`cert.pem`/`key.pem`, 825 days, SAN = `SERVER_HOST` + localhost).

Install the CA once per device (file `nginx/certs/securepatrol-ca.crt`):
* **Android tablets:** the SecurePatrol app does **not** trust user-installed CAs
  (Dart's TLS ignores the Android store, and user CAs would allow interception). Bundle
  the CA into the APK instead: GitHub secret `SECUREPATROL_CA_PEM` = contents of
  `securepatrol-ca.crt` (the release job writes it to `mobile/assets/certs/`), or copy it
  there before a local build — see §3. Installing it in Android settings is only needed
  for the tablet's browser.
* **Windows:** double-click → *Install certificate → Local machine → Trusted Root
  Certification Authorities*.
* **macOS:** Keychain Access → System → import → *Always trust*.
* Firefox uses its own store: *Settings → Certificates → Import*.

**Renewal:** the `CertificateExpiresSoon` alert fires 21 days before expiry. Run
`make certs-renew` (new server certificate from the same CA — devices need no change);
it reloads nginx. The CA itself lasts 10 years.

**Public / ACME certificate (optional):** if the server has a public DNS name, use
certbot with the webroot nginx already serves on port 80:
```bash
docker run --rm -v /etc/letsencrypt:/etc/letsencrypt -v /opt/securepatrol/infra/acme:/var/www/certbot \
  certbot/certbot certonly --webroot -w /var/www/certbot -d patrol.example.co.id
```
(mount `./acme:/var/www/certbot:ro` into the nginx service with a small
`docker-compose.override.yml` and append that file to `COMPOSE_FILE` in `.env` —
with `COMPOSE_FILE` set, Compose does not load the override file by itself), then copy `fullchain.pem` → `nginx/certs/cert.pem`,
`privkey.pem` → `key.pem`, the issuing **root** (e.g. ISRG Root X1) → `ca.pem`, and
`docker compose exec nginx nginx -s reload`. Renew with a monthly cron job doing the
same. Tablets then need no CA installation.

## 7. Releases, deploys and rollback (INF-C1/C2/C5, INF-M4)

Flow: merge to `init` → CI green (backend, web, infra, **e2e stack**) → tag
`vX.Y.Z` → workflow *CD — Release & Deploy* verifies the tag is on `init` and that all
required CI runs succeeded for that commit → builds and pushes
`ghcr.io/<owner>/securepatrol/{api,web,backup-offsite}:vX.Y.Z` (SBOM + provenance,
Trivy) → waits for approval in the **production** environment → SSH to the server →
`infra/scripts/deploy.sh vX.Y.Z`.

`deploy.sh` refuses a dirty work tree, preflights, pulls the images by tag and checks
their digests, **dumps both databases** (`infra/backups/<time>-pre-vX.Y.Z-from-vA.B.C/`),
runs `docker compose up -d --wait`, `keycloak-config`, writes `VERSION`/`IMAGE_REPO`
into `.env`, and keeps the images of the 3 newest versions (never the previous one).
Images are never built on the server. The API applies pending DB migrations on start
(as `APP_DB_USER`, §1); the worker starts only once the API is healthy.

Without the registry (`VERSION=dev`, e.g. a test server): `git pull && make up`
rebuilds the images locally — take `make backup-db` first.

### One-time GitHub setup

* **Settings → Environments → production:** *Required reviewers* = you (or a second
  person); *Deployment branches and tags* → *Selected* → tag rule `v*`.
* **Settings → Rules → Rulesets:** a tag ruleset for `v*` (restrict creation/update/
  deletion to maintainers, block force-push) and a branch ruleset for `init` (PR + CI
  checks required, code-owner review, no force-push).
* **Environment secrets (production):**

| Secret | Required | Meaning |
|---|---|---|
| `DEPLOY_HOST`, `DEPLOY_USER`, `DEPLOY_KEY` | yes | SSH target and private key (deploy user in the `docker` group) |
| `DEPLOY_HOST_FINGERPRINT` | yes | SSH host key fingerprint: `ssh-keyscan -t ed25519 <host> \| ssh-keygen -lf - \| awk '{print $2}'` (`SHA256:…`) |
| `DEPLOY_PORT` | no | SSH port (default 22) |
| `DEPLOY_PATH` | no | repo path on the server (default `/opt/securepatrol`) |
| `SMTP_HOST`, `SMTP_PORT`, `SMTP_USER`, `SMTP_PASSWORD`, `SMTP_FROM`, `DEPLOY_NOTIFY_EMAIL` | no | deploy result e-mail; skipped when unset |

Registry access needs nothing extra: the job passes its short-lived `GITHUB_TOKEN`
(`packages: read`) to the server for the pull and logs out afterwards.

* **Repository secrets for the Android release** (ci-mobile, optional):
  `ANDROID_KEYSTORE_BASE64` (`base64 -w0 upload-keystore.jks`), `ANDROID_KEYSTORE_PASSWORD`,
  `ANDROID_KEY_ALIAS`, `ANDROID_KEY_PASSWORD`. Without them the release job is skipped
  with a notice; a debug-signed APK is never published.

### Server preparation

Clone to `/opt/securepatrol` with a read-only deploy key (the deploy user must be able
to run `docker` and `git fetch`), create `infra/.env` (`make env`), set
`IMAGE_REPO=ghcr.io/<owner>/securepatrol` in it, `make certs`, mount the off-site disk.
Do not edit tracked files on the server — the deploy refuses a dirty work tree;
machine-specific settings belong in `.env`.

### Release / rollback

* Release: `git tag v1.2.3 && git push origin v1.2.3` (or *Run workflow* with an
  existing tag).
* **Rollback without schema change:** run the workflow with the previous tag
  (`cat infra/.deploy/previous` on the server) — its images are still on the server.
* **Rollback across a migration:** migrations only go forward. Stop the app
  (`docker compose stop api notification-worker`), restore the pre-deploy dump
  (`sh scripts/restore-db.sh app infra/backups/<time>-pre-vX.Y.Z-…/securepatrol.dump`,
  same for keycloak if needed), then deploy the previous tag. Data written between
  the deploy and the rollback is lost — announce a short freeze for risky releases.
* Never run `docker image prune -a` on the server: it would delete the rollback images.

## 8. Monitoring (INF-M1/M2, BE-O1)

Grafana `http://127.0.0.1:3001`, Prometheus `:9090`, Alertmanager `:9093` — bound to
localhost; use `ssh -L 3001:127.0.0.1:3001 -L 9090:127.0.0.1:9090 server`.
Dashboards (folder *SecurePatrol*, provisioned from `infra/grafana/dashboards`, edit
by exporting JSON and committing): **Overview** (SOS, worker heartbeat, notifications,
API), **Jobs, backups, certificates**, **Host & containers**.

Health and metrics endpoints of the app:

| Endpoint | Where | Meaning |
|---|---|---|
| `GET /health` | API `:8080` (also via nginx) | DB + Redis reachable |
| `GET /ready` | API `:8080` (also via nginx, status only) | DB, Redis and MinIO reachable (503 otherwise) |
| `GET /metrics` | API `:8080`, internal only | Prometheus: HTTP metrics + Go/process |
| `GET /health` | worker `WORKER_HTTP_ADDR` (`:9091`), internal only | 200 only if the notification dispatcher and the scheduler ticked within 60 s (container healthcheck) |
| `GET /metrics` | worker `:9091`, internal only | Prometheus: notification, job and backup metrics |
| `GET /auth/health/ready` (`/live`, `/started`) | Keycloak management port `:9000`, internal only | Keycloak serves requests (database reachable, start-up/migration done); container healthcheck |
| `GET /auth/metrics` | Keycloak `:9000`, internal only | Prometheus: JVM, HTTP, database pool, cache metrics |

Keycloak 26 serves health and metrics only on its management port 9000, which is not
published on the host and not proxied; its main port 8080 answers 404 for both. nginx
answers 404 for `/metrics`, `/api/metrics`, `/auth/metrics` and `/auth/health` anyway,
so metrics are never reachable from outside. By hand:
`docker compose exec keycloak sh -c 'exec 3<>/dev/tcp/127.0.0.1/9000 && printf "GET /auth/health/ready HTTP/1.1\r\nHost: x\r\nConnection: close\r\n\r\n" >&3 && cat <&3'`.

Scraped: api `:8080/metrics`, worker `:9091/metrics`, Keycloak `:9000/auth/metrics`, MinIO, node-exporter
(host, incl. the off-site textfile metrics), cAdvisor (containers), postgres/redis
exporters, blackbox probes of `https://SERVER_HOST` and `:9443` (TLS chain + expiry).
Alert rules: `infra/prometheus/alerts.yml`; e-mail via `ALERT_EMAIL_TO` + `ALERT_SMTP_*`
(empty = alerts only in the UI). Critical alerts repeat hourly until resolved.

Metric contract with the backend (alert rules and dashboards depend on it):

- API: `sp_http_requests_total{method,route,status}`,
  `sp_http_request_duration_seconds{method,route}` (route = route pattern).
- Worker: `sp_notifications_total{channel,event,result=sent|retry|failed}`,
  `sp_notification_queue_depth{lane=sos|normal}`, `sp_notifications_failed_total{event}`,
  `sp_sos_unacknowledged`, `sp_worker_heartbeat_timestamp_seconds`,
  `sp_job_runs_total{job,result=success|error|skipped}`,
  `sp_job_last_success_timestamp_seconds{job}`,
  `sp_backup_last_success_timestamp_seconds` (0 = no successful backup yet).
- Off-site job (node-exporter textfile): `sp_offsite_backup_last_result`,
  `sp_offsite_backup_last_success_timestamp_seconds`, `sp_offsite_check_last_result`,
  `sp_offsite_check_last_success_timestamp_seconds`.
- Both app processes: Go and process collectors (`go_*`, `process_*`).

Failure results are matched as `result=~"error|fail.*"`. The backend label `job`
collides with Prometheus' target label and appears as **`exported_job`** in queries.

Worker and API tuning (`.env`; defaults in brackets): `DB_MAX_CONNS` [20, per process],
`DB_STATEMENT_TIMEOUT` [30s], `NOTIFY_WORKERS` [4, min 2], `BACKUP_HOUR` [2],
`BACKUP_TZ` [Asia/Jakarta], `BACKUP_TIMEOUT` [1h]. Fixed in `docker-compose.app.yml`:
`WORKER_HTTP_ADDR` [:9091 — healthcheck and Prometheus use it],
`WORKER_SHUTDOWN_TIMEOUT` [40s; the worker container has `stop_grace_period: 60s`],
`BACKUP_KEYCLOAK_DATABASE_URL` (§5).

Logs: every 5xx is logged with its `request_id` (also returned to the client);
notification deliveries log the `request_id` of the API call that caused them.
Docker logs are capped at 5 × 10 MB per container (`x-logging`), so they cannot fill
the disk; `DiskAlmostFull` fires at 85 %.

## 9. Component notes

* **Object storage (INF-D2):** MinIO Inc. stopped publishing community images
  (`minio/minio` no longer exists on Docker Hub). The stack uses **Silo**
  (`pgsty/silo`, the Pigsty-maintained fork of the last open-source MinIO; same
  `MINIO_*` variables, `/minio/*` routes, metrics and on-disk format) and `pgsty/mc`,
  pinned by tag **and digest**. Before each upgrade read the fork's release notes.
  Alternatives if the fork stalls: build MinIO/Silo from source in CI from a pinned
  tag (we already push images to GHCR), or migrate to another S3 server (Garage,
  SeaweedFS, Ceph RGW) — the API only needs S3 GET/PUT, presigned GET and versioning;
  migrate data with `mc mirror`.
* **Images** are pinned to minor/patch tags (Silo, mc and Keycloak also by digest);
  Dependabot proposes updates (Keycloak and Postgres majors are ignored — they need a
  planned migration, §2). Keycloak minor/patch updates migrate its database too: deploy
  them with the §2 *Upgrading Keycloak* steps (backup, smoke test).
* **Container hardening:** `no-new-privileges` everywhere; api/worker/web read-only
  with tmpfs and no capabilities (api/worker run as a non-root user); edge nginx
  read-only; exporters read-only without capabilities; memory limits on Keycloak,
  MinIO, Prometheus, Grafana (`*_MEM_LIMIT` in `.env`). cAdvisor runs privileged
  (host metrics).
* **Least-privilege object storage (INF-S5):** with `MINIO_APP_ACCESS_KEY/SECRET_KEY`
  set (default for `make env`), `minio-init` creates a user limited to the photos and
  backups buckets (no delete) for the API/worker, and with `MINIO_OFFSITE_*` a
  read-only user for the off-site job. Empty = root credentials (old behaviour).
  Uploaded photos cannot be replaced (the API refuses overwrites, the photos bucket
  is versioned); EXIF/GPS metadata is removed before storage.
* **nginx:** upstreams re-resolve via Docker DNS (no 502 after redeploys); every
  location sends the full proxy header set (`snippets/proxy-common.conf`):
  `X-Real-IP $remote_addr`, and `X-Forwarded-For` is the client address only (a
  client-supplied value is never appended). Rate limits for the API (30 r/s, burst
  200, SOS never limited), the token endpoint (1 r/s, burst 30) and `/auth`; the API
  adds its own per-user/per-tablet limits (§10).

## 10. Security settings, audit log and limits (BE-S*)

| Variable | Default | Meaning |
|---|---|---|
| `KEYCLOAK_WEB_CLIENT` | `securepatrol-web` | Web roles (admin, supervisor, hr, client-readonly, platform-admin) only count in tokens of this client (fixed in `docker-compose.app.yml`) |
| `KEYCLOAK_TABLET_CLIENT` | `securepatrol-tablet` | The `device` role only counts in tokens of this client (fixed in `docker-compose.app.yml`) |
| `PLATFORM_ADMIN_REALMS` | *(empty = any)* | Realms whose `platform-admin` users manage channels, ops alerts and backups (§2) |
| `TRUSTED_PROXIES` | `.env`: `${SP_DOCKER_SUBNET}` (172.28.0.0/24); backend default without it: `10.0.0.0/8,172.16.0.0/12,192.168.0.0/16,127.0.0.1/32,::1/128` | Peers whose `X-Real-IP` header is believed (nginx); `X-Forwarded-For` is ignored |
| `WHATSAPP_API_URL_ALLOWLIST` | `https://graph.facebook.com/` | Allowed WhatsApp API URL prefixes (comma-separated) |
| `SMTP_ALLOWED_PRIVATE_NETS` | *(empty)* | Private IPs/CIDRs an SMTP host entered in the UI may resolve to |
| `RATE_LIMIT_USER_WRITES_PER_MIN` | `120` | Writes per web user per minute (0 = off) |
| `RATE_LIMIT_DEVICE_WRITES_PER_MIN` | `600` | Writes per tablet per minute — high enough for an offline backlog |
| `RATE_LIMIT_SOS_PER_MIN` | `5` | New SOS alerts per tablet per minute (retries are never limited) |
| `RATE_LIMIT_AUTH_FAILURES_PER_MIN` | `30` | Failed logins/invalid tokens per client IP before 429 |
| `RATE_LIMIT_GUARD_SESSIONS_PER_MIN` | `20` | Server PIN checks (`POST /device/guard-sessions`) per tablet per minute |
| `GUARD_TOKEN_SECRET` | *(derived)* | HS256 key of guard tokens, ≥ 32 bytes. Unset = derived from `SETTINGS_ENCRYPTION_KEY` (HKDF). Changing it ends all guard sessions (guards unlock again online) |
| `GUARD_SESSION_TTL` | `12h` | Lifetime of a guard token (5m–168h); the tablet re-verifies the PIN after it |
| `GUARD_TOKEN_GRACE` | `168h` | How long after expiry a token still proves records made before its expiry (offline queue; = the 7-day sync window) |
| `QR_SIGNING_SECRET` | *(derived)* | HMAC key of checkpoint QR codes, ≥ 32 bytes. Unset = derived from `SETTINGS_ENCRYPTION_KEY`. **Changing it invalidates every printed signed QR code** (regenerate + reprint) |

nginx must pass the client address as `X-Real-IP $remote_addr` (it does in
`infra/nginx`, §9); otherwise audit IPs and per-IP limits see nginx's address.
Rate limits are kept in Redis and are skipped (logged) while Redis is down.

**Audit log.** Every write request is recorded in `audit_log` (who, realm,
roles, route, resource, client IP, status; a redacted copy of the body for
admin/supervisor/HR changes; before/after for attendance overrides and guard,
site, route, device, shift and template changes). Admins read their realm's
entries in the web app or via `GET /api/v1/audit`. The table is append-only — a
database trigger rejects UPDATE, DELETE and TRUNCATE for every role (§1). Retention:
keep it; if it must be pruned, do it as a DBA (`POSTGRES_USER`) with the trigger
temporarily disabled and record that in the change log.

**Uploads.** Photo metadata (EXIF/GPS) is removed before storage and photos
cannot be replaced once uploaded (the photos bucket is also versioned).

### Data retention and personal data (BE-S9, UU PDP 27/2022)

The worker's `retention` job runs hourly (in batches; a backlog drains over
several runs) and applies these periods (days; `0` = keep), set in `.env`:

| Data | Variable | Default | What happens |
|---|---|---|---|
| Visitor logs + visitor photos | `RETENTION_VISITOR_DAYS` | 365 | row and photo deleted |
| Clock-in selfies | `RETENTION_SELFIE_DAYS` | 365 | photo deleted, reference cleared (attendance kept) |
| Attendance and checkpoint-scan GPS | `RETENTION_GPS_DAYS` | 365 | coordinates cleared |
| Notification contents | `RETENTION_NOTIFICATION_DAYS` | 180 | subject, body, recipient, dedupe key cleared (the row stays for statistics) |
| Incident / defect / checkpoint photos | `RETENTION_EVIDENCE_PHOTO_DAYS` | 0 (keep) | photo deleted, reference cleared — evidence: set only on legal advice |

Photos are deleted **with every version** (the photos bucket is versioned), so
the app's MinIO user needs `s3:DeleteObject`, `s3:DeleteObjectVersion` and
`s3:ListBucketVersions` on the photos bucket only (`minio-init.sh`; re-run with
`docker compose up -d minio-init` after upgrading). Failures are retried next
run; watch `sp_retention_items_total{result="failed"}` and the `retention` job
in `sp_job_runs_total`. Off-site restic snapshots keep deleted data until they
expire (30 daily + 12 monthly). `audit_log` is append-only and not purged in v1.
Backups (`BACKUP_RETENTION_DAYS`) are handled by the bucket lifecycle.

Visitor ID card numbers are encrypted at rest (AES-256-GCM, key derived from
`SETTINGS_ENCRYPTION_KEY` with HKDF, info `securepatrol pii v1`) and shown masked
(`3201********0001`) except on the visitor detail for admins/supervisors. Rows
stored before this change are encrypted by the retention job. **Changing
`SETTINGS_ENCRYPTION_KEY` makes stored ID numbers unreadable** (shown as
`[unreadable]`) — keep the key in the config backup.

### Tablet health

Tablets send a health beacon (`POST /device/telemetry`, ≤ 1/min) — app version,
outbox backlog, SOS pending, clock offset, recent errors. The web *Devices* page
shows the latest one; Prometheus gets `sp_device_outbox_pending{device}` from the
API. A tablet whose beacon is old or whose outbox keeps growing is offline or
stuck (see §3).

## 11. Troubleshooting

| Symptom | Check |
|---|---|
| `make up` stops in preflight | Fix the FAIL lines; `make preflight` re-checks. |
| Browser that used the app before can't reach the start page (endless loading, error after login, wrong organisation) | Open `https://SERVER_HOST/?reset`: clears this browser's sign-in state and the Keycloak session, then sign in again. `/?org` only re-opens the organisation picker. After 20 s of loading the app offers *Reset sign-in* itself. |
| Web login loops / "invalid token" | `KEYCLOAK_PUBLIC_URL` = `https://SERVER_HOST/auth` (the URL in the browser + `/auth`) and the realm in `KEYCLOAK_REALMS`; `make keycloak-config`. |
| "Invalid redirect uri" on login | `make keycloak-config` (redirect URIs follow `PUBLIC_URL`). |
| Keycloak stays "starting"/unhealthy after an upgrade | `docker compose logs keycloak`: schema migration still running (large session tables), an unknown/removed option, or the database is from a **newer** Keycloak (restore the matching dump, §2). The healthcheck allows 3 min + 10 × 20 s. |
| preflight: `KEYCLOAK_ADMIN … Keycloak 24 names` | Rename to `KC_BOOTSTRAP_ADMIN_USERNAME` / `KC_BOOTSTRAP_ADMIN_PASSWORD` (sed command in the message, §2). |
| `make keycloak-config` fails at "logging in" | The bootstrap admin was replaced or got OTP: set `KEYCLOAK_AUTOMATION_CLIENT_ID/SECRET` (§2 *Master realm hardening*). |
| Tablet login "bad credentials" although the password is right | Device account profile incomplete (needs first/last name and e-mail — accounts created in the web app have them) or a required action is pending: *Users → user → Details*. |
| Web roles missing in the token / 403 after a new realm role | The client's role scope: `make keycloak-config`, or add the role under *Clients → securepatrol-web → Client scopes → Dedicated scope → Scope* (§2). |
| Browser warns about the certificate | CA not installed on that device (§6), or `SERVER_HOST` differs from the address typed. |
| Photos don't load | `MINIO_PUBLIC_ENDPOINT` = `SERVER_HOST:9443`, port 9443 reachable, CA trusted. |
| 403 on `/auth/admin` | Your address is not in `ADMIN_ALLOW_CIDRS`; use the SSH tunnel (§2). |
| 429 Too Many Requests from nginx | nginx rate limit; check for a misbehaving client in `docker compose logs nginx`. |
| 429 `rate_limited` from the API | Wait `Retry-After` seconds; raise the `RATE_LIMIT_*` values (§10) if legitimate traffic hits them. |
| No notifications | *Settings → Notification log* errors; `make logs` (worker); rules exist for the site; worker `/health` (`:9091`) is 200; Grafana *Overview* (heartbeat, `sp_notification_queue_depth` not growing). |
| Worker "unhealthy" (503 on `:9091/health`) | Dispatcher or scheduler stalled for > 60 s: check the worker log and Postgres reachability; restart the worker. Redis being down does **not** stop notifications. |
| `ops_backup_failed` / `ops_backup_stale` / `BackupStale` | `make logs` (worker) for the `pg_dump` error; MinIO disk space; `BACKUP_TIMEOUT` too short for the database size. |
| Off-site backup alert | `docker compose logs backup-offsite`; is the disk mounted? `make offsite-backup`. |
| Tablet "device_pending" | Approve it in *Settings → Devices*. |
| Tablet "guard_not_allowed" | Guard inactive or assigned to another site. |
| Tablet "pin_locked" | Five wrong PINs: wait 15 min, or set a new PIN for the guard in *Guards*. |
| Many `guard_unverified` records | Tablets unlock offline or the guard token is rejected: check the tablet's connectivity; after changing `GUARD_TOKEN_SECRET` guards must unlock online once. |
| Scans flagged `qr_invalid` | Sticker printed before *Regenerate QR codes* (or a copied/forged code); reprint the route's QR sheet. `qr_unsigned` = route still uses old plain codes. |
| `patrol_not_started` while patrols happen | The route's schedule window/grace does not match practice, or the tablet is offline (the alert is real until the session syncs). Check *Patrol compliance*. |
| Admin gets 403 `platform_admin_required` on Settings → Channels/Backups | Assign the `platform-admin` role (§2) and check `PLATFORM_ADMIN_REALMS`. |
| Everyone sees 403 after login | Token issued to an unexpected client: check `KEYCLOAK_WEB_CLIENT`/`KEYCLOAK_TABLET_CLIENT` match the Keycloak client IDs. |
| Saving channels: `secret_required` | Re-enter the password/token when changing host/port/user or API URL/phone number ID. |
