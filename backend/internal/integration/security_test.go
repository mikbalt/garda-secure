//go:build integration

package integration

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"

	"github.com/securepatrol/backend/internal/config"
)

// fieldSite is a site with two guards and an approved tablet.
type fieldSite struct {
	site, guard, guard2, tablet string
}

func (e *env) newFieldSite(realm, admin, name string) fieldSite {
	e.t.Helper()
	f := fieldSite{}
	f.site = e.expect(e.do("POST", "/api/v1/sites", admin, "", map[string]any{"name": name}), 201, "site "+name).str("id")
	f.guard = e.expect(e.do("POST", "/api/v1/guards", admin, "", map[string]any{"name": name + " G1", "site_id": f.site}), 201, "guard").str("id")
	f.guard2 = e.expect(e.do("POST", "/api/v1/guards", admin, "", map[string]any{"name": name + " G2", "site_id": f.site}), 201, "guard 2").str("id")
	f.tablet = e.token(realm, "tablet-"+uuid.NewString(), "securepatrol-tablet", "device")
	dev := e.expect(e.do("POST", "/api/v1/device/register", f.tablet, "", map[string]any{"name": name}), 200, "register").str("id")
	e.expect(e.do("PUT", "/api/v1/devices/"+dev, admin, "", map[string]any{"status": "active", "site_id": f.site}), 200, "approve")
	return f
}

// route creates a one-checkpoint route and starts a session for guard.
func (e *env) session(admin string, f fieldSite, qr string) string {
	e.t.Helper()
	route := e.expect(e.do("POST", "/api/v1/patrol/routes", admin, "", map[string]any{
		"site_id": f.site, "name": "R " + qr, "checkpoints": []map[string]any{{"name": "Gate", "qr_code": qr, "order_index": 0}},
	}), 201, "route").str("id")
	return e.expect(e.do("POST", "/api/v1/patrol/sessions", f.tablet, f.guard, map[string]any{"route_id": route}), 201, "session").str("id")
}

// BE-S2: a client-chosen ID that already belongs to another site's, realm's
// or guard's record must not return that record.
func TestIdempotentReplayIsScopedToOwner(t *testing.T) {
	e := newEnv(t)
	adminA := e.web(realm3, "replay-admin-a", "admin")
	adminB := e.web(realm4, "replay-admin-b", "admin")
	a := e.newFieldSite(realm3, adminA, "Replay A")
	a2 := e.newFieldSite(realm3, adminA, "Replay A2") // same realm, other site
	b := e.newFieldSite(realm4, adminB, "Replay B")   // other realm
	now := time.Now().UTC()
	e.expect(e.do("POST", "/api/v1/shifts", adminA, "", map[string]any{
		"site_id": a.site, "guard_id": a.guard, "start_time": rfc(now.Add(-time.Minute)), "end_time": rfc(now.Add(8 * time.Hour)),
	}), 201, "shift")

	type attempt struct {
		who           string
		tablet, guard string
	}
	others := []attempt{
		{"other site, same realm", a2.tablet, a2.guard},
		{"other realm", b.tablet, b.guard},
		{"same site, other guard", a.tablet, a.guard2},
	}
	checkConflict := func(what string, r resp) {
		t.Helper()
		if r.status != 409 || r.str("code") != "id_conflict" {
			t.Fatalf("%s: %d %s, want 409 id_conflict", what, r.status, r.raw)
		}
		if strings.Contains(string(r.raw), a.site) || strings.Contains(string(r.raw), "Andi") {
			t.Fatalf("%s: conflict response leaks the original record: %s", what, r.raw)
		}
	}

	creates := []struct {
		name, path string
		body       map[string]any
	}{
		{"incident", "/api/v1/incidents", map[string]any{"type": "theft", "description": "Andi took it"}},
		{"sos", "/api/v1/sos", map[string]any{}},
		{"defect", "/api/v1/defects", map[string]any{"defect_type": "CCTV", "description": "Andi"}},
		{"visitor", "/api/v1/visitors", map[string]any{"visitor_name": "Andi"}},
		{"clock-in", "/api/v1/attendance/clock-in", map[string]any{}},
	}
	for _, c := range creates {
		id := uuid.NewString()
		c.body["id"] = id
		e.expect(e.do("POST", c.path, a.tablet, a.guard, c.body), 201, c.name+" create")
		e.expect(e.do("POST", c.path, a.tablet, a.guard, c.body), 200, c.name+" retry by owner")
		for _, o := range others {
			checkConflict(c.name+" replay from "+o.who, e.do("POST", c.path, o.tablet, o.guard, c.body))
		}
	}

	// Scans: the ID of a scan in site A's session replayed into other sessions.
	sessA := e.session(adminA, a, "QR-RA")
	sessA2 := e.session(adminA, a2, "QR-RA2")
	sessB := e.session(adminB, b, "QR-RB")
	scanID := uuid.NewString()
	e.expect(e.do("POST", "/api/v1/patrol/sessions/"+sessA+"/scans", a.tablet, a.guard, map[string]any{"id": scanID, "qr_code": "QR-RA"}), 201, "scan")
	e.expect(e.do("POST", "/api/v1/patrol/sessions/"+sessA+"/scans", a.tablet, a.guard, map[string]any{"id": scanID, "qr_code": "QR-RA"}), 200, "scan retry")
	checkConflict("scan replay other site", e.do("POST", "/api/v1/patrol/sessions/"+sessA2+"/scans", a2.tablet, a2.guard, map[string]any{"id": scanID, "qr_code": "QR-RA2"}))
	checkConflict("scan replay other realm", e.do("POST", "/api/v1/patrol/sessions/"+sessB+"/scans", b.tablet, b.guard, map[string]any{"id": scanID, "qr_code": "QR-RB"}))

	// Sessions: the ID of site A's session started again elsewhere.
	routeB := e.expect(e.do("GET", "/api/v1/device/routes", b.tablet, "", nil), 200, "routes B").list()[0].(map[string]any)["id"]
	checkConflict("session replay other realm", e.do("POST", "/api/v1/patrol/sessions", b.tablet, b.guard, map[string]any{"id": sessA, "route_id": routeB}))
}

// BE-S5, BE-S6, BE-S8, BE-S10: limits, token checks, audit log, employee IDs.
func TestSecurityLimitsAndAudit(t *testing.T) {
	e := newEnv(t, func(c *config.Config) {
		c.TrustedProxies = []string{"0.0.0.0"} // app.Test connections come from 0.0.0.0
		c.RateLimitSOS = 2
		c.RateLimitAuthFailures = 3
		c.RateLimitUserWrites = 1000
		c.RateLimitDeviceWrites = 1000
	})
	ctx := context.Background()
	admin := e.web(realm3, "sec-admin", "admin")
	sup := e.web(realm3, "sec-sup", "supervisor")
	otherAdmin := e.web(realm4, "sec-admin-4", "admin")
	a := e.newFieldSite(realm3, admin, "Sec A")
	a2 := e.newFieldSite(realm3, admin, "Sec A2")
	b := e.newFieldSite(realm4, otherAdmin, "Sec B")

	// ── JWT: only access tokens ──────────────────────────────────────────────
	idTok := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims{
		"iss": issuer + realm3, "sub": "sec-admin", "azp": "securepatrol-web", "typ": "ID",
		"exp": time.Now().Add(time.Hour).Unix(), "roles": []string{"admin"},
	})
	idTok.Header["kid"] = "k1"
	raw, _ := idTok.SignedString(e.key)
	e.expect(e.do("GET", "/api/v1/sites", raw, "", nil), 401, "ID token used as bearer")
	e.expect(e.do("GET", "/api/v1/sites", e.token(realm3, "sec-admin", "securepatrol-tablet", "admin"), "", nil), 403, "web role via tablet client")

	// ── Failed authentication is throttled per client IP ─────────────────────
	attacker := map[string]string{"X-Real-IP": "198.51.100.7"}
	for i := 0; i < 3; i++ {
		e.expect(e.doH("GET", "/api/v1/sites", "garbage", "", nil, attacker), 401, "bad token")
	}
	if r := e.doH("GET", "/api/v1/sites", admin, "", nil, attacker); r.status != 429 || r.str("code") != "rate_limited" {
		t.Fatalf("throttled IP: %d %s", r.status, r.raw)
	}
	e.expect(e.doH("GET", "/api/v1/sites", admin, "", nil, map[string]string{"X-Real-IP": "198.51.100.8"}), 200, "other IP unaffected")

	// ── Body and field limits ────────────────────────────────────────────────
	if r := e.do("POST", "/api/v1/incidents", a.tablet, a.guard, map[string]any{"type": "x", "description": strings.Repeat("a", 70<<10)}); r.status != 413 || r.str("code") != "body_too_large" {
		t.Fatalf("70 KB JSON: %d %s", r.status, r.raw)
	}
	if r := e.do("POST", "/api/v1/incidents", a.tablet, a.guard, map[string]any{"type": "x", "description": strings.Repeat("é", 4001)}); r.status != 400 || r.str("code") != "field_too_long" || r.str("field") != "description" {
		t.Fatalf("long description: %d %s", r.status, r.raw)
	}
	if r := e.do("POST", "/api/v1/visitors", a.tablet, a.guard, map[string]any{"visitor_name": "Andi", "id_number": strings.Repeat("9", 65)}); r.status != 400 || r.str("field") != "id_number" {
		t.Fatalf("long id_number: %d %s", r.status, r.raw)
	}
	e.expect(e.do("POST", "/api/v1/incidents", a.tablet, a.guard, map[string]any{"type": "x", "description": strings.Repeat("é", 4000)}), 201, "description at the limit")

	// ── SOS: new alerts limited per tablet, retries never ───────────────────
	first := uuid.NewString()
	e.expect(e.do("POST", "/api/v1/sos", a.tablet, a.guard, map[string]any{"id": first}), 201, "sos 1")
	e.expect(e.do("POST", "/api/v1/sos", a.tablet, a.guard, map[string]any{"id": uuid.NewString()}), 201, "sos 2")
	r := e.do("POST", "/api/v1/sos", a.tablet, a.guard, map[string]any{"id": uuid.NewString()})
	if r.status != 429 || r.str("code") != "rate_limited" {
		t.Fatalf("third new SOS within a minute: %d %s", r.status, r.raw)
	}
	e.expect(e.do("POST", "/api/v1/sos", a.tablet, a.guard, map[string]any{"id": first}), 200, "SOS retry is never throttled")
	e.expect(e.do("POST", "/api/v1/sos", a2.tablet, a2.guard, map[string]any{}), 201, "other tablet has its own SOS budget")

	// ── Employee IDs unique per client realm, not install-wide ──────────────
	g := e.expect(e.do("POST", "/api/v1/guards", admin, "", map[string]any{"name": "E1", "site_id": a.site, "employee_id": "EMP-1"}), 201, "employee id").str("id")
	if r := e.do("POST", "/api/v1/guards", admin, "", map[string]any{"name": "E1b", "site_id": a2.site, "employee_id": "EMP-1"}); r.status != 409 || r.str("code") != "employee_id_taken" {
		t.Fatalf("duplicate employee id in realm: %d %s", r.status, r.raw)
	}
	e.expect(e.do("POST", "/api/v1/guards", otherAdmin, "", map[string]any{"name": "E1", "site_id": b.site, "employee_id": "EMP-1"}), 201, "same employee id in another realm")
	e.expect(e.do("PUT", "/api/v1/guards/"+g, admin, "", map[string]any{"name": "E1 renamed", "site_id": a.site, "employee_id": "EMP-1"}), 200, "keep own employee id")

	// ── Audit log ────────────────────────────────────────────────────────────
	e.expect(e.doH("PUT", "/api/v1/guards/"+g+"/pin", admin, "", map[string]any{"pin": "4826"},
		map[string]string{"X-Real-IP": "203.0.113.9", "X-Forwarded-For": "1.2.3.4"}), 200, "set pin")
	now := time.Now().UTC()
	shift := e.expect(e.do("POST", "/api/v1/shifts", admin, "", map[string]any{
		"site_id": a.site, "guard_id": a.guard2, "start_time": rfc(now.Add(-2 * time.Hour)), "end_time": rfc(now.Add(time.Hour)),
	}), 201, "shift").str("id")
	e.expect(e.do("PUT", "/api/v1/attendance/shifts/"+shift+"/override", admin, "", map[string]any{
		"status": "present", "reason": "Badge reader broken",
	}), 200, "override")
	e.expect(e.do("PUT", "/api/v1/attendance/shifts/"+shift+"/override", admin, "", map[string]any{
		"status": "late", "reason": "Checked CCTV",
	}), 200, "second override")

	e.expect(e.do("GET", "/api/v1/audit", sup, "", nil), 403, "supervisor reads audit")
	pin := e.expect(e.do("GET", "/api/v1/audit?resource_type=guards&resource_id="+g, admin, "", nil), 200, "audit by resource").obj()
	rows := pin["rows"].([]any)
	if len(rows) < 2 {
		t.Fatalf("guard audit rows = %d", len(rows))
	}
	pinRow := rows[0].(map[string]any)
	if pinRow["route"] != "/api/v1/guards/:id/pin" || pinRow["realm"] != realm3 || pinRow["actor"] != "sec-admin" ||
		pinRow["ip"] != "203.0.113.9" || pinRow["status_code"].(float64) != 200 {
		t.Fatalf("pin audit row = %v", pinRow)
	}
	if s, _ := pinRow["summary"].(map[string]any); s == nil || s["pin"] != "[redacted]" {
		t.Fatalf("pin not redacted in summary: %v", pinRow["summary"])
	}
	ov := e.expect(e.do("GET", "/api/v1/audit?resource_type=attendance&limit=1", admin, "", nil), 200, "audit override").obj()
	ovRow := ov["rows"].([]any)[0].(map[string]any)
	if ov["has_more"] != true || ovRow["params"].(map[string]any)["shiftId"] != shift || ovRow["resource_id"] != shift {
		t.Fatalf("override audit row = %v (has_more %v)", ovRow, ov["has_more"])
	}
	before, _ := ovRow["before"].(map[string]any)
	after, _ := ovRow["after"].(map[string]any)
	if before == nil || before["status"] != "present" || after == nil || after["status"] != "late" {
		t.Fatalf("override before/after = %v / %v", ovRow["before"], ovRow["after"])
	}
	created := e.expect(e.do("GET", "/api/v1/audit?resource_type=sites&actor=sec-admin", admin, "", nil), 200, "audit sites").obj()["rows"].([]any)
	if len(created) == 0 || created[0].(map[string]any)["resource_id"] == nil {
		t.Fatalf("site creation not audited with its new id: %v", created)
	}
	if l := e.expect(e.do("GET", "/api/v1/audit?actor=sec-admin", otherAdmin, "", nil), 200, "other realm audit").obj()["rows"].([]any); len(l) != 0 {
		t.Fatalf("other realm sees %d audit rows", len(l))
	}
	if r := e.do("GET", "/api/v1/audit?from=yesterday", admin, "", nil); r.status != 400 {
		t.Fatalf("bad from: %d", r.status)
	}

	// The audit log is append-only for the application.
	if _, err := e.pool.Exec(ctx, `UPDATE audit_log SET action = 'x'`); err == nil {
		t.Fatal("audit_log UPDATE allowed")
	}
	if _, err := e.pool.Exec(ctx, `DELETE FROM audit_log`); err == nil {
		t.Fatal("audit_log DELETE allowed")
	}
}
