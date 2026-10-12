package handler

import (
	"encoding/json"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/securepatrol/backend/internal/metrics"
	"github.com/securepatrol/backend/internal/middleware"
	"github.com/securepatrol/backend/internal/model"
	"github.com/securepatrol/backend/internal/repository"
	"github.com/securepatrol/backend/internal/service"
)

// deviceHealth is the tablet health beacon (MOB-7: sync health). Stored as
// sent (validated), plus the server receipt time.
type deviceHealth struct {
	AppVersion     string     `json:"app_version"`
	AppBuild       string     `json:"app_build"`
	Platform       string     `json:"platform"`
	OSVersion      string     `json:"os_version"`
	OutboxPending  int        `json:"outbox_pending"`
	OutboxFailed   int        `json:"outbox_failed"`
	OutboxPaused   *bool      `json:"outbox_paused,omitempty"`
	SOSPending     int        `json:"sos_pending"`
	LastSync       *time.Time `json:"last_sync,omitempty"`
	RosterSyncedAt *time.Time `json:"roster_synced_at,omitempty"`
	ClockOffsetMs  int64      `json:"clock_offset_ms"`
	PrivateCA      bool       `json:"private_ca"`
	PinFailures    *int       `json:"pin_failures,omitempty"`
	DBKeyReset     *bool      `json:"db_key_reset,omitempty"`
	Errors         []string   `json:"errors"`
	SentAt         *time.Time `json:"sent_at,omitempty"`
	ReceivedAt     time.Time  `json:"received_at"`
}

// Telemetry limits.
const (
	telemetryMaxErrors   = 10
	telemetryMaxErrorLen = 500
	telemetryMaxCount    = 1_000_000
)

// POST /device/telemetry (active device, no guard) — latest health beacon
// of the tablet; 204. At most one per minute per device (429 otherwise).
func (h *Handler) deviceTelemetry(c *fiber.Ctx) error {
	d := middleware.GetDevice(c)
	if ok, retry := h.limiter.Allow(c.Context(), "telemetry:"+d.ID, 1); !ok {
		return middleware.TooManyRequests(c, retry)
	}
	var b deviceHealth
	if err := parse(c, &b); err != nil {
		return h.fail(c, err)
	}
	if err := service.CheckLengths(service.S("app_version", b.AppVersion, 64), service.S("app_build", b.AppBuild, 64),
		service.S("platform", b.Platform, 32), service.S("os_version", b.OSVersion, 100)); err != nil {
		return h.fail(c, err)
	}
	if len(b.Errors) > telemetryMaxErrors {
		return h.fail(c, service.BadRequest("too_many_errors", "at most %d errors per report", telemetryMaxErrors).WithField("errors"))
	}
	for _, e := range b.Errors {
		if err := service.CheckLengths(service.S("errors", e, telemetryMaxErrorLen)); err != nil {
			return h.fail(c, err)
		}
	}
	for _, n := range []int{b.OutboxPending, b.OutboxFailed, b.SOSPending} {
		if n < 0 || n > telemetryMaxCount {
			return h.fail(c, service.BadRequest("invalid_count", "counts must be between 0 and %d", telemetryMaxCount))
		}
	}
	if b.PinFailures != nil && (*b.PinFailures < 0 || *b.PinFailures > telemetryMaxCount) {
		return h.fail(c, service.BadRequest("invalid_count", "pin_failures out of range").WithField("pin_failures"))
	}
	if b.Errors == nil {
		b.Errors = []string{}
	}
	b.ReceivedAt = service.Now()
	raw, err := json.Marshal(b)
	if err != nil {
		return h.fail(c, err)
	}
	if err := repository.SetDeviceHealth(c.Context(), h.DB, d.ID, raw, b.ReceivedAt); err != nil {
		return h.fail(c, err)
	}
	metrics.DeviceOutboxPending.WithLabelValues(d.ID).Set(float64(b.OutboxPending))
	return c.SendStatus(fiber.StatusNoContent)
}

// withHealth fills in the latest health beacon of each device (GET /devices).
func (h *Handler) withHealth(c *fiber.Ctx, devices []model.Device) error {
	ids := make([]string, len(devices))
	for i, d := range devices {
		ids[i] = d.ID
	}
	health, err := repository.ListDeviceHealth(c.Context(), h.DB, ids)
	if err != nil {
		return err
	}
	for i := range devices {
		if hh, ok := health[devices[i].ID]; ok {
			at := hh.At
			devices[i].Health, devices[i].HealthAt = hh.Health, &at
		}
	}
	return nil
}
