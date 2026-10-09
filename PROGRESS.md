# Progress

Live status board for all agents. **Claim a task before working on it** (see
CLAUDE.md → Coordination protocol). Keep entries short; link commits/PRs.

Agent tag format: `<pc-or-user>/<short-name>` — e.g. `muhamhad-pc1/claude`.

## In progress

| Task | Agent | Since | Notes |
|---|---|---|---|
| Mobile tablet app v1 (D-024): device login/registration, offline PIN unlock, clock-in/out, QR patrol, incidents/defects/visitors, SOS, offline outbox; backend `GET /device/status`, `GET /device/sos` | `muhamhad-pc1/claude` | 2026-10-09 | Please don't edit `mobile/` until pushed. |
| Hardening wave 1 — backend reliability: notifications/worker/jobs/backups/observability (audit BE-R*, BE-O1, F2 part, F14) | `muhamhad-pc1/claude-be-rel` | 2026-10-09 | branch `agent/muhamhad-pc1/be-reliability`; migration 004 |
| Hardening wave 1 — backend security: tenancy replay, settings exfil, JWT, limits/rate limit, audit log, deps + Go upgrade (audit BE-S2..S10) | `muhamhad-pc1/claude-be-sec` | 2026-10-09 | branch `agent/muhamhad-pc1/be-security`; migration 005 |
| Hardening wave 1 — infra/ops/CI: first-boot fixes, nginx, TLS/preflight, backups off-host, monitoring, CI smoke + supply chain (audit INF-*) | `muhamhad-pc1/claude-infra` | 2026-10-09 | branch `agent/muhamhad-pc1/infra-hardening` |
| Hardening wave 1 — web: deps, CSP, SOS all sites, idle logout, a11y, i18n errors, tests (audit WEB-*) | `muhamhad-pc1/claude-web` | 2026-10-09 | branch `agent/muhamhad-pc1/web-hardening` |

## Migration numbers

| # | File | Owner | Status |
|---|---|---|---|
| 001 | init_schema | — | pushed |
| 002 | audit_log | — | pushed |
| 003 | v1_core (devices, shift templates, patrol sessions, amendments, notifications, settings, backups) | `muhamhad-pc1/claude` | pushed |
| 004 | be_reliability | `muhamhad-pc1/claude-be-rel` | reserved |
| 005 | be_security | `muhamhad-pc1/claude-be-sec` | reserved |
| 006 | *(next free)* | | |

## Milestones (PRD §11)

| Milestone | Status |
|---|---|
| M1 Infra & Auth | 🟢 backend + config done; ⚠️ full Docker stack not yet smoke-tested (no Docker on the dev PC) |
| M2 Core Attendance | 🟢 backend + web done |
| M3 Patrol | 🟢 backend + web done (live view by polling) |
| M4 Incident & Field Reports | 🟢 backend done; ⚪ mobile deferred |
| M5 Web Dashboard & Reports | 🟢 redesigned (D-022): setup guide, Today dashboard, all pages; verified in demo mode (all roles, desktop + 390 px); ⚠️ not yet exercised against a running stack |
| M6 Notifications & Email | 🟢 backend done (rules, log, retry, SMTP/WhatsApp, monthly recap); needs real SMTP/WA test |
| M7 Hardening & Go-Live | ⚪ not started |

## Backlog (unclaimed)

Verification
- [ ] Smoke-test the full Docker stack (`make up`): Keycloak `start` under `/auth`, realm import, login, MinIO via :9443, backup job (`pg_dump` in image)
- [ ] Real SMTP + WhatsApp template delivery test

Backend
- [ ] WebSocket live patrol updates (D-014 follow-up)
- [ ] Prometheus `/metrics` endpoint for the API (+ postgres/redis exporters)
- [ ] PDF export for incident log (web print view first)
- [ ] Excel (.xlsx) export (CSV with BOM for now)
- [ ] Rate limiting on tablet endpoints (Redis)
- [ ] Retention/cleanup for old backups in MinIO (lifecycle rule)

Web
- [ ] Manual UI pass against a running stack (all roles, both languages)
- [ ] Promote page-local kits to `components/ui`: Avatar/PersonCell, FilterChips/ChipGroup, Segmented, PeriodStepper/DateRangeFilter, ConfirmDialog, Switch, Stat tiles, LiveIndicator (see `pages/guards/PeopleKit.tsx`, `pages/patrol/kit.tsx`, `pages/reports/ReportKit.tsx`, `pages/settings/kit.tsx`)
- [ ] `Table`: optional row-click prop (Patrol history uses its own table markup)
- [ ] Attendance "Perlu ditinjau" chip counts only the selected day while the tab counts all days — align or label
- [ ] Device dialog: "set back to pending" action
- [ ] Web unit tests (no test runner yet — add Vitest)
- [ ] Live patrol map (needs checkpoint lat/lng in routes)

Infra / Ops
- [ ] Configure the `production` environment secrets and do a first tagged deploy (runbook §7)
- [ ] Let's Encrypt / production TLS notes
- [ ] Load test 2 sites / 10 guards (M7)

Mobile (deferred — D-017)
- [ ] Re-enable push/PR triggers in `ci-mobile.yml` once `flutter create` scaffolding (android/, test/) exists
- [ ] Device registration + offline PIN unlock (D-001), roster sync
- [ ] Switch to new endpoints (docs/api.md), `X-Guard-ID`, client UUIDs, JPEG photo keys
- [ ] Fix `connectivity_plus` v6 `List<ConnectivityResult>` check
- [ ] SOS retry every 10 s with the same `id`

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
