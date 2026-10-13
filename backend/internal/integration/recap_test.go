//go:build integration

package integration

import (
	"context"
	"strings"
	"testing"
	"time"
)

// F13 / FR-REP-05: the monthly recap counts guard-days in the site's time
// zone, keyed by the day a shift starts.
func TestMonthlyRecapCountsGuardDays(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	admin := e.web(realm4, "recap-admin", "admin")
	hr := e.web(realm4, "recap-hr", "hr")
	papua := e.expect(e.do("POST", "/api/v1/sites", admin, "", map[string]any{"name": "Recap Papua", "timezone": "Asia/Jayapura"}), 201, "site").str("id")
	java := e.expect(e.do("POST", "/api/v1/sites", admin, "", map[string]any{"name": "Recap Jawa", "timezone": "Asia/Jakarta"}), 201, "site 2").str("id")
	budi := e.expect(e.do("POST", "/api/v1/guards", admin, "", map[string]any{"name": "Recap Budi", "site_id": papua, "employee_id": "RCP-1"}), 201, "guard").str("id")
	sari := e.expect(e.do("POST", "/api/v1/guards", admin, "", map[string]any{"name": "Recap Sari", "site_id": papua}), 201, "guard 2").str("id")
	wit, wib := mustLoc(t, "Asia/Jayapura"), mustLoc(t, "Asia/Jakarta")

	shift := func(guard, site string, start time.Time, hours int) string {
		t.Helper()
		var id string
		if err := e.pool.QueryRow(ctx, `INSERT INTO shifts (guard_id, site_id, start_time, end_time) VALUES ($1, $2, $3, $4) RETURNING id`,
			guard, site, start, start.Add(time.Duration(hours)*time.Hour)).Scan(&id); err != nil {
			t.Fatal(err)
		}
		return id
	}
	// worked records an outcome; clocked shifts worked their full length
	// minus earlyOut minutes.
	worked := func(shiftID, guard, site, status string, start time.Time, hours, earlyOut int, clocked bool) {
		t.Helper()
		var in, out *time.Time
		if clocked {
			i, o := start, start.Add(time.Duration(hours)*time.Hour-time.Duration(earlyOut)*time.Minute)
			in, out = &i, &o
		}
		if _, err := e.pool.Exec(ctx, `INSERT INTO attendance_records (shift_id, guard_id, site_id, status, clock_in_at, clock_out_at, early_out_minutes)
			VALUES ($1, $2, $3, $4, $5, $6, $7)`, shiftID, guard, site, status, in, out, earlyOut); err != nil {
			t.Fatal(err)
		}
	}
	at := func(l *time.Location, m time.Month, d, h, min int) time.Time {
		return time.Date(2026, m, d, h, min, 0, 0, l)
	}

	// Budi, Papua (WIT, +09), August 2026:
	// Aug 1: day shift present (left 30 min early, 7.5 h) + night shift late -> late, early-out day.
	s := at(wit, 8, 1, 7, 0)
	worked(shift(budi, papua, s, 8), budi, papua, "present", s, 8, 30, true)
	s = at(wit, 8, 1, 19, 0)
	worked(shift(budi, papua, s, 8), budi, papua, "late", s, 8, 0, false)
	// Aug 2: present, overridden to absent by an admin -> absent.
	s = at(wit, 8, 2, 7, 0)
	overridden := shift(budi, papua, s, 8)
	worked(overridden, budi, papua, "present", s, 8, 0, false)
	// Aug 20: no record yet -> upcoming, not absent.
	shift(budi, papua, at(wit, 8, 20, 7, 0), 8)
	// Aug 31 22:00 overnight into September: counts in August (8 h).
	s = at(wit, 8, 31, 22, 0)
	worked(shift(budi, papua, s, 8), budi, papua, "present", s, 8, 0, true)
	// Jul 31 22:00 overnight into August: July only.
	s = at(wit, 7, 31, 22, 0)
	worked(shift(budi, papua, s, 8), budi, papua, "absent", s, 8, 0, false)
	// Moved to Java for one day: counted at the Java site only.
	s = at(wib, 8, 15, 8, 0)
	worked(shift(budi, java, s, 8), budi, java, "absent", s, 8, 0, false)

	// Sari: Aug 1 00:30 WIT is still July 31 in UTC -> August at this site;
	// Sep 1 00:30 WIT is August 31 in UTC -> September.
	s = at(wit, 8, 1, 0, 30)
	worked(shift(sari, papua, s, 6), sari, papua, "present", s, 6, 0, true)
	s = at(wit, 9, 1, 0, 30)
	worked(shift(sari, papua, s, 6), sari, papua, "present", s, 6, 0, true)

	e.expect(e.do("PUT", "/api/v1/attendance/shifts/"+overridden+"/override", admin, "", map[string]any{
		"status": "absent", "reason": "Left the post, confirmed by CCTV",
	}), 200, "override")

	recap := func(site string) (map[string]map[string]any, map[string]any) {
		t.Helper()
		r := e.expect(e.do("GET", "/api/v1/reports/attendance/monthly?site_id="+site+"&month=2026-08", hr, "", nil), 200, "monthly").obj()
		rows := map[string]map[string]any{}
		for _, x := range r["rows"].([]any) {
			m := x.(map[string]any)
			rows[m["guard_id"].(string)] = m
		}
		return rows, r["totals"].(map[string]any)
	}
	check := func(what string, got map[string]any, want map[string]float64) {
		t.Helper()
		for k, v := range want {
			if got[k] != v {
				t.Errorf("%s: %s = %v, want %v (row %v)", what, k, got[k], v, got)
			}
		}
	}

	rows, totals := recap(papua)
	if len(rows) != 2 {
		t.Fatalf("papua rows = %v", rows)
	}
	check("budi", rows[budi], map[string]float64{
		"scheduled_days": 4, "days_present": 1, "days_late": 1, "days_absent": 1, "days_upcoming": 1, "early_out_days": 1,
		"scheduled_shifts": 5, "present": 2, "late": 1, "absent": 1, "pending": 1, "early_out": 1, "total_hours": 15.5,
	})
	check("sari", rows[sari], map[string]float64{"scheduled_days": 1, "days_present": 1, "scheduled_shifts": 1, "total_hours": 6})
	check("totals", totals, map[string]float64{
		"guards": 2, "scheduled_days": 5, "days_present": 2, "days_late": 1, "days_absent": 1, "days_upcoming": 1,
		"early_out_days": 1, "scheduled_shifts": 6, "total_hours": 21.5,
	})

	rows, totals = recap(java)
	check("budi at java", rows[budi], map[string]float64{"scheduled_days": 1, "days_absent": 1, "scheduled_shifts": 1})
	check("java totals", totals, map[string]float64{"guards": 1, "scheduled_days": 1, "days_absent": 1})

	status, _, body := e.raw("GET", "/api/v1/reports/attendance/monthly?site_id="+papua+"&month=2026-08&format=csv", hr, nil)
	if status != 200 {
		t.Fatalf("csv: %d %s", status, body)
	}
	csv := string(body)
	for _, want := range []string{
		"Guard,Employee ID,Scheduled days,Days present,Days late,Days absent,Days upcoming,Early-out days,Hours,Scheduled shifts,Patrols,Patrol completion %",
		"Recap Budi,RCP-1,4,1,1,1,1,1,15.5,5,0,0.0",
		"Total (2 guards),,5,2,1,1,1,1,21.5,6,0,0.0",
	} {
		if !strings.Contains(csv, want) {
			t.Errorf("csv lacks %q:\n%s", want, csv)
		}
	}
}
