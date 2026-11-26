# Progress

Live status board for all agents. **Claim a task before working on it** (see
CLAUDE.md → Coordination protocol). Keep entries short; link commits/PRs.

Agent tag format: `<pc-or-user>/<short-name>` — e.g. `muhamhad-pc1/claude`.

## In progress

| Task | Agent | Since | Notes |
|---|---|---|---|

## Migration numbers

| # | File | Owner | Status |
|---|---|---|---|
| 001 | init_schema | — | pushed |
| 002 | audit_log | — | pushed |
| 003 | v1_core (devices, shift templates, patrol sessions, amendments, notifications, settings, backups) | `muhamhad-pc1/claude` | pushed |
| 004 | be_reliability (outbox, ops alerts, SLA, backups) | `muhamhad-pc1/claude-be-rel` | pushed |
| 005 | be_security (audit append-only, received_at/flags, per-site employee_id) | `muhamhad-pc1/claude-be-sec` | pushed |
| 006 | be_field (guard sessions, signed QR, patrol schedules) | `muhamhad-pc1/claude-be-field` | pushed |
| 007 | be_features (overrides history, route snapshots, floor plans, channels, retention, telemetry, incident GPS) | `muhamhad-pc1/claude-be-features` | pushed |
| 008 | push (subscriptions, preferences, device push tokens, SOS escalations) | `muhamhad-pc1/claude-push` | pushed |
| 009 | site_active (`sites.active`) | `mokhataq/site-deactivation` | pushed |
| 010 | sos_unverified_ack (`sos_alerts.unverified_ack_by/at`) | `muhamhad-pc1/claude-verify` | pushed |
| 011 | record_chain (per-site hash chain, TAQ-17) | `mikbalt/taq-17-chain` | pushed |
| 012 | *(next free)* | | |

## Milestones (PRD §11)

| Milestone | Status |
|---|---|
| M1 Infra & Auth | 🟢 backend + config done; full Docker stack smoke-tested 2026-11-23 (TAQ-6, fresh `make env/certs/up` on macOS Docker Desktop, `SERVER_HOST=localhost`) |
| M2 Core Attendance | 🟢 backend + web done |
| M3 Patrol | 🟢 backend + web + mobile: schedules, signed QR, live view on site floor plans (polling, D-014) |
| M4 Incident & Field Reports | 🟢 backend + web + mobile (D-024) |
| M5 Web Dashboard & Reports | 🟢 redesigned (D-022): setup guide, Today dashboard, all pages; verified in demo mode (all roles, desktop + 390 px); ⚠️ not yet exercised against a running stack |
| M6 Notifications & Email | 🟢 backend done (rules, log, retry, SMTP/WhatsApp, monthly recap); needs real SMTP/WA test |
| M7 Hardening & Go-Live | 🟡 audit 2026-10 (docs/audit-2026-10.md): wave 1 merged (backend reliability + security, infra/ops/CI); wave 2 in progress; ⚠️ full stack still never booted — ci-e2e-stack.yml does it on GitHub |

## Backlog (unclaimed)

Planned major upgrades (Node 24, React 19, Vite 8, Zod 4, Tailwind 4, Flutter, Redis 8, Grafana 13): see [docs/maintenance.md](docs/maintenance.md).

Audit 2026-11-21 (`muhamhad-pc1/claude-verify`; read-only audit by three agents, fixed items are in Done) - open findings, most severe first
- [ ] **infra, high** `infra/backup-offsite/offsite.sh:62-70`: `ensure_repo` runs `restic init` whenever the repo is unreadable. With the off-site disk not mounted it silently creates a new repo on the system disk and reports success. Fix: never init during `backup`; fail when `/repo` is not a mountpoint or has no marker. Needs Docker + restic to test.
- [ ] **mobile, low** follow-up of D-065: the tablet says "SOS acknowledged" for an unverified response too, and sends no event time with the acknowledgement. Show "noted, confirm on the web" when the reply has no `acknowledged_at`.
- [ ] **backend, medium** `handler/admin.go:322-344`, `repository/attendance.go:220`, `service/shifts.go:148-178`: moving a guard to another site (or deactivating one) leaves the old site's templates and future shifts; they are flagged absent and alert every day, and count as absent in the monthly recap. Fix in the `updateGuard` transaction: deactivate the guard's templates at the old site, delete future shifts without attendance.
- [ ] **infra, medium** `infra/docker-compose.app.yml:236`, `.env.example:129`: default `ADMIN_ALLOW_CIDRS` contains `172.16.0.0/12`, which includes the stack's own bridge gateway; traffic relayed by docker-proxy (IPv6 on an IPv4-only bridge) passes the Keycloak admin allow-list. Fix: `deny <SP_DOCKER_SUBNET>` before the allows, or publish on `0.0.0.0:443` only.
- [ ] **backend, low-medium** `handler/audit.go:17-21`: `GET /audit` is filtered by realm only; a site-limited admin reads other sites' audit rows. Needs a `site_id` on `audit_log` (migration) or refusing `/audit` to site-limited principals.
- [ ] **backend, low** `handler/incidents.go:157`: incident `photo_object_keys` accept keys of any kind (selfie, visitor ID photo), which `getIncident` then presigns for client-readonly. Fix: require the matching kind segment per field.
- [ ] **backend, low** `handler/shifts.go:255-258`: `updateShift` checks for attendance outside the transaction and treats a DB error as "none". Fix: `AND NOT EXISTS (...)` in the `UPDATE`, 0 rows -> 409.
- [ ] **backend, low** `handler/app.go:96,100`: `POST /sos` passes the per-IP auth-failure limiter and the write limiter before its own exemption; a site behind one NAT IP can get SOS delayed by 429.
- [ ] **backend + web, low** `handler/handler.go:107-110`: the `/devices` group requires `admin`, so the `platform-admin` grant on `POST /devices/:id/reset` (docs/api.md, `web/src/lib/roles.ts`) can never apply. Decide which side is right.
- [ ] **infra, low** `infra/docker-compose.ops.yml:222`: `.env` is bind-mounted as a single file into `backup-offsite`; `sed -i` replaces the inode, so off-site config bundles carry a stale `.env`. Mount the directory.
- [ ] **infra, low** `infra/prometheus/alerts.yml:69-84`: no `absent()` rule for the `sp_offsite_*` series; an off-site backup that never ran never alerts.
- [ ] **infra, low** `.github/workflows/ci-e2e-stack.yml:178-196`: CI never restores an app DB dump or runs `restic restore`.
- [ ] **web, low** `pages/Patrol.tsx:30-35`: a 10 s `refetchInterval` on `['patrol-live']` defeats the 60 s slow poll while the WebSocket is up (D-062). Server load only.
- [ ] **web, low** `pages/settings/LogTab.tsx`, `pages/Audit.tsx`: realm-wide lists format every row in the selected site's time zone.
- [ ] **web, low** `pages/settings/SiteModal.tsx`: no form field for `scan_radius_m` (the value is now kept on save, but can only be set through the API).
Verification
- [x] Smoke-test the full Docker stack (`make up`): done 2026-11-23 (TAQ-6) - logins for every role incl. TOTP enrolment, MinIO via :9443 (pre-signed GET 200), manual + scheduled backup, `restore-db.sh` of both dumps. Not covered: LAN/DNS `SERVER_HOST`, real SMTP/WhatsApp, FCM, off-site restic run
- [ ] Run the `verify` skill tiers 2 and 3 (Go integration tests, Docker stack) on a PC that has Go and Docker; they were not proven where the skill was written
- [ ] Real SMTP + WhatsApp template delivery test — dry run done (TAQ-7, `dryrun_test.go`); the live run needs owner credentials, checklist in `docs/runbook.md` §4

Web
- [ ] HR receives `attendance_review` push but `/attendance?tab=review` is admin/supervisor only — drop HR from the event or give HR a read-only review view
- [ ] Manual UI pass against a running stack (all roles, both languages)
- [x] Promote page-local kits to `components/ui`: Avatar/PersonCell, FilterChips/ChipGroup, Segmented, PeriodStepper, ConfirmDialog, Switch, Stat/StatRow, LiveIndicator, IconTile — done in #41 (DateRangeFilter kept in patrol/kit: coupled to patrol preset logic)
- [ ] `Table`: optional row-click prop (Patrol history uses its own table markup)
- [ ] Attendance "Perlu ditinjau" chip counts only the selected day while the tab counts all days — align or label
- [ ] Device dialog: "set back to pending" action

Infra / Ops
- [ ] Create the Firebase project (Android app `id.co.garda.tablet`), install the service-account file on the server and the 4 `SP_FCM_*` values as GitHub secrets (runbook: Push notifications)
- [x] Rehearse the Keycloak 24→26 DB migration (runbook §2 "Rehearsal on a copy"; run `keycloak-smoke.sh` and a real-authenticator OTP login on production after the real upgrade)
- [ ] Configure the `production` environment secrets and do a first tagged deploy (runbook §7)
- [ ] Let's Encrypt on the real hostname: first `scripts/acme.sh issue` against staging, then production (runbook §6; tooling + renewal tested on Pebble, D-067)
- [x] Load test 2 sites / 10 guards (M7): done 2026-11-23 (TAQ-13), see docs/load-test.md

Mobile (D-024) — open audit items, see docs/audit-2026-10.md
- [ ] First real APK on a tablet: private-CA TLS, camera/QR, kiosk mode (runbook §3)

## Done

| Task | Agent | Date | Ref |
|---|---|---|---|
| Backend: per-site hash chain over attendance, scan and incident records, migration 011, `FirstChainBreak` (D-069, TAQ-17; verification endpoint is TAQ-18) | `mikbalt/taq-17-chain` | 2026-11-26 | this PR |

| Time-bound checkpoint QR: `SP2:` payload with a signed window (`QR_WINDOW`, default 30 d), scans judged by event time with 10 min skew, stale scans accepted but flagged `qr_stale`; mobile parses SP2; no migration (D-070, TAQ-19) | `mokhataq/taq-19` | 2026-11-26 | this PR |
| Web: platform-admin-only user no longer calls `GET /sites` (403 on every page, header 'No sites yet'): `viewSites` capability gates the site list, header says 'Install-wide (no sites)'; API unchanged (D-068, TAQ-41) | `mokhataq/taq-41` | 2026-11-23 | this PR |
| Load test 2 sites / 10 guards (TAQ-13, docs/load-test.md, harness infra/loadtest) and manual UI pass, all roles, EN + ID (TAQ-14, docs/ui-pass-2026-11.md); fixed on the way: API /metrics 500 (#58), fresh-install 503 kc_unavailable (#59); open: TAQ-41 | `mikbalt/agent-taq-13-load` | 2026-11-23 | this PR |
| Audit 2026-11-21 fixes: failure counters at 0 for alerts; rollback with RESTORE_FRESH=1 + VERSION pinned early; offline clock-out with unknown attendance_id; site check on device reset; web: site list retry, SOS polling ungated, site edit keeps scan_radius_m, on-duty night shifts, SOS badge polling, push test language | `muhamhad-pc1/claude-verify` | 2026-11-21 | no migration |
| Mobile outbox: priority-0 items (SOS, SOS ack) are independent, so a rejected or waiting SOS no longer blocks other items or a second SOS (the rest stays strict FIFO); a wait beyond 30 min (device clock set back) counts as due; rejected-SOS state is per item; 3 tests | `muhamhad-pc1/claude-verify` | 2026-11-22 | no migration |
| Mobile: an SOS the tablet could not store (outbox insert fails) shows a red "not saved, press again and call" banner instead of "SOS delivered"; test added. Flutter now on this PC (analyze + 90 tests) | `muhamhad-pc1/claude-verify` | 2026-11-22 | no migration |
| Tablet SOS acknowledgement without a verified PIN is only noted (`unverified_ack_by/at`): tablets go quiet, the web alert stays open and keeps escalating (D-065); web SOS list shows who responded; verify journey `sos-unverified-ack`; client-readonly keeps signed QR codes (D-066) | `muhamhad-pc1/claude-verify` | 2026-11-22 | migration 010 |
| SOS delivery gives up atomically with its ops alert (D-064, finding F1); integration test + model updated | `muhamhad-pc1/claude-verify` | 2026-11-21 | no migration |
| Project skills for agents (D-063): `verify` (web demo driver, 5 proven journeys, feature map) and `sos-delivery` (model check of the SOS outbox) | `muhamhad-pc1/claude-verify` | 2026-11-21 | no migration |
| Promote page-local UI kits to components/ui (issue #41): Avatar/PersonCell, FilterChips/ChipGroup, Segmented, PeriodStepper, ConfirmDialog, Switch, Stat/StatRow, LiveIndicator, IconTile; lib/avatar.ts; kit files re-export for compat | `mokhataq/ui-kits` | 2026-11-18 | no migration |
| Device dialog reset to pending (issue #42): `POST /devices/:id/reset`, `resetDevice` capability, confirm dialog, demo mock | `mokhataq/device-reset` | 2026-11-18 | no migration |
| Attendance chip count fix (issue #44): chip now shows total review count (all days) + Table `onRowClick` prop (issue #43) | `mokhataq/attendance-chip-fix` | 2026-11-18 | no migration |
| WebSocket live patrol updates (issue #38): in-process hub, ticket auth, `usePatrolLive`, nginx upgrade location; D-062. Follow-ups: Redis pub/sub for multi-instance; worker-side session closing does not nudge (60 s poll covers it) | `mokhataq/ws-live-patrol` | 2026-11-18 | no migration |
| Site deactivation / soft delete (issue #34): migration 009, activate/deactivate API, SitesTab UI, demo mock; D-060 | `mokhataq/site-deactivation` | 2026-11-17 | migration 009 |
| Checkpoint GPS learned from scans (worker job `checkpoint_positions`), manual lat/lng removed from the route editor; D-059 | `muhamhad-pc1/claude` | 2026-10-20 | no migration |
| Initial scaffold, infra compose split, backend handlers (first pass) | — | 2026-10-07 | 60b3c05…d189988 |
| PRD v3 decisions OQ-7/8/9, CLAUDE.md / PROGRESS.md / DECISIONS.md | `muhamhad-pc1/claude` | 2026-10-07 | 928fb4a |
| Backend v1 core: multi-realm JWT + RBAC, device/guard auth, migration 003, shift templates + generator, attendance rules, patrol sessions + SLA, incidents/amendments, SOS, defects, visitors, media upload, reports + CSV, dashboard, notifications (rules/log/retry, SMTP/WhatsApp), encrypted channel settings, backups, scheduler; unit + integration tests (embedded Postgres) | `muhamhad-pc1/claude` | 2026-10-07 | ace9df5 |
| Infra: Alpine API image with pg_dump, compose fixes (worker command, Keycloak DB/path/proxy, healthchecks), nginx fixes (/api prefix, web:80, /auth, MinIO :9443, 12 MB uploads), two realm files + generator, env examples, Makefile, CI (integration tests, nginx/realm validation) | `muhamhad-pc1/claude` | 2026-10-07 | ace9df5 |
| Web v1: runtime `/config.js`, multi-realm org picker, role-aware nav, site selector, EN/ID i18n, Dashboard + SOS banner, Attendance (+override, review queue), Schedules, Guards (+PIN), Patrol (route editor + QR print, live, sessions), Incidents/SOS/Defects/Visitors, 5 reports + CSV + print, Settings (sites, devices, rules, channels, log, backups) | `muhamhad-pc1/claude` | 2026-10-07 | 7d851f8 |
| Backend: `guard_name` on list rows, `GET /media/url` | `muhamhad-pc1/claude` | 2026-10-07 | 8cfc11a |
| CI/CD fixes: gosec pinned, web Docker image job, mobile CI manual-only, tag/manual deploy from `infra/` with health wait; actionlint clean | `muhamhad-pc1/claude` | 2026-10-08 | c3ffc59 |
| Web UX overhaul (D-022): design system + `web/DESIGN.md`, new shell & navigation, setup guide + welcome flow, Today dashboard, redesigned attendance/roster/guards, patrol/incidents, reports/settings, organisation picker, demo mode, code-split pages; WhatsApp `08…` normalisation (D-023) | `muhamhad-pc1/claude` | 2026-10-09 | this push |
| Docs: `docs/api.md` (API contract), `docs/runbook.md` (install, users, tablets, backups/restore) | `muhamhad-pc1/claude` | 2026-10-07 | ace9df5 |
| Mobile tablet app v1 (D-024): setup + device sign-in/approval, offline PIN lock screen, home, clock in/out (selfie+GPS), QR + manual patrol with live SLA, incidents/defects/visitors, SOS (hold 2 s, priority outbox, never parked), offline outbox with retry/backoff + outbox screen, EN/ID; 22 tests + golden screenshots; backend `GET /device/status`, `GET /device/sos` | `muhamhad-pc1/claude` | 2026-10-12 | this commit; MOB-1, MOB-10 fixed |
| Hardening wave 1 — backend reliability: transactional notification outbox, SOS lane, Postgres claim, SMTP timeouts, job isolation, SLA coverage, backups (Keycloak DB, TZ, timeout, alerts), ops alerts, metrics, /ready, worker health (D-026..D-030) | `muhamhad-pc1/claude-be-rel` | 2026-10-12 | merge 5c8e2f5 |
| Hardening wave 1 — backend security: Go 1.26 + deps + govulncheck, replay ownership, settings exfil guard, platform-admin, token/azp binding, limits + rate limits, append-only audit + GET /audit, write-once photos + EXIF strip, per-kind photo access, plausibility flags (D-031..D-033) | `muhamhad-pc1/claude-be-sec` | 2026-10-12 | merge e2314d6 |
| Hardening wave 1 — infra/ops/CI: SERVER_HOST + private CA + preflight, healthchecks, nginx (DNS, headers, CSP, rate limits, admin allow-list, :9443), Silo, enforced staff OTP, least privilege, monitoring/alerting, restic off-site backups, deploy/restore scripts, SHA-pinned CI, Trivy, e2e stack job, GHCR CD (D-034) | `muhamhad-pc1/claude-infra` | 2026-10-12 | merge e906804, 05e9cf1 |
| Wave 2 backend — server-verified guard sessions, signed checkpoint QR, patrol schedules + `patrol_not_started` + compliance report (D-036..D-038) | `muhamhad-pc1/claude-be-field` | 2026-10-12 | merge agent/muhamhad-pc1/be-field |
| Wave 2 mobile — hardening MOB-2..MOB-11 (protected sign-out, lock-screen SOS, server time offset, outbox failure handling, encrypted local data, release signing + new app id, bundled CA, site time zone, EXIF strip) + guard-session and signed-QR client; 63 tests (D-039) | `muhamhad-pc1/claude-mobile` | 2026-10-12 | merge d916d4f |
| Wave 2 web — WEB-1..10: deps (0 high), CSP-clean build, realm-wide SOS banner + sound, idle logout, a11y (axe clean), all error codes EN/ID, per-route error boundary, safe URLs, lazy charts, demo guard; UIs for platform-admin settings, ops alerts/log, audit log, plausibility flags; 100 unit + 82 e2e tests (D-040) | `muhamhad-pc1/claude-web` | 2026-10-12 | merge 9feb63b |
| Wave 2 backend — users + device accounts via Keycloak admin API, per-site scoping, realm-wide `GET /sos/active`, `incident_filed`, effective amended values, override history, XLSX/PDF exports + paging, route snapshots, channel registry, floor plans + batched live view, retention + encrypted visitor IDs, tablet telemetry, incident GPS (D-041..D-049) | `muhamhad-pc1/claude-be-features` | 2026-10-12 | ab4c3aa |
| Wave 3 web — Users & access (Keycloak users, roles, site scope, reset password/OTP), tablet accounts + health drawer, setup guide tablet step, scope hint, attendance override history, visitor detail (audited reveal); 135 unit + 97 e2e tests | `muhamhad-pc1/claude-web-people` | 2026-10-13 | merge f5811b7 |
| Wave 3 web — floor plans + checkpoint placement, live patrol on the plan, patrol schedules editor, compliance report + KPI/monthly columns, regenerate QR, Excel/CSV/PDF downloads, amended incidents, incident GPS, `incident_filed`/`patrol_not_started` rules, channel list, paging, `/sos/active`; 156 unit + 119 e2e tests after merge | `muhamhad-pc1/claude-web-patrol` | 2026-10-13 | merge with web-people |
| Monthly recap per guard-day (F13): backend report + totals, CSV/XLSX total row, recap e-mail/WA in site language, web table + totals (D-051) | `muhamhad-pc1/claude-recap-days` | 2026-10-13 | merge recap-days |
| Keycloak 24.0.5 → 26.8.0: hostname v2, management port 9000, bootstrap admin vars, token contract (full scope off, `basic` scope), keycloak-js 26.2.4, `make keycloak-smoke`, `RESTORE_FRESH` rollback, CI stack checks, runbook upgrade procedure (D-052) | `muhamhad-pc1/claude-kc26` | 2026-10-13 | merge keycloak-26 |
| Rebrand SecurePatrol → GARDA: all user-visible names (web, tablet, e-mail/WA, exports, Keycloak display names, alerts, docs, sales guide), app id `id.co.garda.tablet`, single icon source `branding/` + generator, PWA manifest (D-053) | `muhamhad-pc1/claude-rebrand` | 2026-10-14 | merge rebrand-garda |
| Push notifications: Web Push (VAPID) + FCM sender, subscriptions & per-user preferences, Notifications page + service worker, `sos_unacknowledged` reminders, `attendance_review`/`device_pending`/`device_unhealthy`; tablet FCM client with local notifications and push-aware polling (D-054..D-057) | `muhamhad-pc1/claude-push`, `claude-push-mobile` | 2026-10-14 | merges 60141be, ea33a13 |
| Dependabot clean-up: merged minor updates (Go x/crypto + miniredis, npm minor group, Prometheus 3.15, Alpine 3.24), closed majors/obsolete PRs, monthly grouped config without majors; docs/maintenance.md; stabilised push e2e | `muhamhad-pc1/claude` | 2026-10-14 | merge deps-2026-10 |
| CI green on all workflows: excelize CVE fixes, `apk upgrade` in images, full Chromium for e2e, stable axe checks, Keycloak port check; first successful APK build (geolocator_android pinned to 4.6.1 for Flutter 3.24) | `muhamhad-pc1/claude` | 2026-10-14 | 7bbc6bb, 6e617ba |
| Flutter 3.47.7 + AGP 9 / Gradle 9.1 / Kotlin 2.2 (mokhataq), plugins for API 35+, macOS node-exporter mode; CI pins Flutter 3.47.7; installable debug APK artifact on every CI run (D-058) | `mokhataq`, `muhamhad-pc1/claude` | 2026-10-16 | merge merge/mokhataq-2026-10-16 |
