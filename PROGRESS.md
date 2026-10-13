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
| 008 | *(next free)* | | |

## Milestones (PRD §11)

| Milestone | Status |
|---|---|
| M1 Infra & Auth | 🟢 backend + config done; ⚠️ full Docker stack not yet smoke-tested (no Docker on the dev PC) |
| M2 Core Attendance | 🟢 backend + web done |
| M3 Patrol | 🟢 backend + web + mobile: schedules, signed QR, live view on site floor plans (polling, D-014) |
| M4 Incident & Field Reports | 🟢 backend + web + mobile (D-024) |
| M5 Web Dashboard & Reports | 🟢 redesigned (D-022): setup guide, Today dashboard, all pages; verified in demo mode (all roles, desktop + 390 px); ⚠️ not yet exercised against a running stack |
| M6 Notifications & Email | 🟢 backend done (rules, log, retry, SMTP/WhatsApp, monthly recap); needs real SMTP/WA test |
| M7 Hardening & Go-Live | 🟡 audit 2026-10 (docs/audit-2026-10.md): wave 1 merged (backend reliability + security, infra/ops/CI); wave 2 in progress; ⚠️ full stack still never booted — ci-e2e-stack.yml does it on GitHub |

## Backlog (unclaimed)

Verification
- [ ] Smoke-test the full Docker stack (`make up`): Keycloak `start` under `/auth`, realm import, login, MinIO via :9443, backup job (`pg_dump` in image)
- [ ] Real SMTP + WhatsApp template delivery test

Backend
- [ ] WebSocket live patrol updates (D-014 follow-up)

Web
- [ ] Tailwind 4 migration (dev-only `npm audit` findings in tailwind 3 build deps)
- [ ] Manual UI pass against a running stack (all roles, both languages)
- [ ] Promote page-local kits to `components/ui`: Avatar/PersonCell, FilterChips/ChipGroup, Segmented, PeriodStepper/DateRangeFilter, ConfirmDialog, Switch, Stat tiles, LiveIndicator (see `pages/guards/PeopleKit.tsx`, `pages/patrol/kit.tsx`, `pages/reports/ReportKit.tsx`, `pages/settings/kit.tsx`)
- [ ] `Table`: optional row-click prop (Patrol history uses its own table markup)
- [ ] Attendance "Perlu ditinjau" chip counts only the selected day while the tab counts all days — align or label
- [ ] Device dialog: "set back to pending" action

Infra / Ops
- [ ] Rehearse the Keycloak 24→26 DB migration on a copy of the production Keycloak dump before upgrading production (runbook §2/§5)
- [ ] Configure the `production` environment secrets and do a first tagged deploy (runbook §7)
- [ ] Let's Encrypt / production TLS notes
- [ ] Load test 2 sites / 10 guards (M7)

Mobile (D-024) — open audit items, see docs/audit-2026-10.md
- [ ] First real APK on a tablet: private-CA TLS, camera/QR, kiosk mode (runbook §3)

## Done

| Task | Agent | Date | Ref |
|---|---|---|---|
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
