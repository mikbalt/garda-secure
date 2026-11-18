# WebSocket Live Patrol Updates Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** The Patrol Live tab refreshes right after a tablet scan/start/end via a WebSocket "changed" nudge, with polling as fallback.

**Architecture:** An in-process hub (`internal/livehub`) holds per-site WebSocket subscribers and single-use tickets. Patrol write handlers call `hub.Publish(siteID)`. The web hook gets a ticket (Bearer), opens the socket, and invalidates the `['patrol-live', siteId]` query on each nudge.

**Tech Stack:** Go 1.26, Fiber v2, `github.com/gofiber/contrib/websocket`; React 18, TanStack Query, vitest.

**Spec:** `docs/superpowers/specs/2026-11-18-ws-live-patrol-design.md`

## Global Constraints

- No new infrastructure; no Redis use for this feature. Single API instance only (documented).
- Messages carry no patrol data: exactly `{"type":"patrol_changed","site_id":"<uuid>"}`.
- Ticket: 32+ random bytes hex, TTL 30 s, single use, bound to a site id.
- Never put the JWT in a URL. Realm/site check via `h.siteFor` at ticket issue (roles `rolesOpsCli`, same as `/patrol/live`).
- Per-site cap 50 sockets; ping every 25 s; drop a subscriber whose send buffer (cap 8) is full.
- Connected: poll 60 s. Disconnected: poll 10 s (current behaviour).
- Follow CLAUDE.md: update `docs/api.md`, new `DECISIONS.md` entry (append-only, supersedes D-014), conventional commits, commit trailer `Co-Authored-By: Claude Sonnet 5.5 <noreply@anthropic.com>`.

## Review Focus

- Expired/reused/unknown ticket → upgrade refused (401), never subscribes.
- Ticket issued for site A cannot receive site B events.
- Site in another realm → ticket request 404 (same as `siteFor`).
- Publish with no subscribers, or on a nil hub, never blocks or panics.
- Slow client never blocks `Publish`/the scan request.
- Socket drop → hook falls back to 10 s polling and reconnects with backoff; unmount closes the socket.

---

### Task 1: Live hub

**Files:**
- Create: `backend/internal/livehub/hub.go`
- Test: `backend/internal/livehub/hub_test.go`

**Interfaces:**
- Produces:
  - `func New() *Hub`
  - `func (h *Hub) IssueTicket(siteID string) string`
  - `func (h *Hub) Redeem(ticket string) (siteID string, ok bool)` (single use, TTL 30 s)
  - `func (h *Hub) Subscribe(siteID string) (sub *Sub, ok bool)` — `ok=false` at the 50-per-site cap
  - `type Sub struct{ C <-chan []byte }`; `func (h *Hub) Unsubscribe(s *Sub)`
  - `func (h *Hub) Publish(siteID string)` — non-blocking; safe on nil `*Hub`
  - `var ErrNil`-free: all methods nil-safe where noted

- [ ] **Step 1: Write failing tests** in `hub_test.go`: `TestTicketSingleUse`, `TestTicketExpires` (inject clock via unexported `now func() time.Time`), `TestPublishReachesOnlySiteSubscribers` (payload equals `{"type":"patrol_changed","site_id":"A"}`), `TestSubscribeCap` (51st returns ok=false), `TestSlowSubscriberDoesNotBlock` (publish 20 times to an unread sub; returns promptly and the sub is dropped/closed), `TestNilHubPublishNoPanic`.
- [ ] **Step 2:** Run `cd backend && go test ./internal/livehub/` — expect FAIL (package missing).
- [ ] **Step 3:** Implement `hub.go` with a mutex, `map[string]map[*Sub]struct{}`, `map[string]ticket{site, exp}`; purge expired tickets on `IssueTicket`.
- [ ] **Step 4:** Run with `-race` — expect PASS.
- [ ] **Step 5:** Commit `feat(backend): in-process live hub for patrol updates`.

### Task 2: Endpoints, publish hooks, docs

**Files:**
- Create: `backend/internal/handler/patrol_live_ws.go`, `backend/internal/handler/patrol_live_ws_test.go`
- Modify: `backend/internal/handler/handler.go` (field `live *livehub.Hub`; route `POST /ws/ticket` registered in `Register` with `web(rolesOpsCli...)`), `backend/internal/handler/app.go` (`Mount`), `backend/internal/handler/patrol.go` (publish calls), `docs/api.md`, `backend/go.mod`/`go.sum`

**Interfaces:**
- Consumes: Task 1 `Hub`.
- Produces:
  - `func (h *Handler) wsTicket(c *fiber.Ctx) error` — body `{site_id}`; `h.siteFor`; returns `{"ticket": "...", "expires_in": 30}`.
  - `func (h *Handler) wsPatrol() []fiber.Handler` — upgrade check (`websocket.IsWebSocketUpgrade`), redeem `?ticket=`, 401 `{"error","code":"invalid_ticket"}` on failure, 503 `ws_site_full` at cap; then subscribe, forward `C` to the socket, ping every 25 s, unsubscribe on close.
  - `func (h *Handler) publishPatrol(siteID string)` — nil-safe wrapper over `h.live.Publish`.

- [ ] **Step 1: Write failing tests** (fiber `app.Test` for HTTP parts; a real `httptest` listener + `gorilla/websocket`-style client from the contrib dependency for the socket): `TestWsTicketRequiresSiteInRealm` (other realm → 404), `TestWsPatrolRejectsBadTicket` (401, invalid_ticket), `TestWsPatrolDeliversNudge` (issue ticket, connect, `h.publishPatrol(site)`, read message equals the exact JSON), `TestWsPatrolTicketNotReusable`. Follow the harness already used in `app_test.go`.
- [ ] **Step 2:** Run `cd backend && go test ./internal/handler/ -run Ws` — expect FAIL.
- [ ] **Step 3:** `go get github.com/gofiber/contrib/websocket@latest` (pick a version ≥2 weeks old, compatible with fiber v2.52); implement the file above. In `Mount`: `h.live = livehub.New()` and register `app.Get("/api/v1/ws/patrol", h.wsPatrol()...)` **before** the `api` group so the JWT/audit chain is bypassed (auth is the ticket). Add a comment saying so.
- [ ] **Step 4:** In `patrol.go` call `h.publishPatrol(site.ID)` after a successful `startSession` (when `isNew`), `recordScan` (when `isNew`), and `endSession`. Add one handler test asserting a nudge after `recordScan` if the existing test harness supports it; otherwise cover via `TestWsPatrolDeliversNudge`.
- [ ] **Step 5:** Document both endpoints in `docs/api.md` (ticket flow, message shape, single-instance limit, 401/503 codes).
- [ ] **Step 6:** Run `cd backend && go build ./... && go vet ./... && go test ./... && go test -tags integration ./... ` and `govulncheck ./...`; expect PASS.
- [ ] **Step 7:** Commit `feat(backend): websocket live patrol updates (#38)`.

### Task 3: Web hook and Live tab

**Files:**
- Create: `web/src/hooks/usePatrolLive.ts`, `web/src/hooks/usePatrolLive.test.ts`
- Modify: `web/src/pages/patrol/LiveTab.tsx:156-160`, `web/src/demo/mockApi.ts` (demo: ticket request rejects so the hook stays in fallback), `web/src/types/index.ts` only if a type is needed

**Interfaces:**
- Produces: `export function usePatrolLive(siteId: string): { connected: boolean }` and `export const LIVE_POLL_CONNECTED_MS = 60_000`, `LIVE_POLL_FALLBACK_MS = 10_000`, `export function wsUrl(apiBase: string, loc: Location, ticket: string): string` (`wss:`/`ws:` from page protocol; relative base resolved against `loc.host`).

- [ ] **Step 1: Write failing tests** with a fake `WebSocket` class: `wsUrl builds wss from https and relative base`; `invalidates ['patrol-live', siteId] on patrol_changed message`; `connected=false and no throw when ticket request fails`; `reconnects with backoff after close`; `closes socket on unmount`; `ignores messages with another site_id`.
- [ ] **Step 2:** Run `cd web && npx vitest run src/hooks/usePatrolLive.test.ts` — expect FAIL.
- [ ] **Step 3:** Implement: `apiClient.post('/ws/ticket', {site_id})`, open socket, `onmessage` → `queryClient.invalidateQueries({queryKey:['patrol-live', siteId]})`, `onclose` → `connected=false`, retry backoff 1 s doubling to 30 s with jitter. Pause while `document.hidden` is not required.
- [ ] **Step 4:** In `LiveTab.tsx` call `const { connected } = usePatrolLive(site.id)` and set `refetchInterval: connected ? LIVE_POLL_CONNECTED_MS : LIVE_POLL_FALLBACK_MS`.
- [ ] **Step 5:** Run `cd web && npm run lint && npx tsc --noEmit && npm test && npm run build`; expect PASS. Run `npm run e2e` if Chrome is available (CSP `connect-src 'self'` covers same-origin `wss:`; the e2e run must show no CSP violation in demo).
- [ ] **Step 6:** Commit `feat(web): live patrol updates over websocket (#38)`.

### Task 4: Infra, decision, progress

**Files:**
- Modify: `infra/nginx/nginx.conf` (new `location = /api/v1/ws/patrol` before `location /api/`), `DECISIONS.md`, `PROGRESS.md`

- [ ] **Step 1:** Add the nginx location: include `proxy-common.conf` and `headers-api.conf`, `proxy_set_header Host $host; Upgrade $http_upgrade; Connection "upgrade"`, `proxy_read_timeout 120s`, `proxy_pass http://api_backend`. (proxy-common sets `Connection ""`; the later `proxy_set_header Connection` in the location overrides it.) Validate with `docker run --rm -v $PWD/infra/nginx:/etc/nginx/... nginx -t` if Docker is available; otherwise state it was not run.
- [ ] **Step 2:** Append `D-0xx` (next free number) to `DECISIONS.md`: supersedes D-014's "polling first"; nudge-only WebSocket, ticket auth, in-process hub, single-instance limit and the Redis swap point.
- [ ] **Step 3:** `PROGRESS.md`: move the backlog item to Done with follow-ups (multi-instance via Redis pub/sub; worker-side session auto-close does not nudge, covered by the 60 s poll).
- [ ] **Step 4:** Commit `docs: decision and progress for websocket live patrol (#38)`.
