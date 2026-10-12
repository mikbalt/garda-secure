//go:build integration

package integration

import (
	"bytes"
	"context"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/securepatrol/backend/internal/config"
	"github.com/securepatrol/backend/internal/handler"
	"github.com/securepatrol/backend/internal/middleware"
)

// noToken makes doH send the request without the stored guard token.
var noToken = map[string]string{middleware.GuardTokenHeader: ""}

func (e *env) deviceOf(site string) string {
	e.t.Helper()
	var id string
	if err := e.pool.QueryRow(context.Background(), `SELECT id::text FROM devices WHERE site_id = $1`, site).Scan(&id); err != nil {
		e.t.Fatal(err)
	}
	return id
}

// D-035: PIN verified by the server → guard token; lockout; audit.
func TestGuardSessions(t *testing.T) {
	e := newEnv(t)
	admin := e.web(realm3, "gs-admin", "admin")
	f := e.newFieldSite(realm3, admin, "GS A")
	other := e.newFieldSite(realm3, admin, "GS B")

	// Happy path (newFieldSite already logged both guards in; do it again).
	tok := e.loginGuard(f.tablet, f.guard, "4826")
	if strings.Count(tok, ".") != 2 {
		t.Fatalf("token is not a compact JWS: %q", tok)
	}

	// Wrong PINs: 401 with the attempts left, the 5th locks for 15 min.
	for left := 4; left >= 1; left-- {
		r := e.do("POST", "/api/v1/device/guard-sessions", f.tablet, "", map[string]any{"guard_id": f.guard2, "pin": "0000"})
		if r.status != 401 || r.str("code") != "invalid_pin" || r.obj()["attempts_left"].(float64) != float64(left) {
			t.Fatalf("wrong PIN (%d left): %d %s", left, r.status, r.raw)
		}
	}
	r := e.do("POST", "/api/v1/device/guard-sessions", f.tablet, "", map[string]any{"guard_id": f.guard2, "pin": "0000"})
	if r.status != 423 || r.str("code") != "pin_locked" || r.obj()["retry_after"].(float64) < 890 {
		t.Fatalf("5th wrong PIN: %d %s", r.status, r.raw)
	}
	// Locked: even the right PIN is refused, on any tablet of the site.
	if r := e.do("POST", "/api/v1/device/guard-sessions", f.tablet, "", map[string]any{"guard_id": f.guard2, "pin": "5937"}); r.status != 423 {
		t.Fatalf("right PIN while locked: %d %s", r.status, r.raw)
	}
	if n := e.count(`SELECT COUNT(*) FROM guards WHERE id = $1 AND pin_locked_until > now() + interval '14 minutes'`, f.guard2); n != 1 {
		t.Fatal("lock not persisted")
	}
	if n := e.count(`SELECT COUNT(*) FROM audit_log WHERE route = '/api/v1/device/guard-sessions' AND guard_id = $1 AND status_code = 401`, f.guard2); n != 4 {
		t.Fatalf("audited PIN failures = %d, want 4", n)
	}
	if n := e.count(`SELECT COUNT(*) FROM audit_log WHERE route = '/api/v1/device/guard-sessions' AND guard_id = $1 AND status_code = 423`, f.guard2); n != 2 {
		t.Fatalf("audited lock responses = %d, want 2", n)
	}
	if n := e.count(`SELECT COUNT(*) FROM audit_log WHERE route = '/api/v1/device/guard-sessions' AND summary::text LIKE '%5937%'`); n != 0 {
		t.Fatal("PIN leaked into the audit log")
	}
	g := e.expect(e.do("GET", "/api/v1/guards/"+f.guard2, admin, "", nil), 200, "guard").obj()
	if g["pin_locked_until"] == nil {
		t.Fatalf("admin does not see the lock: %v", g)
	}
	// An admin setting a new PIN clears the lock.
	e.setPinAndLogin(admin, f.tablet, f.guard2, "7159")
	// A success resets the counter: 4 more failures do not lock.
	for i := 0; i < 4; i++ {
		e.expect(e.do("POST", "/api/v1/device/guard-sessions", f.tablet, "", map[string]any{"guard_id": f.guard2, "pin": "1111"}), 401, "wrong PIN after reset")
	}
	e.loginGuard(f.tablet, f.guard2, "7159")
	if n := e.count(`SELECT COUNT(*) FROM guards WHERE id = $1 AND pin_failed_attempts = 0 AND pin_locked_until IS NULL`, f.guard2); n != 1 {
		t.Fatal("counter not reset on success")
	}

	// Denials.
	noPin := e.expect(e.do("POST", "/api/v1/guards", admin, "", map[string]any{"name": "No PIN", "site_id": f.site}), 201, "guard").str("id")
	if r := e.do("POST", "/api/v1/device/guard-sessions", f.tablet, "", map[string]any{"guard_id": noPin, "pin": "1234"}); r.status != 409 || r.str("code") != "pin_not_set" {
		t.Fatalf("no PIN: %d %s", r.status, r.raw)
	}
	for name, gid := range map[string]string{"other site": other.guard, "unknown": uuid.NewString(), "malformed": "x"} {
		if r := e.do("POST", "/api/v1/device/guard-sessions", f.tablet, "", map[string]any{"guard_id": gid, "pin": "4826"}); r.status != 403 || r.str("code") != "guard_not_allowed" {
			t.Fatalf("%s guard: %d %s", name, r.status, r.raw)
		}
	}
	e.expect(e.do("PUT", "/api/v1/guards/"+f.guard, admin, "", map[string]any{"name": "GS A G1", "site_id": f.site, "status": "inactive"}), 200, "deactivate")
	if r := e.do("POST", "/api/v1/device/guard-sessions", f.tablet, "", map[string]any{"guard_id": f.guard, "pin": "4826"}); r.status != 403 || r.str("code") != "guard_not_allowed" {
		t.Fatalf("inactive guard: %d %s", r.status, r.raw)
	}
	e.expect(e.do("POST", "/api/v1/device/guard-sessions", admin, "", map[string]any{"guard_id": f.guard2, "pin": "7159"}), 403, "web user")
	e.expect(e.do("POST", "/api/v1/device/guard-sessions", f.tablet, "", map[string]any{"guard_id": f.guard2}), 400, "missing pin")
}

// Rate limit on PIN checks per tablet.
func TestGuardSessionRateLimit(t *testing.T) {
	e := newEnv(t, func(c *config.Config) { c.RateLimitGuardSessions = 2 })
	admin := e.web(realm4, "gsr-admin", "admin")
	f := e.newFieldSite(realm4, admin, "GSR") // two logins: the budget is used up
	if r := e.do("POST", "/api/v1/device/guard-sessions", f.tablet, "", map[string]any{"guard_id": f.guard, "pin": "4826"}); r.status != 429 || r.str("code") != "rate_limited" {
		t.Fatalf("third PIN check within a minute: %d %s", r.status, r.raw)
	}
}

// D-035: records without a valid guard token are flagged guard_unverified;
// bad tokens are rejected (except SOS); expired tokens prove queued records.
func TestGuardVerificationFlags(t *testing.T) {
	e := newEnv(t)
	admin := e.web(realm3, "gv-admin", "admin")
	sup := e.web(realm3, "gv-sup", "supervisor")
	f := e.newFieldSite(realm3, admin, "GV A")
	b := e.newFieldSite(realm3, admin, "GV B")
	now := time.Now().UTC()
	for _, g := range []string{f.guard, f.guard2} {
		e.expect(e.do("POST", "/api/v1/shifts", admin, "", map[string]any{
			"site_id": f.site, "guard_id": g, "start_time": rfc(now.Add(-2 * time.Minute)), "end_time": rfc(now.Add(8 * time.Hour)),
		}), 201, "shift")
	}
	hasFlag := func(r resp, flag string) bool { return slices.Contains(flagsOf(r.obj()["flags"]), flag) }

	// Verified guard: no flag, no review.
	in := e.expect(e.do("POST", "/api/v1/attendance/clock-in", f.tablet, f.guard, map[string]any{}), 201, "verified clock-in")
	if hasFlag(in, "guard_unverified") || in.obj()["needs_review"] != false {
		t.Fatalf("verified clock-in = %s", in.raw)
	}
	st := e.expect(e.do("GET", "/api/v1/device/status", f.tablet, f.guard, nil), 200, "status").obj()
	if st["guard_verified"] != true {
		t.Fatalf("status guard_verified = %v", st["guard_verified"])
	}

	// Unverified guard (offline PIN): every record is flagged, the clock-in
	// goes to the review queue.
	un := e.expect(e.doH("POST", "/api/v1/attendance/clock-in", f.tablet, f.guard2, map[string]any{}, noToken), 201, "unverified clock-in")
	if !hasFlag(un, "guard_unverified") || un.obj()["needs_review"] != true {
		t.Fatalf("unverified clock-in = %s", un.raw)
	}
	if q := e.do("GET", "/api/v1/attendance/review?site_id="+f.site, sup, "", nil).list(); len(q) != 1 {
		t.Fatalf("review queue = %d, want 1", len(q))
	}
	out := e.expect(e.doH("POST", "/api/v1/attendance/clock-out", f.tablet, f.guard, map[string]any{}, noToken), 200, "unverified clock-out")
	if !hasFlag(out, "guard_unverified") {
		t.Fatalf("unverified clock-out = %s", out.raw)
	}
	st = e.expect(e.doH("GET", "/api/v1/device/status", f.tablet, f.guard2, nil, noToken), 200, "status").obj()
	if st["guard_verified"] != false {
		t.Fatalf("status guard_verified without token = %v", st["guard_verified"])
	}
	creates := map[string]struct {
		path string
		body map[string]any
	}{
		"incident": {"/api/v1/incidents", map[string]any{"type": "theft"}},
		"sos":      {"/api/v1/sos", map[string]any{}},
		"defect":   {"/api/v1/defects", map[string]any{"defect_type": "CCTV"}},
		"visitor":  {"/api/v1/visitors", map[string]any{"visitor_name": "Andi"}},
	}
	for name, c := range creates {
		if r := e.expect(e.doH("POST", c.path, f.tablet, f.guard2, c.body, noToken), 201, name); !hasFlag(r, "guard_unverified") {
			t.Fatalf("unverified %s = %s", name, r.raw)
		}
		if r := e.expect(e.do("POST", c.path, f.tablet, f.guard2, c.body), 201, name+" verified"); hasFlag(r, "guard_unverified") {
			t.Fatalf("verified %s flagged: %s", name, r.raw)
		}
	}
	route := e.expect(e.do("POST", "/api/v1/patrol/routes", admin, "", map[string]any{
		"site_id": f.site, "name": "GV", "checkpoints": []map[string]any{{"name": "Gate", "order_index": 0}},
	}), 201, "route").str("id")
	sess := e.expect(e.doH("POST", "/api/v1/patrol/sessions", f.tablet, f.guard2, map[string]any{"route_id": route}, noToken), 201, "unverified session")
	if !hasFlag(sess, "guard_unverified") {
		t.Fatalf("unverified session = %s", sess.raw)
	}
	sc := e.expect(e.doH("POST", "/api/v1/patrol/sessions/"+sess.str("id")+"/scans", f.tablet, f.guard2,
		map[string]any{"checkpoint_id": e.do("GET", "/api/v1/device/routes", f.tablet, "", nil).list()[0].(map[string]any)["checkpoints"].([]any)[0].(map[string]any)["id"], "method": "manual",
			"photo_object_key": "sites/" + f.site + "/checkpoint/" + uuid.NewString() + ".jpg"}, noToken), 201, "unverified scan")
	if !hasFlag(sc, "guard_unverified") {
		t.Fatalf("unverified scan = %s", sc.raw)
	}
	rows := e.expect(e.do("GET", "/api/v1/reports/patrol/sessions?site_id="+f.site, sup, "", nil), 200, "sessions").obj()["rows"].([]any)
	if !slices.Contains(flagsOf(rows[0].(map[string]any)["flags"]), "guard_unverified") {
		t.Fatalf("session report lacks flags: %v", rows[0])
	}

	// Invalid tokens: 403 guard_token_invalid — tampered, another guard's,
	// another tablet's — the tablet resends without it.
	tok := e.guardTokens[f.tablet+"|"+f.guard2]
	otherGuardTok := e.guardTokens[f.tablet+"|"+f.guard]
	otherTabletTok := e.guardTokens[b.tablet+"|"+b.guard2]
	for name, bad := range map[string]string{"tampered": tok[:len(tok)-2] + "xx", "other guard": otherGuardTok, "other tablet": otherTabletTok, "garbage": "abc"} {
		r := e.doH("POST", "/api/v1/incidents", f.tablet, f.guard2, map[string]any{"type": "x"}, map[string]string{middleware.GuardTokenHeader: bad})
		if r.status != 403 || r.str("code") != "guard_token_invalid" {
			t.Fatalf("%s token: %d %s", name, r.status, r.raw)
		}
	}
	// SOS is never rejected for a bad token: it is just unverified.
	sos := e.expect(e.doH("POST", "/api/v1/sos", f.tablet, f.guard2, map[string]any{}, map[string]string{middleware.GuardTokenHeader: "abc"}), 201, "sos with bad token")
	if !hasFlag(sos, "guard_unverified") {
		t.Fatalf("sos with bad token = %s", sos.raw)
	}

	// Expired token (within grace): proves records queued while it was
	// valid, not records made after it expired.
	tokens, _ := handler.FieldAuth(e.cfg)
	old, exp, err := tokens.Issue(f.guard2, e.deviceOf(f.site), f.site, now.Add(-13*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	hdr := map[string]string{middleware.GuardTokenHeader: old}
	queued := e.expect(e.doH("POST", "/api/v1/incidents", f.tablet, f.guard2, map[string]any{"type": "x", "occurred_at": rfc(exp.Add(-time.Hour))}, hdr), 201, "queued incident")
	if hasFlag(queued, "guard_unverified") {
		t.Fatalf("incident made while the token was valid flagged: %s", queued.raw)
	}
	late := e.expect(e.doH("POST", "/api/v1/incidents", f.tablet, f.guard2, map[string]any{"type": "x"}, hdr), 201, "incident after expiry")
	if !hasFlag(late, "guard_unverified") {
		t.Fatalf("incident after token expiry not flagged: %s", late.raw)
	}
	// Beyond the grace period the token proves nothing (but is not rejected).
	ancient, _, _ := tokens.Issue(f.guard2, e.deviceOf(f.site), f.site, now.Add(-8*24*time.Hour-13*time.Hour))
	r := e.doH("POST", "/api/v1/incidents", f.tablet, f.guard2, map[string]any{"type": "x", "occurred_at": rfc(now.Add(-6 * 24 * time.Hour))},
		map[string]string{middleware.GuardTokenHeader: ancient})
	if r.status != 201 || !hasFlag(r, "guard_unverified") {
		t.Fatalf("token beyond grace: %d %s", r.status, r.raw)
	}
}

// D-035: signed checkpoint QR codes.
func TestSignedQRCodes(t *testing.T) {
	e := newEnv(t)
	admin := e.web(realm4, "qr-admin", "admin")
	sup := e.web(realm4, "qr-sup", "supervisor")
	f := e.newFieldSite(realm4, admin, "QR A")
	route := e.expect(e.do("POST", "/api/v1/patrol/routes", admin, "", map[string]any{
		"site_id": f.site, "name": "Signed", "checkpoints": []map[string]any{
			{"name": "Gate", "order_index": 0},
			{"name": "Dock", "qr_code": "LEGACY-DOCK", "order_index": 1},
			{"name": "Roof", "order_index": 2},
		},
	}), 201, "route").obj()
	routeID := route["id"].(string)
	cps := route["checkpoints"].([]any)
	gate, dock := cps[0].(map[string]any), cps[1].(map[string]any)
	gatePayload := gate["qr_code"].(string)
	if gate["qr_signed"] != true || !strings.HasPrefix(gatePayload, "SP1:"+gate["id"].(string)+":") || dock["qr_signed"] != false {
		t.Fatalf("route checkpoints = %v", cps)
	}
	// Web read (QR print sheet) shows the payload; the tablet never does.
	if got := e.do("GET", "/api/v1/patrol/routes/"+routeID, sup, "", nil).obj()["checkpoints"].([]any)[0].(map[string]any)["qr_code"]; got != gatePayload {
		t.Fatalf("web route qr_code = %v", got)
	}
	dr := e.expect(e.do("GET", "/api/v1/device/routes", f.tablet, "", nil), 200, "device routes").list()[0].(map[string]any)["checkpoints"].([]any)
	if c := dr[0].(map[string]any); c["qr_code"] != nil || c["qr_signed"] != true {
		t.Fatalf("device route signed checkpoint = %v", c)
	}
	if c := dr[1].(map[string]any); c["qr_code"] != "LEGACY-DOCK" || c["qr_signed"] != false {
		t.Fatalf("device route legacy checkpoint = %v", c)
	}
	dev := e.do("GET", "/api/v1/device/routes", f.tablet, "", nil)
	if bytes.Contains(dev.raw, []byte(gatePayload)) {
		t.Fatal("device routes leak a signed payload")
	}

	sess := e.expect(e.do("POST", "/api/v1/patrol/sessions", f.tablet, f.guard, map[string]any{"route_id": routeID}), 201, "session").str("id")
	scan := func(body map[string]any) resp {
		return e.do("POST", "/api/v1/patrol/sessions/"+sess+"/scans", f.tablet, f.guard, body)
	}
	ok := e.expect(scan(map[string]any{"qr_payload": gatePayload, "method": "qr"}), 201, "signed scan").obj()
	if fl := flagsOf(ok["flags"]); len(fl) != 0 || ok["checkpoint_id"] != gate["id"] {
		t.Fatalf("valid signed scan = %v", ok)
	}
	if fl := flagsOf(e.expect(scan(map[string]any{"qr_code": "LEGACY-DOCK"}), 201, "legacy scan").obj()["flags"]); !slices.Equal(fl, []string{"qr_unsigned"}) {
		t.Fatalf("legacy scan flags = %v", fl)
	}
	roofID := cps[2].(map[string]any)["id"].(string)
	forged := e.expect(scan(map[string]any{"qr_payload": "SP1:" + roofID + ":AAAAAAAAAAAAAAAAAAAAAA", "method": "qr"}), 201, "forged scan").obj()
	if fl := flagsOf(forged["flags"]); !slices.Equal(fl, []string{"qr_invalid"}) {
		t.Fatalf("forged scan flags = %v", fl)
	}
	if r := scan(map[string]any{"qr_payload": "SP1:" + uuid.NewString() + ":AAAAAAAAAAAAAAAAAAAAAA"}); r.status != 400 || r.str("code") != "unknown_checkpoint" {
		t.Fatalf("payload of a checkpoint not on the route: %d %s", r.status, r.raw)
	}
	// Session progress to the tablet hides payloads; to the web it shows them.
	prog := e.expect(e.do("GET", "/api/v1/patrol/sessions/"+sess, f.tablet, f.guard, nil), 200, "progress tablet")
	if bytes.Contains(prog.raw, []byte(gatePayload)) {
		t.Fatal("tablet progress leaks the signed payload")
	}
	if p := e.expect(e.do("GET", "/api/v1/patrol/sessions/"+sess, sup, "", nil), 200, "progress web"); !bytes.Contains(p.raw, []byte(gatePayload)) {
		t.Fatal("web progress lacks the payload")
	}
	// The flagged scan shows up for review; a lone qr_unsigned does not count.
	sums := e.expect(e.do("GET", "/api/v1/patrol/sessions?site_id="+f.site, sup, "", nil), 200, "sessions").list()
	if s := sums[0].(map[string]any); s["flagged_scans"].(float64) != 1 {
		t.Fatalf("flagged_scans = %v", s["flagged_scans"])
	}

	// Editing the route keeps signed codes stable (even if the client sends
	// something else) and legacy ones as they are.
	gate["qr_code"] = "HACK"
	upd := e.expect(e.do("PUT", "/api/v1/patrol/routes/"+routeID, admin, "", map[string]any{"name": "Signed", "checkpoints": cps}), 200, "update route").obj()
	if c := upd["checkpoints"].([]any)[0].(map[string]any); c["qr_code"] != gatePayload || c["qr_version"].(float64) != 1 {
		t.Fatalf("signed code changed on edit: %v", c)
	}

	// Regenerate: every code signed with a new version; old prints fail.
	e.expect(e.do("POST", "/api/v1/patrol/routes/"+routeID+"/regenerate-qr", sup, "", nil), 403, "supervisor regenerates")
	reg := e.expect(e.do("POST", "/api/v1/patrol/routes/"+routeID+"/regenerate-qr", admin, "", nil), 200, "regenerate").obj()
	rcps := reg["checkpoints"].([]any)
	newGate, newDock := rcps[0].(map[string]any), rcps[1].(map[string]any)
	if newGate["qr_code"] == gatePayload || newGate["qr_version"].(float64) != 2 || newDock["qr_signed"] != true {
		t.Fatalf("regenerated = %v", rcps)
	}
	if fl := flagsOf(e.expect(scan(map[string]any{"qr_payload": gatePayload, "method": "qr"}), 201, "old print").obj()["flags"]); !slices.Equal(fl, []string{"qr_invalid"}) {
		t.Fatalf("old printed code flags = %v", fl)
	}
	if r := scan(map[string]any{"qr_code": "LEGACY-DOCK"}); r.status != 400 {
		t.Fatalf("legacy code after regenerate: %d %s", r.status, r.raw)
	}
	if fl := flagsOf(e.expect(scan(map[string]any{"qr_payload": newDock["qr_code"], "method": "qr"}), 201, "new dock").obj()["flags"]); len(fl) != 0 {
		t.Fatalf("new signed code flags = %v", fl)
	}
	if n := e.count(`SELECT COUNT(*) FROM audit_log WHERE route = '/api/v1/patrol/routes/:id/regenerate-qr' AND status_code = 200 AND before_state IS NOT NULL`); n != 1 {
		t.Fatalf("regenerate audit rows = %d", n)
	}
}

// D-035: patrol schedules, "patrol not started" alerts and compliance.
func TestPatrolSchedules(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	admin := e.web(realm3, "ps-admin", "admin")
	sup := e.web(realm3, "ps-sup", "supervisor")
	client := e.web(realm3, "ps-client", "client-readonly")
	f := e.newFieldSite(realm3, admin, "PS A")
	mkRoute := func(name string) string {
		return e.expect(e.do("POST", "/api/v1/patrol/routes", admin, "", map[string]any{
			"site_id": f.site, "name": name, "checkpoints": []map[string]any{{"name": "Gate", "order_index": 0}},
		}), 201, "route").str("id")
	}
	route, route2, route3 := mkRoute("Ronda"), mkRoute("Ronda 2"), mkRoute("Ronda 3")
	e.expect(e.do("POST", "/api/v1/notifications/rules", admin, "", map[string]any{
		"site_id": f.site, "event_type": "patrol_not_started", "channel": "email", "recipient": "ops@example.com",
	}), 201, "patrol_not_started rule")

	// ── Schedule CRUD ────────────────────────────────────────────────────────
	jkt := mustLoc(t, "Asia/Jakarta")
	now := time.Now().UTC()
	at := now.Add(2 * time.Hour).In(jkt).Format("15:04")
	all := []int{1, 2, 3, 4, 5, 6, 7}
	e.expect(e.do("PUT", "/api/v1/patrol/routes/"+route+"/schedules", sup, "", map[string]any{"schedules": []any{}}), 403, "supervisor edits schedule")
	if r := e.do("PUT", "/api/v1/patrol/routes/"+route+"/schedules", admin, "", map[string]any{"schedules": []map[string]any{
		{"weekdays": all, "start_time": "08:00", "every_minutes": 60},
	}}); r.status != 400 || r.str("code") != "end_time_required" {
		t.Fatalf("invalid schedule: %d %s", r.status, r.raw)
	}
	put := e.expect(e.do("PUT", "/api/v1/patrol/routes/"+route+"/schedules", admin, "", map[string]any{"schedules": []map[string]any{
		{"weekdays": all, "start_time": at},
	}}), 200, "put schedule").obj()
	if put["generated_expectations"].(float64) != 2 || len(put["schedules"].([]any)) != 1 {
		t.Fatalf("put schedule = %v", put)
	}
	got := e.expect(e.do("GET", "/api/v1/patrol/routes/"+route+"/schedules", sup, "", nil), 200, "supervisor reads").obj()
	if s := got["schedules"].([]any)[0].(map[string]any); s["start_time"] != at || s["grace_minutes"].(float64) != 15 || s["every_minutes"] != nil {
		t.Fatalf("schedule = %v", s)
	}
	e.expect(e.do("GET", "/api/v1/patrol/routes/"+route+"/schedules", client, "", nil), 403, "client reads schedule")
	if err := e.sched.MaterialiseExpectations(ctx); err != nil {
		t.Fatal(err)
	}
	if n := e.count(`SELECT COUNT(*) FROM patrol_expectations WHERE route_id = $1`, route); n != 2 {
		t.Fatalf("expectations after job = %d, want 2 (idempotent)", n)
	}
	// The tablet sees the next patrol.
	st := e.expect(e.do("GET", "/api/v1/device/status", f.tablet, f.guard, nil), 200, "status").obj()
	due := st["patrols_due"].([]any)
	if len(due) == 0 || due[0].(map[string]any)["route_name"] != "Ronda" || due[0].(map[string]any)["status"] != "pending" {
		t.Fatalf("patrols_due = %v", due)
	}

	// Past patrols (inserted directly; materialisation only creates future ones).
	insert := func(routeID string, dueAgo, grace time.Duration) string {
		t.Helper()
		var id string
		err := e.pool.QueryRow(ctx, `INSERT INTO patrol_expectations (route_id, site_id, due_at, grace_until)
			VALUES ($1, $2, $3, $4) RETURNING id::text`, routeID, f.site, now.Add(-dueAgo), now.Add(-dueAgo+grace)).Scan(&id)
		if err != nil {
			t.Fatal(err)
		}
		return id
	}
	status := func(id string) (string, bool, *string) {
		t.Helper()
		var st string
		var late bool
		var sess *string
		if err := e.pool.QueryRow(ctx, `SELECT status, late_start, session_id::text FROM patrol_expectations WHERE id = $1`, id).Scan(&st, &late, &sess); err != nil {
			t.Fatal(err)
		}
		return st, late, sess
	}
	start := func(routeID string, ago time.Duration) string {
		return e.expect(e.do("POST", "/api/v1/patrol/sessions", f.tablet, f.guard, map[string]any{
			"id": uuid.NewString(), "route_id": routeID, "started_at": rfc(now.Add(-ago)),
		}), 201, "start session").str("id")
	}

	// ── Not started → missed + notification (once) ──────────────────────────
	e1 := insert(route, 40*time.Minute, 15*time.Minute) // grace ended 25 min ago
	e2 := insert(route, 3*time.Hour, 15*time.Minute)
	for i := 0; i < 2; i++ {
		if err := e.sched.CheckPatrolExpectations(ctx); err != nil {
			t.Fatal(err)
		}
	}
	for _, id := range []string{e1, e2} {
		if st, _, _ := status(id); st != "missed" {
			t.Fatalf("expectation %s = %s, want missed", id, st)
		}
	}
	if n := e.count(`SELECT COUNT(*) FROM notification_log WHERE event_type = 'patrol_not_started' AND site_id = $1 AND status = 'queued'`, f.site); n != 2 {
		t.Fatalf("patrol_not_started deliveries = %d, want 2", n)
	}
	if n := e.count(`SELECT COUNT(*) FROM notification_log WHERE event_type = 'patrol_not_started' AND body LIKE '%Ronda%'`); n == 0 {
		t.Fatal("notification lacks the route name")
	}

	// ── Late patrol: started after the grace period → started, late_start ──
	late := start(route, 20*time.Minute)
	if st, isLate, sess := status(e1); st != "started" || !isLate || sess == nil || *sess != late {
		t.Fatalf("late start: %s %v %v", st, isLate, sess)
	}
	// ── Offline session synced later: matches retroactively, on time ────────
	offline := start(route, 3*time.Hour-5*time.Minute)
	if st, isLate, sess := status(e2); st != "started" || isLate || sess == nil || *sess != offline {
		t.Fatalf("offline on-time start: %s %v %v", st, isLate, sess)
	}

	// ── A session from before the patrol was materialised is matched by the job.
	early := start(route2, 32*time.Minute)
	e3 := insert(route2, 30*time.Minute, 15*time.Minute)
	if err := e.sched.CheckPatrolExpectations(ctx); err != nil {
		t.Fatal(err)
	}
	if st, _, sess := status(e3); st != "started" || sess == nil || *sess != early {
		t.Fatalf("job matching: %s %v", st, sess)
	}

	// ── One session satisfies at most one patrol (nearest, then earliest). ──
	e4 := insert(route3, 60*time.Minute, 15*time.Minute)
	e5 := insert(route3, 50*time.Minute, 15*time.Minute)
	one := start(route3, 55*time.Minute)
	if err := e.sched.CheckPatrolExpectations(ctx); err != nil {
		t.Fatal(err)
	}
	if n := e.count(`SELECT COUNT(*) FROM patrol_expectations WHERE session_id = $1`, one); n != 1 {
		t.Fatalf("session satisfies %d patrols", n)
	}
	if st, _, _ := status(e4); st != "started" {
		t.Fatalf("e4 = %s", st)
	}
	if st, _, _ := status(e5); st != "missed" {
		t.Fatalf("e5 = %s", st)
	}

	// ── Reports ──────────────────────────────────────────────────────────────
	from := now.Add(-24 * time.Hour).In(jkt).Format("2006-01-02")
	to := now.Add(48 * time.Hour).In(jkt).Format("2006-01-02")
	rep := e.expect(e.do("GET", "/api/v1/reports/patrol-compliance?site_id="+f.site+"&from="+from+"&to="+to, client, "", nil), 200, "compliance").obj()
	tot := rep["totals"].(map[string]any)
	// on time: e2, e3, e4; late: e1; missed: e5; pending: the 2 materialised.
	if tot["expected"].(float64) != 7 || tot["started_on_time"].(float64) != 3 || tot["late"].(float64) != 1 ||
		tot["missed"].(float64) != 1 || tot["pending"].(float64) != 2 || tot["compliance_pct"].(float64) != 60 {
		t.Fatalf("compliance totals = %v", tot)
	}
	if rows := rep["rows"].([]any); len(rows) < 3 {
		t.Fatalf("compliance rows = %v", rows)
	}
	csv := e.expect(e.do("GET", "/api/v1/reports/patrol-compliance?site_id="+f.site+"&from="+from+"&to="+to+"&format=csv", sup, "", nil), 200, "compliance csv")
	if !bytes.HasPrefix(csv.raw, []byte("\xEF\xBB\xBFDate,Route,")) || !strings.Contains(string(csv.raw), "Ronda") {
		t.Fatalf("compliance csv = %q", csv.raw)
	}
	sessions := e.expect(e.do("GET", "/api/v1/reports/patrol/sessions?site_id="+f.site+"&from="+from, sup, "", nil), 200, "patrol report").obj()["rows"].([]any)
	matched := 0
	for _, s := range sessions {
		if m := s.(map[string]any); m["expectation_id"] != nil && m["due_at"] != nil {
			matched++
			if m["session_id"] == late && m["late_start"] != true {
				t.Fatalf("late session row = %v", m)
			}
		}
	}
	if matched != 4 {
		t.Fatalf("patrol report rows with a schedule = %d, want 4", matched)
	}
	kpi := e.expect(e.do("GET", "/api/v1/reports/guards/kpi?site_id="+f.site+"&from="+from+"&to="+to, sup, "", nil), 200, "kpi").obj()["rows"].([]any)
	if k := kpi[0].(map[string]any); k["scheduled_patrols"].(float64) != 4 || k["schedule_compliance_pct"].(float64) != 75 {
		t.Fatalf("kpi = %v", k)
	}
	monthly := e.expect(e.do("GET", "/api/v1/reports/attendance/monthly?site_id="+f.site, sup, "", nil), 200, "monthly").obj()
	if monthly["patrol_compliance"] == nil {
		t.Fatalf("monthly recap lacks patrol_compliance: %v", monthly)
	}

	// ── Schedule change regenerates future patrols; deactivation cancels. ──
	at2 := now.Add(3 * time.Hour).In(jkt).Format("15:04")
	e.expect(e.do("PUT", "/api/v1/patrol/routes/"+route+"/schedules", admin, "", map[string]any{"schedules": []map[string]any{
		{"weekdays": all, "start_time": at2, "grace_minutes": 10},
	}}), 200, "change schedule")
	if n := e.count(`SELECT COUNT(*) FROM patrol_expectations WHERE route_id = $1 AND status = 'pending' AND due_at > now()
		AND grace_until - due_at = interval '10 minutes'`, route); n != 2 {
		t.Fatalf("regenerated pending patrols = %d, want 2", n)
	}
	if n := e.count(`SELECT COUNT(*) FROM patrol_expectations WHERE route_id = $1 AND status = 'pending' AND grace_until - due_at = interval '15 minutes'`, route); n != 0 {
		t.Fatalf("%d old pending patrols left", n)
	}
	e.expect(e.do("PUT", "/api/v1/patrol/routes/"+route, admin, "", map[string]any{
		"name": "Ronda", "active": false, "checkpoints": []map[string]any{{"name": "Gate", "order_index": 0}},
	}), 200, "deactivate route")
	if n := e.count(`SELECT COUNT(*) FROM patrol_expectations WHERE route_id = $1 AND status = 'pending'`, route); n != 0 {
		t.Fatalf("pending patrols of an inactive route = %d", n)
	}
	if n := e.count(`SELECT COUNT(*) FROM patrol_expectations WHERE route_id = $1 AND status = 'cancelled'`, route); n != 2 {
		t.Fatalf("cancelled patrols = %d, want 2", n)
	}
}
