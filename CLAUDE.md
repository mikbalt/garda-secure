# CLAUDE.md — SecurePatrol (garda-secure)

Guide for AI agents (and humans) working in this repo. Several agents may work
**at the same time from different PCs**, so follow the coordination protocol
below before touching code.

- Requirements: [SecurePatrol PRD-v3.md](SecurePatrol%20PRD-v3.md) (source of truth for scope)
- Status / who is doing what: [PROGRESS.md](PROGRESS.md)
- Architecture & product decisions: [DECISIONS.md](DECISIONS.md)

## Coordination protocol (multi-agent, multi-PC)

1. **Sync first.** `git pull --rebase` before starting, then read `PROGRESS.md` and `DECISIONS.md`.
2. **Claim before you work.** Move the task to *In progress* in `PROGRESS.md` with
   your agent tag (`<pc-or-user>/<short-name>`) and the date, commit **only that
   change** (`chore(progress): claim <task>`) and push immediately. If someone
   else already claimed it, pick another task.
3. **Stay in your lane.** Only edit files your task needs. Shared hot spots —
   `backend/cmd/securepatrol/main.go` (route wiring), `backend/internal/model/models.go`,
   `web/src/types/index.ts`, `web/src/App.tsx` — keep edits small and rebase often.
4. **Migrations:** numbers are global. Reserve the next number in `PROGRESS.md`
   (section *Migration numbers*) as part of your claim. Never edit a migration
   that has been pushed; add a new one.
5. **Decisions:** anything that changes behaviour, an API contract, the schema or
   a convention gets a new entry in `DECISIONS.md` (append-only; supersede, don't rewrite).
6. **Finish cleanly:** build + tests green, update `PROGRESS.md` (move to *Done*,
   note follow-ups), commit with Conventional Commits (`feat(backend): …`), push.
7. Work on a branch `agent/<tag>/<task>` and merge into `init` (current main) once
   green, or commit directly to `init` for small, isolated changes. Never force-push `init`.

## Repo layout

```
backend/   Go 1.22 + Fiber v2 — REST API; `securepatrol worker` = notification worker + scheduler
web/       React 18 + Vite + TS + TanStack Query + Tailwind — admin/HR/client dashboard
mobile/    Flutter tablet app (DEFERRED — do not change until backend/web v1 is done)
infra/     Docker Compose (postgres, redis, minio, keycloak, nginx, grafana, prometheus)
```

## Build & test

Backend (from `backend/`):
```bash
go build ./... && go vet ./... && go test ./...
go test -tags integration ./...   # spins up an embedded Postgres 16, no Docker needed
```
If Go is not installed, a portable toolchain works: download `go1.22.x` zip,
put its `bin` on PATH and set `GOTOOLCHAIN=local`.

Web (from `web/`): `npm ci && npm run lint && npx tsc --noEmit && npm run build`

Infra (from `infra/`): `cp .env.example .env`, then
`docker compose -f docker-compose.yml -f docker-compose.app.yml up -d`.

## Backend conventions

- **Layers:** `handler` (HTTP, validation, RBAC) → `service` (business rules) →
  `repository` (SQL only). Handlers are methods on `handler.Handler`, which holds
  the dependencies (pool, redis, storage, notifier, config) — do not use `c.Locals`
  for dependencies.
- **Repository:** functions take a `repository.DBTX` (pool or tx). `Get*` return
  `repository.ErrNotFound` when missing — never `nil, nil`. List functions return
  `[]T{}` (not nil) so JSON renders `[]`.
- **Auth:** every `/api/v1` route needs a Keycloak JWT. Roles: `admin`, `supervisor`,
  `hr`, `client-readonly` (web) and `device` (tablet). Check roles per route with
  `middleware.RequireRoles`. Tablet routes use `RequireActiveDevice` + `RequireGuard`;
  the acting guard comes from the `X-Guard-ID` header and is validated against the
  device's site. **Never take guard_id / actor identity from the request body.**
- **Tenancy:** one Keycloak realm per client. A site belongs to one realm
  (`sites.keycloak_realm`); every site-scoped request must check the caller's realm
  owns the site (helper `h.siteForRead/siteForWrite`).
- **Time:** store UTC (`timestamptz`). "Day"/"month" boundaries are computed in the
  **site's time zone** in Go and passed to SQL as timestamp ranges. Never use
  `::date` on a timestamptz in queries.
- **Offline sync:** create endpoints used by tablets accept an optional client
  `id` (UUID) and are idempotent (`ON CONFLICT (id) DO NOTHING` → return existing,
  200 instead of 201). Client timestamps are accepted within a bounded window.
- **Errors:** JSON `{"error": "...", "code": "optional_machine_code"}`; don't leak
  raw DB errors for 5xx.
- **Notifications:** publish domain events via the notifier; never call SMTP/WhatsApp
  from a request handler.

## Web conventions

- API client base URL is `VITE_API_URL` and **already includes `/api/v1`**; call
  paths like `/sites`, `/attendance/daily`.
- All UI strings go through i18n (English + Bahasa Indonesia).
- Every site-scoped page uses the global site selector.

## Don'ts

- Don't commit secrets or `.env` files.
- Don't touch `mobile/` until it is un-deferred in `PROGRESS.md`.
- Don't rewrite pushed history or edit pushed migrations.
