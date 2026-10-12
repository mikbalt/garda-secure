//go:build integration

package integration

import (
	"context"
	"slices"
	"testing"
	"time"
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
	route := e.expect(e.do("POST", "/api/v1/patrol/routes", admin, "", map[string]any{
		"site_id": f.site, "name": "Geo", "checkpoints": []map[string]any{
			{"name": "Gate", "qr_code": "GEO-1", "order_index": 0, "lat": -6.2000, "lng": 106.8000},
			{"name": "Warehouse", "qr_code": "GEO-2", "order_index": 1, "lat": -6.2090, "lng": 106.8000},
		},
	}), 201, "route").str("id")
	sess := e.expect(e.do("POST", "/api/v1/patrol/sessions", f.tablet, f.guard, map[string]any{
		"route_id": route, "started_at": rfc(now.Add(-10 * time.Minute)),
	}), 201, "session").str("id")
	ok := e.expect(e.do("POST", "/api/v1/patrol/sessions/"+sess+"/scans", f.tablet, f.guard, map[string]any{
		"qr_code": "GEO-1", "scanned_at": rfc(now.Add(-5 * time.Minute)), "gps_lat": -6.2001, "gps_lng": 106.8001,
	}), 201, "scan at gate").obj()
	if fl := flagsOf(ok["flags"]); len(fl) != 0 {
		t.Fatalf("plausible scan flagged: %v", fl)
	}
	bad := e.expect(e.do("POST", "/api/v1/patrol/sessions/"+sess+"/scans", f.tablet, f.guard, map[string]any{
		"qr_code": "GEO-2", "scanned_at": rfc(now.Add(-4 * time.Minute)), "gps_lat": -6.2000, "gps_lng": 106.8000,
	}), 201, "scan 1 km away a minute later").obj()
	if fl := flagsOf(bad["flags"]); !slices.Contains(fl, "gps_far") || !slices.Contains(fl, "too_fast") {
		t.Fatalf("implausible scan flags = %v", fl)
	}
	prog := e.expect(e.do("GET", "/api/v1/patrol/sessions/"+sess, sup, "", nil), 200, "progress").obj()
	if cp := prog["checkpoints"].([]any)[1].(map[string]any); len(flagsOf(cp["scan_flags"])) != 2 {
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
