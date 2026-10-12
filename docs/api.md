# SecurePatrol API v1

Base path: `/api/v1` (behind nginx: `https://<host>/api/v1`). All endpoints need
`Authorization: Bearer <Keycloak access token>` — an **access** token (`typ`
`Bearer`); ID and refresh tokens are rejected with 401. JSON in/out; timestamps
are RFC 3339 (UTC in responses). Errors: `{"error": "...", "code": "machine_code",
"field"?: "input field"}`; 5xx never carry internal details (`code: internal`)
but carry `request_id` (the `X-Request-ID` the API logs with the error) for
support.

Unauthenticated operational endpoints on the API port (outside `/api/v1`):
`GET /health` (DB + Redis; proxied by nginx), `GET /ready` (DB, Redis and
MinIO reachable → 200 `{status: ready, db, redis, minio}`, else 503) and
`GET /metrics` (Prometheus). nginx proxies `/health` and `/ready` (status
only, for uptime checks) but answers 404 for `/metrics`; scrape it on the
internal network. The worker serves `/health` and
`/metrics` on `WORKER_HTTP_ADDR` (default `:9091`).

Roles (Keycloak realm roles): `admin`, `supervisor`, `hr`, `client-readonly`,
`platform-admin` (web, client `securepatrol-web`) and `device` (tablet, client
`securepatrol-tablet`). Roles are bound to the client the token was issued to:
web roles only count in web-client tokens, `device` only in tablet-client
tokens (a person logging in through the tablet client gets no roles).
`platform-admin` is the install-wide operator (channel settings, backups); see
*Administration*. Each client company has its own realm; web users only see
sites of their realm (other realms' resources answer 404).

**Tablet ("field") endpoints** additionally need the device to be approved and
the header `X-Guard-ID: <guard uuid>` (guard who unlocked the tablet with their
PIN). Create endpoints accept an optional client `id` (UUID) and event time; a
repeat with the same `id` by the **same site and guard** returns the stored row
with **200** instead of 201. An `id` that already belongs to another site's,
realm's or guard's record answers **409 `id_conflict`** (the record is not
returned). Event times must be within [now − 7 days, now + 2 min].

**Limits.** JSON bodies ≤ 64 KB (413 `body_too_large`; `POST /media` keeps its
photo limit). Free-text fields have length limits (characters): names 200,
types/categories/ID types 100, locations 300, descriptions 4000, reasons/notes/
purposes 1000, addresses 500, employee IDs/ID numbers/plates 64 — over the limit
answers 400 `field_too_long` with `field`. Rate limits answer **429
`rate_limited`** with `Retry-After` (seconds): writes per web user (default
120/min) and per tablet (600/min), new SOS alerts per tablet (5/min; retries of
a known `id` are never limited), and failed authentications per client IP
(30/min). Limits fail open when Redis is down.

**Plausibility flags.** Attendance records, checkpoint scans, incidents and SOS
alerts carry `flags: []` (never null) and the server receipt time
`received_at`. Values: `late_sync` (received > 10 min after the event time),
`clock_ahead` (event time > 30 s ahead of the server), `backdated_on_time`
(on-time clock-in received > 10 min late while the tablet was online after the
claimed time — also sets `needs_review`), `gps_far` (scan GPS farther from the
checkpoint than the site's `scan_radius_m`), `too_fast` (> 3 m/s from the
previous scanned checkpoint). Flags never reject a record.

Dates in query strings are site-local calendar dates `YYYY-MM-DD`; months `YYYY-MM`.
Reports accept `format=csv` (UTF-8 with BOM, opens in Excel).

## Identity

| Method | Path | Roles | Notes |
|---|---|---|---|
| GET | `/me` | any | `{subject, username, realm, client_id, roles, device?}` |

## Tablet bootstrap (role `device`)

| Method | Path | Notes |
|---|---|---|
| POST | `/device/register` | `{name}` → device (`status: pending`) until an admin approves |
| GET | `/device/me` | `{device, site?, config?}` — config = site settings incl. `idle_lock_minutes` |
| GET | `/device/roster` | active device: `{site_id, guards:[{id,name,employee_id,role,pin_hash}], generated_at}` — bcrypt PIN hashes for offline unlock |
| GET | `/device/routes` | active routes of the site (cache for offline QR scanning) |
| GET | `/device/shifts` | field: acting guard's shifts −1 day … +7 days |
| GET | `/device/status` | field: `{server_time, config, guard, shift? (running or next), shift_attendance?, open_attendance?, session? (progress of the guard's running patrol)}` — call after unlock/restart |
| GET | `/device/sos` | active device: unacknowledged SOS alerts of the site (any tablet), for supervisors on duty |

Denials: `403 {code: device_unregistered | device_pending | device_revoked | guard_unknown | guard_not_allowed}`, `400 {code: guard_required}`.

## Sites — `GET` web roles, writes `admin`

| Method | Path | Body |
|---|---|---|
| GET | `/sites` | — |
| GET | `/sites/:id` | — |
| POST | `/sites` | `{name, client_id?, address?, timezone (IANA, default Asia/Jakarta), config_json?}` |
| PUT | `/sites/:id` | same |

`config_json` (defaults): `clock_window_minutes` 15, `late_grace_minutes` 5,
`early_out_grace_minutes` 5, `absent_after_minutes` 30, `idle_lock_minutes` 5,
`language` `"id"|"en"`, `scan_radius_m` 150 (0–5000; 0 disables the `gps_far`
check). Responses always contain the full effective config.

## Guards — read `admin, supervisor, hr`; write `admin`

| Method | Path | Body / notes |
|---|---|---|
| GET | `/guards?site_id=` | without `site_id`: all guards of the realm's sites |
| GET | `/guards/:id` | guard has `has_pin` |
| POST | `/guards` | `{name, employee_id?, site_id, role: guard\|supervisor, status: active\|inactive, keycloak_user_id?}` — `employee_id` is unique per client realm: 409 `employee_id_taken` |
| PUT | `/guards/:id` | same (full replace) |
| PUT | `/guards/:id/pin` | `{pin}` 4–6 digits; trivial PINs rejected |

## Devices — `admin`

| Method | Path | Body |
|---|---|---|
| GET | `/devices` | devices of the realm |
| PUT | `/devices/:id` | `{name?, site_id, status: pending\|active\|revoked}` — `active` requires `site_id` |

## Schedules — read `admin, supervisor, hr`; write `admin`

| Method | Path | Body / notes |
|---|---|---|
| GET | `/shift-templates?site_id=` | |
| POST | `/shift-templates` | `{site_id, guard_id, start_time "HH:MM", end_time "HH:MM" (≤ start = next day), weekdays [1=Mon…7=Sun], valid_from "YYYY-MM-DD", valid_until?, active?}` → `{template, generated_shifts}` |
| PUT | `/shift-templates/:id` | same minus site_id → regenerates future shifts |
| DELETE | `/shift-templates/:id` | removes future shifts without attendance |
| GET | `/shifts?site_id=&from=&to=&guard_id=` | max 62 days |
| POST | `/shifts` | one-off `{site_id, guard_id, start_time, end_time}` (RFC 3339) |
| PUT | `/shifts/:id` | `{guard_id?, start_time, end_time}` — detaches from template; 409 if attendance exists |
| DELETE | `/shifts/:id` | 409 if attendance exists |

## Attendance

| Method | Path | Roles | Body / notes |
|---|---|---|---|
| GET | `/attendance/daily?site_id=&date=` | web | `{site_id, date, rows:[{shift_id, guard_id, guard_name, employee_id, shift_start, shift_end, attendance_id?, status: present\|late\|absent\|pending, clock_in_at?, clock_out_at?, late_minutes, early_out_minutes, hours_worked, needs_review, override_reason?, selfie_object_key?, flags[]}]}` |
| GET | `/attendance/review?site_id=` | admin, supervisor | records flagged `needs_review` (offline clock-in after an absent mark, or `backdated_on_time`) |
| POST | `/attendance/:id/review` | admin, supervisor | `{note?}` |
| PUT | `/attendance/shifts/:shiftId/override` | admin | `{status: present\|late\|absent, reason (required), clock_in_at?, clock_out_at?}` |
| POST | `/attendance/clock-in` | field | `{id?, clock_in_at?, selfie_object_key?, gps_lat?, gps_lng?}` → record incl. `received_at`, `flags` — 400 `no_shift`, 409 `already_clocked_in` (another record exists for the shift), `id_conflict`; concurrent retries with the same `id` all get the stored record (one 201, the rest 200) |
| POST | `/attendance/clock-out` | field | `{attendance_id?, clock_out_at?, gps_lat?, gps_lng?}` — 409 `not_clocked_in`, `clock_out_window_closed` |

## Patrol

| Method | Path | Roles | Body / notes |
|---|---|---|---|
| GET | `/patrol/routes?site_id=&active=true` | admin, supervisor, client | |
| GET | `/patrol/routes/:id` | same | |
| POST | `/patrol/routes` | admin | `{site_id, name, active?, checkpoints:[{id?, name, qr_code? (generated), sla_minutes? (null = no SLA), order_index, lat?, lng?}]}` — ≤ 500 checkpoints; `lat/lng` enable the `gps_far`/`too_fast` scan checks |
| PUT | `/patrol/routes/:id` | admin | `{name, checkpoints, active?}` |
| GET | `/patrol/live?site_id=` | admin, supervisor, client | `[{guard_name, progress}]` for running sessions — poll every 10 s |
| GET | `/patrol/sessions?site_id=&from=&to=` | admin, supervisor, client | session summaries incl. `flagged_scans` |
| GET | `/patrol/sessions/:id` | web or field | progress: `{session, route_name, checkpoints:[{…checkpoint, scanned_at?, method?, scan_flags?, due_at?, status: on_time\|late\|overdue\|missed\|pending}], total, scanned, completion_pct, next_index?}` |
| POST | `/patrol/sessions` | field | `{id?, route_id, started_at?}` |
| POST | `/patrol/sessions/:id/scans` | field | `{id?, checkpoint_id \| qr_code, method: qr\|manual, photo_object_key? (required for manual), scanned_at?, gps_lat?, gps_lng?}` → scan incl. `flags` (`gps_far`, `too_fast`, …) |
| POST | `/patrol/sessions/:id/end` | field | `{ended_at?, abandoned?}` |

## Incidents, SOS, defects, visitors

| Method | Path | Roles | Body / notes |
|---|---|---|---|
| GET | `/incidents?site_id=&from=&to=&type=&severity=&status=` | admin, supervisor, client | |
| GET | `/incidents/:id` | same | `{incident, guard_name, amendments[], photos:[{key, url?, uploaded}]}` |
| POST | `/incidents` | field | `{id?, type, severity: low\|medium\|high\|critical, description?, location?, photo_object_keys[≤5], occurred_at?}` |
| PUT | `/incidents/:id/status` | admin, supervisor | `{status: open\|in_progress\|resolved}` |
| POST | `/incidents/:id/amendments` | admin, supervisor | `{changes:{type?,severity?,description?,location?}, reason}` — original stays immutable |
| GET | `/sos?site_id=&unack_only=true` | admin, supervisor | |
| POST | `/sos` | field | `{id?, fired_at?, gps_lat?, gps_lng?}` — retry with same `id` until 2xx; new alerts are rate limited per tablet (429 `rate_limited`, retries of a known `id` never are). The alert and its notification deliveries are stored in one transaction (a 2xx means the notifications are queued); a replay re-creates deliveries that are missing (e.g. a recipient added since), within 24 h |
| POST | `/sos/:id/acknowledge` | admin, supervisor, or field guard with role supervisor | |
| GET | `/defects?site_id=&status=` | admin, supervisor | |
| POST | `/defects` | field | `{id?, defect_type, location?, description?, photo_object_key?, reported_at?}` |
| PUT | `/defects/:id/status` | admin, supervisor | `{status}` |
| GET | `/visitors?site_id=&from=&to=` | admin, supervisor | |
| POST | `/visitors` | field | `{id?, visitor_name, id_type?, id_number?, plate?, host_name?, entry_at?, purpose?, photo_object_key?}` |
| POST | `/visitors/:id/exit` | field | `{exit_at?}` |

## Media (tablet)

`POST /media` multipart: `file` (JPEG/PNG/WebP ≤ 10 MB), `kind`
(`selfie|checkpoint|incident|defect|visitor`), `id?` (UUID). Returns
`{object_key, content_type, size, sha256}`. Keys are deterministic:
`sites/<site_id>/<kind>/<id>.<ext>` — a tablet can reference
`sites/<site>/<kind>/<id>.jpg` in a record **before** uploading (offline) as long
as it later uploads a JPEG with the same `id`. Readers show `uploaded: false` until then.

The server strips metadata before storing (EXIF incl. GPS, XMP, IPTC, comments;
PNG text chunks; WebP EXIF/XMP) — JPEG orientation is kept; `size`/`sha256`
describe the stored (stripped) file. Photos are **write-once evidence**: an
identical re-upload of the same `id` (same stripped size and SHA-256) answers
**200** with the same body (safe retry); any other upload to an existing key
answers **409 `media_exists`** — tablets should treat it as "already uploaded"
and never retry it. Unreadable images: 400 `invalid_image`.

`GET /media/url?key=` (web roles) → `{key, url?, uploaded}` — pre-signed URL (15 min)
for a photo of a site the caller can read, if the role may see that kind:

| kind | roles |
|---|---|
| `selfie` | admin, supervisor, hr |
| `visitor`, `defect` | admin, supervisor |
| `incident`, `checkpoint` | admin, supervisor, client-readonly |

Other roles get 403 `media_forbidden`.

List rows of `/incidents`, `/sos`, `/defects`, `/visitors` and attendance records
(`/attendance/review`, clock-in, override) include `guard_name`. Incidents,
SOS alerts, scans and attendance records include `flags` (and `received_at`).

## Reports & dashboard

| Method | Path | Roles |
|---|---|---|
| GET | `/dashboard/summary?site_id=` | web — `{shifts_today, attended_today, absent_today, active_patrols, open_incidents, sos_alerts_unacknowledged, open_defects, attendance_pending_review, weekly_attendance:[{date,present,late,absent}], incidents_by_severity}` |
| GET | `/reports/attendance/daily?site_id=&date=` | web |
| GET | `/reports/attendance/monthly?site_id=&month=` | web — rows `{guard_name, employee_id, scheduled_shifts, present, late, absent, early_out, pending, total_hours, patrol_sessions, patrol_completion_pct}` |
| GET | `/reports/patrol/sessions?site_id=&from=&to=` | admin, supervisor, client |
| GET | `/reports/incidents?…same filters as /incidents` | admin, supervisor, client |
| GET | `/reports/guards/kpi?site_id=&from=&to=` | admin, supervisor, hr — rows `{guard_name, attendance_rate_pct, on_time_rate_pct, patrol_completion_pct, incidents_filed, sla_breaches}` |

## Administration

Per-realm administration — `admin`:

| Method | Path | Body / notes |
|---|---|---|
| GET | `/notifications/event-types` | `{event_types, channels}` |
| GET | `/notifications/rules?site_id=` | |
| POST | `/notifications/rules` | `{site_id, event_type, channel: email\|whatsapp, recipient}` — WhatsApp numbers are stored as digits in international format (`08…` → `628…`) |
| PUT | `/notifications/rules/:id` | `{enabled}` |
| DELETE | `/notifications/rules/:id` | |
| GET | `/notifications/log?site_id=&status=queued\|sent\|failed&limit=` | |
| POST | `/notifications/log/:id/retry` | failed deliveries only; queues **one** more attempt |
| GET | `/audit?from=&to=&actor=&resource_type=&resource_id=&limit=&offset=` | audit log of the caller's realm, newest first → `{rows, limit, offset, has_more}`. `from`/`to`: RFC 3339 or `YYYY-MM-DD` (UTC, `to` inclusive); `actor`: username or guard ID; `resource_type`: e.g. `guards`, `patrol` (matches `patrol.routes`, `patrol.sessions`), `attendance.shifts`; `limit` ≤ 500 (default 100) |

Audit rows: `{id, user_id, realm, actor, guard_id?, roles[], action ("PUT /api/v1/guards/:id"), method, route, resource_type, resource_id?, params? ({"shiftId": …}), request_id?, summary? (redacted JSON body of staff writes — passwords, tokens, PINs, ID numbers removed), before?, after? (attendance override; guard, site, route, device, shift and template changes), ip, user_agent, status_code, created_at}`. Every POST/PUT/PATCH/DELETE is recorded; the table is append-only.

Install-wide settings, operations alerts and backups — **`platform-admin`**
(if `PLATFORM_ADMIN_REALMS` is set, only platform admins of those realms;
others get 403 `platform_admin_required`):

| Method | Path | Body / notes |
|---|---|---|
| GET | `/notifications/ops-event-types` | `{event_types: [ops_backup_failed, ops_backup_stale, ops_sos_undelivered, ops_job_failing], channels}` |
| GET | `/notifications/ops-rules` | install-wide ops alert recipients `[{id, event_type, channel, recipient, enabled, created_by?, created_at}]` |
| POST | `/notifications/ops-rules` | `{event_type (ops_*), channel: email\|whatsapp, recipient}` → 201 (same recipient rules as site rules); 409 `duplicate` |
| PUT | `/notifications/ops-rules/:id` | `{enabled}` |
| DELETE | `/notifications/ops-rules/:id` | 204 |
| GET | `/notifications/ops-log?status=&limit=` | deliveries of ops alerts (rows without a site) |
| POST | `/notifications/ops-log/:id/retry` | failed ops deliveries only |
| GET | `/settings/channels` | `{smtp:{host,port,user,password,from}, whatsapp:{api_url,token,phone_number_id,template_name,template_lang}}` — secrets masked as `********` |
| PUT | `/settings/channels` | same shape; send `********` to keep a stored secret — only allowed while SMTP host/port/user and WhatsApp `api_url`/`phone_number_id` are unchanged, otherwise 400 `secret_required` (re-enter the password/token). 400 `invalid_smtp_port` (allowed 25, 465, 587, 2525), `invalid_smtp_host`, `smtp_host_not_allowed` (resolves to loopback/private/link-local; allow-list via `SMTP_ALLOWED_PRIVATE_NETS`), `smtp_host_unresolvable`, `whatsapp_url_not_allowed` (must start with a `WHATSAPP_API_URL_ALLOWLIST` prefix, default `https://graph.facebook.com/`), `invalid_phone_number_id` |
| POST | `/settings/channels/test` | `{channel, recipient}` — sends immediately; on failure 502 `{code: delivery_failed, reason}` with `reason` one of `not_configured`, `invalid_recipient`, `destination_not_allowed`, `timeout`, `connection_failed`, `tls_failed`, `auth_failed`, `rejected`, `failed` (details only in the server log) |
| GET | `/backups` | last 60 runs `{id, object_key, size_bytes?, status, trigger, error?, requested_by?, created_at, started_at?, finished_at?, keycloak_object_key?, keycloak_size_bytes?}` — the Keycloak database is dumped into `keycloak_object_key` when `BACKUP_KEYCLOAK_DATABASE_URL` is set |
| POST | `/backups` | queue a manual backup (202); the worker runs it within a minute |

Event types: `sos`, `sla_missed`, `absent`, `late_clock_in`, `incident_high`,
`defect_opened`, `monthly_recap` (sent from the 1st at 07:00 site time; if the
worker was down it catches up until the 5th; e-mail gets an HTML table,
WhatsApp a plain-text summary). `absent` covers shifts up to 3 days back
(older flags found after a worker outage are recorded but not notified after
24 h). `sla_missed` is raised for late scans, for unscanned SLA checkpoints of
a session that ended (also early or auto-abandoned), and for overdue
checkpoints of a running session only when the tablet was online across the
due time or a later scan already arrived — sessions recorded offline are
judged when they sync/end, by event time.

**Ops alerts** are install-wide (no site; managed by `platform-admin`, like
the channel settings): `ops_backup_failed` (a backup run failed),
`ops_backup_stale` (no successful backup for 26 h; daily), `ops_sos_undelivered`
(an SOS delivery gave up after ~30 min of retries, or an SOS fired at a site
with no SOS rule), `ops_job_failing` (a scheduled job failed 5 times in a row;
hourly). Retry schedules: SOS 10 s, 20 s, 30 s, then every 60 s for 30 min;
other events 30 s, 2 min, 10 min, 30 min, then `failed`.
