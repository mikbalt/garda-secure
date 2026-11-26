//go:build integration

package integration

import (
	"context"
	"testing"
	"time"
)

// TAQ-21: fraud-signal dashboard. GET /flags lists the plausibility flags of
// field records; POST /flags/review marks one reviewed without touching the
// record itself.
func TestFlagDashboard(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	admin := e.web(realm3, "fd-admin", "admin")
	sup := e.web(realm3, "fd-sup", "supervisor")
	hr := e.web(realm3, "fd-hr", "hr")
	client := e.web(realm3, "fd-client", "client-readonly")
	otherAdmin := e.web(realm4, "fd-other-admin", "admin")
	a := e.newFieldSite(realm3, admin, "Flag Dash A")
	b := e.newFieldSite(realm3, admin, "Flag Dash B")
	day1 := time.Date(2026, 3, 10, 9, 0, 0, 0, time.UTC)
	day2 := day1.AddDate(0, 0, 2)

	var att1, att2, attB, attClean, attUnsigned, sos string
	ins := func(dst *string, sql string, args ...any) {
		t.Helper()
		if err := e.pool.QueryRow(ctx, sql, args...).Scan(dst); err != nil {
			t.Fatal(err)
		}
	}
	const insAtt = `INSERT INTO attendance_records (guard_id, site_id, clock_in_at, status, received_at, flags)
		VALUES ($1, $2, $3, 'present', $3, $4) RETURNING id::text`
	ins(&att1, insAtt, a.guard, a.site, day1, []string{"late_sync", "backdated_on_time"})
	ins(&att2, insAtt, a.guard2, a.site, day2, []string{"clock_ahead"})
	ins(&attB, insAtt, b.guard, b.site, day1, []string{"late_sync"})
	ins(&attClean, insAtt, a.guard, a.site, day1, []string{})
	ins(&attUnsigned, insAtt, a.guard, a.site, day1, []string{"qr_unsigned"})
	ins(&sos, `INSERT INTO sos_alerts (guard_id, site_id, fired_at, flags) VALUES ($1, $2, $3, $4) RETURNING id::text`,
		a.guard, a.site, day1, []string{"guard_unverified"})

	list := func(token, query string) []map[string]any {
		t.Helper()
		r := e.expect(e.do("GET", "/api/v1/flags"+query+limitParam(query), token, "", nil), 200, "list flags "+query)
		out := []map[string]any{}
		for _, row := range r.obj()["rows"].([]any) {
			m := row.(map[string]any)
			// The database is shared with other tests of the realm: look at our sites only.
			if m["site_id"] == a.site || m["site_id"] == b.site {
				out = append(out, m)
			}
		}
		return out
	}
	has := func(rows []map[string]any, kind, id, flag string) bool {
		for _, r := range rows {
			if r["kind"] == kind && r["record_id"] == id && r["flag"] == flag {
				return true
			}
		}
		return false
	}

	// One row per flag; clean records and the noisy qr_unsigned are left out.
	all := list(sup, "")
	if len(all) != 5 {
		t.Fatalf("all rows = %d, want 5: %v", len(all), all)
	}
	for _, want := range [][3]string{
		{"attendance", att1, "late_sync"}, {"attendance", att1, "backdated_on_time"}, {"attendance", att2, "clock_ahead"},
		{"attendance", attB, "late_sync"}, {"sos", sos, "guard_unverified"},
	} {
		if !has(all, want[0], want[1], want[2]) {
			t.Fatalf("missing %v in %v", want, all)
		}
	}
	if has(all, "attendance", attClean, "") || has(all, "attendance", attUnsigned, "qr_unsigned") {
		t.Fatalf("clean or qr_unsigned record listed: %v", all)
	}
	if r := all[0]; r["site_name"] == nil || r["guard_name"] == nil || r["event_at"] == nil || r["reviewed_at"] != nil {
		t.Fatalf("row shape = %v", r)
	}
	if got := list(sup, "?flag=qr_unsigned"); len(got) != 1 || got[0]["record_id"] != attUnsigned {
		t.Fatalf("flag=qr_unsigned = %v", got)
	}

	// Filters: site, guard, date range, flag.
	if got := list(sup, "?site_id="+b.site); len(got) != 1 || got[0]["record_id"] != attB {
		t.Fatalf("site filter = %v", got)
	}
	if got := list(sup, "?guard_id="+a.guard2); len(got) != 1 || got[0]["record_id"] != att2 {
		t.Fatalf("guard filter = %v", got)
	}
	if got := list(sup, "?from=2026-03-11&to=2026-03-12"); len(got) != 1 || got[0]["record_id"] != att2 {
		t.Fatalf("date filter = %v", got)
	}
	if got := list(sup, "?flag=late_sync"); len(got) != 2 {
		t.Fatalf("flag filter = %v", got)
	}
	e.expect(e.do("GET", "/api/v1/flags?from=nope", sup, "", nil), 400, "bad date")
	e.expect(e.do("GET", "/api/v1/flags?site_id="+a.site+"&guard_id="+b.guard, sup, "", nil), 200, "guard of other site is just empty")

	// Review: stores a mark, the record stays as it was.
	snapshot := func() string {
		var s string
		if err := e.pool.QueryRow(ctx, `SELECT (SELECT row_to_json(r)::text FROM attendance_records r WHERE id = $1)`, att1).Scan(&s); err != nil {
			t.Fatal(err)
		}
		return s
	}
	before := snapshot()
	rv := map[string]any{"kind": "attendance", "record_id": att1, "flag": "late_sync", "note": "checked with the guard"}
	r := e.expect(e.do("POST", "/api/v1/flags/review", sup, "", rv), 200, "review").obj()
	if r["reviewed_by"] == nil || r["reviewed_at"] == nil || r["note"] != "checked with the guard" {
		t.Fatalf("review response = %v", r)
	}
	e.expect(e.do("POST", "/api/v1/flags/review", sup, "", rv), 200, "review is idempotent")
	if after := snapshot(); after != before {
		t.Fatalf("record changed by review:\n%s\n%s", before, after)
	}
	if n := e.count(`SELECT count(*) FROM flag_reviews`); n != 1 {
		t.Fatalf("flag_reviews = %d, want 1", n)
	}
	rows := list(sup, "?site_id="+a.site+"&guard_id="+a.guard)
	for _, row := range rows {
		reviewed := row["reviewed_at"] != nil
		if want := row["record_id"] == att1 && row["flag"] == "late_sync"; reviewed != want {
			t.Fatalf("reviewed(%v) = %v, want %v", row, reviewed, want)
		}
	}
	if got := list(sup, "?reviewed=false"); len(got) != 4 || has(got, "attendance", att1, "late_sync") {
		t.Fatalf("reviewed=false = %v", got)
	}
	if got := list(sup, "?reviewed=true"); len(got) != 1 {
		t.Fatalf("reviewed=true = %v", got)
	}

	// Review input is checked: the flag must exist on the record.
	e.expect(e.do("POST", "/api/v1/flags/review", sup, "", map[string]any{"kind": "attendance", "record_id": att1, "flag": "gps_far"}), 404, "flag not on record")
	e.expect(e.do("POST", "/api/v1/flags/review", sup, "", map[string]any{"kind": "nope", "record_id": att1, "flag": "late_sync"}), 400, "bad kind")
	e.expect(e.do("POST", "/api/v1/flags/review", sup, "", map[string]any{"kind": "attendance", "record_id": "x", "flag": "late_sync"}), 400, "bad id")

	// Roles: admin and supervisor only; other realms see and review nothing.
	for name, tok := range map[string]string{"hr": hr, "client-readonly": client, "tablet": a.tablet} {
		e.expect(e.do("GET", "/api/v1/flags", tok, "", nil), 403, "list as "+name)
		e.expect(e.do("POST", "/api/v1/flags/review", tok, "", map[string]any{"kind": "sos", "record_id": sos, "flag": "guard_unverified"}), 403, "review as "+name)
	}
	e.expect(e.do("GET", "/api/v1/flags", "", "", nil), 401, "no token")
	if got := list(otherAdmin, ""); len(got) != 0 {
		t.Fatalf("other realm sees %v", got)
	}
	e.expect(e.do("POST", "/api/v1/flags/review", otherAdmin, "", map[string]any{"kind": "sos", "record_id": sos, "flag": "guard_unverified"}), 404, "other realm review")
	e.expect(e.do("GET", "/api/v1/flags?site_id="+a.site, otherAdmin, "", nil), 404, "other realm site filter")
}

// limitParam asks for the largest page so rows of other tests cannot push ours out.
func limitParam(query string) string {
	if query == "" {
		return "?limit=200"
	}
	return "&limit=200"
}
