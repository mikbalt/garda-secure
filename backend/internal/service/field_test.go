package service

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/securepatrol/backend/internal/fieldauth"
	"github.com/securepatrol/backend/internal/model"
)

func testSigner() *fieldauth.QRSigner {
	return fieldauth.NewQRSigner(fieldauth.DeriveKey("", "unit-test-key", fieldauth.QRInfo))
}

func TestRouteQRAssignment(t *testing.T) {
	qr := testSigner()
	r := model.Route{Name: "R", Checkpoints: []model.Checkpoint{
		{Name: "Gate", OrderIndex: 0},
		{Name: "Dock", OrderIndex: 1, QRCode: "CUSTOM-DOCK"},
		{Name: "Roof", OrderIndex: 2, QRCode: "SP1:forged:sig"},
	}}
	if err := NormalizeRoute(&r, nil, qr); err != nil {
		t.Fatal(err)
	}
	gate, dock, roof := r.Checkpoints[0], r.Checkpoints[1], r.Checkpoints[2]
	if !gate.QRSigned || gate.QRVersion != 1 || gate.QRCode != qr.Payload(gate.ID, 1) {
		t.Fatalf("new checkpoint not signed v1: %+v", gate)
	}
	if dock.QRSigned || dock.QRCode != "CUSTOM-DOCK" {
		t.Fatalf("custom code not kept as legacy: %+v", dock)
	}
	if !roof.QRSigned || roof.QRCode != qr.Payload(roof.ID, 1) {
		t.Fatalf("client-supplied SP1 text must be replaced by the server payload: %+v", roof)
	}

	// Update: the client echoes the codes back, tries to change a signed
	// one and clears the legacy one.
	prev := r
	prev.Checkpoints = slices.Clone(r.Checkpoints)
	upd := model.Route{Name: "R", Checkpoints: []model.Checkpoint{
		{ID: gate.ID, Name: "Gate", QRCode: "MY-OWN", QRVersion: 9, OrderIndex: 0},
		{ID: dock.ID, Name: "Dock", QRCode: "", OrderIndex: 1},
		{ID: roof.ID, Name: "Roof", QRCode: roof.QRCode, OrderIndex: 2},
		{Name: "Lobby", QRCode: "LOBBY-1", OrderIndex: 3},
	}}
	if err := NormalizeRoute(&upd, &prev, qr); err != nil {
		t.Fatal(err)
	}
	if c := upd.Checkpoints[0]; !c.QRSigned || c.QRVersion != 1 || c.QRCode != gate.QRCode {
		t.Fatalf("signed checkpoint must stay signed with its version: %+v", c)
	}
	if c := upd.Checkpoints[1]; !c.QRSigned || c.QRVersion != 1 {
		t.Fatalf("cleared legacy code should become signed: %+v", c)
	}
	if c := upd.Checkpoints[3]; c.QRSigned || c.QRCode != "LOBBY-1" {
		t.Fatalf("new custom code: %+v", c)
	}

	RegenerateQR(&upd, qr)
	for _, c := range upd.Checkpoints {
		if !c.QRSigned || c.QRCode != qr.Payload(c.ID, c.QRVersion) {
			t.Fatalf("regenerate: %+v", c)
		}
	}
	if upd.Checkpoints[0].QRVersion != 2 || upd.Checkpoints[3].QRVersion != 1 {
		t.Fatalf("regenerate versions: %d, %d", upd.Checkpoints[0].QRVersion, upd.Checkpoints[3].QRVersion)
	}

	stale := upd
	stale.Checkpoints = slices.Clone(upd.Checkpoints)
	stale.Checkpoints[0].QRCode = "stale"
	DecorateQR(&stale, qr)
	if stale.Checkpoints[0].QRCode != upd.Checkpoints[0].QRCode {
		t.Fatal("DecorateQR did not restore the payload")
	}
	p := SessionProgress{Checkpoints: []CheckpointProgress{{Checkpoint: upd.Checkpoints[0]}, {Checkpoint: dock}}}
	HideSignedQR(&p)
	if p.Checkpoints[0].QRCode != "" || p.Checkpoints[1].QRCode != "CUSTOM-DOCK" {
		t.Fatalf("HideSignedQR: %+v", p.Checkpoints)
	}

	// Without a signer (legacy behaviour) codes are generated in plain text.
	plain := model.Route{Name: "P", Checkpoints: []model.Checkpoint{{Name: "A"}}}
	if err := NormalizeRoute(&plain, nil, nil); err != nil || plain.Checkpoints[0].QRSigned ||
		!strings.HasPrefix(plain.Checkpoints[0].QRCode, "SP-") {
		t.Fatalf("plain route: %+v %v", plain.Checkpoints[0], err)
	}
}

func TestResolveCheckpointQR(t *testing.T) {
	qr := testSigner()
	r := model.Route{Name: "R", Checkpoints: []model.Checkpoint{
		{Name: "Signed"}, {Name: "Legacy", QRCode: "QR-LEGACY"},
	}}
	if err := NormalizeRoute(&r, nil, qr); err != nil {
		t.Fatal(err)
	}
	signed, legacy := r.Checkpoints[0], r.Checkpoints[1]
	cases := []struct {
		name   string
		in     ScanInput
		wantID string
		flag   string
	}{
		{"valid signed payload", ScanInput{QRPayload: signed.QRCode, Method: "qr"}, signed.ID, ""},
		{"signed payload sent as qr_code", ScanInput{QRCode: signed.QRCode, Method: "qr"}, signed.ID, ""},
		{"old version", ScanInput{QRPayload: qr.Payload(signed.ID, 0), Method: "qr"}, signed.ID, FlagQRInvalid},
		{"forged signature", ScanInput{QRPayload: "SP1:" + signed.ID + ":AAAAAAAAAAAAAAAAAAAAAA", Method: "qr"}, signed.ID, FlagQRInvalid},
		{"payload for a legacy checkpoint", ScanInput{QRPayload: qr.Payload(legacy.ID, 1), Method: "qr"}, legacy.ID, FlagQRInvalid},
		{"payload of another route", ScanInput{QRPayload: qr.Payload("9d5c1c35-0000-4000-8000-000000000000", 1), Method: "qr"}, "", ""},
		{"legacy code", ScanInput{QRCode: "QR-LEGACY", Method: "qr"}, legacy.ID, FlagQRUnsigned},
		{"legacy code as raw payload", ScanInput{QRPayload: "QR-LEGACY", Method: "qr"}, legacy.ID, FlagQRUnsigned},
		{"unknown text", ScanInput{QRPayload: "nope", Method: "qr"}, "", ""},
		{"checkpoint id only", ScanInput{CheckpointID: signed.ID, Method: "qr"}, signed.ID, FlagQRUnsigned},
		{"manual check-in", ScanInput{CheckpointID: signed.ID, Method: "manual"}, signed.ID, ""},
		{"signed code text is not a legacy code", ScanInput{QRPayload: "x" + signed.QRCode, Method: "qr"}, "", ""},
	}
	for _, tc := range cases {
		tc.in.QR = qr
		cp, flag := resolveCheckpoint(&r, tc.in)
		gotID := ""
		if cp != nil {
			gotID = cp.ID
		}
		if gotID != tc.wantID || flag != tc.flag {
			t.Errorf("%s: checkpoint %q flag %q, want %q %q", tc.name, gotID, flag, tc.wantID, tc.flag)
		}
	}
}

func TestGuardFlags(t *testing.T) {
	orig := Now
	now := time.Date(2026, 10, 12, 10, 0, 0, 0, time.UTC)
	Now = func() time.Time { return now }
	defer func() { Now = orig }()
	if f := GuardFlags(nil, now); !slices.Equal(f, []string{FlagGuardUnverified}) {
		t.Fatalf("no token: %v", f)
	}
	valid := &fieldauth.GuardAuth{Present: true, Usable: true, ExpiresAt: now.Add(time.Hour)}
	if f := GuardFlags(valid, now); len(f) != 0 {
		t.Fatalf("valid token: %v", f)
	}
	expired := &fieldauth.GuardAuth{Present: true, Usable: true, ExpiresAt: now.Add(-time.Hour)}
	if f := GuardFlags(expired, now.Add(-2*time.Hour)); len(f) != 0 {
		t.Fatalf("queued record made while the token was valid: %v", f)
	}
	if f := GuardFlags(expired, now); len(f) != 1 {
		t.Fatalf("record made after expiry: %v", f)
	}
}

func TestValidateSchedules(t *testing.T) {
	ip := func(v int) *int { return &v }
	sp := func(v string) *string { return &v }
	ok, err := ValidateSchedules([]ScheduleInput{
		{Weekdays: []int16{5, 1, 1}, StartTime: "08:00"},
		{Weekdays: []int16{1, 2, 3, 4, 5, 6, 7}, StartTime: "22:00", EndTime: sp("06:00"), EveryMinutes: ip(120), GraceMinutes: ip(30)},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(ok[0].Weekdays, []int16{1, 5}) || ok[0].GraceMinutes != DefaultGraceMinutes || !ok[0].Active {
		t.Fatalf("normalised = %+v", ok[0])
	}
	bad := map[string]ScheduleInput{
		"invalid_weekdays":  {Weekdays: []int16{0}, StartTime: "08:00"},
		"invalid_start":     {Weekdays: []int16{1}, StartTime: "8am"},
		"end_time_required": {Weekdays: []int16{1}, StartTime: "08:00", EveryMinutes: ip(60)},
		"invalid_every":     {Weekdays: []int16{1}, StartTime: "08:00", EndTime: sp("09:00"), EveryMinutes: ip(5)},
		"grace_too_long":    {Weekdays: []int16{1}, StartTime: "08:00", EndTime: sp("12:00"), EveryMinutes: ip(30), GraceMinutes: ip(20)},
		"invalid_grace":     {Weekdays: []int16{1}, StartTime: "08:00", GraceMinutes: ip(-1)},
		"invalid_end_time":  {Weekdays: []int16{1}, StartTime: "08:00", EndTime: sp("25:00")},
	}
	for name, in := range bad {
		if _, err := ValidateSchedules([]ScheduleInput{in}); err == nil {
			t.Errorf("%s accepted", name)
		}
	}
	if _, err := ValidateSchedules(make([]ScheduleInput, MaxSchedulesPerRoute+1)); err == nil {
		t.Error("too many windows accepted")
	}
}

func TestScheduleOccurrences(t *testing.T) {
	loc := jakarta(t) // UTC+7, no DST
	ip := func(v int) *int { return &v }
	sp := func(v string) *string { return &v }
	local := func(d, hh, mm int) time.Time { return time.Date(2026, 10, d, hh, mm, 0, 0, loc).UTC() }
	// 2026-10-05 is a Monday.
	from, to := local(5, 0, 0), local(7, 0, 0) // (Mon 00:00, Wed 00:00]

	once := model.RouteSchedule{Weekdays: []int16{1, 2}, StartTime: "08:00", GraceMinutes: 15}
	occ, err := ScheduleOccurrences(once, loc, from, to)
	if err != nil {
		t.Fatal(err)
	}
	if len(occ) != 2 || !occ[0].At.Equal(local(5, 8, 0)) || !occ[1].At.Equal(local(6, 8, 0)) ||
		!occ[0].GraceUntil.Equal(local(5, 8, 15)) {
		t.Fatalf("once = %+v", occ)
	}

	// Night window Sunday 22:00 → Monday 06:00 every 2 h: Sunday's window
	// reaches into Monday (00:00 is excluded by the open range start, 02:00
	// and 04:00 are in; 06:00 is the excluded end).
	night := model.RouteSchedule{Weekdays: []int16{7, 1}, StartTime: "22:00", EndTime: sp("06:00"), EveryMinutes: ip(120), GraceMinutes: 10}
	occ, err = ScheduleOccurrences(night, loc, from, to)
	if err != nil {
		t.Fatal(err)
	}
	want := []time.Time{local(5, 2, 0), local(5, 4, 0), local(5, 22, 0), local(6, 0, 0), local(6, 2, 0), local(6, 4, 0)}
	if len(occ) != len(want) {
		t.Fatalf("night = %d occurrences %+v, want %d", len(occ), occ, len(want))
	}
	for i := range want {
		if !occ[i].At.Equal(want[i]) {
			t.Errorf("night[%d] = %v, want %v", i, occ[i].At.In(loc), want[i].In(loc))
		}
	}

	// Equal start and end = a 24 h window.
	allDay := model.RouteSchedule{Weekdays: []int16{1}, StartTime: "06:00", EndTime: sp("06:00"), EveryMinutes: ip(360), GraceMinutes: 15}
	occ, _ = ScheduleOccurrences(allDay, loc, from, to)
	if len(occ) != 4 || !occ[3].At.Equal(local(6, 0, 0)) {
		t.Fatalf("24h window = %+v", occ)
	}

	// UTC zone: same arithmetic, no surprises.
	utcOcc, _ := ScheduleOccurrences(once, time.UTC, time.Date(2026, 10, 5, 0, 0, 0, 0, time.UTC), time.Date(2026, 10, 6, 0, 0, 0, 0, time.UTC))
	if len(utcOcc) != 1 || !utcOcc[0].At.Equal(time.Date(2026, 10, 5, 8, 0, 0, 0, time.UTC)) {
		t.Fatalf("utc = %+v", utcOcc)
	}
}

// D-059: checkpoint GPS positions are server-owned (learned from scans).
func TestRouteKeepsLearnedPositions(t *testing.T) {
	lat, lng := -6.2, 106.8
	prev := model.Route{Name: "R", Checkpoints: []model.Checkpoint{{ID: "a", Name: "Gate", QRCode: "A", Lat: &lat, Lng: &lng}}}
	bogus := 1.0
	upd := model.Route{Name: "R", Checkpoints: []model.Checkpoint{
		{ID: "a", Name: "Gate", QRCode: "A"},
		{Name: "New", QRCode: "B", OrderIndex: 1, Lat: &bogus, Lng: &bogus},
	}}
	if err := NormalizeRoute(&upd, &prev, nil); err != nil {
		t.Fatal(err)
	}
	if c := upd.Checkpoints[0]; c.Lat == nil || *c.Lat != lat || *c.Lng != lng {
		t.Fatalf("stored position not kept: %+v", c)
	}
	if c := upd.Checkpoints[1]; c.Lat != nil || c.Lng != nil {
		t.Fatalf("submitted position not ignored: %+v", c)
	}
}
