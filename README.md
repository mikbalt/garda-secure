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

## Quick Start (local dev)

### Prerequisites
- Docker 24+ and Docker Compose v2
- Go 1.22+
- Node.js 20+
- Flutter 3.24+

### 1. Infrastructure
```bash
cd infra
cp .env.example .env
# Edit .env with your values
# Generate a self-signed TLS cert for local dev:
mkdir -p nginx/certs
openssl req -x509 -nodes -days 365 -newkey rsa:2048 \
  -keyout nginx/certs/key.pem \
  -out nginx/certs/cert.pem \
  -subj "/CN=localhost"
make up
```

### 2. Backend (local dev without Docker)
```bash
cd backend
cp ../.env.example .env  # adjust DATABASE_URL etc for local ports
go run ./cmd/securepatrol
```

### 3. Web (local dev)
```bash
cd web
npm install
cp .env.example .env.local  # set VITE_API_URL, VITE_KEYCLOAK_URL etc
npm run dev
```

### 4. Mobile (local dev)
```bash
cd mobile
flutter pub get
cp .env.example .env  # set API_URL, KEYCLOAK_URL etc
flutter run
```

## Branch Strategy

| Branch | Purpose |
|---|---|
| `main` | Production-ready; protected; deploy via CD pipeline |
| `develop` | Integration; merges trigger staging deploy |
| `feat/*` | Feature branches; PR into develop |
| `fix/*` | Bug fixes; PR into develop |
| `chore/*` | Deps, config, docs |

## CI/CD

GitHub Actions pipelines run on every PR:
- **ci-backend**: lint (golangci-lint) → test (race) → security (gosec) → docker build
- **ci-web**: lint (ESLint) → typecheck → build
- **ci-mobile**: analyze → test → APK build (smoke)
- **ci-infra**: compose config validate → nginx -t

On merge to `main`: SSH deploy to production server.

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

## PRD

See [SecurePatrol PRD](https://kirocrew.app/artifacts/securepatrol-prd) for full requirements.
