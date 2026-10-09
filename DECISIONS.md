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
- **Date:** 2026-10-07 · **Status:** Accepted
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
