# WebSocket live patrol updates (issue #38, D-014 follow-up)

## Intent
The dashboard patrol Live tab polls `/patrol/live` every 10 s. Push a cheap
"something changed" nudge so it updates right after a tablet scan, while keeping
infra cost near zero (single API container, no new services).

## Goals
- `/api/v1/ws/patrol` WebSocket, authenticated, scoped to one site/realm.
- Broadcast a nudge when a checkpoint scan / patrol start / patrol finish commits.
- Web `usePatrolLive` hook: on nudge, refetch `/patrol/live`.
- Fallback: poll 10 s while disconnected; 60 s safety net while connected.

## Non-goals / anti-goals
- No full-payload events (the REST endpoint stays the single source of truth).
- No Redis pub/sub, no new infrastructure. Not usable across multiple API instances
  (documented; hub is the swap point for Redis later).

## Approaches
1. In-process hub + nudge (chosen): one dependency (gofiber/contrib/websocket), no infra.
2. Redis pub/sub from day one: multi-instance safe, more code/failure modes, not needed now.
3. SSE: no dependency, but departs from the issue/PRD wording.

## Design
- **Ticket auth:** `POST /ws/ticket {site_id}` (Bearer, roles admin/supervisor/hr/client-readonly,
  realm checked via `h.siteFor`) returns a ~30 s single-use in-memory ticket bound to site.
  `GET /ws/patrol?ticket=` upgrades; no JWT in URL.
- **Hub:** `map[siteID]set[conn]`, per-site connection cap, ping/pong, drop slow/dead conns.
  `Publish(siteID)` is non-blocking.
- **Hook points:** after commit of scan, patrol start, patrol finish in the field handlers.
- **Web:** `usePatrolLive(siteId)` with backoff reconnect; `LiveTab` uses it instead of fixed interval.
- **Also:** nginx upgrade headers, CSP `connect-src` `wss:`, `docs/api.md`, new DECISIONS entry
  superseding D-014, demo mock, tests (hub/ticket in Go, hook in vitest).

## Left to the builder
Exact caps/timeouts, message field names, hook placement details.
