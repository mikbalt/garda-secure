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

*Settings → Channels*: SMTP and WhatsApp credentials (stored encrypted). Use
**Send test** after saving.

WhatsApp Business Cloud API only delivers business-initiated messages through an
**approved template**. Create one in Meta Business Manager with a single body
variable, e.g. `SecurePatrol: {{1}}`, and set its name/language in Settings.

*Settings → Notification rules*: per site, which events go to which e-mail/phone.
*Settings → Notification log*: failed deliveries show the error and can be retried.

## 5. Backups and restore (NFR-07)

- Automatic: the worker runs `pg_dump` daily at `BACKUP_HOUR` (server `TZ`) into the
  MinIO bucket `MINIO_BUCKET_BACKUPS` (versioned).
- Manual: web app → *Settings → Backups → Back up now*.
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

## 6. Updating

```bash
git pull
cd infra && make up      # rebuilds images; the API applies DB migrations on start
```

## 7. Troubleshooting

| Symptom | Check |
|---|---|
| Web login loops / "invalid token" | `KEYCLOAK_PUBLIC_URL` must equal the URL in the browser + `/auth`, and the realm must be in `KEYCLOAK_REALMS`. |
| Photos don't load | `MINIO_PUBLIC_ENDPOINT` must be `<server>:9443` reachable from the browser; certificate trusted. |
| No notifications | *Settings → Notification log* errors; `make logs` (worker); rules exist for the site. |
| Tablet "device_pending" | Approve it in *Settings → Devices*. |
| Tablet "guard_not_allowed" | Guard inactive or assigned to another site. |
