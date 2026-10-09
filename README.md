# SecurePatrol

Enterprise-grade security patrol and attendance management system.

## Monorepo Structure

```
securepatrol/
├── backend/        # Go 1.22 + Fiber — REST API + notification worker
├── web/            # React 18 + Vite + TypeScript — admin web dashboard
├── mobile/         # Flutter — iOS/Android tablet app for field guards
└── infra/          # Docker Compose, Keycloak, Nginx, Prometheus
```

## Docs

- Requirements: [SecurePatrol PRD-v3.md](SecurePatrol%20PRD-v3.md)
- API contract: [docs/api.md](docs/api.md)
- Operations (install, Keycloak users, tablets, backups/restore): [docs/runbook.md](docs/runbook.md)
- Working in this repo (incl. multi-agent rules): [CLAUDE.md](CLAUDE.md), [PROGRESS.md](PROGRESS.md), [DECISIONS.md](DECISIONS.md)

## Quick Start

### Full stack (Docker)
```bash
cd infra
cp .env.example .env   # edit every change_me value
make certs             # self-signed TLS cert for dev/LAN
make up                # https://localhost
```
Then create users in Keycloak (`https://localhost/auth/admin/`) — see the runbook.

### Local development
```bash
# 1. Infrastructure only (set KEYCLOAK_PUBLIC_URL=http://localhost:8081/auth in infra/.env)
cd infra && make up-infra

# 2. Backend — env vars from backend/.env.example
cd backend && go run ./cmd/securepatrol          # API on :8080
cd backend && go run ./cmd/securepatrol worker   # notifications + scheduled jobs
go test ./... && go test -tags integration ./internal/integration/   # embedded Postgres, no Docker

# 3. Web — env vars from web/.env.example
cd web && npm install && npm run dev

# Web without any backend (sample data, role switcher) — for design review & demos
cd web && npm run dev:demo
```
The Flutter tablet app (`mobile/`) is deferred until backend/web v1 is done.

## Branch Strategy

| Branch | Purpose |
|---|---|
| `main` | Production-ready; protected; deploy via CD pipeline |
| `develop` | Integration; merges trigger staging deploy |
| `feat/*` | Feature branches; PR into develop |
| `fix/*` | Bug fixes; PR into develop |
| `chore/*` | Deps, config, docs |

## CI/CD

GitHub Actions pipelines run on pushes and PRs that touch each folder:
- **ci-backend**: lint (golangci-lint) → unit tests (race) + integration tests (embedded Postgres) → security (gosec) → docker build
- **ci-web**: lint (ESLint) → typecheck → build → docker build (+ runtime config check)
- **ci-infra**: compose config validate → Keycloak realm JSON → nginx -t
- **ci-mobile**: manual only while the app is deferred

Deploy (**cd-deploy**): push a tag `v*` or run it manually with a branch/tag —
see [docs/runbook.md](docs/runbook.md) §7 for the required secrets.

## Tech Stack

| Layer | Technology |
|---|---|
| API | Go 1.22 + Fiber v2 |
| Auth | Keycloak 24 (OIDC, RBAC, TOTP) |
| Database | PostgreSQL 16 |
| Object Storage | MinIO (photos, backups) |
| Cache / Queue | Redis 7 |
| Web Frontend | React 18 + Vite + TanStack Query |
| Mobile App | Flutter 3.24 |
| Reverse Proxy | Nginx |
| Monitoring | Grafana + Prometheus |
| Orchestration | Docker Compose |

