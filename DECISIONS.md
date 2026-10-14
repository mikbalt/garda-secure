# Decisions Log

Append-only. To change a decision, add a new entry that supersedes the old one
and mark the old one `Superseded by D-xxx`. Product-level questions also appear
in PRD §9 (OQ-x). New entries go at the bottom with the next free number.

---

### D-001 — Shared tablets: device token + local PIN (PRD OQ-9)
- **Date:** 2026-10-07 · **Status:** Accepted
- Each tablet is a Keycloak user with realm role `device`, logging in through the
  public client `securepatrol-tablet` (password grant + offline token).
- The tablet calls `POST /device/register`; it stays `pending` until an admin
  approves it and assigns exactly one site. Admin can revoke at any time.
- Guards unlock the tablet with a 4–6 digit PIN verified **locally** against a
  bcrypt hash from `GET /device/roster`, so switching guards works offline.
- Every tablet request sends the device token plus `X-Guard-ID`; the API checks
  the guard is active and assigned to the device's site.
- Trade-off: PIN hashes on the device are brute-forceable offline (short PINs).
  Mitigation: bcrypt cost, encrypted on-device storage, device revocation.

### D-002 — UI languages: English + Bahasa Indonesia (PRD OQ-7)
- **Date:** 2026-10-07 · **Status:** Accepted. Thai dropped. Notification language per site (`config_json.language`, default `id`).

### D-003 — Recurring shifts = templates + pre-generated shifts (PRD OQ-8)
- **Date:** 2026-10-07 · **Status:** Accepted
- `shift_templates` (guard, site, local start/end time, ISO weekdays, validity).
  End ≤ start means the shift ends the next day.
- The worker generates concrete `shifts` 14 days ahead (idempotent via unique
  `(template_id, start_time)`). Editing a template deletes its future shifts
  without attendance and regenerates. Editing a single shift detaches it from
  its template. One-off shifts have no template.
- `shifts.repeat_rule` was dropped (migration 003).

### D-004 — Attendance model
- **Date:** 2026-10-07 · **Status:** Accepted
- One attendance record per shift (unique `shift_id`); clock-in requires a shift
  whose window `[start − clock_window, end)` contains the clock-in time.
- `status` = arrival outcome: `present | late | absent`. Lateness and early
  departure are stored as `late_minutes` / `early_out_minutes`, so "late and
  left early" keeps both labels. A shift without a record reports `pending`.
- Per-site settings in `sites.config_json`: `clock_window_minutes` (15),
  `late_grace_minutes` (5), `early_out_grace_minutes` (5), `absent_after_minutes`
  (30), `idle_lock_minutes` (5), `language` (`id`).
- Admin override upserts the record of a shift with a mandatory reason (audited).

### D-005 — Multi-tenancy via Keycloak realms
- **Date:** 2026-10-07 · **Status:** Accepted
- `KEYCLOAK_REALMS` lists trusted realms; issuer = `KEYCLOAK_PUBLIC_URL/realms/<r>`,
  keys from `KEYCLOAK_URL/realms/<r>/…/certs`. Only tokens with `azp` in
  `KEYCLOAK_ALLOWED_CLIENTS` are accepted.
- Sites belong to a realm (`sites.keycloak_realm`, set from the creator's token).
  All site-scoped data is visible only to principals of that realm.
- Install-wide settings (notification channels, backups) are editable by an
  `admin` of any realm — admins are security-company staff, not clients.

### D-006 — Offline sync is idempotent
- **Date:** 2026-10-07 · **Status:** Accepted
- Tablet create endpoints accept a client UUID `id`; repeats return the existing
  row (200) instead of creating duplicates (critical for SOS retries every 10 s).
- Client event timestamps (`clock_in_at`, `scanned_at`, `fired_at`, `occurred_at`)
  are accepted if within [now − 7 days, now + 2 min]; otherwise 400.

### D-007 — Attendance sync conflicts go to a review queue
- **Date:** 2026-10-07 · **Status:** Accepted
- If an offline clock-in arrives after the shift was auto-flagged absent, the
  record is converted to the clock-in and flagged `needs_review`. Supervisors
  clear it via the review queue (PRD FR-OFF-02).

### D-008 — Patrol sessions and SLA
- **Date:** 2026-10-07 · **Status:** Accepted
- A patrol is a `patrol_session` (route, guard, start/end). Scans belong to a session.
- SLA per checkpoint (`sla_minutes`, nullable). Due time = scan time of the
  previous checkpoint in route order (session start for the first; if the previous
  was missed, its due time). Breaches are stored once per (session, checkpoint)
  in `sla_breaches`; the worker checks every 30 s and notifies on first insert.
- Sessions left open for 12 h are auto-abandoned.

### D-009 — Incidents are immutable; corrections are amendments
- **Date:** 2026-10-07 · **Status:** Accepted
- Original incident fields never change. Supervisors/admins add
  `incident_amendments` (changes JSON + mandatory reason). Status workflow
  (open → in_progress → resolved) is not an amendment.

### D-010 — Notifications pipeline
- **Date:** 2026-10-07 · **Status:** Accepted
- Per-site `notification_rules` (event × channel × recipient).
  Events: `sos`, `sla_missed`, `absent`, `late_clock_in`, `incident_high`,
  `defect_opened`, `monthly_recap`.
- `notification_log` (Postgres) is the source of truth, one row per recipient,
  with an optional `dedupe_key`; Redis only carries row IDs (queue + delayed-retry
  ZSET). Max 3 attempts, then `failed`; admin can retry from the UI.
- Channels implement a small interface so LINE/Telegram can be added in v2.
- WhatsApp Cloud API only allows business-initiated messages through **approved
  templates**: configure a template with one body parameter; free-form text is
  only a fallback.

### D-011 — Channel secrets stored encrypted
- **Date:** 2026-10-07 · **Status:** Accepted
- Admin edits SMTP/WhatsApp settings in the web UI (PRD FR-NOT-02). Stored in
  `system_settings`, secrets AES-GCM encrypted with `SETTINGS_ENCRYPTION_KEY`;
  env vars act as defaults. Secrets are never returned (masked).

### D-012 — Media storage
- **Date:** 2026-10-07 · **Status:** Accepted
- Object keys: `sites/<site_id>/<kind>/<uuid>.<ext>`; tablets may reference a key
  before uploading it (offline) — readers show such photos as pending.
- Uploads go through the API (`POST /media`, images ≤ 10 MB). Reads use pre-signed
  URLs on `MINIO_PUBLIC_ENDPOINT`.

### D-013 — Times and time zones
- **Date:** 2026-10-07 · **Status:** Accepted
- All timestamps UTC. Day/month boundaries computed in the site's IANA zone in Go;
  the binary embeds `time/tzdata`. Night shifts belong to the day they start.

### D-014 — Live patrol view: polling first
- **Date:** 2026-10-07 · **Status:** Accepted (temporary)
- v1 web polls `GET /patrol/live` every 10 s. WebSocket (PRD §6) and a map view
  (needs checkpoint coordinates, now optional `lat/lng` on checkpoints) are follow-ups.

### D-015 — Backups
- **Date:** 2026-10-07 · **Status:** Accepted
- Worker runs `pg_dump -Fc` daily at `BACKUP_HOUR` and on demand from the web UI,
  streaming into the versioned MinIO backups bucket. Runtime image is Alpine with
  `postgresql16-client` (replaces distroless). Restore steps in `docs/runbook.md`.

### D-016 — Backend code structure
- **Date:** 2026-10-07 · **Status:** Accepted
- `handler.Handler` struct holds dependencies; repository functions take `DBTX`
  and return `ErrNotFound`; see CLAUDE.md conventions.

### D-017 — Mobile app deferred
- **Date:** 2026-10-07 · **Status:** Superseded by D-024
- Focus on backend, web and infra for v1. The Flutter app will be updated later to
  the device-token/PIN flow (D-001) and the new endpoints.

### D-018 — Guards always belong to a site
- **Date:** 2026-10-07 · **Status:** Accepted
- `site_id` is mandatory when creating/updating a guard. The guards table has no
  realm column; the site is what scopes a guard to a client realm, so unassigned
  guards would leak across realms.

### D-019 — Deployment topology details
- **Date:** 2026-10-07 · **Status:** Accepted
- Keycloak uses its own database (`keycloak`) in the same Postgres server, created
  by `infra/postgres/init`. Keycloak runs in production mode (`start`) under
  `/auth` behind nginx; `KEYCLOAK_PUBLIC_URL` is both Keycloak's hostname URL and
  the token issuer prefix the API trusts.
- Browsers fetch photos through nginx on port **9443** (TLS) which proxies to MinIO
  preserving the Host header, so pre-signed URLs stay valid
  (`MINIO_PUBLIC_ENDPOINT=<host>:9443`). MinIO's own ports are bound to localhost.
- Web container gets runtime config (`/config.js`) from env vars; images are
  environment-independent.
- The API runs DB migrations on start; the worker waits for a healthy API.

### D-020 — Supervisor read access in the web app
- **Date:** 2026-10-08 · **Status:** Accepted (confirmed by product owner)
- Supervisors see Schedules, Guards and Patrol routes (incl. QR printing)
  read-only, matching what the API allows them to read. Only admins edit.
  Role → page mapping lives in `web/src/lib/roles.ts`.

### D-021 — CI/CD
- **Date:** 2026-10-08 · **Status:** Accepted
- Production deploys run only from a version tag (`v*`) or a manual
  `workflow_dispatch` with a branch/tag — never on every push. The server
  checks out that ref and runs the compose stack from `infra/` with
  `--wait`, so an unhealthy service fails the deploy. Mail notification
  is sent only when the SMTP secrets are configured.
- `gosec` is pinned to v2.20.0 (newer versions need Go ≥ 1.25; the repo
  targets Go 1.22).
- Mobile CI is manual-only while the app is deferred (D-017).

### D-022 — Web UX: design system, setup guide, demo mode
- **Date:** 2026-10-09 · **Status:** Accepted
- Product owner found the first web version confusing. The web app now has a
  documented design system (`web/DESIGN.md`): Inter (self-hosted for
  on-prem), slate neutrals, brand blue, dark navy shell, shared UI kit.
- Information architecture: grouped navigation (Today · Operations · People ·
  Insights · Administration); a location picker in the top bar.
- "Where do I start?" is answered by a **setup guide** (`/setup`) whose six
  steps (location → guards & PINs → schedule → routes & QR → tablet → alerts)
  complete themselves from live data, a first-run welcome screen, and a
  progress card in the sidebar/dashboard. Pages open their create form from
  `?new=1` deep links.
- The dashboard is "Today": needs-attention list first, then KPIs, live
  patrols, guards on duty, recent incidents.
- **Demo mode** (`npm run dev:demo`, or `DEMO=true` on the web container):
  sample data answered in the browser by `src/demo/mockApi.ts`, role switcher,
  empty-workspace mode. Used for design review, training and client demos; the
  mock must be kept in sync with docs/api.md. It is a lazily loaded chunk and
  never active unless configured.

### D-023 — WhatsApp recipients stored in international format
- **Date:** 2026-10-09 · **Status:** Accepted
- Numbers entered as Indonesian local format (`08…`) are stored as `628…`
  (API and web both normalise); WhatsApp requires the country code.

### D-024 — Mobile tablet app un-deferred (supersedes D-017)
- **Date:** 2026-10-09 · **Status:** Accepted
- Flutter 3.24 app for the shared post tablet, built against docs/api.md:
  first-run server setup (URL + organisation realm), device login with the
  Keycloak device account (password grant, offline token kept in secure
  storage), registration → wait for approval, roster sync, guard picker +
  local bcrypt PIN unlock, auto-lock after the site's idle time.
- Every field action goes through a persistent SQLite **outbox** with client
  UUIDs and event timestamps (idempotent replays, D-006); photos are saved
  locally and uploaded first with deterministic keys
  `sites/<site>/<kind>/<uuid>.jpg`. SOS jumps the queue and retries every 10 s.
- State management: `provider`; config via first-run screen with
  `--dart-define` defaults (no bundled .env asset).
- Two tablet endpoints added to the API: `GET /device/status` (acting guard's
  current shift, open attendance and running patrol) and `GET /device/sos`
  (unacknowledged SOS of the site, so supervisors on any tablet see them).

### D-025 — Ops alerts are platform-admin only
- **Date:** 2026-10-12 · **Status:** Accepted
- The install-wide operations alerts (`/notifications/ops-event-types`,
  `/notifications/ops-rules`, `/notifications/ops-log`, from the reliability
  work) need the `platform-admin` role, like the channel settings and backups
  (security work, BE-S3). They cover every tenant, so a realm `admin` must not
  read or receive them. Per-realm notification rules and logs stay `admin`.

### D-026 — Notification outbox and dispatch (amends D-010)
- **Date:** 2026-10-12 · **Status:** Accepted (audit BE-R1/R2/R5)
- Deliveries (`notification_log`) are written in the same transaction as the
  SOS/incident/defect/clock-in that caused them; a replay re-stages missing
  deliveries (24 h, same site). Workers claim due rows from Postgres
  (`FOR UPDATE SKIP LOCKED` + lease); Redis `notifications:wake` only wakes them.
- `NOTIFY_WORKERS` senders (default 4); one serves only SOS. Every send has a
  timeout (SMTP dial + deadline). SOS retries 10/20/30 s then every 60 s for
  30 min; other events 30 s, 2, 10, 30 min. A manual retry = one attempt.

### D-027 — Install-wide ops alerts
- **Date:** 2026-10-12 · **Status:** Accepted
- `ops_alert_rules` (no site) for `ops_backup_failed`, `ops_backup_stale`
  (> 26 h), `ops_sos_undelivered` (SOS delivery gave up or site has no SOS
  rule) and `ops_job_failing`. Platform-admin only (D-025). Platform alerts
  (worker down, disk, certs, SOS unacknowledged) come from Alertmanager.

### D-028 — SLA evaluation (amends D-008)
- **Date:** 2026-10-12 · **Status:** Accepted (audit F2)
- Ended sessions (incl. abandoned/early-ended) are evaluated once
  (`sla_checked_at`): late and missed checkpoints are breaches. A running
  session breaches only when a later scan arrived or the tablet has been online
  (`devices.online_since`) across the due time (1 min grace), so offline
  patrols synced later produce no false alerts.

### D-029 — Backups and DR (amends D-015)
- **Date:** 2026-10-12 · **Status:** Accepted (audit BE-R3, INF-B*)
- RPO 24 h / RTO 4 h. Daily `pg_dump` of the app DB **and** Keycloak DB at
  `BACKUP_HOUR` in `BACKUP_TZ` (Asia/Jakarta), `BACKUP_TIMEOUT` per dump,
  password via `PGPASSWORD`, serialised by an advisory lock; failures/staleness
  raise ops alerts. Nightly encrypted restic copy (backups + photos + config
  bundle) to `OFFSITE_BACKUP_REPO` on another disk/NAS, 30 daily + 12 monthly,
  monthly `restic check`. Pre-deploy dumps. Quarterly restore drill (runbook §5).

### D-030 — Observability contract
- **Date:** 2026-10-12 · **Status:** Accepted (audit BE-O1, INF-M1/M2)
- Metric names `sp_*` are a contract (runbook §8). API `/metrics` is never
  proxied publicly; `/health` and `/ready` are. Worker `/health` + `/metrics` on
  `:9091`; health requires dispatcher + scheduler ticks < 60 s. 5xx bodies carry
  `request_id`. Prometheus + Alertmanager (e-mail) + provisioned Grafana, bound
  to localhost (`docker-compose.ops.yml`).

### D-031 — Toolchain policy (supersedes the gosec pin in D-021)
- **Date:** 2026-10-12 · **Status:** Accepted (audit BE-S7)
- Backend targets the oldest *supported* Go release (now 1.26); CI reads it
  from `go.mod`. golangci-lint v2, gosec 2.29, govulncheck (fails on reachable
  vulns), Trivy image scans, `npm audit --audit-level=high`, Dependabot. All
  GitHub Actions pinned by commit SHA.

### D-032 — platform-admin and token binding
- **Date:** 2026-10-12 · **Status:** Accepted (audit BE-S3/S6; user decision)
- Install-wide settings (channels, backups, ops alerts) need realm role
  `platform-admin` (optionally limited by `PLATFORM_ADMIN_REALMS`) — held only
  by the operator. Realm `admin` administers its own realm only.
- Only access tokens (`typ: Bearer`) are accepted; web roles count only from
  the web client, `device` only from the tablet client.
- Changing a channel's destination requires re-entering its secret; WhatsApp
  URLs allow-listed; SMTP hosts must be public or allow-listed (checked again at
  connect time).

### D-033 — Data integrity and evidence
- **Date:** 2026-10-12 · **Status:** Accepted (audit BE-S1/S2/S4/S8/S9)
- Idempotent replays return a record only to the same site/session/guard,
  otherwise 409 `id_conflict`.
- Photos are write-once (identical retry 200, otherwise 409 `media_exists`),
  metadata (EXIF/GPS) stripped server-side, photos bucket versioned; photo
  access depends on kind and role.
- `audit_log` is append-only (trigger), written synchronously with a redacted
  body summary and before/after for key changes; `GET /audit`. Client IP from
  `X-Real-IP` of trusted proxies only.
- `received_at` + `flags` (`late_sync`, `clock_ahead`, `backdated_on_time`,
  `gps_far`, `too_fast`) on attendance/scans/incidents/SOS; suspicious
  clock-ins go to the review queue.
- Limits: 64 KB JSON bodies, field length limits, Redis rate limits (fail open).

### D-034 — Deployment model
- **Date:** 2026-10-12 · **Status:** Accepted (audit INF-*)
- One `SERVER_HOST` drives every URL, the cert SAN and realm redirect URIs;
  `make preflight` enforces consistency and strong secrets. Private,
  name-constrained CA by default (install the CA on tablets/PCs).
- Object storage = Silo (maintained MinIO fork) pinned by digest — the
  community `minio/minio` images are gone.
- Staff OTP is enforced by `keycloak-config.sh` (`mfa-required` composite role
  + `browser-mfa` flow), not by realm import. `/auth/admin` and the master realm
  only from `ADMIN_ALLOW_CIDRS`.
- Least privilege: separate DB roles, restricted MinIO users, hardened
  containers, log rotation.
- CD: images built once in CI and pushed to GHCR, deploy gated on green CI for
  the tag, `production` approval, SSH host-key pinning, pre-deploy dumps, keep
  3 image versions. `COMPOSE_FILE` lists infra/app/ops so plain
  `docker compose up -d` works.

### D-035 — Product decisions from the 2026-10 audit (user, 2026-10-12)
- **Date:** 2026-10-12 · **Status:** Accepted
- **Guard identity — both:** when online, unlocking verifies the PIN on the
  server and yields a short-lived signed guard token sent with every request;
  offline work stays possible but is flagged `guard_unverified` for review.
  Checkpoint QR codes are signed (HMAC) and verified by the server.
- **Patrol schedules:** routes get schedules; a "patrol not started" alert
  fires when a scheduled patrol is late; reports show schedule compliance.
- **Client scoping per site:** a user's allowed sites (Keycloak user attribute,
  managed from the web user-management page); empty = whole realm.
- **Incident notifications:** new event `incident_filed` (every incident) next
  to `incident_high`; admins choose per site via rules.
- **Amendments:** stay direct (supervisor/admin, reason required); reports and
  exports show the effective (amended) values.
- **Live map:** site floor plan (uploaded image) with checkpoints placed on it —
  fully on-prem; no online map tiles.
- **Clock window:** keep current behaviour (clock-in from start − window until
  shift end, late after start; clock-out until end + window). The PRD follows.

### D-036 — Guard sessions (implements D-035 "both")
- **Date:** 2026-10-12 · **Status:** Accepted
- `POST /device/guard-sessions` checks the PIN on the server and returns a
  12 h HS256 guard token bound to guard, device and site (`X-Guard-Token`).
  5 wrong PINs lock the guard for 15 min, counted server-side across tablets;
  every attempt is audited. Verification is by event time with a 7-day grace
  for queued records; a bad token is 403 `guard_token_invalid` (never for SOS).
  Records without proof are flagged `guard_unverified`; such clock-ins go to
  review. Offline unlock with the local bcrypt PIN stays possible.

### D-037 — Signed checkpoint QR codes
- **Date:** 2026-10-12 · **Status:** Accepted
- `SP1:<checkpoint_id>:<sig>` (HMAC, version stored in the checkpoint JSON).
  Tablets never receive payloads. Custom codes remain legacy (`qr_unsigned`);
  bad/outdated signatures are accepted but flagged `qr_invalid`.
  `POST /patrol/routes/:id/regenerate-qr` = reprint; rotating
  `QR_SIGNING_SECRET` invalidates every printed code.

### D-038 — Patrol schedules
- **Date:** 2026-10-12 · **Status:** Accepted
- Schedule windows per route (weekdays, start/end, every N minutes, grace);
  expectations materialised 48 h ahead; matched by session event time, one
  session per expected patrol, late starts recorded within 12 h. Missed patrols
  raise `patrol_not_started` even when the tablet is offline. Compliance = on
  time / decided (`GET /reports/patrol-compliance`, KPI, monthly recap).

### D-039 — Tablet app hardening (audit MOB-2..MOB-11)
- **Date:** 2026-10-12 · **Status:** Accepted
- App id `id.co.securepatrol.tablet` (installs as a new app: drain the old
  app's outbox, then uninstall it). Release builds only with the release key
  (`android/key.properties` from CI secrets); never debug-signed by accident.
- Local data: field-level AES-256-GCM (key in secure storage) for every cached
  value and outbox body/token — chosen over SQLCipher (no native build to verify
  here); `allowBackup=false`, data-extraction rules exclude everything.
- TLS: system CAs only; a private CA is bundled into the APK as a Flutter asset
  (`SECUREPATROL_CA_PEM` secret in CI), because Dart's TLS ignores the Android
  network-security config. `http://` servers refused outside debug builds.
- Protected actions: device sign-out, server change and discarding outbox items
  need a supervisor PIN or the device account password, and are refused while
  items are queued. SOS works from the lock screen without a PIN (flagged
  `guard_unverified`). Unlock is refused after 7 days without a roster sync.
- Outbox: priority then local sequence; server-time offset applied to event
  times; poison items parked after 50 attempts / 3 days; replayed "already
  done" answers count as sent; device-level 403s pause the queue; 429 honours
  Retry-After.
- Health beacon every 15 min to `POST /device/telemetry` (backend endpoint to
  be added); crashes kept in a local ring buffer.

### D-040 — Web hardening conventions (audit WEB-*)
- **Date:** 2026-10-12 · **Status:** Accepted
- Settings split: Channels, Backups, Ops alerts and Ops log are visible to
  `platform-admin` only; realm admins see an explanation.
- App idle logout after `IDLE_LOGOUT_MINUTES` (default 30, 0 = off) of no
  keyboard/pointer input; polling never counts as activity.
- SOS banner covers every site the user may see (`GET /sos/active`, falling
  back to per-site polling), with sound, title flash and optional desktop
  notifications.
- Demo mode refuses to start when the runtime config points at a real API or
  Keycloak unless `DEMO_FORCE=true`.
- Tests: Vitest unit tests + Playwright smoke with axe (serious/critical fail)
  in demo mode for every role at 1280 px and 390 px; the e2e server sends the
  production CSP, so CSP violations fail CI.

### D-041 — User and device-account management via Keycloak Admin REST (F1, NFR-09)
- **Date:** 2026-10-12 · **Status:** Accepted
- Per realm a confidential client `securepatrol-admin-api` (service account with
  only `view-users`, `query-users`, `manage-users`, `view-realm`), created by
  `keycloak-config.sh`; secrets in `infra/secrets/keycloak-admin-clients`
  (compose secret, re-read by the API).
- `/users` and `/device-accounts` (admin, own realm): assignable roles admin,
  supervisor, hr, client-readonly; `platform-admin` only by a platform-admin;
  no self-lockout; site-scoped admins confined to their sites. Temporary
  passwords shown once, never audited. Tablet accounts `device-<slug>` with only
  the `device` role and an `@device.invalid` e-mail (Keycloak 24 profile).

### D-042 — Site scoping by token claim `sites` (F4, implements D-035)
- **Date:** 2026-10-12 · **Status:** Accepted
- User attribute `sites` → access-token claim via a mapper on the web client.
  Non-empty claim limits `siteFor` and every realm-wide list; other sites answer
  404. Empty/missing = whole realm; a claim with only invalid values = no site.
  `/audit` stays realm-wide (admins only).

### D-043 — Effective incident values (F7, amends D-009)
- **Date:** 2026-10-12 · **Status:** Accepted
- Reads, filters, dashboard and exports use the latest amendment per field
  (`incidents_effective` view) with `amended` + `original_*`; the stored
  original row never changes.

### D-044 — Route snapshots (F9)
- **Date:** 2026-10-12 · **Status:** Accepted
- A session stores its route's checkpoints at start; progress, reports and SLA
  use the snapshot. Scan matching uses the current route so a QR regeneration
  (D-037) takes effect immediately.

### D-045 — Exports and paging (F5)
- **Date:** 2026-10-12 · **Status:** Accepted
- `format=csv|xlsx` on every report; server-side PDF for the incident log and
  incident detail (DejaVu fonts embedded). Limits 50 000 rows (CSV/XLSX) and
  2 000 (PDF) → 400 `export_too_large`; never silent truncation. Paged lists
  return `X-Total-Count` / `X-Truncated`.

### D-046 — Retention and PII (BE-S9, UU PDP)
- **Date:** 2026-10-12 · **Status:** Accepted
- Hourly retention job: visitor logs + photos 365 d, selfies 365 d, attendance/
  scan GPS 365 d, notification bodies/recipients 180 d, evidence photos kept
  (`RETENTION_*_DAYS`, 0 = forever); all object versions deleted. `audit_log` is
  never purged in v1. Visitor ID numbers AES-GCM encrypted (HKDF from
  `SETTINGS_ENCRYPTION_KEY`, info "securepatrol pii v1"), masked except on the
  visitor detail for admin/supervisor.

### D-047 — Channels in code, `incident_filed` (F8, F10)
- **Date:** 2026-10-12 · **Status:** Accepted
- Channel registry `notification.Channels` replaces the DB CHECK
  (`GET /notifications/channels`). `incident_filed` fires for every incident,
  next to `incident_high` (D-035).

### D-048 — Site floor plans (F3, implements D-035)
- **Date:** 2026-10-12 · **Status:** Accepted
- Admin-uploaded floor-plan images per site (write-once, max 20); checkpoints
  carry `floorplan_id` + relative `plan_x`/`plan_y` (0..1); a plan can be deleted
  only when unused. `GET /patrol/live` returns plan positions and last scan
  (fixed number of queries).

### D-049 — Tablet health beacon, incident GPS
- **Date:** 2026-10-12 · **Status:** Accepted
- `POST /device/telemetry` (≤ 1/min) stores the latest beacon in
  `devices.health`, shown in `GET /devices`; gauge `sp_device_outbox_pending`.
  Incidents accept optional `gps_lat`/`gps_lng`, kept as evidence.

### D-050 — Notification channels: e-mail and WhatsApp only
- **Date:** 2026-10-13 · **Status:** Accepted (user decision; supersedes the
  "LINE/Telegram in v2" parts of D-010 and D-047)
- LINE and Telegram are dropped from the roadmap. The channel registry stays as
  the internal validation mechanism (no DB CHECK), with exactly `email` and
  `whatsapp`; unknown channels are rejected (`invalid_channel`).

### D-051 — Monthly recap counts guard-days (F13, FR-REP-05)
- **Date:** 2026-10-13 · **Status:** Accepted (user decision)
- The recap (`/reports/attendance/monthly`, exports, `monthly_recap` e-mail and
  WhatsApp) counts **guard-days**: a calendar day in the site's time zone, keyed
  by the shift's scheduled start (an overnight shift counts on its start day and
  month); all shifts of a guard on one day are one day.
- Day status after overrides: absent if any shift is absent, else late if any
  is late, else present; a day with only pending shifts is `upcoming`.
  `early_out_days` = days with at least one early departure. A guard is counted
  per site for the shifts scheduled there.
- Per-shift fields keep their names (KPI rates stay per shift); responses add
  per-site `totals`; exports end with a total row; e-mail/WhatsApp in the site's
  language. Indonesian zones have no DST.

### D-052 — Keycloak 26 (supersedes the Keycloak 24 parts of D-034 and the "Keycloak 24 profile" note in D-041)
- **Date:** 2026-10-13 · **Status:** Accepted (user decision)
- Keycloak `26.8.0` pinned by digest, stock image with `start`. Hostname v2
  `KC_HOSTNAME=KEYCLOAK_PUBLIC_URL` (issuer unchanged:
  `https://<host>/auth/realms/<realm>`); `xforwarded` headers trusted only from
  the stack network; nginx drops client `X-Forwarded-Prefix`. Health and
  metrics only on management port 9000 (`/auth/...`), never published/proxied.
- Bootstrap admin via `KC_BOOTSTRAP_ADMIN_*` (preflight rejects the old
  `KEYCLOAK_ADMIN*` names).
- Token contract: Full Scope Allowed off on web, tablet and admin-api clients
  with explicit role scope (web: the 5 web roles; tablet: `device`; admin-api:
  its 4 realm-management roles); the tablet client always has default scope
  `basic` (`sub`). Device accounts keep a full profile (VERIFY_PROFILE).
- Every Keycloak update migrates its DB one way: backup → deploy →
  `make keycloak-smoke`. Rollback = previous image + pre-upgrade dump restored
  with `RESTORE_FRESH=1`. Rehearse the 24→26 migration on a copy of the
  production Keycloak dump first. keycloak-js follows its own 26.x line.

### D-053 — Product name GARDA (rebrand from SecurePatrol)
- **Date:** 2026-10-14 · **Status:** Accepted (user decision)
- The product is called **GARDA** (all caps, no tagline) everywhere users see
  it: web, tablet app, e-mail/WhatsApp texts, exports, Keycloak display names
  (`GARDA — <client>`), alerts, dashboards and docs. Tablet app id
  `id.co.garda.tablet` (Android + iOS); nothing was deployed, so no migration.
- Technical identifiers keep `securepatrol` (Go module/binary, Dart package,
  Keycloak client and realm IDs, database/MinIO/Redis names, `sp_` metrics, `SP1:`
  QR prefix, HKDF info strings, method channel, Docker images/network):
  renaming them would break tokens, keys and data for no user benefit.
- One icon source `branding/icon.svg`; `branding/generate.sh` produces every
  web/PWA/launcher icon and the web and tablet brand marks render that icon. The
  web app ships `/manifest.webmanifest` (needed for iOS web push).
