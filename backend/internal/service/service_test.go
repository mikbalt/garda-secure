package service

import (
	"testing"
	"time"

	"github.com/securepatrol/backend/internal/model"
)

func jakarta(t *testing.T) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation("Asia/Jakarta")
	if err != nil {
		t.Fatal(err)
	}
	return loc
}

func TestOccurrencesDayAndNightShift(t *testing.T) {
	loc := jakarta(t)
	tpl := model.ShiftTemplate{
		StartTime: "22:00", EndTime: "06:00", Weekdays: []int16{1, 3}, // Mon, Wed
		ValidFrom: "2026-10-01",
	}
	// 2026-10-05 is a Monday.
	from := time.Date(2026, 10, 5, 0, 0, 0, 0, loc)
	occ, err := Occurrences(tpl, loc, from, from.AddDate(0, 0, 6))
	if err != nil {
		t.Fatal(err)
	}
	if len(occ) != 2 {
		t.Fatalf("want 2 occurrences (Mon, Wed), got %d", len(occ))
	}
	wantStart := time.Date(2026, 10, 5, 22, 0, 0, 0, loc).UTC()
	wantEnd := time.Date(2026, 10, 6, 6, 0, 0, 0, loc).UTC()
	if !occ[0].Start.Equal(wantStart) || !occ[0].End.Equal(wantEnd) {
		t.Errorf("night shift = %v–%v, want %v–%v", occ[0].Start, occ[0].End, wantStart, wantEnd)
	}
	if occ[1].Start.In(loc).Weekday() != time.Wednesday {
		t.Errorf("second occurrence on %v, want Wednesday", occ[1].Start.In(loc).Weekday())
	}
}

func TestOccurrencesRespectsValidity(t *testing.T) {
	loc := jakarta(t)
	until := "2026-10-07"
	tpl := model.ShiftTemplate{
		StartTime: "07:00", EndTime: "15:00", Weekdays: []int16{1, 2, 3, 4, 5, 6, 7},
		ValidFrom: "2026-10-06", ValidUntil: &until,
	}
	from := time.Date(2026, 10, 1, 0, 0, 0, 0, loc)
	occ, err := Occurrences(tpl, loc, from, from.AddDate(0, 0, 30))
	if err != nil {
		t.Fatal(err)
	}
	if len(occ) != 2 {
		t.Fatalf("want 2 occurrences (6th and 7th), got %d", len(occ))
	}
}

func TestValidateTemplate(t *testing.T) {
	ok := model.ShiftTemplate{StartTime: "07:00", EndTime: "15:00", Weekdays: []int16{5, 1, 1}, ValidFrom: "2026-10-01"}
	if err := ValidateTemplate(&ok); err != nil {
		t.Fatalf("valid template rejected: %v", err)
	}
	if len(ok.Weekdays) != 2 || ok.Weekdays[0] != 1 {
		t.Errorf("weekdays not normalised: %v", ok.Weekdays)
	}
	bad := []model.ShiftTemplate{
		{StartTime: "7am", EndTime: "15:00", Weekdays: []int16{1}, ValidFrom: "2026-10-01"},
		{StartTime: "07:00", EndTime: "07:00", Weekdays: []int16{1}, ValidFrom: "2026-10-01"},
		{StartTime: "07:00", EndTime: "15:00", Weekdays: []int16{8}, ValidFrom: "2026-10-01"},
		{StartTime: "07:00", EndTime: "15:00", Weekdays: nil, ValidFrom: "2026-10-01"},
		{StartTime: "07:00", EndTime: "15:00", Weekdays: []int16{1}, ValidFrom: "01/10/2026"},
	}
	for i, b := range bad {
		if err := ValidateTemplate(&b); err == nil {
			t.Errorf("case %d: expected error", i)
		}
	}
}

func TestArrivalStatus(t *testing.T) {
	start := time.Date(2026, 10, 7, 7, 0, 0, 0, time.UTC)
	cases := []struct {
		at          time.Time
		status      string
		lateMinutes int
	}{
		{start.Add(-10 * time.Minute), "present", 0},
		{start.Add(5 * time.Minute), "present", 0}, // within 5-minute grace
		{start.Add(5*time.Minute + time.Second), "late", 5},
		{start.Add(42 * time.Minute), "late", 42},
	}
	for _, c := range cases {
		s, m := arrivalStatus(start, c.at, 5)
		if s != c.status || m != c.lateMinutes {
			t.Errorf("at %v: got %s/%d, want %s/%d", c.at.Sub(start), s, m, c.status, c.lateMinutes)
		}
	}
}

func TestEarlyOutMinutes(t *testing.T) {
	end := time.Date(2026, 10, 7, 15, 0, 0, 0, time.UTC)
	if m := earlyOutMinutes(end, end.Add(-3*time.Minute), 5); m != 0 {
		t.Errorf("within grace: got %d", m)
	}
	if m := earlyOutMinutes(end, end.Add(-90*time.Minute), 5); m != 90 {
		t.Errorf("90 min early: got %d", m)
	}
	if m := earlyOutMinutes(end, end.Add(10*time.Minute), 5); m != 0 {
		t.Errorf("after end: got %d", m)
	}
}

func TestDayRangeUsesSiteZone(t *testing.T) {
	loc := jakarta(t)
	from, to, err := DayRange("2026-10-07", loc)
	if err != nil {
		t.Fatal(err)
	}
	// Midnight WIB = 17:00 UTC the previous day.
	if want := time.Date(2026, 10, 6, 17, 0, 0, 0, time.UTC); !from.Equal(want) {
		t.Errorf("from = %v, want %v", from, want)
	}
	if to.Sub(from) != 24*time.Hour {
		t.Errorf("range = %v, want 24h", to.Sub(from))
	}
	if _, _, err := DayRange("07-10-2026", loc); err == nil {
		t.Error("bad date accepted")
	}
}

func TestDateRangeLimits(t *testing.T) {
	loc := jakarta(t)
	if _, _, err := DateRange("2026-10-07", "2026-10-01", loc, 31); err == nil {
		t.Error("reversed range accepted")
	}
	if _, _, err := DateRange("2026-01-01", "2026-12-31", loc, 31); err == nil {
		t.Error("over-long range accepted")
	}
	from, to, err := DateRange("2026-10-01", "2026-10-07", loc, 31)
	if err != nil || to.Sub(from) != 7*24*time.Hour {
		t.Errorf("7-day inclusive range: %v %v", to.Sub(from), err)
	}
}

func TestClientTimeWindow(t *testing.T) {
	now := time.Date(2026, 10, 7, 12, 0, 0, 0, time.UTC)
	orig := Now
	Now = func() time.Time { return now }
	defer func() { Now = orig }()

	if got, err := ClientTime(nil); err != nil || !got.Equal(now) {
		t.Errorf("nil → %v, %v", got, err)
	}
	past := now.Add(-48 * time.Hour)
	if got, err := ClientTime(&past); err != nil || !got.Equal(past) {
		t.Errorf("2 days ago rejected: %v", err)
	}
	old := now.Add(-8 * 24 * time.Hour)
	if _, err := ClientTime(&old); err == nil {
		t.Error("8 days ago accepted")
	}
	future := now.Add(10 * time.Minute)
	if _, err := ClientTime(&future); err == nil {
		t.Error("10 minutes in the future accepted")
	}
}

func intp(v int) *int { return &v }

func TestComputeProgressSLA(t *testing.T) {
	start := time.Date(2026, 10, 7, 20, 0, 0, 0, time.UTC)
	route := model.Route{Name: "Night", Checkpoints: []model.Checkpoint{
		{ID: "a", Name: "Gate", SLAMinutes: intp(10)},
		{ID: "b", Name: "Warehouse", SLAMinutes: intp(15)},
		{ID: "c", Name: "Parking"}, // no SLA
		{ID: "d", Name: "Roof", SLAMinutes: intp(5)},
	}}
	session := model.PatrolSession{ID: "s", StartedAt: start, Status: "in_progress"}
	scans := []model.CheckpointScan{
		{ID: "1", CheckpointID: "a", ScannedAt: start.Add(8 * time.Minute)},  // on time (due +10)
		{ID: "2", CheckpointID: "b", ScannedAt: start.Add(30 * time.Minute)}, // due 8+15=23 → late
		{ID: "3", CheckpointID: "a", ScannedAt: start.Add(40 * time.Minute)}, // duplicate, ignored
	}
	p := ComputeProgress(route, session, scans, start.Add(45*time.Minute))

	want := []string{"on_time", "late", "pending", "overdue"}
	for i, cp := range p.Checkpoints {
		if cp.Status != want[i] {
			t.Errorf("checkpoint %s: status %s, want %s", cp.ID, cp.Status, want[i])
		}
	}
	if p.Scanned != 2 || p.Total != 4 || p.CompletionPct != 50 {
		t.Errorf("scanned %d/%d (%.1f%%)", p.Scanned, p.Total, p.CompletionPct)
	}
	if p.NextIndex == nil || *p.NextIndex != 2 {
		t.Errorf("next index = %v, want 2", p.NextIndex)
	}
	// "d" has an SLA anchored on b's scan (30) — but c is unscanned without SLA,
	// so the anchor stays at b's scan time: due = 30 + 5 = 35 → overdue at 45.
	p = ComputeProgress(route, session, scans[:2], start.Add(45*time.Minute))
	if p.Checkpoints[3].Status != "overdue" {
		t.Errorf("roof status = %s, want overdue", p.Checkpoints[3].Status)
	}
	if due := p.Checkpoints[3].DueAt; due == nil || !due.Equal(start.Add(35*time.Minute)) {
		t.Errorf("roof due = %v", due)
	}

	ended := session
	ended.Status = "completed"
	p = ComputeProgress(route, ended, scans, start.Add(45*time.Minute))
	if p.Checkpoints[2].Status != "missed" || p.NextIndex != nil {
		t.Errorf("ended session: parking %s, next %v", p.Checkpoints[2].Status, p.NextIndex)
	}
}

func TestNormalizeRoute(t *testing.T) {
	r := model.Route{Name: " Night ", Checkpoints: []model.Checkpoint{
		{Name: "B", OrderIndex: 5},
		{Name: "A", OrderIndex: 1, QRCode: "QR-A"},
	}}
	if err := NormalizeRoute(&r, nil, nil); err != nil {
		t.Fatal(err)
	}
	if r.Name != "Night" || r.Checkpoints[0].Name != "A" || r.Checkpoints[1].OrderIndex != 1 {
		t.Errorf("not normalised: %+v", r)
	}
	if r.Checkpoints[1].ID == "" || r.Checkpoints[1].QRCode == "" {
		t.Error("missing generated id/qr code")
	}
	dup := model.Route{Name: "x", Checkpoints: []model.Checkpoint{{Name: "A", QRCode: "Q"}, {Name: "B", QRCode: "Q"}}}
	if err := NormalizeRoute(&dup, nil, nil); err == nil {
		t.Error("duplicate QR codes accepted")
	}
	badSLA := model.Route{Name: "x", Checkpoints: []model.Checkpoint{{Name: "A", SLAMinutes: intp(0)}}}
	if err := NormalizeRoute(&badSLA, nil, nil); err == nil {
		t.Error("zero SLA accepted")
	}
}

func TestCheckObjectKey(t *testing.T) {
	site := "0b6a7c3e-1f2d-4c5b-9a8e-7d6c5b4a3f21"
	good := ObjectKey(site, "incident", "8f14e45f-ceea-467f-a8f8-1b2c3d4e5f60", "jpg")
	if err := CheckObjectKey(&good, site); err != nil {
		t.Errorf("good key rejected: %v", err)
	}
	for _, k := range []string{
		"sites/" + site + "/incident/../../etc/passwd",
		"sites/11111111-1111-1111-1111-111111111111/incident/8f14e45f-ceea-467f-a8f8-1b2c3d4e5f60.jpg",
		"sites/" + site + "/other/8f14e45f-ceea-467f-a8f8-1b2c3d4e5f60.jpg",
	} {
		k := k
		if err := CheckObjectKey(&k, site); err == nil {
			t.Errorf("bad key accepted: %s", k)
		}
	}
}
