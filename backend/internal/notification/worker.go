package notification

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/rs/zerolog"

	"github.com/securepatrol/backend/internal/repository"
	"github.com/securepatrol/backend/internal/settings"
)

const (
	maxAttempts = 3
	sendTimeout = 30 * time.Second
	lockTTL     = 2 * time.Minute
)

// retryDelays is the wait before attempt n+1 (index = attempts already made).
var retryDelays = []time.Duration{10 * time.Second, time.Minute}

// Worker sends queued deliveries.
type Worker struct {
	db       repository.DBTX
	rdb      *redis.Client
	settings *settings.Store
	channels map[string]Channel
	log      zerolog.Logger
}

// NewWorker creates a dispatcher with the v1 channels (e-mail, WhatsApp).
func NewWorker(db repository.DBTX, rdb *redis.Client, st *settings.Store, log zerolog.Logger) *Worker {
	return &Worker{
		db:       db,
		rdb:      rdb,
		settings: st,
		channels: defaultChannels(),
		log:      log.With().Str("component", "notification-worker").Logger(),
	}
}

func defaultChannels() map[string]Channel {
	return map[string]Channel{
		"email":    emailChannel{},
		"whatsapp": whatsAppChannel{client: &http.Client{Timeout: sendTimeout}},
	}
}

// SendTest delivers a test message immediately, bypassing the queue, so an
// admin can verify channel settings from the web UI.
func SendTest(ctx context.Context, st *settings.Store, channel, recipient string) error {
	ch, ok := defaultChannels()[channel]
	if !ok {
		return errors.New("unsupported channel " + channel)
	}
	cfg, err := st.Channels(ctx)
	if err != nil {
		return err
	}
	sctx, cancel := context.WithTimeout(ctx, sendTimeout)
	defer cancel()
	return ch.Send(sctx, cfg, recipient, "SecurePatrol test", "Test notification from SecurePatrol / Notifikasi uji dari SecurePatrol.")
}

// Run dispatches until ctx is cancelled.
func (w *Worker) Run(ctx context.Context) {
	go w.every(ctx, time.Second, w.promoteDelayed)
	go w.every(ctx, time.Minute, w.sweep)
	for ctx.Err() == nil {
		res, err := w.rdb.BRPop(ctx, 2*time.Second, queueKey).Result()
		if err != nil {
			if !errors.Is(err, redis.Nil) && ctx.Err() == nil {
				w.log.Warn().Err(err).Msg("brpop failed")
				time.Sleep(time.Second)
			}
			continue
		}
		if len(res) == 2 {
			w.process(ctx, res[1])
		}
	}
}

func (w *Worker) every(ctx context.Context, d time.Duration, fn func(context.Context)) {
	t := time.NewTicker(d)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			fn(ctx)
		}
	}
}

// process sends one delivery. A short Redis lock prevents two copies of the
// same ID (e.g. sweeper + retry) from being sent concurrently.
func (w *Worker) process(ctx context.Context, id string) {
	lock := "notifications:lock:" + id
	ok, err := w.rdb.SetNX(ctx, lock, "1", lockTTL).Result()
	if err != nil || !ok {
		return
	}
	defer w.rdb.Del(context.Background(), lock)

	l, err := repository.GetNotificationLog(ctx, w.db, id)
	if err != nil {
		if !errors.Is(err, repository.ErrNotFound) {
			w.log.Error().Err(err).Str("id", id).Msg("load delivery failed")
		}
		return
	}
	if l.Status != "queued" {
		return
	}

	sendErr := w.send(ctx, l.Channel, l.Recipient, l.Subject, l.Body)
	if sendErr == nil {
		if err := repository.MarkNotificationAttempt(ctx, w.db, id, "sent", nil); err != nil {
			w.log.Error().Err(err).Str("id", id).Msg("mark sent failed")
		}
		w.log.Info().Str("id", id).Str("event", l.EventType).Str("channel", l.Channel).Msg("notification sent")
		return
	}

	msg := sendErr.Error()
	attempts := l.Attempts + 1
	status := "queued"
	if attempts >= maxAttempts {
		status = "failed"
	}
	if err := repository.MarkNotificationAttempt(ctx, w.db, id, status, &msg); err != nil {
		w.log.Error().Err(err).Str("id", id).Msg("mark attempt failed")
	}
	w.log.Warn().Str("id", id).Str("channel", l.Channel).Int("attempts", attempts).Str("error", msg).Msg("delivery failed")
	if status == "queued" {
		delay := retryDelays[min(attempts-1, len(retryDelays)-1)]
		w.rdb.ZAdd(ctx, delayedKey, redis.Z{Score: float64(time.Now().Add(delay).Unix()), Member: id})
	}
}

func (w *Worker) send(ctx context.Context, channel, recipient, subject, body string) error {
	ch, ok := w.channels[channel]
	if !ok {
		return errors.New("unsupported channel " + channel)
	}
	cfg, err := w.settings.Channels(ctx)
	if err != nil {
		return err
	}
	sctx, cancel := context.WithTimeout(ctx, sendTimeout)
	defer cancel()
	return ch.Send(sctx, cfg, recipient, subject, body)
}

// promoteDelayed moves retries whose time has come back onto the queue.
func (w *Worker) promoteDelayed(ctx context.Context) {
	now := strconv.FormatInt(time.Now().Unix(), 10)
	ids, err := w.rdb.ZRangeArgs(ctx, redis.ZRangeArgs{
		Key: delayedKey, Start: "-inf", Stop: now, ByScore: true, Count: 100,
	}).Result()
	if err != nil {
		return
	}
	for _, id := range ids {
		if n, _ := w.rdb.ZRem(ctx, delayedKey, id).Result(); n == 1 {
			_ = Enqueue(ctx, w.rdb, id)
		}
	}
}

// sweep re-enqueues deliveries stuck in "queued" (e.g. Redis was down when
// they were published), skipping those waiting for a scheduled retry.
func (w *Worker) sweep(ctx context.Context) {
	ids, err := repository.ListQueuedNotificationIDs(ctx, w.db, 120)
	if err != nil {
		w.log.Warn().Err(err).Msg("sweep failed")
		return
	}
	for _, id := range ids {
		if _, err := w.rdb.ZScore(ctx, delayedKey, id).Result(); err == nil {
			continue
		}
		_ = Enqueue(ctx, w.rdb, id)
	}
}
