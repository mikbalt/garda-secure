# SecurePatrol

Enterprise-grade security patrol and attendance management system, self-hosted
on one on-premise server (data stays on site — NFR-02).

## Monorepo Structure

```
garda-secure/
├── backend/        # Go + Fiber — REST API + notification worker/scheduler
├── web/            # React 18 + Vite + TypeScript — admin/HR/client dashboard
├── mobile/         # Flutter — shared site tablet app (D-024)
├── infra/          # Docker Compose stack, nginx, Keycloak, monitoring, backups, ops scripts
└── docs/           # API contract, operations runbook, audits
```

## Docs

- Requirements: [SecurePatrol PRD-v3.md](SecurePatrol%20PRD-v3.md)
- API contract: [docs/api.md](docs/api.md)
- Operations (install, certificates, Keycloak/MFA, backups + restore drill, deploy/rollback, monitoring): [docs/runbook.md](docs/runbook.md)
- Working in this repo (incl. multi-agent rules): [CLAUDE.md](CLAUDE.md), [PROGRESS.md](PROGRESS.md), [DECISIONS.md](DECISIONS.md)
- Latest audit: [docs/audit-2026-10.md](docs/audit-2026-10.md)

## Quick Start

### Full stack (Docker, production-like)
```bash
cd infra
make env SERVER_HOST=patrol.example.co.id   # .env with random secrets (DNS name or LAN IP)
make certs                                  # private CA + server certificate for SERVER_HOST
make up                                     # preflight checks, start everything, configure Keycloak
```
Then install `infra/nginx/certs/securepatrol-ca.crt` on admin PCs/tablets, create users
in Keycloak (`https://<SERVER_HOST>/auth/admin/`, LAN only) — see the runbook.
`COMPOSE_FILE` in `.env` makes plain `docker compose up -d` / `ps` / `logs` act on the
whole stack.

### Local development
```bash
# 1. Infrastructure only (Postgres, Redis, MinIO, Keycloak on :8081)
cd infra && make env SERVER_HOST=localhost
#    in .env: KEYCLOAK_PUBLIC_URL=http://localhost:8081/auth, KEYCLOAK_DEV_REDIRECTS=true
make up-infra && make keycloak-config

# 2. Backend — env vars from backend/.env.example
cd backend && go run ./cmd/securepatrol          # API on :8080
cd backend && go run ./cmd/securepatrol worker   # notifications + scheduled jobs
go test ./... && go test -tags integration ./internal/integration/   # embedded Postgres, no Docker

# 3. Web — env vars from web/.env.example
cd web && npm install && npm run dev

# Web without any backend (sample data, role switcher) — for design review & demos
cd web && npm run dev:demo

# 4. Tablet app
cd mobile && flutter pub get && flutter run
```

## Branch Strategy

| Branch / ref | Purpose |
|---|---|
| `init` | Integration branch (current main). Protected; CI must be green. |
| `agent/<tag>/<task>` | Work branches (see CLAUDE.md); merged into `init` when green. |
| tag `vX.Y.Z` | Release. Pushing a tag on `init` starts the gated production deploy. |

## CI/CD

GitHub Actions run on pushes and PRs that touch each folder; every action is pinned
by commit SHA and Dependabot keeps them (and Go/npm/pub/Docker dependencies) current.

| Workflow | What it checks |
|---|---|
| **ci-backend** | golangci-lint → unit tests (race) + integration tests (embedded Postgres) → gosec → Docker build + Trivy |
| **ci-web** | ESLint → typecheck → `npm audit` (prod deps) → build → Docker build (read-only, runtime config, security headers) + Trivy |
| **ci-infra** | shellcheck, `make env/certs/preflight`, compose config (all overlays, env coverage), realm JSON, `nginx -t`, promtool/amtool/blackbox, dashboards |
| **ci-e2e-stack** | Boots the whole stack with `docker compose up --wait` and tests it from outside: TLS, OIDC discovery, tablet token → API, internal endpoints not exposed, Prometheus targets, backups |
| **ci-mobile** | `flutter analyze` + tests; on tags a signed release APK (needs signing secrets) |
| **cd-deploy** | On tag `v*`: verify tag is on `init` and all CI green for that commit → build + push images to GHCR → approval (environment `production`) → SSH deploy by tag/digest with pre-deploy DB backup |

Deploy secrets, environment protection and rollback: [docs/runbook.md](docs/runbook.md) §7.

## Tech Stack

| Layer | Technology |
|---|---|
| API | Go 1.26 + Fiber v2 |
| Auth | Keycloak 26 (OIDC, RBAC, enforced TOTP for staff roles); keycloak-js 26 in the web app |
| Database | PostgreSQL 16 |
| Object Storage | S3-compatible (Silo — maintained MinIO fork): photos, backups |
| Cache / Queue | Redis 7 |
| Web Frontend | React 18 + Vite + TanStack Query |
| Mobile App | Flutter 3.24 |
| Reverse Proxy | nginx 1.30 (TLS, CSP, rate limits) |
| Monitoring | Prometheus + Alertmanager + Grafana, node/cAdvisor/Postgres/Redis/blackbox exporters |
| Backups | pg_dump to object storage + encrypted off-site restic copy |
| Orchestration | Docker Compose |
