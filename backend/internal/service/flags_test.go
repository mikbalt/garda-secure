package service

import (
	"math"
	"slices"
	"testing"
	"time"

	"github.com/securepatrol/backend/internal/model"
)

func f64(v float64) *float64 { return &v }

func TestTimeAndClockInFlags(t *testing.T) {
	now := time.Date(2026, 10, 9, 8, 0, 0, 0, time.UTC)
	if f := TimeFlags(now.Add(-time.Minute), now); len(f) != 0 {
		t.Errorf("fresh event flagged: %v", f)
	}
	if f := TimeFlags(now.Add(-11*time.Minute), now); !slices.Equal(f, []string{FlagLateSync}) {
		t.Errorf("late sync = %v", f)
	}
	if f := TimeFlags(now.Add(90*time.Second), now); !slices.Equal(f, []string{FlagClockAhead}) {
		t.Errorf("clock ahead = %v", f)
	}

	event := now.Add(-30 * time.Minute)
	online := now.Add(-time.Minute)
	offline := now.Add(-40 * time.Minute)
	if f, review := ClockInFlags(event, now, "present", &online); !review || !slices.Contains(f, FlagBackdatedOnTime) {
		t.Errorf("backdated on-time clock-in while online: %v %v", f, review)
	}
	if f, review := ClockInFlags(event, now, "present", &offline); review || slices.Contains(f, FlagBackdatedOnTime) || !slices.Contains(f, FlagLateSync) {
		t.Errorf("offline-queued clock-in: %v %v", f, review)
	}
	if _, review := ClockInFlags(event, now, "late", &online); review {
		t.Error("late clock-in needs no review (no advantage from backdating)")
	}
	if _, review := ClockInFlags(now.Add(-5*time.Minute), now, "present", &online); review {
		t.Error("5 minutes delay flagged")
	}
}

func TestScanFlags(t *testing.T) {
	gate := model.Checkpoint{ID: "a", Lat: f64(-6.2000), Lng: f64(106.8000)}
	wh := model.Checkpoint{ID: "b", Lat: f64(-6.2090), Lng: f64(106.8000)} // ~1 km south
	at := time.Date(2026, 10, 9, 8, 0, 0, 0, time.UTC)

	if d := DistanceMeters(*gate.Lat, *gate.Lng, *wh.Lat, *wh.Lng); math.Abs(d-1000) > 10 {
		t.Fatalf("distance = %.1f m, want ~1000", d)
	}
	cases := []struct {
		name string
		sc   ScanContext
		want []string
	}{
		{"at the checkpoint", ScanContext{RadiusMeters: 150, Checkpoint: gate, GPSLat: f64(-6.2001), GPSLng: f64(106.8001)}, []string{}},
		{"far away", ScanContext{RadiusMeters: 150, Checkpoint: wh, GPSLat: gate.Lat, GPSLng: gate.Lng}, []string{FlagGPSFar}},
		{"radius off", ScanContext{Checkpoint: wh, GPSLat: gate.Lat, GPSLng: gate.Lng}, []string{}},
		{"no checkpoint coordinates", ScanContext{RadiusMeters: 150, Checkpoint: model.Checkpoint{ID: "c"}, GPSLat: gate.Lat, GPSLng: gate.Lng}, []string{}},
		{"walking pace", ScanContext{Checkpoint: wh, Previous: &gate, PreviousAt: at.Add(-10 * time.Minute)}, []string{}},
		{"1 km in a minute", ScanContext{Checkpoint: wh, Previous: &gate, PreviousAt: at.Add(-time.Minute)}, []string{FlagTooFast}},
		{"same checkpoint again", ScanContext{Checkpoint: gate, Previous: &gate, PreviousAt: at}, []string{}},
	}
	for _, c := range cases {
		if got := ScanFlags(at, c.sc); !slices.Equal(got, c.want) {
			t.Errorf("%s: %v, want %v", c.name, got, c.want)
		}
	}
}
