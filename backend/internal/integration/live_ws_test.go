//go:build integration

package integration

import (
	"testing"
	"time"
)

// Issue #38: live patrol nudges and the ticket that opens the socket.
func TestLivePatrolTicketAndNudge(t *testing.T) {
	e := newEnv(t)
	admin := e.web(realm3, "live-admin", "admin")
	hr := e.web(realm3, "live-hr", "hr")
	otherRealm := e.web(realm2, "live-other", "admin")
	f := e.newFieldSite(realm3, admin, "Live A")
	now := time.Now().UTC()

	// Ticket: roles and realm are enforced like GET /patrol/live.
	tk := e.expect(e.do("POST", "/api/v1/ws/ticket", admin, "", map[string]any{"site_id": f.site}), 200, "ticket").obj()
	if tk["ticket"] == "" || tk["expires_in"].(float64) != 30 {
		t.Fatalf("ticket = %v", tk)
	}
	if site, ok := e.live.Redeem(tk["ticket"].(string)); !ok || site != f.site {
		t.Fatalf("ticket redeems to %q,%v", site, ok)
	}
	e.expect(e.do("POST", "/api/v1/ws/ticket", hr, "", map[string]any{"site_id": f.site}), 403, "hr ticket")
	e.expect(e.do("POST", "/api/v1/ws/ticket", otherRealm, "", map[string]any{"site_id": f.site}), 404, "other realm ticket")
	e.expect(e.do("POST", "/api/v1/ws/ticket", f.tablet, "", map[string]any{"site_id": f.site}), 403, "tablet ticket")
	e.expect(e.do("POST", "/api/v1/ws/ticket", admin, "", map[string]any{}), 400, "no site")

	// Nudges: session start, new scan, end — and not for an idempotent replay.
	sub, ok := e.live.Subscribe(f.site)
	if !ok {
		t.Fatal("subscribe")
	}
	got := func(what string) {
		t.Helper()
		select {
		case <-sub.C:
		case <-time.After(time.Second):
			t.Fatalf("no nudge after %s", what)
		}
	}
	none := func(what string) {
		t.Helper()
		select {
		case m := <-sub.C:
			t.Fatalf("unexpected nudge after %s: %s", what, m)
		case <-time.After(100 * time.Millisecond):
		}
	}
	route := e.expect(e.do("POST", "/api/v1/patrol/routes", admin, "", map[string]any{
		"site_id": f.site, "name": "Live", "checkpoints": []map[string]any{{"name": "Gate", "qr_code": "LIVE-1", "order_index": 0}},
	}), 201, "route").str("id")
	none("route create")
	sess := e.expect(e.do("POST", "/api/v1/patrol/sessions", f.tablet, f.guard, map[string]any{
		"route_id": route, "started_at": rfc(now.Add(-10 * time.Minute)),
	}), 201, "session").str("id")
	got("session start")
	scan := map[string]any{"id": "7b0f7c1e-5d8a-4e0b-9a55-1d2c3b4a5f60", "qr_code": "LIVE-1", "scanned_at": rfc(now.Add(-5 * time.Minute))}
	e.expect(e.do("POST", "/api/v1/patrol/sessions/"+sess+"/scans", f.tablet, f.guard, scan), 201, "scan")
	got("scan")
	e.expect(e.do("POST", "/api/v1/patrol/sessions/"+sess+"/scans", f.tablet, f.guard, scan), 200, "scan replay")
	none("scan replay")
	e.expect(e.do("POST", "/api/v1/patrol/sessions/"+sess+"/end", f.tablet, f.guard, nil), 200, "end")
	got("session end")
}
