package metrics

import (
	"errors"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
)

func TestFiberRecordsRoutePatternAndFinalStatus(t *testing.T) {
	app := fiber.New()
	app.Use(Fiber())
	app.Get("/api/v1/sites/:id", func(c *fiber.Ctx) error { return c.SendString("ok") })
	app.Get("/boom", func(c *fiber.Ctx) error { return errors.New("db down") })

	for _, path := range []string{"/api/v1/sites/abc", "/api/v1/sites/def", "/boom"} {
		res, err := app.Test(httptest.NewRequest("GET", path, nil))
		if err != nil {
			t.Fatal(err)
		}
		if path == "/boom" && res.StatusCode != 500 {
			t.Errorf("/boom status = %d", res.StatusCode)
		}
	}
	if n := counterValue(HTTPRequests.WithLabelValues("GET", "/api/v1/sites/:id", "200")); n != 2 {
		t.Errorf("route-pattern counter = %v, want 2", n)
	}
	if n := counterValue(HTTPRequests.WithLabelValues("GET", "/boom", "500")); n != 1 {
		t.Errorf("error counter = %v, want 1 (final status)", n)
	}
}

func TestRegisterSeparatesAPIAndWorkerMetrics(t *testing.T) {
	api := prometheus.NewRegistry()
	RegisterAPI(api)
	worker := prometheus.NewRegistry()
	RegisterWorker(worker)
	WorkerHeartbeat.SetToCurrentTime()

	apiNames := names(t, api)
	if strings.Contains(apiNames, "sp_worker_heartbeat_timestamp_seconds") {
		t.Error("API exports the worker heartbeat")
	}
	if !strings.Contains(apiNames, "go_goroutines") || !strings.Contains(apiNames, "process_") {
		t.Errorf("API lacks Go/process collectors: %s", apiNames)
	}
	if w := names(t, worker); !strings.Contains(w, "sp_worker_heartbeat_timestamp_seconds") {
		t.Errorf("worker lacks heartbeat: %s", w)
	}
}

func names(t *testing.T, g prometheus.Gatherer) string {
	t.Helper()
	mfs, err := g.Gather()
	if err != nil {
		t.Fatal(err)
	}
	var b strings.Builder
	for _, mf := range mfs {
		b.WriteString(mf.GetName() + " ")
	}
	return b.String()
}

func TestHeartbeat(t *testing.T) {
	var h Heartbeat
	if h.Fresh(time.Now(), time.Minute) {
		t.Error("never-beaten heartbeat is fresh")
	}
	h.Beat()
	if !h.Fresh(time.Now(), time.Minute) {
		t.Error("just-beaten heartbeat is stale")
	}
	if h.Fresh(time.Now().Add(2*time.Minute), time.Minute) {
		t.Error("old heartbeat is fresh")
	}
}

func counterValue(c prometheus.Counter) float64 {
	var m dto.Metric
	if err := c.Write(&m); err != nil {
		return -1
	}
	return m.GetCounter().GetValue()
}
