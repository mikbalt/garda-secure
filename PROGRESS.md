# Progress

Live status board for all agents. **Claim a task before working on it** (see
CLAUDE.md → Coordination protocol). Keep entries short; link commits/PRs.

Agent tag format: `<pc-or-user>/<short-name>` — e.g. `muhamhad-pc1/claude`.

## In progress

| Task | Agent | Since | Notes |
|---|---|---|---|
| *(nothing claimed)* | | | |

## Migration numbers

| # | File | Owner | Status |
|---|---|---|---|
| 001 | init_schema | — | pushed |
| 002 | audit_log | — | pushed |
| 003 | v1_core (devices, shift templates, patrol sessions, amendments, notifications, settings, backups) | `muhamhad-pc1/claude` | pushed |
| 004 | *(next free)* | | |

## Milestones (PRD §11)

| Milestone | Status |
|---|---|
| M1 Infra & Auth | 🟢 backend + config done; ⚠️ full Docker stack not yet smoke-tested (no Docker on the dev PC) |
| M2 Core Attendance | 🟢 backend + web done |
| M3 Patrol | 🟢 backend + web done (live view by polling) |
| M4 Incident & Field Reports | 🟢 backend done; ⚪ mobile deferred |
| M5 Web Dashboard & Reports | 🟢 all pages built (lint/typecheck/build green); ⚠️ not yet exercised against a running stack |
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
- [ ] gosec job in CI uses `@latest` with Go 1.22 — pin a compatible gosec version

Web
- [ ] Manual UI pass against a running stack (all roles, both languages)
- [ ] Device dialog: "set back to pending" action
- [ ] Web unit tests (no test runner yet — add Vitest)
- [ ] Live patrol map (needs checkpoint lat/lng in routes)

Infra / Ops
- [ ] `cd-deploy.yml` targets branch `main` and runs compose from repo root — align with `init` branch and `infra/` overlay
- [ ] Let's Encrypt / production TLS notes
- [ ] Load test 2 sites / 10 guards (M7)

Mobile (deferred — D-017)
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
| Web v1: runtime `/config.js`, multi-realm org picker, role-aware nav, site selector, EN/ID i18n, Dashboard + SOS banner, Attendance (+override, review queue), Schedules, Guards (+PIN), Patrol (route editor + QR print, live, sessions), Incidents/SOS/Defects/Visitors, 5 reports + CSV + print, Settings (sites, devices, rules, channels, log, backups) | `muhamhad-pc1/claude` | 2026-10-07 | this push |
| Backend: `guard_name` on list rows, `GET /media/url` | `muhamhad-pc1/claude` | 2026-10-07 | 8cfc11a |
| Docs: `docs/api.md` (API contract), `docs/runbook.md` (install, users, tablets, backups/restore) | `muhamhad-pc1/claude` | 2026-10-07 | ace9df5 |
