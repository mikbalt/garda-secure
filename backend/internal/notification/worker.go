package notification

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	"github.com/rs/zerolog"

	"github.com/securepatrol/backend/internal/metrics"
	"github.com/securepatrol/backend/internal/repository"
	"github.com/securepatrol/backend/internal/settings"
)

const (
	// sendTimeout bounds one delivery attempt (dial + TLS + protocol).
	sendTimeout = 30 * time.Second
	// leaseFor is how long a claimed row stays invisible to other workers;
	// longer than sendTimeout so a slow send is never sent twice.
	leaseFor = sendTimeout + 30*time.Second
	// pollInterval is the fallback when no Redis wake-up arrives.
	pollInterval = 2 * time.Second
	// dbTimeout bounds the bookkeeping queries of one delivery.
	dbTimeout = 10 * time.Second
	// gaugeInterval is how often queue depth and unacknowledged SOS are sampled.
	gaugeInterval = 15 * time.Second
)

// Retry schedules (audit BE-R2). SOS keeps trying for at least 30 minutes:
// 10 s, 20 s, 30 s, then every 60 s; other events back off to 30 minutes and
// give up after 5 attempts (~42 min).
var (
	sosDelays      = []time.Duration{10 * time.Second, 20 * time.Second, 30 * time.Second}
	sosLaterDelay  = time.Minute
	sosMaxAttempts = 33 // 10+20+30 s + 29 × 60 s = 30 min of waiting
	normalDelays   = []time.Duration{30 * time.Second, 2 * time.Minute, 10 * time.Minute, 30 * time.Minute}
)

// RetryDelay returns the wait before the next attempt after `attempts`
// failed attempts on a lane, or ok=false when the delivery should give up.
// An admin retry of a failed delivery therefore gets exactly one attempt.
func RetryDelay(lane string, attempts int) (time.Duration, bool) {
	if attempts < 1 {
		attempts = 1
	}
	if lane == LaneSOS {
		if attempts >= sosMaxAttempts {
			return 0, false
		}
		if attempts <= len(sosDelays) {
			return sosDelays[attempts-1], true
		}
		return sosLaterDelay, true
	}
	if attempts > len(normalDelays) {
		return 0, false
	}
	return normalDelays[attempts-1], true
}

// Worker sends queued deliveries with a pool of goroutines. Goroutine 0 only
// serves the SOS lane, so an SOS never waits behind a slow e-mail; the others
// serve SOS first, then the normal lane.
type Worker struct {
	db       *pgxpool.Pool // nil in unit tests (no gauges)
	store    deliveryStore
	rdb      *redis.Client
	config   func(context.Context) (settings.Channels, error)
	channels map[string]Channel
	notifier *Notifier
	workers  int
	log      zerolog.Logger

	poll       time.Duration
	heartbeat  metrics.Heartbeat
	wakeMu     sync.Mutex
	wakeCh     chan struct{}
	redisAlive bool
}

// NewWorker creates a dispatcher with the v1 channels (e-mail, WhatsApp).
// workers < 2 is raised to 2 (one goroutine is reserved for SOS).
func NewWorker(pool *pgxpool.Pool, rdb *redis.Client, st *settings.Store, n *Notifier, workers int, log zerolog.Logger) *Worker {
	if workers < 2 {
		workers = 2
	}
	return &Worker{
		db: pool, store: pgStore{pool}, rdb: rdb, config: st.Channels, channels: defaultChannels(),
		notifier: n, workers: workers,
		log:    log.With().Str("component", "notification-worker").Logger(),
		poll:   pollInterval,
		wakeCh: make(chan struct{}),
	}
}

// defaultChannels returns a sender for every registered channel (F10).
func defaultChannels() map[string]Channel { return senders() }

// SendTest delivers a test message immediately, bypassing the queue, so an
// admin can verify channel settings from the web UI.
func SendTest(ctx context.Context, st *settings.Store, channel, recipient string) error {
	spec, ok := Channels[channel]
	if !ok {
		return fmt.Errorf("%w: unsupported channel %q", ErrNotConfigured, channel)
	}
	ch := spec.New()
	cfg, err := st.Channels(ctx)
	if err != nil {
		return err
	}
	sctx, cancel := context.WithTimeout(ctx, sendTimeout)
	defer cancel()
	return ch.Send(sctx, cfg, recipient, "GARDA test", "Test notification from GARDA / Notifikasi uji dari GARDA.")
}

// Heartbeat reports when a dispatch loop last made progress.
func (w *Worker) Heartbeat() time.Time { return w.heartbeat.Last() }

// Run dispatches until ctx is cancelled, then waits for in-flight sends to
// finish (each is bounded by sendTimeout) and returns.
func (w *Worker) Run(ctx context.Context) {
	w.heartbeat.Beat()
	var wg sync.WaitGroup
	for i := 0; i < w.workers; i++ {
		lanes := []string{LaneSOS, LaneNormal}
		if i == 0 {
			lanes = []string{LaneSOS}
		}
		wg.Add(1)
		go func(lanes []string) {
			defer wg.Done()
			w.loop(ctx, lanes)
		}(lanes)
	}
	wg.Add(1)
	go func() { defer wg.Done(); w.listen(ctx) }()
	if w.db != nil {
		wg.Add(1)
		go func() { defer wg.Done(); w.sampleGauges(ctx) }()
	}
	wg.Wait()
	w.log.Info().Msg("notification worker stopped")
}

// loop claims and sends due deliveries of its lanes (in priority order).
func (w *Worker) loop(ctx context.Context, lanes []string) {
	for ctx.Err() == nil {
		w.heartbeat.Beat()
		woken := w.wakeSignal() // taken before claiming so no wake-up is lost
		if w.claimAndSend(ctx, lanes) {
			continue
		}
		t := time.NewTimer(w.poll)
		select {
		case <-ctx.Done():
		case <-woken:
		case <-t.C:
		}
		t.Stop()
	}
}

// claimAndSend sends one due delivery of the first lane that has one.
func (w *Worker) claimAndSend(ctx context.Context, lanes []string) bool {
	for _, lane := range lanes {
		cctx, cancel := context.WithTimeout(ctx, dbTimeout)
		d, err := w.store.Claim(cctx, lane, leaseFor)
		cancel()
		if errors.Is(err, repository.ErrNotFound) {
			continue
		}
		if err != nil {
			if ctx.Err() == nil {
				w.log.Error().Err(err).Str("lane", lane).Msg("claim delivery failed")
			}
			return false
		}
		w.deliver(d)
		return true
	}
	return false
}

// deliver sends one claimed delivery and records the outcome. It does not
// use the run context: on shutdown an in-flight send finishes (bounded by
// sendTimeout) instead of being cut off half-way.
func (w *Worker) deliver(d *repository.Delivery) {
	log := w.log.With().Str("id", d.ID).Str("event", d.EventType).Str("channel", d.Channel).
		Str("lane", d.Lane).Int("attempt", d.Attempts+1).Logger()
	if d.RequestID != nil {
		log = log.With().Str("request_id", *d.RequestID).Logger()
	}

	sendErr := w.send(d.Channel, d.Recipient, d.Subject, d.Body)

	ctx, cancel := context.WithTimeout(context.Background(), dbTimeout)
	defer cancel()
	if sendErr == nil {
		if err := w.store.MarkSent(ctx, d.ID); err != nil {
			log.Error().Err(err).Msg("mark sent failed (may be sent again after the lease)")
		}
		metrics.Notifications.WithLabelValues(d.Channel, d.EventType, metrics.ResultSent).Inc()
		log.Info().Msg("notification sent")
		return
	}

	msg := sendErr.Error()
	attempts := d.Attempts + 1
	delay, retry := RetryDelay(d.Lane, attempts)
	if retry {
		next := time.Now().Add(delay)
		if err := w.store.MarkRetry(ctx, d.ID, msg, next); err != nil {
			log.Error().Err(err).Msg("mark retry failed")
		}
		metrics.Notifications.WithLabelValues(d.Channel, d.EventType, metrics.ResultRetry).Inc()
		log.Warn().Str("error", msg).Dur("retry_in", delay).Msg("delivery failed; will retry")
		return
	}

	if err := w.store.MarkFailed(ctx, d.ID, msg); err != nil {
		log.Error().Err(err).Msg("mark failed failed")
	}
	metrics.Notifications.WithLabelValues(d.Channel, d.EventType, metrics.ResultFailed).Inc()
	metrics.NotificationsFailed.WithLabelValues(d.EventType).Inc()
	log.Error().Str("error", msg).Msg("delivery failed permanently")
	if d.EventType == EventSOS {
		w.alertSOSUndelivered(ctx, d, msg)
	}
}

// alertSOSUndelivered tells the ops recipients that an SOS delivery gave up.
func (w *Worker) alertSOSUndelivered(ctx context.Context, d *repository.Delivery, reason string) {
	if w.notifier == nil {
		return
	}
	_, err := w.notifier.Publish(ctx, Event{
		Type: EventOpsSOSUndelivered, DedupeKey: "ops:sos_failed:" + d.ID,
		Subject: "[OPS] SOS tidak terkirim / not delivered: " + d.Subject,
		Body: "Pengiriman ke / delivery to " + d.Channel + " " + d.Recipient + " gagal / gave up after " +
			strconv.Itoa(d.Attempts+1) + " attempts: " + reason +
			"\n\nHubungi lokasi secara langsung / contact the site directly.\n\n---\n" + d.Body,
	})
	if err != nil {
		w.log.Error().Err(err).Str("id", d.ID).Msg("could not raise ops alert for undelivered SOS")
	}
}

func (w *Worker) send(channel, recipient, subject, body string) error {
	ch, ok := w.channels[channel]
	if !ok {
		return errors.New("unsupported channel " + channel)
	}
	ctx, cancel := context.WithTimeout(context.Background(), sendTimeout)
	defer cancel()
	cfg, err := w.config(ctx)
	if err != nil {
		return err
	}
	return ch.Send(ctx, cfg, recipient, subject, body)
}

// ── Store ────────────────────────────────────────────────────────────────────

// deliveryStore is the worker's view of notification_log.
type deliveryStore interface {
	Claim(ctx context.Context, lane string, lease time.Duration) (*repository.Delivery, error)
	MarkSent(ctx context.Context, id string) error
	MarkRetry(ctx context.Context, id, msg string, next time.Time) error
	MarkFailed(ctx context.Context, id, msg string) error
}

type pgStore struct{ db *pgxpool.Pool }

func (s pgStore) Claim(ctx context.Context, lane string, lease time.Duration) (*repository.Delivery, error) {
	return repository.ClaimDelivery(ctx, s.db, lane, lease)
}
func (s pgStore) MarkSent(ctx context.Context, id string) error {
	return repository.MarkDeliverySent(ctx, s.db, id)
}
func (s pgStore) MarkRetry(ctx context.Context, id, msg string, next time.Time) error {
	return repository.MarkDeliveryRetry(ctx, s.db, id, msg, next)
}
func (s pgStore) MarkFailed(ctx context.Context, id, msg string) error {
	return repository.MarkDeliveryFailed(ctx, s.db, id, msg)
}

// ── Wake-ups ─────────────────────────────────────────────────────────────────

// wakeSignal returns a channel closed by the next wake-up.
func (w *Worker) wakeSignal() <-chan struct{} {
	w.wakeMu.Lock()
	defer w.wakeMu.Unlock()
	return w.wakeCh
}

// broadcast wakes every waiting dispatch loop.
func (w *Worker) broadcast() {
	w.wakeMu.Lock()
	close(w.wakeCh)
	w.wakeCh = make(chan struct{})
	w.wakeMu.Unlock()
}

// listen turns Redis wake-ups into broadcasts. Without Redis the loops poll.
func (w *Worker) listen(ctx context.Context) {
	if w.rdb == nil {
		<-ctx.Done()
		return
	}
	w.redisAlive = true
	for ctx.Err() == nil {
		_, err := w.rdb.BRPop(ctx, time.Second, wakeKey).Result()
		switch {
		case err == nil:
			if !w.redisAlive {
				w.log.Info().Msg("redis wake-ups restored")
				w.redisAlive = true
			}
			w.broadcast()
		case errors.Is(err, redis.Nil) || ctx.Err() != nil:
		default:
			if w.redisAlive {
				w.log.Warn().Err(err).Msg("redis unavailable; dispatching by polling postgres")
				w.redisAlive = false
			}
			sleep(ctx, time.Second)
		}
	}
}

// sampleGauges keeps the queue-depth and unacknowledged-SOS gauges current.
func (w *Worker) sampleGauges(ctx context.Context) {
	t := time.NewTicker(gaugeInterval)
	defer t.Stop()
	for {
		qctx, cancel := context.WithTimeout(ctx, dbTimeout)
		if counts, err := repository.CountQueuedByLane(qctx, w.db); err == nil {
			for _, lane := range []string{LaneSOS, LaneNormal} {
				metrics.QueueDepth.WithLabelValues(lane).Set(float64(counts[lane]))
			}
		} else if ctx.Err() == nil {
			w.log.Warn().Err(err).Msg("sample queue depth failed")
		}
		if n, err := repository.CountUnacknowledgedSos(qctx, w.db); err == nil {
			metrics.SOSUnacknowledged.Set(float64(n))
		} else if ctx.Err() == nil {
			w.log.Warn().Err(err).Msg("sample unacknowledged SOS failed")
		}
		cancel()
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

func sleep(ctx context.Context, d time.Duration) {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
	case <-t.C:
	}
}
