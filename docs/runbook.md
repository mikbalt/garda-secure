# SecurePatrol — Operations Runbook

For the single operator running the on-premise stack (PRD NFR-01, NFR-07, NFR-09).
All commands run on the server from `infra/` unless noted.

## 1. First install

Requirements: Docker 24+ with Compose **v2.24+** (the app overlay uses `!override`),
`make`, `openssl`.

```bash
cd infra
cp .env.example .env          # then edit every change_me value
# PUBLIC_URL / KEYCLOAK_PUBLIC_URL / MINIO_PUBLIC_ENDPOINT must use the name
# or IP that browsers and tablets use to reach the server.
openssl rand -base64 32       # paste as SETTINGS_ENCRYPTION_KEY
make certs                    # self-signed; replace nginx/certs/*.pem with real certs if available
make up                       # builds api/worker/web and starts everything
make ps                       # all services should become "healthy"
```

Self-signed certificates: install `nginx/certs/cert.pem` as a trusted
certificate on admin PCs and tablets, otherwise browsers block the login page and
photo links (`:9443`).

## 2. Keycloak: users and roles

Admin console: `https://<server>/auth/admin/` (user `KEYCLOAK_ADMIN`). Each client
company is a realm (`securepatrol-client1`, …). In each realm:

1. **Users → Add user** for every person; set a password under *Credentials*.
2. **Role mapping → Assign role**: `admin`, `supervisor`, `hr` or `client-readonly`.
3. Admin and HR accounts: *Required user actions → Configure OTP* to enforce 2FA
   (PRD FR-AUTH-02).
4. Guards do **not** need Keycloak accounts — they are created in the web app
   (*Guards*) and unlock tablets with a PIN.

### The operator: role `platform-admin`

Notification channels (SMTP/WhatsApp credentials) and backups are
install-wide, so a client company's `admin` cannot change them. Only users
with the realm role **`platform-admin`** can open *Settings → Channels* and
*Settings → Backups* (API: `/settings/*`, `/backups/*`).

1. Pick the realm the operator logs in to (ideally a dedicated operator realm
   listed in `KEYCLOAK_REALMS`, otherwise the first client realm).
2. If the realm was imported before this role existed, create it:
   *Realm roles → Create role* → name `platform-admin`. (Realm files are only
   imported when a realm is created; `infra/keycloak/realms/*.json` contain the
   role for new installs.)
3. *Users → <operator> → Role mapping → Assign role* → `platform-admin` (plus
   `admin` if the operator also manages that realm's sites). Enforce OTP for
   this user.
4. Set `PLATFORM_ADMIN_REALMS=<that realm>` in `.env` so a `platform-admin`
   role created by mistake in a client realm has no effect, then `make up`.
   Empty means "any configured realm".

Never give `platform-admin` to client staff.

### Adding a client company (new realm)

```bash
cd infra/keycloak && ./new-realm.sh securepatrol-client3 "PT Contoh"
```
Add the realm to `KEYCLOAK_REALMS` and `WEB_KEYCLOAK_REALMS` in `.env`, then
`make up` (Keycloak imports new realm files on start; existing realms are untouched).
Also add the production redirect URI (`https://<server>/*`) to the
`securepatrol-web` client of every realm if `PUBLIC_URL` is not `https://localhost`.

## 3. Tablets (shared per site — D-001)

1. In the site's realm create a user like `tablet-gudang-a-1`, set a strong
   password, assign realm role **`device`** (nothing else).
2. On the tablet app, log in once with that account; it registers itself and
   shows "waiting for approval".
3. Web app → *Settings → Devices*: approve the tablet and pick its site.
4. Web app → *Guards*: make sure each guard is assigned to the site and has a PIN.
   The tablet syncs the roster and guards unlock it with their PIN (works offline).
5. Lost/stolen tablet: *Settings → Devices → Revoke* (takes effect immediately), and
   disable the Keycloak user.

## 4. Notifications

*Settings → Channels* (`platform-admin` only): SMTP and WhatsApp credentials
(stored encrypted). Use **Send test** after saving; a failure shows a short
reason (`auth_failed`, `timeout`, `destination_not_allowed`, …) — the full error
is in the API log (`make logs`, message "test notification failed").

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
*Settings → Notification log*: failed deliveries show the error and can be retried.

## 5. Backups and restore (NFR-07)

- Automatic: the worker runs `pg_dump` daily at `BACKUP_HOUR` (server `TZ`) into the
  MinIO bucket `MINIO_BUCKET_BACKUPS` (versioned).
- Manual: web app → *Settings → Backups → Back up now* (role `platform-admin`).
- Photos live in MinIO (`MINIO_BUCKET_PHOTOS`); back up the `miniodata` volume or
  mirror the bucket to external storage with `mc mirror`.

Restore the database from a dump:

```bash
cd infra
set -a; . ./.env; set +a   # load POSTGRES_* / MINIO_* into the shell
# 1. Download the dump (MinIO console: http://127.0.0.1:9001 on the server, or mc):
docker compose exec minio mc alias set local http://localhost:9000 "$MINIO_ROOT_USER" "$MINIO_ROOT_PASSWORD"
docker compose exec minio mc cp local/securepatrol-backups/postgres/<file>.dump /tmp/restore.dump
docker compose cp minio:/tmp/restore.dump ./restore.dump
# 2. Stop writers, restore, start again:
docker compose -f docker-compose.yml -f docker-compose.app.yml stop api notification-worker
docker compose cp ./restore.dump postgres:/tmp/restore.dump
docker compose exec postgres pg_restore --clean --if-exists --no-owner \
  -U "$POSTGRES_USER" -d "$POSTGRES_DB" /tmp/restore.dump
docker compose -f docker-compose.yml -f docker-compose.app.yml start api notification-worker
```
Practise the restore on a spare machine at least once before go-live.

Keycloak data (users, realms) is in the separate `keycloak` database; back it up with
`docker compose exec postgres pg_dump -Fc -U "$POSTGRES_USER" keycloak > keycloak.dump`
after user changes.

## 6. Security settings, audit log and limits

| Variable | Default | Meaning |
|---|---|---|
| `KEYCLOAK_WEB_CLIENT` | `securepatrol-web` | Web roles (admin, supervisor, hr, client-readonly, platform-admin) only count in tokens of this client |
| `KEYCLOAK_TABLET_CLIENT` | `securepatrol-tablet` | The `device` role only counts in tokens of this client |
| `PLATFORM_ADMIN_REALMS` | *(empty = any)* | Realms whose `platform-admin` users manage channels and backups |
| `TRUSTED_PROXIES` | `10.0.0.0/8,172.16.0.0/12,192.168.0.0/16,127.0.0.1/32,::1/128` | Peers whose `X-Real-IP` header is believed (nginx); `X-Forwarded-For` is ignored |
| `WHATSAPP_API_URL_ALLOWLIST` | `https://graph.facebook.com/` | Allowed WhatsApp API URL prefixes (comma-separated) |
| `SMTP_ALLOWED_PRIVATE_NETS` | *(empty)* | Private IPs/CIDRs an SMTP host entered in the UI may resolve to |
| `RATE_LIMIT_USER_WRITES_PER_MIN` | `120` | Writes per web user per minute (0 = off) |
| `RATE_LIMIT_DEVICE_WRITES_PER_MIN` | `600` | Writes per tablet per minute — high enough for an offline backlog |
| `RATE_LIMIT_SOS_PER_MIN` | `5` | New SOS alerts per tablet per minute (retries are never limited) |
| `RATE_LIMIT_AUTH_FAILURES_PER_MIN` | `30` | Failed logins/invalid tokens per client IP before 429 |

nginx must pass the client address as `X-Real-IP $remote_addr` (it does in
`infra/nginx`); otherwise audit IPs and per-IP limits see nginx's address.
Rate limits are kept in Redis and are skipped (logged) while Redis is down.

**Audit log.** Every write request is recorded in `audit_log` (who, realm,
roles, route, resource, client IP, status; a redacted copy of the body for
admin/supervisor/HR changes; before/after for attendance overrides and guard,
site, route, device, shift and template changes). Admins read their realm's
entries in the web app or via `GET /api/v1/audit`. The table is append-only — a
database trigger rejects UPDATE, DELETE and TRUNCATE. Retention: keep it; if it
must be pruned, do it as a DBA with the trigger temporarily disabled and record
that in the change log.

**Uploads.** Photo metadata (EXIF/GPS) is removed before storage and photos
cannot be replaced once uploaded (the photos bucket is also versioned).

## 7. Updating

```bash
git pull
cd infra && make up      # rebuilds images; the API applies DB migrations on start
```

## 8. Automatic deploys (GitHub Actions)

One-time server setup: clone the repo to `/opt/securepatrol` (or set
`DEPLOY_PATH`), create `infra/.env` and the TLS cert, and make sure the deploy
user can run `docker` and `git fetch` (deploy key with read access).

Repository → Settings → Environments → **production** → secrets:

| Secret | Required | Meaning |
|---|---|---|
| `DEPLOY_HOST`, `DEPLOY_USER`, `DEPLOY_KEY` | yes | SSH target and private key |
| `DEPLOY_PORT` | no | SSH port (default 22) |
| `DEPLOY_PATH` | no | repo path on the server (default `/opt/securepatrol`) |
| `SMTP_HOST`, `SMTP_PORT`, `SMTP_USER`, `SMTP_PASSWORD`, `SMTP_FROM`, `DEPLOY_NOTIFY_EMAIL` | no | deploy result e-mail; skipped when unset |

Deploy a release: `git tag v1.0.0 && git push origin v1.0.0`, or Actions →
*CD — Deploy to Production* → *Run workflow* with a branch/tag. Roll back by
deploying the previous tag.

## 9. Troubleshooting

| Symptom | Check |
|---|---|
| Web login loops / "invalid token" | `KEYCLOAK_PUBLIC_URL` must equal the URL in the browser + `/auth`, and the realm must be in `KEYCLOAK_REALMS`. |
| Photos don't load | `MINIO_PUBLIC_ENDPOINT` must be `<server>:9443` reachable from the browser; certificate trusted. |
| No notifications | *Settings → Notification log* errors; `make logs` (worker); rules exist for the site. |
| Tablet "device_pending" | Approve it in *Settings → Devices*. |
| Tablet "guard_not_allowed" | Guard inactive or assigned to another site. |
| Admin gets 403 `platform_admin_required` on Settings → Channels/Backups | Assign the `platform-admin` role (section 2) and check `PLATFORM_ADMIN_REALMS`. |
| Everyone sees 403 after login | Token issued to an unexpected client: check `KEYCLOAK_WEB_CLIENT`/`KEYCLOAK_TABLET_CLIENT` match the Keycloak client IDs. |
| 429 `rate_limited` | Wait `Retry-After` seconds; raise the `RATE_LIMIT_*` values if legitimate traffic hits them. |
| Saving channels: `secret_required` | Re-enter the password/token when changing host/port/user or API URL/phone number ID. |
