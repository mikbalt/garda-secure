# SecurePatrol API v1

Base path: `/api/v1` (behind nginx: `https://<host>/api/v1`). All endpoints need
`Authorization: Bearer <Keycloak access token>`. JSON in/out; timestamps are
RFC 3339 (UTC in responses). Errors: `{"error": "...", "code": "machine_code"}`.

Roles (Keycloak realm roles): `admin`, `supervisor`, `hr`, `client-readonly`
(web, client `securepatrol-web`) and `device` (tablet, client `securepatrol-tablet`).
Each client company has its own realm; web users only see sites of their realm
(other realms' resources answer 404).

**Tablet ("field") endpoints** additionally need the device to be approved and
the header `X-Guard-ID: <guard uuid>` (guard who unlocked the tablet with their
PIN). Create endpoints accept an optional client `id` (UUID) and event time; a
repeat with the same `id` returns the stored row with **200** instead of 201.
Event times must be within [now − 7 days, now + 2 min].

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
`language` `"id"|"en"`. Responses always contain the full effective config.

## Guards — read `admin, supervisor, hr`; write `admin`

| Method | Path | Body / notes |
|---|---|---|
| GET | `/guards?site_id=` | without `site_id`: all guards of the realm's sites |
| GET | `/guards/:id` | guard has `has_pin` |
| POST | `/guards` | `{name, employee_id?, site_id, role: guard\|supervisor, status: active\|inactive, keycloak_user_id?}` |
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
| GET | `/attendance/daily?site_id=&date=` | web | `{site_id, date, rows:[{shift_id, guard_id, guard_name, employee_id, shift_start, shift_end, attendance_id?, status: present\|late\|absent\|pending, clock_in_at?, clock_out_at?, late_minutes, early_out_minutes, hours_worked, needs_review, override_reason?, selfie_object_key?}]}` |
| GET | `/attendance/review?site_id=` | admin, supervisor | records flagged `needs_review` |
| POST | `/attendance/:id/review` | admin, supervisor | `{note?}` |
| PUT | `/attendance/shifts/:shiftId/override` | admin | `{status: present\|late\|absent, reason (required), clock_in_at?, clock_out_at?}` |
| POST | `/attendance/clock-in` | field | `{id?, clock_in_at?, selfie_object_key?, gps_lat?, gps_lng?}` — 400 `no_shift`, 409 `already_clocked_in` |
| POST | `/attendance/clock-out` | field | `{attendance_id?, clock_out_at?, gps_lat?, gps_lng?}` — 409 `not_clocked_in`, `clock_out_window_closed` |

## Patrol

| Method | Path | Roles | Body / notes |
|---|---|---|---|
| GET | `/patrol/routes?site_id=&active=true` | admin, supervisor, client | |
| GET | `/patrol/routes/:id` | same | |
| POST | `/patrol/routes` | admin | `{site_id, name, active?, checkpoints:[{id?, name, qr_code? (generated), sla_minutes? (null = no SLA), order_index, lat?, lng?}]}` |
| PUT | `/patrol/routes/:id` | admin | `{name, checkpoints, active?}` |
| GET | `/patrol/live?site_id=` | admin, supervisor, client | `[{guard_name, progress}]` for running sessions — poll every 10 s |
| GET | `/patrol/sessions?site_id=&from=&to=` | admin, supervisor, client | session summaries |
| GET | `/patrol/sessions/:id` | web or field | progress: `{session, route_name, checkpoints:[{…checkpoint, scanned_at?, method?, due_at?, status: on_time\|late\|overdue\|missed\|pending}], total, scanned, completion_pct, next_index?}` |
| POST | `/patrol/sessions` | field | `{id?, route_id, started_at?}` |
| POST | `/patrol/sessions/:id/scans` | field | `{id?, checkpoint_id \| qr_code, method: qr\|manual, photo_object_key? (required for manual), scanned_at?, gps_lat?, gps_lng?}` |
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
| POST | `/sos` | field | `{id?, fired_at?, gps_lat?, gps_lng?}` — retry with same `id` until 2xx |
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
`{object_key, content_type, size}`. Keys are deterministic:
`sites/<site_id>/<kind>/<id>.<ext>` — a tablet can reference
`sites/<site>/<kind>/<id>.jpg` in a record **before** uploading (offline) as long
as it later uploads a JPEG with the same `id`. Readers show `uploaded: false` until then.

`GET /media/url?key=` (web roles) → `{key, url?, uploaded}` — pre-signed URL (15 min)
for any photo key of a site the caller can read (selfies, defects, visitors, scans).

List rows of `/incidents`, `/sos`, `/defects`, `/visitors` and attendance records
(`/attendance/review`, clock-in, override) include `guard_name`.

## Reports & dashboard

| Method | Path | Roles |
|---|---|---|
| GET | `/dashboard/summary?site_id=` | web — `{shifts_today, attended_today, absent_today, active_patrols, open_incidents, sos_alerts_unacknowledged, open_defects, attendance_pending_review, weekly_attendance:[{date,present,late,absent}], incidents_by_severity}` |
| GET | `/reports/attendance/daily?site_id=&date=` | web |
| GET | `/reports/attendance/monthly?site_id=&month=` | web — rows `{guard_name, employee_id, scheduled_shifts, present, late, absent, early_out, pending, total_hours, patrol_sessions, patrol_completion_pct}` |
| GET | `/reports/patrol/sessions?site_id=&from=&to=` | admin, supervisor, client |
| GET | `/reports/incidents?…same filters as /incidents` | admin, supervisor, client |
| GET | `/reports/guards/kpi?site_id=&from=&to=` | admin, supervisor, hr — rows `{guard_name, attendance_rate_pct, on_time_rate_pct, patrol_completion_pct, incidents_filed, sla_breaches}` |

## Administration — `admin`

| Method | Path | Body / notes |
|---|---|---|
| GET | `/notifications/event-types` | `{event_types, channels}` |
| GET | `/notifications/rules?site_id=` | |
| POST | `/notifications/rules` | `{site_id, event_type, channel: email\|whatsapp, recipient}` — WhatsApp numbers are stored as digits in international format (`08…` → `628…`) |
| PUT | `/notifications/rules/:id` | `{enabled}` |
| DELETE | `/notifications/rules/:id` | |
| GET | `/notifications/log?site_id=&status=queued\|sent\|failed&limit=` | |
| POST | `/notifications/log/:id/retry` | failed deliveries only |
| GET | `/settings/channels` | `{smtp:{host,port,user,password,from}, whatsapp:{api_url,token,phone_number_id,template_name,template_lang}}` — secrets masked as `********` |
| PUT | `/settings/channels` | same shape; send `********` to keep a stored secret |
| POST | `/settings/channels/test` | `{channel, recipient}` — sends immediately; 502 with error on failure |
| GET | `/backups` | last 60 runs |
| POST | `/backups` | queue a manual backup (202); the worker runs it within a minute |

Event types: `sos`, `sla_missed`, `absent`, `late_clock_in`, `incident_high`,
`defect_opened`, `monthly_recap` (e-mailed on the 1st at 07:00 site time).
