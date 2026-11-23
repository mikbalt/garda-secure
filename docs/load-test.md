# Load test: 2 sites, 10 guards (TAQ-13)

Run 2026-11-23 on a fresh local stack (`make up`, `SERVER_HOST=localhost`, macOS Docker
Desktop, 7.6 GB VM; see runbook "full stack on one machine"). Harness:
[infra/loadtest/loadtest.py](../infra/loadtest/loadtest.py) (Python 3 standard library only).

## What was simulated

* 2 clients (realms), 1 site each, **5 guards per site = 10 guards**, 1 shared tablet per site,
  1 route of 10 signed checkpoints per site, one-off shifts covering the run.
* Per guard, in a loop for 60 minutes: PIN check, clock-in, patrol (start, a QR scan every ~20 s,
  end), every second patrol an incident with photo (EXIF-bearing JPEG through the media
  pipeline), a visitor with exit and a defect. Real patrols are slower; the pace here is about
  3 to 6 times a real shift.
* Tablets: health beacon every 61 s, roster/routes refresh every 5 min.
* Web: per realm admin, supervisor, HR and client-readonly (8 users) reload the page-load request
  set of their dashboard in parallel every 10 s (sites, summary, SOS banner, live patrol,
  daily attendance, incidents) and open reports every minute (monthly, patrol sessions,
  compliance, KPI, XLSX export).
* **Sync:** every 10 minutes all 10 guards replay an offline queue at the same moment: patrol
  session + 10 scans + end + incident with photo + visitor + exit (16 requests, event times up
  to 25 minutes old) followed by an idempotent replay of the session (must answer 200).
* All traffic goes through nginx over TLS (private CA) like production.

Run it: `python3 infra/loadtest/loadtest.py --creds creds.json --state state.json setup`, then
`... run --minutes 60 --report out/`. `creds.json` (passwords, TOTP secrets) stays outside the
repo; its format is in the script header. Staff accounts need TOTP enrolled first.

## Result (60 minutes, 20 140 requests, about 5 per second)

| Criterion | Target | Measured | |
|---|---|---|---|
| Dashboard load (page request set, parallel, 2 868 loads) | < 2 s | p50 13 ms, p95 21 ms, max 45 ms | pass |
| Real browser, cold navigation to a rendered page incl. network idle (Chrome, 5 pages) | < 2 s | 0.60 to 0.64 s | pass |
| Sync of a 16-request offline queue, 10 guards at once (60 queues) | < 10 s | p50 0.11 s, max 0.12 s, none failed | pass |
| Errors over one hour | none | 0 HTTP 5xx, 0 429, 0 timeouts, 0 unexpected statuses | pass |
| API/worker log errors | none | 1: the SOS I fired by hand ("SOS has no recipient rule", expected: no rules configured) | pass |

Data written: about 4 300 scans, 480 patrol sessions, 240 incidents. Container peaks: api 89 %
of one core (one start-up spike) but 2.6 % on average and 41 MB; Postgres 7 % / 150 MB;
Keycloak 15 % / 745 MB; nginx 2 %; Redis 2 %.

| Endpoint | n | p50 ms | p95 ms | max ms |
|---|---|---|---|---|
| GET /device/me | 24 | 2 | 3 | 3 |
| GET /device/roster | 24 | 2 | 3 | 3 |
| GET /device/routes | 26 | 2 | 3 | 4 |
| GET /device/status | 172 | 12 | 17 | 31 |
| GET /patrol/routes | 2 | 3 | 3 | 3 |
| GET /patrol/sessions/:id | 671 | 3 | 5 | 7 |
| POST /attendance/clock-in | 10 | 3 | 7 | 7 |
| POST /defects | 82 | 3 | 4 | 4 |
| POST /device/guard-sessions | 10 | 235 | 265 | 265 |
| POST /device/telemetry | 118 | 11 | 17 | 26 |
| POST /incidents | 82 | 4 | 5 | 7 |
| POST /media | 142 | 7 | 12 | 19 |
| POST /patrol/sessions | 172 | 5 | 8 | 9 |
| POST /patrol/sessions/:id/end | 163 | 4 | 5 | 9 |
| POST /patrol/sessions/:id/scans | 1678 | 12 | 16 | 33 |
| POST /visitors | 82 | 3 | 4 | 8 |
| POST /visitors/:id/exit | 82 | 3 | 4 | 5 |
| page /attendance/daily | 2152 | 7 | 15 | 41 |
| page /dashboard/summary | 2868 | 10 | 17 | 42 |
| page /guards | 717 | 6 | 15 | 27 |
| page /incidents | 2151 | 11 | 17 | 42 |
| page /patrol/live | 2151 | 13 | 19 | 43 |
| page /sites | 2868 | 7 | 15 | 28 |
| page /sos/active | 1435 | 9 | 15 | 41 |
| report attendance/monthly | 354 | 6 | 10 | 15 |
| report defects | 118 | 2 | 3 | 6 |
| report guards/kpi | 236 | 4 | 6 | 12 |
| report patrol-compliance | 118 | 2 | 3 | 4 |
| report patrol/sessions | 354 | 7 | 13 | 17 |
| report sos | 118 | 2 | 3 | 4 |
| sync POST end | 60 | 4 | 8 | 14 |
| sync POST incidents | 60 | 4 | 7 | 7 |
| sync POST scans | 600 | 5 | 8 | 15 |
| sync POST sessions | 60 | 17 | 22 | 23 |
| sync POST visitor exit | 60 | 4 | 5 | 6 |
| sync POST visitors | 60 | 3 | 5 | 5 |
| sync replay sessions | 60 | 3 | 4 | 5 |

`POST /device/guard-sessions` (235 ms) is the bcrypt PIN check, by design.

## Headroom and limits

A second, 8-minute run at roughly 10 times that pace (scan every 1.5 s, dashboards every 1.5 s,
replay every 2 min) was refused by **nginx** with 429 (`sp_api`: 30 requests/s per client IP,
burst 200) at about 37 requests/s from one IP, while the API used 7 % CPU. So the ceiling is
the per-IP limit, not the server. One site behind one NAT address shares that budget: 10 guards
plus several dashboards are about 5 requests/s at the pace above, so there is a factor of about
6 left, but a larger office with many dashboards open on one LAN address would hit it first.

## Not covered

* Loopback only: no LAN round trip, Wi-Fi loss or real tablet clock/battery behaviour (add the
  network RTT to the figures; 2 s leaves a lot of room).
* One tablet per site, shared by the 5 guards (the server does not care, the per-tablet limit of
  600 writes/min was never reached).
* No real SMTP/WhatsApp/FCM: notification delivery was not loaded (no rules configured).
* WebSocket live patrol (single instance) and long-running browser tabs were not loaded.

## Defects found while setting this up (fixed separately)

* `GET /metrics` on the API answered 500 once records had been posted (method label aliased
  the request buffer): PR #58.
* `make up` on a fresh install left `/users` and `/device-accounts` at 503 `kc_unavailable`
  (secrets file replaced by `mv`, stale single-file bind mount): PR #59.
