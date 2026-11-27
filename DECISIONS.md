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

### D-058 — Flutter 3.47 toolchain (supersedes the Flutter 3.24 pins of D-039/D-057)
- **Date:** 2026-10-16 · **Status:** Accepted
- Tablet app on Flutter 3.47.7 (Dart 3.13), AGP 9 / Gradle 9.1 / Kotlin 2.2,
  Java 17; CI pins the same `FLUTTER_VERSION` — keep it equal to the developers'
  `flutter --version`. AGP 9 needs `buildFeatures.resValues` for the FCM
  resources written from the dart-defines.
- Plugins moved to current majors (Android API 35+): firebase_core 5,
  firebase_messaging 17, flutter_local_notifications 22, flutter_secure_storage 11
  (its own Keystore ciphers; EncryptedSharedPreferences is gone — no tablets
  were deployed, so no data migration), connectivity_plus 7, image_picker 1.2,
  timezone 0.11; the geolocator_android pin is gone.
- FCM still uses registration tokens (`getToken`, deprecated in
  firebase_messaging 17) because the backend sends via the FCM HTTP v1 API with
  tokens; moving to Firebase Installation IDs (`register()`/`onRegistered`) is a
  backend + app change for later (docs/maintenance.md).
- Dependabot for `mobile/` is active again (minor/patch, monthly).

### D-059 — Checkpoint GPS positions are learned from scans (supersedes the manual `lat/lng` of D-014/D-035)
- **Date:** 2026-10-20 · **Status:** Accepted (user decision)
- Admins no longer type checkpoint coordinates; the route editor only places
  checkpoints on the floor plan (D-048). A floor-plan position is not a GPS
  position, and a web map would need a foreign tile origin (CSP, on-premise).
- `lat/lng` on route checkpoints are server-owned: POST/PUT ignore submitted
  values and an edit keeps the stored ones (same checkpoint `id`).
- Worker job `checkpoint_positions` (every 6 h) sets each checkpoint to the
  median GPS fix of its QR scans of the last 30 days, once there are ≥ 5 such
  scans by ≥ 2 guards; manual check-ins and `qr_invalid` scans are ignored.
  The median shrugs off outliers and follows a QR sticker that was moved.
  Sessions keep their route snapshot, so new positions apply to sessions started
  afterwards. `gps_far`/`too_fast` stay off until a checkpoint is learned.
- Known limit: if every early scan is made from the wrong place (e.g. a
  photographed QR scanned at the post), that place is learned. Scan photos,
  `qr_invalid` and the live patrol view remain the checks for that case.

### D-060 — Sites are deactivated (soft delete), never hard-deleted
- **Date:** 2026-11-17 · **Status:** Accepted (issue #34)
- `sites.active` (migration 009). Hard delete would cascade over attendance,
  patrol and incident history, so there is no `DELETE /sites/:id`.
- `PATCH /sites/:id/deactivate|activate` (admin). Deactivating is refused (409)
  while the site has shifts starting in the future or active tablets.
- `GET /sites` returns active sites only; admins can pass `include_inactive=true`.
  Inactive sites are read-only (PUT answers 409 `site_inactive`) and skipped by
  scheduler jobs (`ListAllSites`). `GET /sites/:id` still works for history.


### D-061 — HR no longer receives `attendance_review` pushes
The review queue (`/attendance?tab=review`, `reviewAttendance`) is admin/supervisor only, so the push linked to a page HR cannot open (#40). HR is dropped from the `attendance_review` default audience; HR keeps `absent` and `late_clock_in`. Supersedes the HR default of D-007's push matrix.

### D-062 — Live patrol updates: WebSocket nudge (supersedes the "polling first" of D-014)
- **Date:** 2026-11-18 · **Status:** Accepted
- `GET /api/v1/ws/patrol?ticket=` pushes `{"type":"patrol_changed","site_id"}` when a
  patrol of the site starts, gets a new scan or ends. No patrol data travels on the
  socket: the web client refetches `GET /patrol/live`, so permissions and payload stay
  in one place.
- Auth: browsers cannot set `Authorization` on a WebSocket and a JWT in the URL would
  reach proxy logs, so `POST /ws/ticket {site_id}` (Bearer, same roles and realm/site
  check as `/patrol/live`) returns a 30 s single-use ticket. The socket route sits
  outside the JWT chain; the ticket is its credential.
- Cost: an in-process hub (`internal/livehub`), no new service. Limit: single API
  instance (the worker does not publish; its automatic session closing is covered by
  polling). To scale out, swap the hub for Redis pub/sub behind the same API.
- Polling stays as the fallback: 60 s while connected (safety net), 10 s while not.
  Max 50 sockets per site. CSP `connect-src 'self'` covers same-origin `wss:` in
  current browsers, so the CSP is unchanged.

### D-063 — Project skills for agents live in `.claude/skills/` and are committed

- `.claude/skills/` is tracked (the rest of `.claude/` stays ignored), so every agent on
  every PC gets the same skills.
- `verify`: drives the web dashboard on the demo build under the production CSP and keeps
  evidence in `.claude/verify/evidence/` (ignored). A pass there proves the web app only,
  not the API, SQL or Keycloak; the skill names the tiers for those.
- `sos-delivery`: an exhaustive model check of the SOS delivery outbox (claim, lease,
  retry, give-up, ops alert). A change to `notification/worker.go` or
  `repository/notification_outbox.go` runs `node .claude/skills/sos-delivery/check.mjs`
  and updates the model with the code. Plain Node, no new dependency.
- Open finding F1 from that check: `MarkDeliveryFailed` and the `ops_sos_undelivered`
  alert are two transactions, so an SOS delivery can end `failed` with no ops alert
  (Publish error, or the process dies in between). Not fixed here; tracked in PROGRESS.md.

### D-064 — An SOS delivery gives up in one transaction with its ops alert (closes finding F1 of D-063)

- `Worker.deliver` used to run `MarkDeliveryFailed` and then publish `ops_sos_undelivered`
  in a second transaction. A publish error, or a process death in between, left the SOS
  delivery `failed` with no ops alert, and nothing claimed it again.
- Now the store's `MarkFailed(ctx, id, msg, also)` runs the status change and
  `Notifier.Stage` of the ops alert in one transaction. When it fails the row stays
  queued and leased, and the last attempt is made again after the lease.
- A failed `MarkFailed` no longer counts as a failed delivery in the metrics (it is
  counted when it commits).
- Still configuration, not code: with no ops alert rule for `ops_sos_undelivered` the
  alert has no recipient (`stageOps` logs a warning).
- Checked by `TestUndeliveredSOSFailsOnlyWithItsOpsAlert` and by the model in
  `.claude/skills/sos-delivery` (`NoSilentLoss` now holds; `mutant-alert` is the old shape).

### D-065 — A tablet SOS acknowledgement counts only with a verified PIN; without one it is noted and the alert stays open

- Before: `POST /sos/:id/acknowledge` from a tablet needed only `X-Guard-ID` of a supervisor.
  Whoever held the device token could close an alarm for everyone, with no trace of that.
- Decided by the product owner (2026-11-22): do not refuse the unverified acknowledgement
  (a supervisor who unlocked the tablet offline has no guard token), record it apart.
- Verified guard token, or a web user: `acknowledged_by/at`, as before.
- No verified guard token: `unverified_ack_by` (guard name) and `unverified_ack_at`
  (migration 010). The site's tablets get `sos_ack` once and `GET /device/sos` stops
  listing the alert. `acknowledged_at` stays NULL, so the web banner, the unacknowledged
  counts and the `sos_unacknowledged` reminders continue until a verified or web
  acknowledgement. The web SOS list shows who responded on the tablet.
- Accepted limit: a holder of the device token can still quiet the tablets of that site.
  They cannot quiet the web, push or e-mail/WhatsApp side.
- After an unverified response the alert is closed from the web in practice: the tablets no
  longer list it, so the app cannot offer a second, verified acknowledgement (the API would
  accept one).
- The acknowledgement carries no event time. One that was queued offline with a valid token
  and synced after the token expired is recorded as unverified; the web then asks to confirm.
- Tablet app: no change. Its "SOS acknowledged" message does not yet tell the two cases apart.

### D-066 — client-readonly keeps receiving signed checkpoint QR codes

- `GET /patrol/routes` returns valid signed QR payloads to `client-readonly` (audit 2026-11-21).
- Decided by the product owner (2026-11-22): keep it. The codes are printed and stuck at the
  checkpoints, so they are not secret from anyone on site; the client prints sheets too.
- The protection of a scan is the tablet, the guard and the scan radius, not the secrecy of the code.


### D-067 — Public certificates through `scripts/acme.sh` (certbot webroot), kept next to the private CA

- Convention change (TAQ-10): `make acme-issue` / `make acme-renew` (`infra/scripts/acme.sh`) obtain and renew a
  Let's Encrypt certificate with certbot in a container, validate it with the preflight rules, install it as
  `nginx/certs/cert.pem` + `key.pem` and reload nginx. The private CA flow (`make certs`) stays the default.
- `docker-compose.app.yml` mounts `infra/acme/webroot` into nginx at `/var/www/certbot` (the `:80`
  challenge location already existed); ACME state in `infra/acme/config` is git-ignored.
- Tested against a local Pebble ACME server only; a real Let's Encrypt issue needs a public hostname (open item in the runbook §6).

### D-068 — A platform-admin-only user has no sites: the web does not ask for `/sites`

- **Date:** 2026-11-23 · **Status:** Accepted (TAQ-41; default taken by the agent, owner may overrule)
- `platform-admin` is install-wide (D-025, D-032) and belongs to no site. `GET /sites` (and every other
  site-scoped route) stays realm-role only, so the API answers it with 403 - least privilege, and no
  cross-realm site list is built for the operator. An operator who also holds a realm role (usually `admin`)
  sees that realm's sites as before.
- The bug was in the web: `SiteProvider` fetched `/sites` for everyone, so an operator-only login got a 403 on
  every page and the header said "No sites yet". New capability `viewSites` (the four realm roles); the query
  is `enabled` only with it, and the header shows "Install-wide (no sites)" instead.
- Install-wide pages (Settings: channels, backups, ops alerts/log) do not need a site. A site-less platform
  view across realms (site/guard overview per realm) is not planned; it would need its own endpoints.

### D-069 — Per-site hash chain over attendance, scan and incident records (TAQ-17)
- Migration 011: `record_chain` (append-only, like `audit_log`) + `record_chain_heads`. Each write of an attendance
  record, checkpoint scan, incident or incident amendment appends an entry in the same transaction
  (`repository.inTx` + `chain_append`): `record_hash` = sha256 of the record's canonical form, `entry_hash` =
  sha256(version, site, seq, kind, record id, record_hash, prev_hash, origin, created_at). The first `prev_hash` is a per-site genesis value.
- Canonical form: the listed columns as jsonb text (keys sorted), timestamps in UTC, function-pinned session settings.
  Columns added later are not covered until a new `hash_version` lists them (a new column is not a break).
- Order: `chain_append` locks the site's head row, so writers of one site take turns; `seq` has no gaps.
- Every legitimate change (clock-out, status, override, retention clearing GPS/photos) appends a new entry; the
  verifier compares the live record with its latest entry. `chain_first_break(site)` / `repository.FirstChainBreak`
  returns the first broken seq (gap, link, entry content, record edited/deleted, dropped tail). Endpoint/CLI: TAQ-18.
- Existing rows are chained by the migration (`origin = 'backfill'`, oldest first per site): they vouch for the state
  at migration time only. Run the migration with the app stopped; later unchained writes by old code are not flagged.
- Also found: records with no chain entry (forged inserts, or entries deleted together with the head), a missing
  or mismatching head, reordered entries. The head row only advances by one (trigger); deleting or resetting it is blocked.
- **Open limit (not solved here):** this detects edits and deletions made *outside* the application path. The app/owner
  role (the one that runs migrations and owns the tables) can disable the triggers, call `chain_append` and
  `chain_entry_hash` and advance the head by one (the tests do `ALTER TABLE ... DISABLE TRIGGER USER` through the app
  pool), so an edit followed by `chain_append` is chained as the new truth, and anyone with superuser rights can
  rewrite records, chain and head together. Hardening follow-ups: split the migration owner from the runtime role and
  REVOKE EXECUTE on `chain_append`, together with the signer service. Closing it needs an external anchor (head hash published outside the
  database) or signed heads. It is not "verifiable without trusting the operator" yet; the owner decides how to handle it.

### D-072 - Time-bound checkpoint QR codes (TAQ-19)

- **Date:** 2026-11-26 · **Status:** Accepted (defaults taken by the agent, owner may overrule)
- The printed text is now `SP2:<checkpoint id>:<from>:<to>:<sig>` (Unix seconds). `sig` is the first 22 base64url
  characters of HMAC-SHA256 over `SP2:<id>:<qr_version>:<from>:<to>` with the **same QR key** as D-037 (HKDF info
  unchanged, so `QR_SIGNING_SECRET` rotation and *Regenerate QR codes* still invalidate every print). The `SP2:`
  prefix is inside the signed text, so an SP1 signature cannot be replayed as SP2.
- Issued on every read: `GET /patrol/routes`, `GET /patrol/routes/:id` and the create/update/regenerate responses
  decorate each signed checkpoint with a window of `[now, now + QR_WINDOW]`. Nothing is stored; **the print flow for
  admins is unchanged** - printing issues a fresh window.
- `QR_WINDOW` default **720h (30 days)**, allowed 1h..8784h. Reason: a printed sticker cannot rotate, so the window
  is the replay horizon of a photo of the sticker and also the reprint interval. Short windows would make guards'
  scans stale on schedule. Per-site / per-route windows are not built (simpler first).
- Verification uses the scan's **event time** (`scanned_at`) with `QRSkew` = 10 min at both ends for tablet clock
  drift, **and** the server's receipt time: `scanned_at` is client-supplied and `ClientTime` accepts anything from the
  last 7 days, so the event time alone would let a photo of a code that expired days ago be backdated into the
  window. A scan is therefore `qr_stale` unless the event time is inside the window **and** it is received no later
  than `QROfflineGrace` (24 h) + skew after the window ends. A scan captured offline inside the window still
  verifies when it syncs, including up to a day after expiry; the offline queue does not need the window open.
- **Replay horizon:** window + 10 min + 24 h (about 31 days), not window + 7 days. Remaining limit: a guard who
  is offline (or whose tablet clock is set back) and lies about the time can still replay a photo of a code until
  that horizon; `late_sync`/`clock_ahead` and the GPS/photo checks are the other signals. A genuine offline scan
  synced more than a day after the window ended is flagged, not dropped.
- Stale scans are **accepted and flagged `qr_stale`** (never rejected, never dropped), like `qr_invalid` (D-037):
  an offline scan older than the window must not be lost, and a wrong clock or a late reprint must not stop a
  patrol. A genuine windowless `SP1:` code is flagged `qr_stale` too (reprint to get a window); the tampered
  window, wrong version or wrong key remain `qr_invalid`. Both count as flagged scans for review.
- Tablets still never receive payloads; the mobile app parses SP2 only to find the checkpoint, forwards the raw text
  with `scanned_at`, and does not judge the window (no key). The clock-skew and offline rules are the server's.
- Known limits: `scanned_at` is client-supplied (bounded by `ClientTime`, plus `late_sync`/`clock_ahead`), and the web
  session-progress view still shows the stored windowless SP1 text. The signed-heads signer is a separate task.

### D-070 — Fraud-signal dashboard: review marks live in their own table

- **Date:** 2026-11-26 · **Status:** Accepted (TAQ-21; defaults taken by the agent, owner may overrule)
- `GET /flags` lists the existing plausibility flags (D-035) across attendance, scans, sessions, incidents, SOS,
  defects and visitors, one row per flag; `POST /flags/review` marks one reviewed. Roles: `admin`, `supervisor`
  (same as the attendance review queue). Read-only for `hr` and `client-readonly`: they get 403.
- The mark is a row in `flag_reviews` keyed by (kind, record id, flag). The flagged record is never written, so
  records covered by a hash chain stay intact. The first review wins; there is no un-review.
- `qr_unsigned` (legacy plain QR) is hidden unless asked for, as in the patrol report's flagged-scan count.
- A repeat review (also two at once) keeps the first reviewer, time and note and answers `already_reviewed: true`;
  the UI says so. `flag_reviews` rows cannot be updated (trigger in migration 012). A REVOKE was not used: the app
  role owns its tables (infra/postgres/init/01-roles.sh has no separate migration role), so it would not bind.
- `GET /flags` needs `from` and `to` (max 92 days) and offset <= 5000; ids, sites and days are filtered inside each
  record table before the union.
- The record link carries `?site=` (the page switches the top-bar site) and, for attendance, scans, sessions and
  incidents, `?date=`.
- The page has its own site/guard filters (all sites by default) instead of the global site selector, like the
  audit log; it is realm-wide. "Open record" opens the right page, tab and (attendance) day; the pages do not
  focus a single record yet.

### D-071 — Signed chain heads, signer service, owner/runtime role split (TAQ-56)
- Closes the "edit laundering" limit of D-069 as far as it can be closed in software: a separate **signer** container
  (own uid 10010, Ed25519 key in a `0400` file on its own volume, DB role `sp_signer`: SELECT on the chain tables,
  INSERT on `signed_heads` and `keyring`) signs `{site, seq, entry_hash, time, key_id}` hourly, at the site's day end
  and on demand. It refuses a lower seq, a different hash at a signed seq, a changed entry at its last signed seq and
  broken links since. Migration 013 also enforces the first two with a trigger (the signer role cannot bypass it).
- `signed_heads` and `keyring` are append-only. Keyring: `genesis` (epoch 1), `rotation` (same epoch; signed by the new
  key and the old key), `new_epoch` (after loss/compromise; signed by the new key only, so every client must re-pin its
  fingerprint out of band). A key is retired when its successor appears; heads it signs later are refused.
  `signer.VerifyHeads` / `repository.VerifyChain` = chain check + signatures + "the entry at each signed seq still
  has the signed hash" + "the chain still reaches the highest signed seq". It catches a rewritten and re-chained
  past, which `chain_first_break` alone accepts. Endpoint and offline CLI: TAQ-18.
- **Role split.** The migration owner (`APP_DB_USER`, `MIGRATION_DATABASE_URL`) owns the tables; api and worker run as
  `sp_runtime` (DML, no ownership, no DDL, no write on the chain/signer tables, cannot `DISABLE TRIGGER`). New tables
  reach it through default privileges. Without the roles (dev DB) the grants are skipped and one role owns everything.
- **EXECUTE decision.** `chain_entry_hash` and `chain_links_break` are revoked from PUBLIC (runtime has no direct access;
  `chain_first_break` and `chain_append` now run as the owner, `SECURITY DEFINER`, fixed `search_path`).
  **`chain_append` stays executable by `sp_runtime`**: the repository calls it in the transaction of every record write
  and a trigger-based redesign would rewrite D-069. Compensating controls: it is `SECURITY DEFINER` so the runtime role
  needs no write access to the chain; it only appends an entry that reflects the record's *current* state (visible, with
  `origin` and time); the past entries are append-only and anchored by `signed_heads`.
- **Does not buy:** the app role can still edit a record and chain the false state as a new entry; a superuser/host
  owner can rewrite database and chain, but then not the signed past, unless they also take the signer's key (separate
  volume/uid; an operator with root on the host can read it: file-based key, TPM sealing is the upgrade path, no HSM) or
  the client's pinned fingerprint. Claim wording: "tamper-evident history anchored outside the app", not "no trust in the vendor".
- **The signer does not trust the database for its memory (review of #66).** `signed_heads` is writable by the owner role,
  so on its own it is no defence: wipe it, rewrite the past, re-chain, and the signer would sign the rewrite with the
  same legitimate key. The signer therefore keeps its own append-only record, `/keys/signed-heads.jsonl` on its volume,
  with every head (and signature) it signed, written and fsynced BEFORE the database insert. It refuses a lower seq or a
  different hash at a seq in that file, whatever the database says, and when the file holds heads the database lacks it
  refuses to sign that site, logs "SIGNED HEADS LOST FROM THE DATABASE" and the health goes red (the one repaired case:
  the newest head, when the chain still holds exactly that entry, i.e. a crash between disk and insert). `signer
  restore-heads` re-inserts from the file after the loss has been looked at. `signer export` writes the heads (from the
  file) and the keyring as JSON, to be copied off the box (daily delivery: TAQ-22). `signer verify` also checks the file.
- Verifier rules (review of #66): a key is trusted only if reachable from a pinned fingerprint (a pinned key is trusted as
  such; with no pin at all only the first genesis key); a key that more than one valid handover leads away from is a fork
  and none of its successors is trusted; site ids are validated and canonicalised (lowercase, hyphenated) before signing and
  in the payload; the table checks hash/signature/key lengths; a chain with entries and no signed head is `UNSIGNED`;
  `--max-age` reports `STALE` when entries follow the newest head and that head is older than the limit (a quiet site is
  never stale).
- **What remains.** Someone who controls the host as root and can write the signer volume (or has the key) can rewrite
  the file and the database together and sign a rewritten past. This raises the bar from "database owner" to "host root
  plus the signer volume", and the export gives an anchor that can leave the box: once a head has been copied off
  (TAQ-22) or pinned, even that attacker cannot change what was already anchored without it showing. Between two
  exports the newest heads are only as safe as the host.
- Migration numbers: 011 (#63), 012 (#65, TAQ-21), 013 (this). golang-migrate tolerates the gap while 012 is missing, but
  a database that has applied 013 would skip a later-arriving 012: merge order is #63, #65, #66.
- The signer binary ships in the api image (`/app/signer`) and runs as its own service; a minimal separate image is a
  later hardening. The key is created at install time (`make signer-key`), never in an image, `.env` or log.

### D-073 - Chain verification: one verifier, a bundle, an endpoint and an offline CLI (TAQ-18)
- **One engine.** `internal/chainproof` verifies a *bundle* (pure Go, no database). The online endpoint builds the bundle
  from the database and runs the same `Verify`; the offline `garda-verify` reads the bundle from a file. Same data, same
  answer. It reuses the signer's key rules (`signer.NewTrust`: pins, rotation, forks, retirement) and recomputes the
  hash_version 1 entry hash of migration 011. It does not touch chain writing or signing.
- **Bundle** (zip, `garda-chain-bundle/v1`): `manifest.json` (site, range, `anchor_seq`, seq range, the chain's own head, sha256 of
  the other files), `entries.jsonl`, `heads.json` + `keyring.json` (the `signer export` shapes; public keys and signatures only),
  `README.txt`. The sha256 of `manifest.json` is the bundle's integrity hash (header `X-Bundle-SHA256`, printed by the CLI): it
  catches damage and a casual edit; an edit made together with the manifest is caught by the chain and the signed heads.
- **Record data.** Each entry carries the exact text that `record_hash` hashes (`record`) for the newest entry of a record
  (new function `chain_record_canonical`, migration 014; a test fails if it drifts from `chain_record_hash`). Older states are
  not kept anywhere, so superseded entries are checked as chain links only (`record_state`, `next_seq`); `records_not_checked`
  counts entries whose next state lies beyond the range. A record deleted or edited in the database shows as such.
  The text holds personal data (guard ids, GPS, descriptions, photo keys), so the export is `admin`/`supervisor` only; the
  verify result (no content) is also open to `client-readonly`.
- **Range by checkpoint.** A date range maps to a seq range; the bundle starts at the newest signed head before it (the anchor:
  its signature vouches for the prev_hash of the first entry) and ends at the first signed head at or after it (or the chain's end).
  A month therefore costs the distance between heads, not the site's history. With no earlier head it starts at the genesis value.
  Tests show the range gives the same first break as the full check for any edit inside it; an edit before the anchor is outside what
  that range proves (the full check finds it). Not covered by the bundle: records with no chain entry at all (`chain_unchained_records`
  scans the whole site) and the age of a *range* that nobody asked about; an empty range is reported as "no entries".
- **Bounded cost.** Online: read-only repeatable-read transaction, `SET LOCAL statement_timeout` 15 s, ≤ 20 000 entries,
  two at a time per process; over the limit or timed out -> 400 `verify_too_large` ("use the offline bundle"). Export: 60 s, 250 000
  entries. Index `(site_id, created_at)` finds the range without a scan. No background job: a result is returned or refused.
- **Trust.** The CLI refuses to run without `--pin` (or the explicit `--trust-first-use`, which says so in its output). Wrong fingerprint
  = exit 4. Exit codes: 0 valid, 1 chain/record broken, 2 usage, 3 bundle unreadable/changed, 4 head/key/signature, 5 UNSIGNED, 6 STALE
  (`--max-age`, default 26 h), 7 UNSIGNED TAIL, 8 UNVERIFIED RECORDS, 9 NO ENTRIES, 10 RANGE MISMATCH, 11 RANGE NOT AUTHENTICATED, 12 UNVERIFIED RECORDS ACCEPTED. A broken chain or head outranks
  the rest; the first break by seq wins.
- **Review hardening (PR #67): the bundle builder is not trusted.** Whoever builds the bundle must not be able to get a plain VALID:
  (1) a bundle with no entries is NO ENTRIES (9), never valid: a signature cannot show that a site had none. (2) Entries after the
  newest signed head are UNSIGNED TAIL (7) (STALE if that head is old); the CLI prints the newest head's `signed_at` and `exported_at`,
  and warns when the bundle is older than `--max-age`. (3) A record said to be superseded by an entry beyond the bundle is UNVERIFIED
  RECORDS (8), not skipped; `--allow-unverified-records` accepts it and the output still says `VALID WITH UNVERIFIED RECORDS`.
  (4) The manifest range is only a claim: `--expect-from/--expect-to` (dates, site zone) require the anchor head to be signed before the
  range starts and the newest head after it ends (else RANGE MISMATCH, 10); without them the output says `NOT AUTHENTICATED`. To make that
  possible the exporter now picks the anchor as the newest head signed at or before `from` and the end as the first head signed at or
  after `to`. Entries must also fit the manifest range. Cost: a bundle may start up to an hour earlier and end later than the range.
  (5) Online: no pin -> `valid: null`, `trust: "unverified_trust"` (the server vouches for itself); `valid_pinned` when a pin is given.
  (6) Second review: no silent exit 0. Without `--expect-from/--expect-to` the result is RANGE NOT AUTHENTICATED (11) (a cut at an older head, a
  late anchor and a replayed old bundle are otherwise indistinguishable from a good one); `--no-range-check` restores exit 0 and prints
  `VALID (range not checked)`. `--allow-unverified-records` no longer gives exit 0: 12, with the count and share of entries not checked.
  An expected range ending today or later cannot pass (a head cannot be signed after the end of today). The online endpoint passes the
  requested range itself (`NoRangeCheck`); read `status`, not `valid`. Follow-up issue (not in this PR): an on-demand "sign head now" at
  export time, and extending the bundle end to cover `next_seq` heads / shipping superseding entries.
  Verify and export have separate slots (2 each), one running and 6 per minute per user, and the zip held in memory is capped (256 MB).
- **Does not do:** prove the date range is complete without `--expect-*` (the heads before and after it prove nothing is missing *between*
  them); defend against a host root plus the signer's key volume (no key custody boundary is left to cut); replace the signer's own record
  (`signer export` heads can be cross-checked by hand; an `--anchor` option is a follow-up with TAQ-22).
- **Known gaps (review notes).** Exports are not audit-logged yet (they hold personal data; admin/supervisor only): add with the audit work.
  The manifest file hashes only catch damage and careless edits, not a forger; the chain and the signed heads do the protecting.

### D-074 - Signed monthly proof pack (TAQ-20)
- **What.** `GET /sites/:id/proof-pack?month=&lang=` returns a PDF (`internal/proofpack`, built with the existing `export.Doc`
  on go-pdf/fpdf: no new dependency, no migration). It holds the month's attendance (guard-days and shifts), patrols,
  incident counts by severity and warning-flag counts (TAQ-21), per day and per guard (names and counts only: no employee
  id, photo, GPS or incident text), the chain section (entries in the month, the chain head, the newest signed head with
  seq, signed_at, key_id, signature, and the key fingerprint to pin) and, on its own last page, the steps to check it with
  `garda-verify` and the "What VALID does NOT prove" list. English and Indonesian ("Satpam"); default is the site language.
- **Same engine, no second opinion.** The chain section comes from `chainproof.Export` and `chainproof.Verify` (the code
  behind the export and `garda-verify`), run for the month with `--expect-from/--expect-to` as the instructions tell the
  reader. The pack never signs, hashes or judges the chain itself. The command printed on the last page is the one that
  gives the result the pack states.
- **Honest status.** `signed` only when `Verify` says VALID, i.e. a signature made after the month ended covers the last
  entry. No signed head, entries after the newest one, or a newest head older than the end of the month is **UNSIGNED
  TAIL**, in a banner on the first page and again on the last; a broken chain is **CHAIN CHECK FAILED**; a month without
  chain entries is **NO ACTIVITY** (a pack, not an error, and not a proof that nothing happened). The "passed" wording says
  the server checked itself and points to the client's own check. The current month can be generated; it says it is not over.
- **Who.** `admin` and `supervisor` (the export's roles; the pack names guards and counts, the bundle behind it holds
  record text). Another realm's site is 404. It uses the chain export's slots, per-user limit and size limits.
- **Schedule.** Job `proof_packs` (every 10 minutes, same window as the monthly recap: from 07:00 site time on day 1 to day 5)
  keeps the previous month's pack in the backups bucket. Rules: `proof-packs/<site>/<month>.pdf` (final) is written once, only
  when the pack's status is `signed`, and a month that has it is skipped. Until then the latest pack is kept as
  `<month>.open.pdf` whatever its status (`unsigned_tail`, `failed`, `not_proven`, `no_activity`); the status is stored as object
  metadata (`Proofpack-Status`) and served as `X-Proofpack-Status`, so an open copy can never be mistaken for a signed one. The
  open copy is written at the first run and replaced by the latest pack in the first ten minutes of each hour (the signer signs
  hourly), never after the final copy exists. A month too large to check here (or timing out) still gets a pack, `not_proven`,
  saying the chain was not checked. Existence checks and a conditional write make a rerun, a second worker or a crash harmless.
  `?stored=true` reads the kept copy (final first), with the same per-user limit and slots as the other chain reads.
  A month nobody generated before the window closed has no kept copy: generate it from the web.
- **Figures are not signed (review of PR #68).** The attendance, patrol, incident and flag figures are read from the database
  when the pack is made; `garda-verify` does not check them and no signature covers them. The PDF says so directly under the
  summary heading ("as of <time>; not signed"), in the signed banner ("record chain only") and as a "does NOT prove" bullet.
  Records changed again after the month are counted in the chain section, and `unverified_records` has its own banner.
  The chain and the figures are read in one snapshot inside the export's slot and statement timeout. Guards whose only record
  is an incident get a row, so the table adds up to the total.
- **Known, later.** Empty detail in "CHAIN CHECK FAILED. ." when the verifier gives no reason; `time.Now` vs `service.Now`;
  a stored copy is always in the site language whatever `lang` is asked; the flagged-scan count (session start) and the flag
  table (event time) use different time bases; the hourly retry gate is `now.Minute() < 10`; an empty month is retried hourly
  for five days; a signer outage of about four days or more leaves only an open copy and no alert.
- **Web.** Reports → Proof pack (admin, supervisor): month, language, download the pack, the kept copy, and the export bundle
  its last page asks for (`GET /sites/:id/chain/export`, unchanged).
- **Limits kept.** The pack is made from the database as it is now; garda-verify on a fresh export is still the check that
  does not trust the server. Pinning needs the fingerprint from installation: the one printed in the pack is the server's
  claim, and the instructions say to compare it with the one the client holds.

### D-075 - Verification usability: sign on export, amended records, online states, export audit (TAQ-58)
- **Status.** Accepted 2026-11-27. Follows D-071 (signer), D-073 (verification), D-074 (proof pack). Migration 015. Does not change the
  verifier's rules, the exit codes or the signer's refusal rules.
- **Why.** Four things made the honest case look like the bad one: a bundle ending today was `UNSIGNED TAIL` (exit 7) until the next hourly
  head; a record amended after the bundle's end head gave exit 8, so `--allow-unverified-records` became habit; the online check showed
  `unsigned_tail`, `unverified_records` and `no_entries` as `valid:false`, like a broken chain; and exports, which hold personal data, left no trace.
- **Sign on export.** Before the snapshot (so the snapshot sees the new row), the bundle export, the proof-pack endpoint, the stored-pack
  job and the scheduled job call the signer's existing on-demand `POST /sign` (`signer.Client`, `SIGNER_URL`, internal network only, 8 s). The
  signer decides: its refusal rules (lower seq, other hash at a signed seq, broken links, lost heads) are untouched and a refusal is passed on, never
  retried or worked around. An unreachable, refusing or unconfigured signer never fails the export: the bundle is made, ends on whatever signed
  head exists, and says why in `manifest.json` (`signing`, `signing_reason`) and `X-Bundle-Signing`. The manifest is unsigned and only explains: the
  verifier still decides from the signatures (a manifest claiming "signed" cannot turn an unsigned tail into valid; tested). The online check never signs.
  *Rate limit:* a head obtained for a site is reused for 10 s (`signer.ReuseWindow`, per API process), so a burst of exports signs once; entries made in
  those seconds can still be an unsigned tail. An unchanged head is idempotent in the signer anyway.
- **Range ending today.** Unchanged and on purpose: `--expect-to` for a day that is not over cannot be authenticated, because no signature can vouch for entries made after it
  (exit 10). The honest flow is to export a day after it ended; the fresh head is then signed after the range ended. Accepting an open range would let an exporter cut a bundle at an old head
  and hide the hours after it, which is what the check exists to catch.
- **Amended records.** `chainproof.Export` finds the newest entry that supersedes an entry of the bundle; if it lies beyond the end, the end moves on to the first signed head at or
  after it (else the chain's end), repeating until nothing points past the end, bounded by the entry limit. The verifier is unchanged: `superseded` with a `next_seq` beyond the bundle is still
  exit 8 (a bundle cut back, or a label nothing proves; the attacker tests of D-073 stand, plus `TestForgedSupersededLabelStillExits8`). Cost: a record amended much later pulls the bundle
  to a later head; past the entry limit it stops extending and the records stay unverified instead of failing the export.
- **Online states.** The response gains `summary: {state, tone, message}`: `valid_pinned`, `unverified_trust`, `unsigned_tail` (also `unsigned` and `stale`), `unverified_records`, `no_entries`, `broken`
  (also `bundle_invalid`). `valid` is `true` only for a pinned pass, `false` only for `broken`, `null` for everything else (D-073 kept `null` for "not shown to the caller"; it now also
  means "not proven yet"). `status` stays the raw verifier status. No web page shows the check today; the contract is in `docs/api.md`.
- **Export audit.** Table `export_audit` (append-only by trigger like `audit_log`): realm, site, actor (user name, or `system:scheduler`), kind (`bundle`, `proof_pack`), range, bytes, result
  (`ok`, `too_large`, `failed`), signing outcome, short detail; no record content. Written for every bundle and proof pack, including the stored copy read and the scheduled keep, and for
  refused or failed ones. Fail closed: if the row cannot be written the file is not sent. `GET /sites/:id/chain/exports` (admin, supervisor, the site's realm; other realm 404, hr and
  client-readonly 403). Not tamper-proof against the database owner, and it does not follow the file after download.
- **Review changes (same PR).** (1) Requests turned away at the gate (`verify_busy`: slot or per-user rate limit) are not audited, on both the bundle and the proof-pack path: the table is append-only, so
  an unaudited loop of rejected calls would otherwise fill it for good; real attempts and real failures (too large, failed) are. (2) Real scale of the scheduler's rows: the open copy is refreshed in the first ten minutes of every hour
  while no signed copy exists, about 24 rows a day and 720 a month per site if each refresh were audited, with no retention. Decision: the job audits only the final kept copy (once per site and month) and its failures (at most one a day per
  site), as `kind scheduled`, actor `system`; the hourly refresh is not recorded. The list leaves `scheduled` rows out unless `kind=scheduled` or `include_system=true`, and takes `actor` and `kind` filters. Migration 015 was changed
  for this before it was merged (kind `scheduled`). (3) The signer's own refusal text no longer goes into the manifest, the verifier output or the audit: reasons are fixed codes (`signer_unreachable`, `signer_refused`, `signer_unconfigured`,
  `signer_error`); the raw text is in the server log only. (4) An unsigned tail after a reused head says so. (5) A failure of the signer is remembered for 5 s (all sites for an outage, the site for a refusal), so a dead signer holds an export slot
  for its timeout once, not on every export. (6) Tests of the amended record and the forged label also run with `--expect-from/--expect-to` and with an anchor, and the extension stopping at the entry limit is tested.
- **Known, later.** A site-limited admin (`sites` claim) reads the audit of the sites they may see only (via `siteFor`); a web page for the audit; Prometheus alert on `signing != signed` rates; the reuse window is per process, not shared between API replicas; 
