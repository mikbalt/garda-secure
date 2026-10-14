package jobs

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestEscalationDueSchedule(t *testing.T) {
	recv := time.Date(2026, 10, 14, 10, 0, 0, 0, time.UTC)
	after, every := 3*time.Minute, 5*time.Minute
	at := func(d time.Duration) time.Time { return recv.Add(d) }
	cases := []struct {
		now  time.Duration
		done int
		want int
	}{
		{2*time.Minute + 59*time.Second, 0, 0}, // not yet
		{3 * time.Minute, 0, 1},                // first reminder at +3m
		{4 * time.Minute, 1, 0},                // already sent
		{8 * time.Minute, 1, 2},                // +3m+5m
		{12 * time.Minute, 2, 0},
		{13 * time.Minute, 2, 3}, // +3m+10m, the last one
		{17 * time.Minute, 3, 0},
		{18 * time.Minute, 2, 0}, // third window over (outage): stale, never late spam
		{14 * time.Minute, 0, 3}, // after an outage only the latest due one
		{9 * time.Minute, 0, 2},  // ... not a burst of 1 and 2
		{-time.Minute, 0, 0},     // clock oddities
		{30 * time.Minute, 0, 0}, // long gone
		{3 * time.Minute, 3, 0},  // done
		{3 * time.Minute, 10, 0}, // more than max recorded
		{13 * time.Minute, 1, 3}, // skipped 2 while down
		{12*time.Minute + 59*time.Second, 1, 2},
	}
	for _, c := range cases {
		if got := EscalationDue(recv, at(c.now), c.done, after, every); got != c.want {
			t.Errorf("at +%s with %d done: %d, want %d", c.now, c.done, got, c.want)
		}
	}
	if EscalationDue(recv, at(time.Hour), 0, after, 0) != 0 {
		t.Error("zero interval must not escalate")
	}
}

func TestUnhealthyReasons(t *testing.T) {
	loc, _ := time.LoadLocation("Asia/Jakarta")
	now := time.Date(2026, 10, 14, 15, 0, 0, 0, loc)
	ptr := func(t time.Time) *time.Time { return &t }
	beacon := func(pending int, lastSync *time.Time, offsetMs int64) json.RawMessage {
		b, _ := json.Marshal(map[string]any{"outbox_pending": pending, "last_sync": lastSync, "clock_offset_ms": offsetMs})
		return b
	}
	fresh := ptr(now.Add(-5 * time.Minute))

	if r := UnhealthyReasons(beacon(0, ptr(now.Add(-2*time.Minute)), 500), fresh, fresh, now, loc, "id"); len(r) != 0 {
		t.Errorf("healthy tablet: %v", r)
	}
	if r := UnhealthyReasons(nil, nil, fresh, now, loc, "id"); len(r) != 0 {
		t.Errorf("tablet without beacons (old app): %v", r)
	}
	r := UnhealthyReasons(beacon(7, ptr(now.Add(-90*time.Minute)), 0), fresh, fresh, now, loc, "en")
	if len(r) != 1 || !strings.Contains(r[0], "7 records not synced") || !strings.Contains(r[0], "13:30") {
		t.Errorf("stuck outbox: %v", r)
	}
	if r := UnhealthyReasons(beacon(7, ptr(now.Add(-30*time.Minute)), 0), fresh, fresh, now, loc, "en"); len(r) != 0 {
		t.Errorf("recent sync is not stuck: %v", r)
	}
	if r := UnhealthyReasons(beacon(3, nil, 0), fresh, fresh, now, loc, "id"); len(r) != 1 || !strings.Contains(r[0], "3 data belum terkirim") {
		t.Errorf("pending, never synced: %v", r)
	}
	r = UnhealthyReasons(beacon(0, ptr(now), -180_000), fresh, fresh, now, loc, "id")
	if len(r) != 1 || !strings.Contains(r[0], "selisih 3 menit") {
		t.Errorf("clock skew: %v", r)
	}
	if r := UnhealthyReasons(beacon(0, ptr(now), 119_000), fresh, fresh, now, loc, "id"); len(r) != 0 {
		t.Errorf("skew under 2 min: %v", r)
	}
	// No beacon for over an hour, the tablet was active today.
	old := ptr(now.Add(-70 * time.Minute))
	r = UnhealthyReasons(beacon(9, nil, 900_000), old, old, now, loc, "en")
	if len(r) != 1 || !strings.Contains(r[0], "no status beacon since 13:50") {
		t.Errorf("missing beacon: %v (an old beacon must not add stale reasons)", r)
	}
	// ... but not when it was last active yesterday (site time).
	yesterday := ptr(time.Date(2026, 10, 13, 23, 0, 0, 0, loc))
	if r := UnhealthyReasons(beacon(0, nil, 0), yesterday, yesterday, now, loc, "en"); len(r) != 0 {
		t.Errorf("tablet idle since yesterday: %v", r)
	}
	// Seen today by other requests, beacons stopped yesterday evening.
	if r := UnhealthyReasons(beacon(0, nil, 0), yesterday, fresh, now, loc, "en"); len(r) != 1 {
		t.Errorf("requests today without beacons: %v", r)
	}
}
