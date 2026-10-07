# Progress

Live status board for all agents. **Claim a task before working on it** (see
CLAUDE.md → Coordination protocol). Keep entries short; link commits/PRs.

Agent tag format: `<pc-or-user>/<short-name>` — e.g. `muhamhad-pc1/claude`.

## In progress

| Task | Agent | Since | Notes |
|---|---|---|---|
| Backend v1 core rework (auth, schema 003, all domain endpoints, worker, scheduler) | `muhamhad-pc1/claude` | 2026-10-07 | Uncommitted on that PC until it builds + tests pass. Please don't edit `backend/` until pushed. |
| Infra fixes (nginx, keycloak realms, compose, env, Dockerfile) | `muhamhad-pc1/claude` | 2026-10-07 | Queued after backend. |
| Web: align with new API + missing pages | `muhamhad-pc1/claude` | 2026-10-07 | Queued after backend API is stable. |

## Migration numbers

| # | File | Owner | Status |
|---|---|---|---|
| 001 | init_schema | — | pushed |
| 002 | audit_log | — | pushed |
| 003 | v1_core (devices, shift templates, patrol sessions, amendments, notifications, settings, backups) | `muhamhad-pc1/claude` | in progress |
| 004 | *(next free)* | | |

## Milestones (PRD §11)

| Milestone | Status |
|---|---|
| M1 Infra & Auth | 🟡 compose/keycloak exist; multi-realm, device auth, nginx fixes in progress |
| M2 Core Attendance | 🟡 being rebuilt on shift templates (D-003/D-004) |
| M3 Patrol | 🟡 sessions + SLA in progress |
| M4 Incident & Field Reports | 🟡 backend in progress; mobile deferred |
| M5 Web Dashboard & Reports | 🟡 partial pages exist; needs alignment |
| M6 Notifications & Email | 🟡 worker exists; rules/log/retry/scheduler in progress |
| M7 Hardening & Go-Live | ⚪ not started |

## Backlog (unclaimed)

Backend
- [ ] WebSocket live patrol updates (D-014 follow-up)
- [ ] Prometheus `/metrics` endpoint for the API
- [ ] PDF export for incident log (CSV first)
- [ ] Excel (.xlsx) export (CSV with BOM for now)
- [ ] Rate limiting on tablet endpoints (Redis)

Web
- [ ] i18n (English + Bahasa Indonesia) across all pages
- [ ] Live patrol map (needs checkpoint lat/lng)
- [ ] WCAG 2.1 AA pass on forms/tables/alerts (NFR-08)

Infra / Ops
- [ ] Second client realm for testing isolation (`securepatrol-client2`)
- [ ] Backup/restore runbook (`docs/runbook.md`)
- [ ] Let's Encrypt / production TLS notes
- [ ] Load test 2 sites / 10 guards (M7)

Mobile (deferred — D-017)
- [ ] Device registration + offline PIN unlock (D-001)
- [ ] Switch to new endpoints, `X-Guard-ID`, client UUIDs
- [ ] Fix `connectivity_plus` v6 `List<ConnectivityResult>` check

## Done

| Task | Agent | Date | Ref |
|---|---|---|---|
| Initial scaffold, infra compose split, backend handlers (first pass) | — | 2026-10-07 | 60b3c05…d189988 |
| PRD v3 decisions OQ-7/8/9, CLAUDE.md / PROGRESS.md / DECISIONS.md | `muhamhad-pc1/claude` | 2026-10-07 | uncommitted |
