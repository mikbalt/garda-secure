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
