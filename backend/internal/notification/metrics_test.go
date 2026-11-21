package notification

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/securepatrol/backend/internal/metrics"
)

// A failure counter must exist at 0 before the first failure: the
// NotificationsFailing alert uses increase(), which sees no rise in a series
// whose first sample is already 1.
func TestFailureCountersExistBeforeTheFirstFailure(t *testing.T) {
	initFailedSeries()
	reg := prometheus.NewRegistry()
	reg.MustRegister(metrics.NotificationsFailed)
	families, err := reg.Gather()
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, f := range families {
		for _, m := range f.GetMetric() {
			for _, l := range m.GetLabel() {
				if l.GetName() == "event" {
					seen[l.GetValue()] = true
				}
			}
		}
	}
	for _, ev := range []string{EventSOS, EventSOSUnacknowledged, EventAbsent, EventOpsSOSUndelivered, EventOpsBackupFailed} {
		if !seen[ev] {
			t.Errorf("sp_notifications_failed_total has no series for %q before any failure", ev)
		}
	}
}
