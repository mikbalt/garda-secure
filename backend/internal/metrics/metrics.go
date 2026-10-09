// Package metrics defines the Prometheus metrics of the API and the worker
// (audit BE-O1). Metric names are a contract with the alert rules in infra/;
// do not rename them.
//
// The API registers only the HTTP metrics and the worker only the worker
// metrics, so an alert on e.g. the worker heartbeat never sees a zero series
// exported by the API. Updating a metric that is not registered is harmless.
package metrics

import (
	"strconv"
	"sync/atomic"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
)

// Notification results used in sp_notifications_total.
const (
	ResultSent   = "sent"
	ResultRetry  = "retry"
	ResultFailed = "failed"
)

var (
	// HTTPRequests counts API requests by Fiber route pattern (not raw path).
	HTTPRequests = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "sp_http_requests_total",
		Help: "API requests by method, route pattern and final status code.",
	}, []string{"method", "route", "status"})

	// HTTPDuration observes request latency by route pattern.
	HTTPDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "sp_http_request_duration_seconds",
		Help:    "API request duration by method and route pattern.",
		Buckets: []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10, 30},
	}, []string{"method", "route"})

	// Notifications counts delivery attempts (result = sent | retry | failed).
	Notifications = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "sp_notifications_total",
		Help: "Notification delivery attempts by channel, event type and result (sent, retry, failed).",
	}, []string{"channel", "event", "result"})

	// QueueDepth is the number of queued deliveries per lane (sos | normal).
	QueueDepth = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "sp_notification_queue_depth",
		Help: "Deliveries waiting in notification_log (status queued) per lane.",
	}, []string{"lane"})

	// NotificationsFailed counts deliveries that permanently failed.
	NotificationsFailed = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "sp_notifications_failed_total",
		Help: "Deliveries that gave up after all retries, by event type.",
	}, []string{"event"})

	// SOSUnacknowledged is the number of unacknowledged SOS alerts (all sites).
	SOSUnacknowledged = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "sp_sos_unacknowledged",
		Help: "SOS alerts not yet acknowledged, all sites.",
	})

	// WorkerHeartbeat is set while the dispatcher and the scheduler are alive.
	WorkerHeartbeat = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "sp_worker_heartbeat_timestamp_seconds",
		Help: "Unix time of the last moment both the notification dispatcher and the scheduler were ticking.",
	})

	// JobRuns counts scheduled job runs (result = success | error | skipped).
	JobRuns = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "sp_job_runs_total",
		Help: "Scheduled job runs by job and result (success, error, skipped = another replica holds the lock).",
	}, []string{"job", "result"})

	// JobLastSuccess is the Unix time of each job's last successful run.
	JobLastSuccess = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "sp_job_last_success_timestamp_seconds",
		Help: "Unix time of the last successful run per job.",
	}, []string{"job"})

	// BackupLastSuccess is the finish time of the newest successful backup
	// (0 = no successful backup yet).
	BackupLastSuccess = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "sp_backup_last_success_timestamp_seconds",
		Help: "Unix time the newest successful database backup finished (0 = none yet).",
	})
)

// RegisterAPI registers the API metrics and the Go/process collectors.
func RegisterAPI(r prometheus.Registerer) {
	r.MustRegister(HTTPRequests, HTTPDuration)
	registerRuntime(r)
}

// RegisterWorker registers the worker metrics and the Go/process collectors.
func RegisterWorker(r prometheus.Registerer) {
	r.MustRegister(Notifications, QueueDepth, NotificationsFailed, SOSUnacknowledged,
		WorkerHeartbeat, JobRuns, JobLastSuccess, BackupLastSuccess)
	registerRuntime(r)
}

// registerRuntime adds the Go and process collectors unless the registry
// already has them (the default registry does).
func registerRuntime(r prometheus.Registerer) {
	for _, c := range []prometheus.Collector{
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
	} {
		if err := r.Register(c); err != nil {
			if _, dup := err.(prometheus.AlreadyRegisteredError); !dup {
				panic(err)
			}
		}
	}
}

// Fiber returns a middleware that records request count and latency per
// route pattern. Mount it before the request logger so the status is final
// (the logger renders handler errors).
func Fiber() fiber.Handler {
	return func(c *fiber.Ctx) error {
		start := time.Now()
		err := c.Next()
		if err != nil {
			// Render the error now so the recorded status is the real one.
			if herr := c.App().ErrorHandler(c, err); herr != nil {
				_ = c.SendStatus(fiber.StatusInternalServerError)
			}
			err = nil
		}
		route := "unmatched"
		if r := c.Route(); r != nil && r.Path != "" {
			route = r.Path
		}
		method := c.Method()
		HTTPRequests.WithLabelValues(method, route, strconv.Itoa(c.Response().StatusCode())).Inc()
		HTTPDuration.WithLabelValues(method, route).Observe(time.Since(start).Seconds())
		return err
	}
}

// Heartbeat records the last time a loop made progress. Safe for concurrent use.
type Heartbeat struct{ unixNano atomic.Int64 }

// Beat records now.
func (h *Heartbeat) Beat() { h.unixNano.Store(time.Now().UnixNano()) }

// Last returns the time of the last beat (zero if never).
func (h *Heartbeat) Last() time.Time {
	n := h.unixNano.Load()
	if n == 0 {
		return time.Time{}
	}
	return time.Unix(0, n)
}

// Fresh reports whether the last beat is within maxAge of now.
func (h *Heartbeat) Fresh(now time.Time, maxAge time.Duration) bool {
	last := h.Last()
	return !last.IsZero() && now.Sub(last) <= maxAge
}
