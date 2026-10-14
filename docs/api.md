# GARDA API v1

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
PIN), optionally with `X-Guard-Token` (see *Guard verification*). Create endpoints accept an optional client `id` (UUID) and event time; a
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
`received_at`; patrol sessions, defects and visitor logs carry `flags: []`
too. Values: `late_sync` (received > 10 min after the event time),
`clock_ahead` (event time > 30 s ahead of the server), `backdated_on_time`
(on-time clock-in received > 10 min late while the tablet was online after the
claimed time — also sets `needs_review`), `gps_far` (scan GPS farther from the
checkpoint than the site's `scan_radius_m`), `too_fast` (> 3 m/s from the
previous scanned checkpoint), `guard_unverified` (made without a valid guard
token, see below — on a clock-in it also sets `needs_review`; on a clock-out it
is added to the record), `qr_invalid` (scan of a signed QR payload whose
signature does not match the checkpoint's current code: forged, or printed
before *regenerate-qr*), `qr_unsigned` (QR scan without a signed payload:
legacy plain code, or `checkpoint_id` only). Flags never reject a record.

**Guard verification (D-035).** When online, the tablet verifies the PIN with
the server: `POST /device/guard-sessions {guard_id, pin}` → `{token,
guard_id, expires_at}`. The token is a compact JWS (HS256) with claims `{sub:
guard_id, dev: device_id, site: site_id, iat, exp}`, valid `GUARD_SESSION_TTL`
(default 12 h). The tablet sends it with every request of that guard as
`X-Guard-Token: <token>` (next to `X-Guard-ID`):

| Header | Result |
|---|---|
| none | guard **unverified** — new records get `guard_unverified` |
| signed by the server for this `X-Guard-ID`, this device and its site | **verified** for a record whose event time is ≤ `exp`, or when the token is still valid now. Expired tokens are accepted up to `GUARD_TOKEN_GRACE` (default 7 days = the offline window) so queued records made before the expiry stay verified; records made after the expiry are unverified (not rejected). Beyond the grace: unverified |
| anything else (bad signature, malformed, other guard/device/site) | **403 `guard_token_invalid`** — resend the same item without the token (it is then stored unverified). `POST /sos` never rejects: a bad token just counts as unverified |

The same check applies to every endpoint taking `X-Guard-ID` (GET too).

Dates in query strings are site-local calendar dates `YYYY-MM-DD`; months `YYYY-MM`.
Reports accept `format=csv` (UTF-8 with BOM, opens in Excel) and `format=xlsx`
(sheet *Report* with a bold frozen header, typed numbers and site-local
date/times; sheet *Info* with title, filters and generation time); the incident
log also `format=pdf`. Any other value: 400 `invalid_format`. Exports never
truncate: above 50 000 rows (CSV/XLSX) or 2 000 rows (PDF) they answer 400
`export_too_large` — narrow the range. Text cells starting with `= + - @` are
prefixed with `'` in CSV/XLSX (formula injection).

**Paged lists.** `GET /incidents`, `/reports/incidents` (JSON), `/sos`,
`/defects`, `/visitors` take `limit` and `offset` (defaults/maxima: incidents
1000/1000, SOS 200/500, defects 500/1000, visitors 1000/1000; out of range: 400
`invalid_limit` / `invalid_offset`) and answer the header `X-Total-Count` (rows
matching the filter) plus `X-Truncated: true` when more rows exist beyond this
page. The body stays a JSON array.

**Site scoping (F4, D-035).** A web user may be limited to some sites of the
realm: Keycloak user attribute `sites` → access-token claim `sites` (list of
site UUIDs). Then every site-scoped endpoint answers 404 for other sites, and
realm-wide lists (`/sites`, `/guards` without `site_id`, `/devices`,
`/notifications/log`, `/sos/active`, `/users`) contain only the allowed sites.
No claim (or an empty one) = the whole realm; a claim whose values are all
invalid allows no site (fail closed). `GET /me` shows `sites` and
`sites_restricted`.

## Identity

| Method | Path | Roles | Notes |
|---|---|---|---|
| GET | `/me` | any | `{subject, username, realm, client_id, roles, sites[], sites_restricted, device?}` |

## Tablet bootstrap (role `device`)

| Method | Path | Notes |
|---|---|---|
| POST | `/device/register` | `{name}` → device (`status: pending`) until an admin approves |
| GET | `/device/me` | `{device, site?, config?}` — config = site settings incl. `idle_lock_minutes` |
| GET | `/device/roster` | active device: `{site_id, guards:[{id,name,employee_id,role,pin_hash}], generated_at}` — bcrypt PIN hashes for offline unlock |
| GET | `/device/routes` | active routes of the site (offline cache): `[{id, site_id, name, active, created_at, checkpoints:[{id, name, qr_code, qr_signed, sla_minutes?, order_index, lat?, lng?}]}]` — signed checkpoints have `qr_code: null, qr_signed: true` (the tablet never learns valid payloads; it sends what it scans as `qr_payload`); legacy checkpoints `qr_code: "<text>", qr_signed: false` |
| GET | `/device/shifts` | field: acting guard's shifts −1 day … +7 days |
| GET | `/device/status` | field: `{server_time, config, guard, guard_verified (valid X-Guard-Token now), shift? (running or next), shift_attendance?, open_attendance?, session? (progress of the guard's running patrol; signed checkpoints have qr_code ""), patrols_due}` — call after unlock/restart. `patrols_due`: up to 5 scheduled patrols of the site not started yet, due within the next 24 h or overdue (up to 12 h): `[{route_id, route_name, due_at, grace_until, status: pending\|missed}]`, earliest first |
| GET | `/device/sos` | active device: unacknowledged SOS alerts of the site (any tablet), for supervisors on duty |
| POST | `/device/telemetry` | active device, no guard header: health beacon `{app_version, app_build, platform, os_version, outbox_pending, outbox_failed, outbox_paused?, sos_pending, last_sync?, roster_synced_at?, clock_offset_ms, private_ca, pin_failures?, db_key_reset?, errors[] (≤ 10, each ≤ 500 chars), sent_at?}` → **204**; the latest beacon is stored per device (shown in `GET /devices`). At most 1 per minute per device (429 `rate_limited` + `Retry-After`); 400 `too_many_errors`, `invalid_count`, `field_too_long` |
| POST | `/device/guard-sessions` | active device: `{guard_id, pin}` → 200 `{token, guard_id, expires_at}` (`Cache-Control: no-store`). 401 `{code: invalid_pin, attempts_left}`; 423 `{code: pin_locked, retry_after}` (seconds, also `Retry-After`) — 5 consecutive wrong PINs of a guard (counted on the server across tablets, reset on success or when an admin sets a PIN) lock the check for 15 min; the 5th wrong PIN already answers 423; 403 `guard_not_allowed` (unknown/inactive guard or other site); 409 `pin_not_set`; 400 `pin_required`; 429 `rate_limited` (`RATE_LIMIT_GUARD_SESSIONS_PER_MIN` per tablet, default 20). Every attempt is audited with the guard (`resource_type` `device.guard-sessions`, failures/locks with the lock state as `before`) |

Denials: `403 {code: device_unregistered | device_pending | device_revoked | guard_unknown | guard_not_allowed | guard_token_invalid}`, `400 {code: guard_required}`.

## Sites — `GET` web roles, writes `admin`

| Method | Path | Body |
|---|---|---|
| GET | `/sites` | — |
| GET | `/sites/:id` | — |
| POST | `/sites` | `{name, client_id?, address?, timezone (IANA, default Asia/Jakarta), config_json?}` |
| PUT | `/sites/:id` | same |
| GET | `/sites/:id/floorplans` | web roles: `[{id, site_id, name, object_key, content_type, width, height (px, as displayed), size_bytes, created_by?, created_at, url (pre-signed, 15 min)}]` |
| POST | `/sites/:id/floorplans` | admin, multipart `file` (JPEG/PNG/WebP ≤ 10 MB) + `name` → 201 floor plan; metadata stripped, stored write-once as `sites/<site>/floorplan/<uuid>.<ext>`; ≤ 20 per site (409 `too_many_floorplans`); 400 `name_required`, `file_required`, `file_too_large`, `invalid_file_type`, `invalid_image` |
| DELETE | `/sites/:id/floorplans/:floorplanId` | admin → 204; 409 `floorplan_in_use` while a checkpoint of the site's routes is placed on it |

`config_json` (defaults): `clock_window_minutes` 15, `late_grace_minutes` 5,
`early_out_grace_minutes` 5, `absent_after_minutes` 30, `idle_lock_minutes` 5,
`language` `"id"|"en"`, `scan_radius_m` 150 (0–5000; 0 disables the `gps_far`
check). Responses always contain the full effective config.

## Guards — read `admin, supervisor, hr`; write `admin`

| Method | Path | Body / notes |
|---|---|---|
| GET | `/guards?site_id=` | without `site_id`: all guards of the realm's sites |
| GET | `/guards/:id` | guard has `has_pin`, `pin_locked_until?` (server PIN check locked) |
| POST | `/guards` | `{name, employee_id?, site_id, role: guard\|supervisor, status: active\|inactive, keycloak_user_id?}` — `employee_id` is unique per client realm: 409 `employee_id_taken` |
| PUT | `/guards/:id` | same (full replace) |
| PUT | `/guards/:id/pin` | `{pin}` 4–6 digits; trivial PINs rejected; clears a PIN lock |

## Devices — `admin`

| Method | Path | Body |
|---|---|---|
| GET | `/devices` | devices of the realm (site-scoped admins: their sites + unassigned) incl. `health?` (the latest `/device/telemetry` beacon plus `received_at`) and `health_at?` |
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
| PUT | `/attendance/shifts/:shiftId/override` | admin | `{status: present\|late\|absent, reason (required), clock_in_at?, clock_out_at?}` — every override also writes a history row (F6) |
| GET | `/attendance/:id/history` | admin, supervisor, hr | `{attendance, overrides:[{id, attendance_id, shift_id?, site_id, before_status? (null = no record before), before_clock_in_at?, before_clock_out_at?, before_reason?, after_status, after_clock_in_at?, after_clock_out_at?, reason, actor, created_at}]}` oldest first |
| GET | `/attendance/shifts/:shiftId/history` | admin, supervisor, hr | same, by shift (`{attendance: null, overrides: []}` without a record) |
| POST | `/attendance/clock-in` | field | `{id?, clock_in_at?, selfie_object_key?, gps_lat?, gps_lng?}` → record incl. `received_at`, `flags` — 400 `no_shift`, 409 `already_clocked_in` (another record exists for the shift), `id_conflict`; concurrent retries with the same `id` all get the stored record (one 201, the rest 200) |
| POST | `/attendance/clock-out` | field | `{attendance_id?, clock_out_at?, gps_lat?, gps_lng?}` — 409 `not_clocked_in`, `clock_out_window_closed` |

## Patrol

| Method | Path | Roles | Body / notes |
|---|---|---|---|
| GET | `/patrol/routes?site_id=&active=true` | admin, supervisor, client | checkpoints `{id, name, qr_code, qr_signed, qr_version, sla_minutes?, order_index, lat?, lng?}` — `qr_code` is the text to print (signed: `SP1:<checkpoint id>:<sig>`) |
| GET | `/patrol/routes/:id` | same | |
| POST | `/patrol/routes` | admin | `{site_id, name, active?, checkpoints:[{id?, name, qr_code? , sla_minutes? (null = no SLA), order_index, lat?, lng?}]}` — ≤ 500 checkpoints; `lat/lng` enable the `gps_far`/`too_fast` scan checks. QR: without `qr_code` the server generates a **signed** code (`qr_signed: true`, `qr_version: 1`); a custom `qr_code` is a **legacy** (unsigned) code |
| PUT | `/patrol/routes/:id` | admin | `{name, checkpoints, active?}` — checkpoints may carry a floor-plan position `floorplan_id, plan_x, plan_y` (all three or none; x/y 0..1 from the top-left of the image; the plan must belong to the route's site: 400 `invalid_plan_position`, `unknown_floorplan`; same on POST). Editing a route does not change sessions already started (they keep their snapshot) — a signed checkpoint (same `id`) keeps its code and version whatever `qr_code` is sent; a legacy one keeps the sent code, or becomes signed when `qr_code` is empty. `qr_signed`/`qr_version` in the body are ignored. Deactivating cancels the route's pending scheduled patrols; reactivating plans them again |
| POST | `/patrol/routes/:id/regenerate-qr` | admin | → route. Every checkpoint gets a new signed code (`qr_version` + 1, legacy codes become signed); all previously printed codes of the route then scan as `qr_invalid` — reprint the sheet |
| GET | `/patrol/routes/:id/schedules` | admin, supervisor | `{route_id, schedules:[{id, route_id, weekdays, start_time, end_time?, every_minutes, grace_minutes, active, created_at, updated_at}]}` |
| PUT | `/patrol/routes/:id/schedules` | admin | `{schedules:[{weekdays [1=Mon…7=Sun], start_time "HH:MM", end_time? "HH:MM", every_minutes? (null = one patrol at start_time; 15–720), grace_minutes? (default 15, 0–180, ≤ every_minutes/2), active? (default true)}]}` (≤ 20, replaces the list; `[]` removes the schedule) → `{route_id, schedules, generated_expectations}`. Repeating windows: patrols due at start, start+every, … strictly before end; `end_time` ≤ `start_time` crosses midnight (equal = 24 h); weekdays = the day the window starts; site time zone. Errors 400: `invalid_weekdays`, `invalid_start_time`, `invalid_end_time`, `end_time_required`, `invalid_every`, `invalid_grace`, `grace_too_long`, `too_many_schedules` (with `field`). Future pending patrols are regenerated |
| GET | `/patrol/live?site_id=` | admin, supervisor, client | `[{guard_name, last_scan_at?, progress}]` for running sessions — poll every 10 s. `progress.checkpoints[]` carry `floorplan_id?, plan_x?, plan_y?` and `status` (`on_time\|late\|overdue\|pending`) for drawing them on the site floor plan |
| GET | `/patrol/sessions?site_id=&from=&to=` | admin, supervisor, client | session summaries (same rows as the patrol report) incl. `flagged_scans` (scans with a flag other than `qr_unsigned`), session `flags`, `expectation_id`, `due_at`, `late_start` |
| GET | `/patrol/sessions/:id` | web or field | progress (against the route **as it was when the session started** — F9 snapshot; older sessions use the current route; summaries, reports and SLA likewise): `{session, route_name, checkpoints:[{…checkpoint, scanned_at?, method?, scan_flags?, due_at?, status: on_time\|late\|overdue\|missed\|pending}], total, scanned, completion_pct, next_index?}` — for tablets signed checkpoints have `qr_code: ""` |
| POST | `/patrol/sessions` | field | `{id?, route_id, started_at?}` → session incl. `flags` (`guard_unverified`). Satisfies a scheduled patrol of the route by `started_at` (see *Patrol schedules*) |
| POST | `/patrol/sessions/:id/scans` | field | `{id?, qr_payload? \| qr_code? \| checkpoint_id?, method: qr\|manual, photo_object_key? (required for manual), scanned_at?, gps_lat?, gps_lng?}` → scan incl. `flags`. `qr_payload` = the raw scanned text: a signed `SP1:<id>:<sig>` names the checkpoint (400 `unknown_checkpoint` if not on the session's route) and is verified against its current version — a mismatch is **accepted** but flagged `qr_invalid`; plain text is matched against legacy codes (`qr_unsigned`). `qr_code` (legacy, still accepted; an `SP1:` value is treated as `qr_payload`) → `qr_unsigned`; `checkpoint_id` with `method: qr` → `qr_unsigned`. Manual check-ins get no QR flag. Other flags: `gps_far`, `too_fast`, `late_sync`, `clock_ahead`, `guard_unverified` |
| POST | `/patrol/sessions/:id/end` | field | `{ended_at?, abandoned?}` |

## Incidents, SOS, defects, visitors

| Method | Path | Roles | Body / notes |
|---|---|---|---|
| GET | `/incidents?site_id=&from=&to=&type=&severity=&status=&limit=&offset=` | admin, supervisor, client | paged (see *Paged lists*). `type`/`severity` match the **effective** values |
| GET | `/incidents/:id` | same | `{incident, guard_name, amendments[], photos:[{key, url?, uploaded}]}` |
| GET | `/incidents/:id/pdf` | same | printable PDF: effective values with the filed originals of amended fields, GPS, amendment history, photos (scaled; "not available" when not uploaded) |
| POST | `/incidents` | field | `{id?, type, severity: low\|medium\|high\|critical, description?, location?, photo_object_keys[≤5], occurred_at?, gps_lat?, gps_lng?}` — GPS both or none, lat −90..90, lng −180..180 (400 `invalid_gps`). Stages `incident_filed` (every incident) and, for high/critical, `incident_high` |
| PUT | `/incidents/:id/status` | admin, supervisor | `{status: open\|in_progress\|resolved}` |
| POST | `/incidents/:id/amendments` | admin, supervisor | `{changes:{type?,severity?,description?,location?}, reason}` — original stays immutable |
| GET | `/sos?site_id=&unack_only=true&limit=&offset=` | admin, supervisor | paged |
| GET | `/sos/active` | admin, supervisor | unacknowledged SOS of **every** site the caller may see (realm, narrowed by site scoping), newest first, ≤ 100 rows (`X-Total-Count`, `X-Truncated`): `[{id, site_id, site_name, guard_id, guard_name, fired_at, received_at, gps_lat?, gps_lng?}]` — for the banner on every page; poll it |
| POST | `/sos` | field | `{id?, fired_at?, gps_lat?, gps_lng?}` — retry with same `id` until 2xx; new alerts are rate limited per tablet (429 `rate_limited`, retries of a known `id` never are). The alert and its notification deliveries are stored in one transaction (a 2xx means the notifications are queued); a replay re-creates deliveries that are missing (e.g. a recipient added since), within 24 h |
| POST | `/sos/:id/acknowledge` | admin, supervisor, or field guard with role supervisor | |
| GET | `/defects?site_id=&status=&limit=&offset=` | admin, supervisor | paged |
| POST | `/defects` | field | `{id?, defect_type, location?, description?, photo_object_key?, reported_at?}` |
| PUT | `/defects/:id/status` | admin, supervisor | `{status}` |
| GET | `/visitors?site_id=&from=&to=&limit=&offset=` | admin, supervisor | paged; `id_number` **masked** (`3201********0001`) |
| GET | `/visitors/:id` | admin, supervisor | `{visitor (full id_number), photos:[{key, url?, uploaded}]}` |
| POST | `/visitors` | field | `{id?, visitor_name, id_type?, id_number?, plate?, host_name?, entry_at?, purpose?, photo_object_key?}` — `id_number` is stored encrypted (AES-GCM); responses to tablets show it masked |
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
| `floorplan` | admin, supervisor, hr, client-readonly |

Other roles get 403 `media_forbidden`.

Incident rows (`/incidents`, `/reports/incidents`, detail, dashboard counts)
carry the **effective** values (D-009 amendments: latest amendment per field)
in `type`, `severity`, `description`, `location`, plus `amended` (bool) and,
for fields that differ, `original_type?`, `original_severity?`,
`original_description?`, `original_location?` (the filed values; the stored
original never changes). Also `gps_lat?`, `gps_lng?`.

List rows of `/incidents`, `/sos`, `/defects`, `/visitors` and attendance records
(`/attendance/review`, clock-in, override) include `guard_name`. Incidents,
SOS alerts, scans and attendance records include `flags` (and `received_at`).

## Reports & dashboard

| Method | Path | Roles |
|---|---|---|
| GET | `/dashboard/summary?site_id=` | web — `{shifts_today, attended_today, absent_today, active_patrols, open_incidents, sos_alerts_unacknowledged, open_defects, attendance_pending_review, weekly_attendance:[{date,present,late,absent}], incidents_by_severity}` |
| GET | `/reports/attendance/daily?site_id=&date=` | web |
| GET | `/reports/attendance/monthly?site_id=&month=` | web — `{site_id, month, rows, totals, patrol_compliance}`; rows `{guard_id, guard_name, employee_id, scheduled_days, days_present, days_late, days_absent, days_upcoming, early_out_days, scheduled_shifts, present, late, absent, early_out, pending, total_hours, patrol_sessions, patrol_completion_pct}`; `totals` = the same counts summed over the site plus `guards` (patrol completion = session-weighted average); `patrol_compliance` = the site's schedule compliance totals of the month (shape of `/reports/patrol-compliance` `totals`). **Attendance is counted per guard-day** (FR-REP-05): a calendar day in the site's time zone, keyed by the day the shift *starts* (a night shift crossing midnight or the month end counts on its start day, in the month it starts); several shifts of one guard on one day are one day. Day status after overrides: `absent` if any shift is absent, else `late` if any is late, else `present` if a shift is present; a day whose shifts are all still pending (future, ongoing or not yet flagged) is `upcoming`. `early_out_days` = days with at least one early departure (independent of the status). A guard who worked at several sites is counted at each site for the shifts scheduled there. `scheduled_shifts, present, late, absent, early_out, pending` stay per shift (transparency; basis of the guard KPIs). CSV/XLSX columns: *Guard, Employee ID, Scheduled days, Days present, Days late, Days absent, Days upcoming, Early-out days, Hours, Scheduled shifts, Patrols, Patrol completion %*, last row *Total (N guards)* |
| GET | `/reports/patrol/sessions?site_id=&from=&to=` | admin, supervisor, client — `{site_id, rows:[{session_id, route_id, route_name, guard_id, guard_name, started_at, ended_at?, status, total_checkpoints, scanned_count, sla_breaches, flagged_scans, completion_pct, flags, expectation_id (null = not a scheduled patrol), due_at, late_start}]}`; CSV adds *Scheduled for* and *Late start* |
| GET | `/reports/incidents?…same filters as /incidents` | admin, supervisor, client — JSON paged like `/incidents`; CSV/XLSX/PDF (all rows, effective values, columns *Amended*, *Original values*, *GPS*; PDF without the ID column) |
| GET | `/reports/guards/kpi?site_id=&from=&to=` | admin, supervisor, hr — rows `{guard_name, attendance_rate_pct, on_time_rate_pct, patrol_completion_pct, incidents_filed, sla_breaches, scheduled_patrols, schedule_compliance_pct}` — `scheduled_patrols` = scheduled patrols due in the range that the guard's sessions satisfied, `schedule_compliance_pct` = share started within the grace period (null when 0; missed patrols are not attributable to a guard) |
| GET | `/reports/patrol-compliance?site_id=&from=&to=` | admin, supervisor, client — `{site_id, from, to, rows:[{date (site-local, by due time), route_id, route_name, expected, started_on_time, late, missed, pending, compliance_pct}], totals:{expected, started_on_time, late, missed, pending, compliance_pct}}`; `compliance_pct` = on time / (on time + late + missed), null when nothing is decided yet; `expected` excludes cancelled patrols. ≤ 92 days, `format=csv` |

## Administration

Per-realm administration — `admin`:

| Method | Path | Body / notes |
|---|---|---|
| GET | `/notifications/event-types` | `{event_types, channels}` |
| GET | `/notifications/channels` | `[{name, configured}]` — registered delivery channels (`email`, `whatsapp`; validated in code, F10) and whether the install-wide settings configure them |
| GET | `/notifications/rules?site_id=` | |
| POST | `/notifications/rules` | `{site_id, event_type, channel (a registered channel, else 400 invalid_channel), recipient}` — WhatsApp numbers are stored as digits in international format (`08…` → `628…`) |
| PUT | `/notifications/rules/:id` | `{enabled}` |
| DELETE | `/notifications/rules/:id` | |
| GET | `/notifications/log?site_id=&status=queued\|sent\|failed&limit=` | |
| POST | `/notifications/log/:id/retry` | failed deliveries only; queues **one** more attempt |
| GET | `/audit?from=&to=&actor=&resource_type=&resource_id=&limit=&offset=` | audit log of the caller's realm, newest first → `{rows, limit, offset, has_more}`. `from`/`to`: RFC 3339 or `YYYY-MM-DD` (UTC, `to` inclusive); `actor`: username or guard ID; `resource_type`: e.g. `guards`, `patrol` (matches `patrol.routes`, `patrol.sessions`), `attendance.shifts`; `limit` ≤ 500 (default 100) |

Audit rows: `{id, user_id, realm, actor, guard_id?, roles[], action ("PUT /api/v1/guards/:id"), method, route, resource_type, resource_id?, params? ({"shiftId": …}), request_id?, summary? (redacted JSON body of staff writes — passwords, tokens, PINs, ID numbers removed), before?, after? (attendance override; guard, site, route, device, shift and template changes), ip, user_agent, status_code, created_at}`. Every POST/PUT/PATCH/DELETE is recorded; the table is append-only.

### Users and tablet accounts — `admin` (F1)

Managed in the caller's realm through the Keycloak Admin REST API (confidential
client `securepatrol-admin-api` per realm). Keycloak unreachable, or the realm
has no admin-client secret: **503 `kc_unavailable`**. Every change is audited
with before/after state (never passwords).

| Method | Path | Body / notes |
|---|---|---|
| GET | `/users?search=&role=&enabled=&first=&max=` | people of the realm (tablet accounts excluded), by username; `max` ≤ 500 (default 100), `X-Total-Count` → `[{id, username, email, first_name, last_name, enabled, roles[], sites[], otp_configured, required_actions[], created_at}]`. Site-scoped admins only see users limited to (a subset of) their sites |
| GET | `/users/:id` | one user (404 `user_not_found` also for tablet accounts) |
| POST | `/users` | `{username (3–64, a-z 0-9 . _ @ -, not device-/service-account-), email?, first_name?, last_name?, roles[] (≥ 1 of admin, supervisor, hr, client-readonly; platform-admin only by a platform admin), sites[] (site UUIDs of the realm; empty = whole realm; required for site-scoped admins), send_invite (bool), temporary_password?}` → 201 `{user, invite_sent, invite_error? ("invite_failed": e-mail could not be sent; set a password instead), temporary_password? (only when not invited: the given one or a generated one, shown once)}`. Required actions: `UPDATE_PASSWORD`, plus `CONFIGURE_TOTP` for admin/supervisor/hr/platform-admin. Errors: 400 `invalid_username`, `invalid_email`, `email_required` (invite without e-mail), `roles_required`, `invalid_site`, `password_policy`; 403 `role_not_allowed`, `sites_required`; 409 `username_taken`, `email_taken` |
| PUT | `/users/:id` | `{first_name?, last_name?, email?, enabled?, roles?, sites?}` (omitted = unchanged) → user. Disabling ends the sessions. 403 `cannot_modify_self` (removing your own admin/platform-admin role, disabling or re-scoping yourself), `role_not_allowed` (platform-admin users can only be changed by a platform admin) |
| POST | `/users/:id/reset-password` | `{send_email?: bool, temporary_password?}` → `{temporary_password}` (shown once; must be changed at next login) or `{email_sent: true}`; 403 `cannot_modify_self`; 400 `email_required` |
| POST | `/users/:id/reset-otp` | removes the OTP credentials, adds `CONFIGURE_TOTP`, ends the sessions → `{user_id, otp_removed, otp_configured: false}`; 403 `cannot_modify_self` |
| POST | `/users/:id/logout` | ends every session → `{logged_out: true}` |
| GET | `/device-accounts` | tablet accounts (Keycloak users with only role `device`) → `[{id, username, name, enabled, created_at, device? (the registered tablet using this account, see /devices)}]` |
| POST | `/device-accounts` | `{name}` → 201 `{account, password}` — username `device-<slug of name>`, random 24-character password shown **once** (enter it on the tablet); 409 `username_taken`, 400 `invalid_name` |
| POST | `/device-accounts/:id/reset-password` | → `{account, password}` (shown once); the tablet's sessions end |
| PUT | `/device-accounts/:id` | `{enabled}` → account; disabling ends its sessions (a tablet keeps working offline until it next talks to the server; revoke the device in `/devices` too) |

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

Event types: `sos`, `sla_missed`, `absent`, `late_clock_in`, `incident_high`
(high/critical incidents), `incident_filed` (every new incident, any severity —
both fire for a high incident when both have rules), `defect_opened`, `monthly_recap` (sent from the 1st at 07:00 site time; if the
worker was down it catches up until the 5th; e-mail gets an HTML table,
WhatsApp a plain-text summary, both per guard-day as in `/reports/attendance/monthly`
with a site total, in the site's language (`id`/`en`); both include the site's patrol schedule
compliance when it has scheduled patrols), `patrol_not_started` (a scheduled
patrol was not started within its grace period — see *Patrol schedules*). `absent` covers shifts up to 3 days back
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

## Patrol schedules (D-035)

Routes get schedule windows (`PUT /patrol/routes/:id/schedules`). The worker
materialises the scheduled patrols ("expectations") 48 h ahead (hourly,
idempotent) in the site's time zone; changing the schedule replaces the
pending future ones. Each patrol has `due_at` and `grace_until = due_at +
grace`; status `pending | started | missed | cancelled`.

* **Matching** (by the session's event time `started_at`, also for sessions
  synced later from an offline tablet): a session of the route started in
  `[due_at − grace, grace_until]` satisfies the nearest open patrol → `started`.
  A session started after `grace_until` (and before the next patrol's window
  opens, at most 12 h after `due_at`) satisfies the most recent missed/overdue
  patrol → `started` with `late_start: true`. One session satisfies at most one
  patrol.
* **Not started:** every minute, patrols past `grace_until` without a session
  become `missed` and `patrol_not_started` is staged (transactional outbox,
  site rules; params `route`, `due`, `date`, `grace_until`, `grace`). An offline
  tablet does not suppress it — a session synced later still flips the patrol
  to `started`. Patrols found missed more than 24 h after their grace period
  (worker outage) are recorded without a notification.
* Deactivating a route cancels its pending patrols.
