//go:build integration

package integration

import (
	"context"
	"math"
	"slices"
	"testing"
	"time"

	"github.com/securepatrol/backend/internal/repository"
)

func flagsOf(v any) []string {
	out := []string{}
	l, _ := v.([]any)
	for _, f := range l {
		out = append(out, f.(string))
	}
	return out
}

// BE-S1: receipt times and plausibility flags on field records.
func TestPlausibilityFlags(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	admin := e.web(realm3, "flags-admin", "admin")
	sup := e.web(realm3, "flags-sup", "supervisor")
	f := e.newFieldSite(realm3, admin, "Flags A")
	now := time.Now().UTC()

	// ── Clock-in: backdated on-time while the tablet was online → review ────
	for _, g := range []string{f.guard, f.guard2} {
		e.expect(e.do("POST", "/api/v1/shifts", admin, "", map[string]any{
			"site_id": f.site, "guard_id": g, "start_time": rfc(now.Add(-30 * time.Minute)), "end_time": rfc(now.Add(7 * time.Hour)),
		}), 201, "shift")
	}
	setLastSeen := func(at time.Time) {
		t.Helper()
		if _, err := e.pool.Exec(ctx, `UPDATE devices SET last_seen_at = $1 WHERE site_id = $2`, at, f.site); err != nil {
			t.Fatal(err)
		}
	}
	setLastSeen(now.Add(-time.Minute)) // tablet was online after the claimed time
	rec := e.expect(e.do("POST", "/api/v1/attendance/clock-in", f.tablet, f.guard, map[string]any{
		"clock_in_at": rfc(now.Add(-29 * time.Minute)),
	}), 201, "backdated clock-in").obj()
	if fl := flagsOf(rec["flags"]); !slices.Contains(fl, "backdated_on_time") || !slices.Contains(fl, "late_sync") ||
		rec["needs_review"] != true || rec["status"] != "present" || rec["received_at"] == nil {
		t.Fatalf("backdated clock-in = %v", rec)
	}
	setLastSeen(now.Add(-45 * time.Minute)) // tablet offline since before the claimed time
	rec = e.expect(e.do("POST", "/api/v1/attendance/clock-in", f.tablet, f.guard2, map[string]any{
		"clock_in_at": rfc(now.Add(-29 * time.Minute)),
	}), 201, "offline clock-in").obj()
	if fl := flagsOf(rec["flags"]); !slices.Equal(fl, []string{"late_sync"}) || rec["needs_review"] != false {
		t.Fatalf("offline-queued clock-in = %v", rec)
	}
	daily := e.expect(e.do("GET", "/api/v1/attendance/daily?site_id="+f.site, sup, "", nil), 200, "daily").obj()["rows"].([]any)
	flagged := 0
	for _, r := range daily {
		if len(flagsOf(r.(map[string]any)["flags"])) > 0 {
			flagged++
		}
	}
	if flagged != 2 {
		t.Fatalf("daily rows with flags = %d, want 2", flagged)
	}
	if q := e.do("GET", "/api/v1/attendance/review?site_id="+f.site, sup, "", nil).list(); len(q) != 1 {
		t.Fatalf("review queue = %d, want 1", len(q))
	}

	// ── Scans: GPS far from the checkpoint and impossible speed ─────────────
	// Submitted coordinates are ignored; positions are learned from scans (D-059).
	routeObj := e.expect(e.do("POST", "/api/v1/patrol/routes", admin, "", map[string]any{
		"site_id": f.site, "name": "Geo", "checkpoints": []map[string]any{
			{"name": "Gate", "qr_code": "GEO-1", "order_index": 0, "lat": -6.2000, "lng": 106.8000},
			{"name": "Warehouse", "qr_code": "GEO-2", "order_index": 1},
		},
	}), 201, "route").obj()
	route := routeObj["id"].(string)
	cps := routeObj["checkpoints"].([]any)
	if _, has := cps[0].(map[string]any)["lat"]; has {
		t.Fatalf("submitted checkpoint lat must be ignored: %v", cps[0])
	}
	gateID, whID := cps[0].(map[string]any)["id"].(string), cps[1].(map[string]any)["id"].(string)
	seed := func(cp, guard, method string, lat, lng float64, flags []string) {
		t.Helper()
		if _, err := e.pool.Exec(ctx, `INSERT INTO checkpoint_scans (route_id, checkpoint_id, guard_id, scanned_at, method, gps_lat, gps_lng, flags)
			VALUES ($1, $2, $3, now() - interval '1 day', $4, $5, $6, COALESCE($7, '{}'::text[]))`, route, cp, guard, method, lat, lng, flags); err != nil {
			t.Fatal(err)
		}
	}
	for i, g := range []string{f.guard, f.guard, f.guard, f.guard2, f.guard2} {
		d := float64(i) * 0.00001
		seed(gateID, g, "qr", -6.2000+d-0.00002, 106.8000, nil)
		seed(whID, g, "qr", -6.2090, 106.8000+d-0.00002, nil)
	}
	seed(gateID, f.guard, "qr", -6.3000, 106.9000, nil)                    // outlier: median ignores it
	seed(gateID, f.guard, "manual", -7.0000, 107.0000, nil)                // manual entries do not count
	seed(gateID, f.guard, "qr", -7.0000, 107.0000, []string{"qr_invalid"}) // forged QR does not count
	seed(whID, f.guard, "qr", -6.2090, 106.8000, nil)
	learn := func() int64 {
		t.Helper()
		n, err := repository.LearnCheckpointPositions(ctx, e.pool, now.Add(-30*24*time.Hour), 5, 2)
		if err != nil {
			t.Fatal(err)
		}
		return n
	}
	if n := learn(); n != 1 {
		t.Fatalf("learned routes = %d, want 1", n)
	}
	if n := learn(); n != 0 {
		t.Fatalf("unchanged positions rewrote %d routes", n)
	}
	var gLat, gLng, wLat, wLng float64
	if err := e.pool.QueryRow(ctx, `SELECT (checkpoints->0->>'lat')::float8, (checkpoints->0->>'lng')::float8,
		(checkpoints->1->>'lat')::float8, (checkpoints->1->>'lng')::float8 FROM routes WHERE id = $1`, route).Scan(&gLat, &gLng, &wLat, &wLng); err != nil {
		t.Fatal(err)
	}
	near := func(a, b float64) bool { return math.Abs(a-b) < 0.00002 } // ~2 m
	if !near(gLat, -6.2) || !near(gLng, 106.8) || !near(wLat, -6.209) || !near(wLng, 106.8) {
		t.Fatalf("learned positions gate=(%v,%v) warehouse=(%v,%v)", gLat, gLng, wLat, wLng)
	}
	// An admin edit keeps the learned positions.
	e.expect(e.do("PUT", "/api/v1/patrol/routes/"+route, admin, "", map[string]any{
		"name": "Geo", "active": true, "checkpoints": []map[string]any{
			{"id": gateID, "name": "Gate", "qr_code": "GEO-1", "order_index": 0},
			{"id": whID, "name": "Warehouse", "qr_code": "GEO-2", "order_index": 1, "lat": 1, "lng": 1},
		},
	}), 200, "route edit")
	if n := e.count(`SELECT COUNT(*) FROM routes WHERE id = $1 AND (checkpoints->1->>'lat')::float8 = -6.209`, route); n != 1 {
		t.Fatal("route edit dropped or overwrote the learned position")
	}
	sess := e.expect(e.do("POST", "/api/v1/patrol/sessions", f.tablet, f.guard, map[string]any{
		"route_id": route, "started_at": rfc(now.Add(-10 * time.Minute)),
	}), 201, "session").str("id")
	ok := e.expect(e.do("POST", "/api/v1/patrol/sessions/"+sess+"/scans", f.tablet, f.guard, map[string]any{
		"qr_code": "GEO-1", "scanned_at": rfc(now.Add(-5 * time.Minute)), "gps_lat": -6.2001, "gps_lng": 106.8001,
	}), 201, "scan at gate").obj()
	// Custom codes are legacy (unsigned) QR codes: qr_unsigned only.
	if fl := flagsOf(ok["flags"]); !slices.Equal(fl, []string{"qr_unsigned"}) {
		t.Fatalf("plausible scan flagged: %v", fl)
	}
	bad := e.expect(e.do("POST", "/api/v1/patrol/sessions/"+sess+"/scans", f.tablet, f.guard, map[string]any{
		"qr_code": "GEO-2", "scanned_at": rfc(now.Add(-4 * time.Minute)), "gps_lat": -6.2000, "gps_lng": 106.8000,
	}), 201, "scan 1 km away a minute later").obj()
	if fl := flagsOf(bad["flags"]); !slices.Contains(fl, "gps_far") || !slices.Contains(fl, "too_fast") {
		t.Fatalf("implausible scan flags = %v", fl)
	}
	prog := e.expect(e.do("GET", "/api/v1/patrol/sessions/"+sess, sup, "", nil), 200, "progress").obj()
	if cp := prog["checkpoints"].([]any)[1].(map[string]any); len(flagsOf(cp["scan_flags"])) != 3 {
		t.Fatalf("progress lacks scan flags: %v", cp)
	}
	sums := e.expect(e.do("GET", "/api/v1/patrol/sessions?site_id="+f.site, sup, "", nil), 200, "sessions").list()
	if s := sums[0].(map[string]any); s["flagged_scans"].(float64) != 1 {
		t.Fatalf("session summary = %v", s)
	}

	// ── Incidents and SOS carry receipt-time flags ──────────────────────────
	inc := e.expect(e.do("POST", "/api/v1/incidents", f.tablet, f.guard, map[string]any{
		"type": "theft", "occurred_at": rfc(now.Add(-time.Hour)),
	}), 201, "late incident").obj()
	if !slices.Equal(flagsOf(inc["flags"]), []string{"late_sync"}) || inc["received_at"] == nil {
		t.Fatalf("incident = %v", inc)
	}
	sos := e.expect(e.do("POST", "/api/v1/sos", f.tablet, f.guard, map[string]any{"fired_at": rfc(now.Add(time.Minute))}), 201, "sos ahead").obj()
	if !slices.Equal(flagsOf(sos["flags"]), []string{"clock_ahead"}) {
		t.Fatalf("sos = %v", sos)
	}
	if l := e.do("GET", "/api/v1/incidents?site_id="+f.site, sup, "", nil).list(); len(flagsOf(l[0].(map[string]any)["flags"])) != 1 {
		t.Fatalf("incident list lacks flags: %v", l[0])
	}
}
