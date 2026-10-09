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

**Ops alerts** (install-wide, not per site): configure recipients via
`/api/v1/notifications/ops-rules` (web UI to follow) for
`ops_backup_failed`, `ops_backup_stale` (no successful backup for 26 h),
`ops_sos_undelivered` (an SOS delivery gave up, or an SOS fired at a site with
no SOS rule) and `ops_job_failing` (a scheduled job failed 5 times in a row).
Configure at least one e-mail **and** one WhatsApp recipient for
`ops_sos_undelivered` and the backup alerts before go-live.

## 5. Backups and restore (NFR-07)

- Automatic: the worker runs `pg_dump` daily at `BACKUP_HOUR` in `BACKUP_TZ`
  (default `Asia/Jakarta`, independent of the container `TZ`) into the MinIO
  bucket `MINIO_BUCKET_BACKUPS` (versioned): `postgres/securepatrol-<UTC time>.dump`.
- Keycloak: set `BACKUP_KEYCLOAK_DATABASE_URL` (e.g.
  `postgres://<user>:<password>@postgres:5432/keycloak?sslmode=disable`) and every
  run also dumps the Keycloak database to `keycloak/keycloak-<UTC time>.dump`
  (listed as `keycloak_object_key` in *Settings → Backups*).
- Each dump is limited by `BACKUP_TIMEOUT` (default `1h`). The database password
  is passed to `pg_dump` via `PGPASSWORD`, never on the command line.
- A failed backup raises the ops alert `ops_backup_failed`; no successful backup
  for 26 h raises `ops_backup_stale` (once a day). Backups left "running" by a
  crashed worker are marked failed automatically when the worker restarts.
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

Keycloak data (users, realms) is in the separate `keycloak` database. With
`BACKUP_KEYCLOAK_DATABASE_URL` set it is in every backup run (restore it like the
application dump, with `-d keycloak`, while Keycloak is stopped). Without it, back
it up manually with
`docker compose exec postgres pg_dump -Fc -U "$POSTGRES_USER" keycloak > keycloak.dump`
after user changes.

## 6. Monitoring

| Endpoint | Where | Meaning |
|---|---|---|
| `GET /health` | API `:8080` (also via nginx) | DB + Redis reachable |
| `GET /ready` | API `:8080`, internal only | DB, Redis and MinIO reachable (503 otherwise) |
| `GET /metrics` | API `:8080`, internal only | Prometheus: HTTP metrics + Go/process |
| `GET /health` | worker `WORKER_HTTP_ADDR` (`:9091`) | 200 only if the notification dispatcher and the scheduler ticked within 60 s |
| `GET /metrics` | worker `:9091` | Prometheus: notification, job and backup metrics |

nginx proxies only `/api/`, `/health`, `/auth/` and the web app, so `/ready` and
`/metrics` are not reachable from outside. Metric names (alert rules depend on them):

- API: `sp_http_requests_total{method,route,status}`,
  `sp_http_request_duration_seconds{method,route}` (route = route pattern).
- Worker: `sp_notifications_total{channel,event,result=sent|retry|failed}`,
  `sp_notification_queue_depth{lane=sos|normal}`, `sp_notifications_failed_total{event}`,
  `sp_sos_unacknowledged`, `sp_worker_heartbeat_timestamp_seconds`,
  `sp_job_runs_total{job,result=success|error|skipped}`,
  `sp_job_last_success_timestamp_seconds{job}`,
  `sp_backup_last_success_timestamp_seconds` (0 = no successful backup yet).
- Both: Go and process collectors (`go_*`, `process_*`).

Worker and API environment (defaults in brackets): `DB_MAX_CONNS` [20],
`DB_STATEMENT_TIMEOUT` [30s], `NOTIFY_WORKERS` [4, min 2], `WORKER_HTTP_ADDR` [:9091],
`WORKER_SHUTDOWN_TIMEOUT` [40s — give the container a `stop_grace_period` of at
least 60s], `BACKUP_HOUR` [2], `BACKUP_TZ` [Asia/Jakarta], `BACKUP_TIMEOUT` [1h],
`BACKUP_KEYCLOAK_DATABASE_URL` [unset].

Logs: every 5xx is logged with its `request_id` (also returned to the client);
notification deliveries log the `request_id` of the API call that caused them.

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
| No notifications | *Settings → Notification log* errors; `make logs` (worker); rules exist for the site; worker `/health` (`:9091`) is 200; `sp_notification_queue_depth` not growing. |
| Worker unhealthy (503 on `:9091/health`) | Dispatcher or scheduler stalled for > 60 s: check the worker log, Postgres reachability; restart the worker. Redis being down does **not** stop notifications. |
| `ops_backup_failed` / `ops_backup_stale` | `make logs` (worker) for the `pg_dump` error; MinIO disk space; `BACKUP_TIMEOUT` too short for the database size. |
| Tablet "device_pending" | Approve it in *Settings → Devices*. |
| Tablet "guard_not_allowed" | Guard inactive or assigned to another site. |
