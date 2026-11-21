package jobs

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/securepatrol/backend/internal/config"
	"github.com/securepatrol/backend/internal/metrics"
)

// Every job's error counter must exist at 0 before its first failure: the
// JobNotSucceeding alert uses increase(), which sees no rise in a series
// whose first sample is already 1.
func TestJobErrorCountersExistBeforeTheFirstFailure(t *testing.T) {
	s := &Scheduler{cfg: &config.Config{}}
	s.initMetrics()
	reg := prometheus.NewRegistry()
	reg.MustRegister(metrics.JobRuns)
	families, err := reg.Gather()
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, f := range families {
		for _, m := range f.GetMetric() {
			job, result := "", ""
			for _, l := range m.GetLabel() {
				switch l.GetName() {
				case "job":
					job = l.GetValue()
				case "result":
					result = l.GetValue()
				}
			}
			if result == "error" {
				seen[job] = true
			}
		}
	}
	for _, j := range s.jobs() {
		if !seen[j.name] {
			t.Errorf("sp_job_runs_total has no error series for job %q before any failure", j.name)
		}
	}
}
