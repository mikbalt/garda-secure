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

Admin console: `https://<SERVER_HOST>/auth/admin/` (user `KEYCLOAK_ADMIN`) — reachable
only from `ADMIN_ALLOW_CIDRS` (default: private LAN ranges + localhost; INF-N2). From
elsewhere use an SSH tunnel: `ssh -L 8081:127.0.0.1:8081 server`, then
`http://localhost:8081/auth/admin/` (the console redirects to the public URL for login;
add your VPN range to `ADMIN_ALLOW_CIDRS` if needed, then `docker compose up -d nginx`).

Each client company is a realm (`securepatrol-client1`, …). In each realm:

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
`PUBLIC_URL`, the MFA flow/roles, brute-force protection (client realms + master).
Run it by hand after changing `SERVER_HOST`, `KEYCLOAK_DEV_REDIRECTS` or a realm list:
`make keycloak-config` (or `sh scripts/keycloak-config.sh <realm>`). Other realm
changes (token lifetimes, password policy, new realm roles) are made in the console or
with `docker compose exec keycloak /opt/keycloak/bin/kcadm.sh …`.

### Master realm hardening

The master realm is the Keycloak super-admin; it is never used by the app.

1. Brute-force protection is switched on by `make keycloak-config`.
2. **Automation client first** (so the script keeps working once the admin has OTP):
   master realm → *Clients → Create* `securepatrol-automation`, *Client authentication*
   ON, *Service accounts* ON, standard flow and direct access OFF → *Service account
   roles → Assign role* `admin` → *Credentials*: copy the secret into `.env`
   (`KEYCLOAK_AUTOMATION_CLIENT_ID=securepatrol-automation`,
   `KEYCLOAK_AUTOMATION_CLIENT_SECRET=…`), run `make keycloak-config` to verify.
3. **OTP for the master admin:** *Users → admin → Required user actions → Configure
   OTP*, log out and in again, scan the QR code. Keep a second, sealed break-glass admin
   with OTP. Change the bootstrap `KEYCLOAK_ADMIN_PASSWORD` in the console too (the env
   value is only used when the master realm is first created).

### Adding a client company (new realm)

```bash
cd infra/keycloak && ./new-realm.sh securepatrol-client3 "PT Contoh"
```
The file gets the redirect URI `https://<SERVER_HOST>/*` (JSON-safe, via jq). Add the
realm to `KEYCLOAK_REALMS` and `WEB_KEYCLOAK_REALMS` in `.env`, then `make up`
(Keycloak imports new realm files on start, existing realms are untouched;
`keycloak-config` then sets redirect URIs and enforces MFA).

### Keycloak 24 settings and the 26 upgrade (INF-D8)

Keycloak runs `start` (production mode) on plain HTTP behind nginx under `/auth`:
`KC_PROXY_HEADERS=xforwarded`, `KC_HOSTNAME_URL`/`KC_HOSTNAME_ADMIN_URL` =
`KEYCLOAK_PUBLIC_URL` (the token issuer), `KC_HOSTNAME_STRICT=true`,
`KC_HOSTNAME_STRICT_BACKCHANNEL=false` (the API fetches keys via `http://keycloak:8080`).
Keycloak 24 serves `/auth/health` and `/auth/metrics` on the main port; nginx blocks
both publicly.

Keycloak 24 no longer receives fixes. Migration to 26.x (separate change, test on a
copy first — §5 restore drill):
1. Back up the Keycloak DB (`make backup-db`).
2. Image `quay.io/keycloak/keycloak:26.x`; replace `KC_HOSTNAME_URL`/`KC_HOSTNAME_ADMIN_URL`
   with `KC_HOSTNAME=${KEYCLOAK_PUBLIC_URL}` (hostname v2), drop
   `KC_HOSTNAME_STRICT_BACKCHANNEL` (use `KC_HOSTNAME_BACKCHANNEL_DYNAMIC=true`);
   `KEYCLOAK_ADMIN*` become `KC_BOOTSTRAP_ADMIN_USERNAME/PASSWORD`.
3. Health and metrics move to the management port 9000: healthcheck
   `http://127.0.0.1:9000/auth/health/ready`, Prometheus target `keycloak:9000`
   (path `/auth/metrics`), and the nginx `/auth/metrics|health` blocks become moot.
4. Keycloak migrates its schema on first start; check `make keycloak-config`, a web
   login with OTP and a tablet token afterwards.

PostgreSQL majors (16 → 17) likewise need a planned dump/restore migration, and the
API image's `pg_dump` (`postgresql16-client`) must be upgraded with the server.

## 3. Tablets (shared per site — D-001)

1. In the site's realm create a user like `tablet-gudang-a-1`, set a strong password,
   assign realm role **`device`** (nothing else).
2. Install the SecurePatrol CA on the tablet (§6), then log in once in the app with that
   account; it registers itself and shows "waiting for approval".
3. Web app → *Settings → Devices*: approve the tablet and pick its site.
4. Web app → *Guards*: make sure each guard is assigned to the site and has a PIN.
   The tablet syncs the roster and guards unlock it with their PIN (works offline).
5. Lost/stolen tablet: *Settings → Devices → Revoke* (takes effect immediately), and
   disable the Keycloak user.

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
* **Android tablets:** copy the file to the tablet → *Settings → Security → Encryption &
  credentials → Install a certificate → CA certificate*. (The app must trust user CAs
  for this host — mobile network-security config.)
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

nginx answers 404 for `/metrics`, `/api/metrics`, `/auth/metrics` and `/auth/health`,
so metrics are never reachable from outside.

Scraped: api `:8080/metrics`, worker `:9091/metrics`, Keycloak, MinIO, node-exporter
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
* **Images** are pinned to minor/patch tags; Dependabot proposes updates (Keycloak and
  Postgres majors are ignored — they need a planned migration, §2).
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

## 11. Troubleshooting

| Symptom | Check |
|---|---|
| `make up` stops in preflight | Fix the FAIL lines; `make preflight` re-checks. |
| Web login loops / "invalid token" | `KEYCLOAK_PUBLIC_URL` = `https://SERVER_HOST/auth` (the URL in the browser + `/auth`) and the realm in `KEYCLOAK_REALMS`; `make keycloak-config`. |
| "Invalid redirect uri" on login | `make keycloak-config` (redirect URIs follow `PUBLIC_URL`). |
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
| Admin gets 403 `platform_admin_required` on Settings → Channels/Backups | Assign the `platform-admin` role (§2) and check `PLATFORM_ADMIN_REALMS`. |
| Everyone sees 403 after login | Token issued to an unexpected client: check `KEYCLOAK_WEB_CLIENT`/`KEYCLOAK_TABLET_CLIENT` match the Keycloak client IDs. |
| Saving channels: `secret_required` | Re-enter the password/token when changing host/port/user or API URL/phone number ID. |
