package sla

import (
	"testing"
	"time"

	"github.com/securepatrol/backend/internal/model"
	"github.com/securepatrol/backend/internal/repository"
	"github.com/securepatrol/backend/internal/service"
)

var t0 = time.Date(2026, 10, 9, 20, 0, 0, 0, time.UTC)

func ptr[T any](v T) *T { return &v }

// route: A (5 min SLA), B (5 min after A), C (no SLA).
func route() model.Route {
	return model.Route{ID: "r", Name: "Night", Checkpoints: []model.Checkpoint{
		{ID: "A", Name: "Gate", SLAMinutes: ptr(5)},
		{ID: "B", Name: "Warehouse", SLAMinutes: ptr(5)},
		{ID: "C", Name: "Parking"},
	}}
}

func session(status string) model.PatrolSession {
	return model.PatrolSession{ID: "s", RouteID: "r", StartedAt: t0, Status: status}
}

func scan(cp string, at time.Time) model.CheckpointScan {
	return model.CheckpointScan{ID: "scan-" + cp, CheckpointID: cp, ScannedAt: at, Method: "qr"}
}

func ids(cps []service.CheckpointProgress) []string {
	out := []string{}
	for _, c := range cps {
		out = append(out, c.ID)
	}
	return out
}

func eq(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestBreaches(t *testing.T) {
	online := &repository.DeviceContact{OnlineSince: ptr(t0.Add(-time.Hour)), LastSeen: ptr(t0.Add(20 * time.Minute))}
	reconnected := &repository.DeviceContact{OnlineSince: ptr(t0.Add(20 * time.Minute)), LastSeen: ptr(t0.Add(20 * time.Minute))}
	cases := []struct {
		name    string
		status  string
		scans   []model.CheckpointScan
		contact *repository.DeviceContact
		now     time.Time
		want    []string
	}{
		{"online tablet, A not scanned past due+grace", "in_progress", nil, online,
			t0.Add(6*time.Minute + time.Second), []string{"A"}},
		{"within grace: no alarm yet", "in_progress", nil, online,
			t0.Add(5*time.Minute + 30*time.Second), []string{}},
		{"tablet offline at due time (reconnected later): wait for the scans", "in_progress", nil, reconnected,
			t0.Add(21 * time.Minute), []string{}},
		{"offline scans synced later, on time by event time: no alarm", "in_progress",
			[]model.CheckpointScan{scan("A", t0.Add(4*time.Minute)), scan("B", t0.Add(8*time.Minute))}, reconnected,
			t0.Add(21 * time.Minute), []string{}},
		{"later scan arrived: earlier unscanned checkpoint is certainly missed", "in_progress",
			[]model.CheckpointScan{scan("B", t0.Add(7*time.Minute))}, reconnected,
			t0.Add(8 * time.Minute), []string{"A"}},
		{"late scan is a breach", "in_progress",
			[]model.CheckpointScan{scan("A", t0.Add(7*time.Minute))}, online,
			t0.Add(8 * time.Minute), []string{"A"}},
		{"session ended early with unscanned SLA checkpoints", "completed",
			[]model.CheckpointScan{scan("A", t0.Add(2*time.Minute))}, reconnected,
			t0.Add(3 * time.Minute), []string{"B"}},
		{"abandoned offline session: all SLA checkpoints missed", "abandoned", nil, nil,
			t0.Add(time.Minute), []string{"A", "B"}},
		{"unknown device: time decides", "in_progress", nil, nil,
			t0.Add(7 * time.Minute), []string{"A"}},
		{"device never seen: no alarm until end", "in_progress", nil, &repository.DeviceContact{},
			t0.Add(30 * time.Minute), []string{}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p := service.ComputeProgress(route(), session(c.status), c.scans, c.now)
			got := ids(Breaches(p, c.contact, c.now))
			if !eq(got, c.want) {
				t.Errorf("breaches = %v, want %v (statuses: %v)", got, c.want, statuses(p))
			}
		})
	}
}

func statuses(p service.SessionProgress) []string {
	out := []string{}
	for _, c := range p.Checkpoints {
		out = append(out, c.ID+"="+c.Status)
	}
	return out
}
