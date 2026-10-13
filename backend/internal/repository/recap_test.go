package repository

import (
	"testing"
	"time"
)

func loc(t *testing.T, name string) *time.Location {
	t.Helper()
	l, err := time.LoadLocation(name)
	if err != nil {
		t.Fatal(err)
	}
	return l
}

func TestAggregateRecapDayStatus(t *testing.T) {
	jkt := loc(t, "Asia/Jakarta")
	at := func(d, h int) time.Time { return time.Date(2026, 10, d, h, 0, 0, 0, jkt) }
	shifts := []RecapShift{
		// Day 1: late + present -> late.
		{GuardID: "g", Start: at(1, 7), Status: "late", Hours: 7.5},
		{GuardID: "g", Start: at(1, 19), Status: "present", Hours: 8},
		// Day 2: absent + present -> absent (early out still counted).
		{GuardID: "g", Start: at(2, 7), Status: "present", EarlyOut: true, Hours: 6},
		{GuardID: "g", Start: at(2, 19), Status: "absent"},
		// Day 3: absent + late -> absent.
		{GuardID: "g", Start: at(3, 7), Status: "late"},
		{GuardID: "g", Start: at(3, 19), Status: "absent"},
		// Day 4: present + pending (ongoing) -> present.
		{GuardID: "g", Start: at(4, 7), Status: "present", Hours: 8},
		{GuardID: "g", Start: at(4, 19), Status: "pending"},
		// Day 5: all pending -> upcoming, not absent.
		{GuardID: "g", Start: at(5, 7), Status: "pending"},
		{GuardID: "g", Start: at(5, 19), Status: "pending"},
		// Day 6: late and left early on both shifts -> one late day, one early-out day.
		{GuardID: "g", Start: at(6, 7), Status: "late", EarlyOut: true, Hours: 5},
		{GuardID: "g", Start: at(6, 19), Status: "present", EarlyOut: true, Hours: 5},
		// Another guard, same days, counted separately.
		{GuardID: "h", Start: at(1, 7), Status: "present", Hours: 8},
	}
	got := AggregateRecap(shifts, jkt)
	g := got["g"]
	want := RecapCounts{
		ScheduledDays: 6, DaysPresent: 1, DaysLate: 2, DaysAbsent: 2, DaysUpcoming: 1, EarlyOutDays: 2,
		ScheduledShifts: 12, Present: 4, Late: 3, Absent: 2, EarlyOut: 3, Pending: 3, TotalHours: 39.5,
	}
	if *g != want {
		t.Errorf("guard g:\n got %+v\nwant %+v", *g, want)
	}
	if h := got["h"]; h.ScheduledDays != 1 || h.DaysPresent != 1 || h.ScheduledShifts != 1 {
		t.Errorf("guard h = %+v", *h)
	}
}

// Days are calendar days of the shift start in the site's zone: the same two
// UTC instants form one or two days depending on the zone. Indonesian zones
// have no DST, so a day is always 24 h.
func TestAggregateRecapSiteTimeZone(t *testing.T) {
	shifts := []RecapShift{
		{GuardID: "g", Start: time.Date(2026, 10, 10, 15, 30, 0, 0, time.UTC), Status: "present"},
		{GuardID: "g", Start: time.Date(2026, 10, 10, 16, 30, 0, 0, time.UTC), Status: "absent"},
	}
	for _, c := range []struct {
		zone          string
		days, absents int
	}{
		{"UTC", 1, 1},            // 15:30, 16:30 on the 10th
		{"Asia/Jakarta", 1, 1},   // 22:30, 23:30 on the 10th
		{"Asia/Makassar", 2, 1},  // 23:30 on the 10th, 00:30 on the 11th
		{"Asia/Jayapura", 1, 1},  // 00:30, 01:30 on the 11th
		{"Asia/Singapore", 2, 1}, // +08 as Makassar
	} {
		g := AggregateRecap(shifts, loc(t, c.zone))["g"]
		if g.ScheduledDays != c.days || g.DaysAbsent != c.absents || g.DaysPresent != c.days-c.absents {
			t.Errorf("%s: %+v", c.zone, *g)
		}
	}
}

// A night shift belongs to the day it starts, even when it ends the next
// day or month (D-013).
func TestAggregateRecapOvernightShift(t *testing.T) {
	jkt := loc(t, "Asia/Jakarta")
	shifts := []RecapShift{
		{GuardID: "g", Start: time.Date(2026, 10, 31, 22, 0, 0, 0, jkt), Status: "late", Hours: 8},
		{GuardID: "g", Start: time.Date(2026, 10, 31, 6, 0, 0, 0, jkt), Status: "present", Hours: 8},
	}
	g := AggregateRecap(shifts, jkt)["g"]
	if g.ScheduledDays != 1 || g.DaysLate != 1 || g.TotalHours != 16 {
		t.Errorf("overnight: %+v", *g)
	}
}

func TestRecapTotals(t *testing.T) {
	rows := []MonthlyRecapRow{
		{GuardID: "a", RecapCounts: RecapCounts{ScheduledDays: 20, DaysPresent: 18, DaysLate: 1, DaysAbsent: 1,
			EarlyOutDays: 2, ScheduledShifts: 22, Present: 20, TotalHours: 160.4, PatrolSessions: 3, PatrolCompletePct: 100}},
		{GuardID: "b", RecapCounts: RecapCounts{ScheduledDays: 10, DaysPresent: 8, DaysUpcoming: 2,
			ScheduledShifts: 10, Pending: 2, TotalHours: 64.3, PatrolSessions: 1, PatrolCompletePct: 60}},
		{GuardID: "c", RecapCounts: RecapCounts{}},
	}
	tot := RecapTotals(rows)
	want := MonthlyRecapTotals{Guards: 3, RecapCounts: RecapCounts{ScheduledDays: 30, DaysPresent: 26, DaysLate: 1,
		DaysAbsent: 1, DaysUpcoming: 2, EarlyOutDays: 2, ScheduledShifts: 32, Present: 20, Pending: 2,
		TotalHours: 224.7, PatrolSessions: 4, PatrolCompletePct: 90}}
	if tot != want {
		t.Errorf("totals:\n got %+v\nwant %+v", tot, want)
	}
	if empty := RecapTotals(nil); empty.Guards != 0 || empty.PatrolCompletePct != 0 {
		t.Errorf("empty totals = %+v", empty)
	}
}
