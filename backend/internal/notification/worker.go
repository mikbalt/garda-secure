// Package notification implements a Redis-queue-backed notification worker
// that dispatches SMTP emails and WhatsApp messages.
package notification

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/smtp"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/rs/zerolog"

	"github.com/securepatrol/backend/internal/config"
)

const (
	queueKey       = "notifications:queue"
	deadLetterKey  = "notifications:dead"
	maxRetries     = 3
	visibilityTime = 30 * time.Second
	pollInterval   = 2 * time.Second
)

// Kind identifies the notification channel.
type Kind string

const (
	KindEmail    Kind = "email"
	KindWhatsApp Kind = "whatsapp"
)

// Message is a notification job stored in Redis.
type Message struct {
	ID        string          `json:"id"`
	Kind      Kind            `json:"kind"`
	To        string          `json:"to"`
	Subject   string          `json:"subject,omitempty"`
	Body      string          `json:"body"`
	Retries   int             `json:"retries"`
	CreatedAt time.Time       `json:"created_at"`
	Meta      json.RawMessage `json:"meta,omitempty"`
}

// Worker polls the Redis queue and dispatches notifications.
type Worker struct {
	rdb    *redis.Client
	cfg    *config.Config
	log    zerolog.Logger
	done   chan struct{}
}

// NewWorker creates a new notification worker.
func NewWorker(rdb *redis.Client, cfg *config.Config, log zerolog.Logger) *Worker {
	return &Worker{
		rdb:  rdb,
		cfg:  cfg,
		log:  log.With().Str("component", "notification-worker").Logger(),
		done: make(chan struct{}),
	}
}

// Enqueue pushes a notification message onto the Redis queue.
func Enqueue(ctx context.Context, rdb *redis.Client, msg Message) error {
	msg.CreatedAt = time.Now().UTC()
	b, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}
	return rdb.LPush(ctx, queueKey, b).Err()
}

// Start begins the worker loop in a goroutine.
func (w *Worker) Start(ctx context.Context) {
	go w.loop(ctx)
}

// Stop signals the worker to stop.
func (w *Worker) Stop() {
	close(w.done)
}

func (w *Worker) loop(ctx context.Context) {
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-w.done:
			return
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.processOne(ctx)
		}
	}
}

func (w *Worker) processOne(ctx context.Context) {
	// BRPOP blocks up to 1s — low-latency without busy-polling.
	result, err := w.rdb.BRPop(ctx, 1*time.Second, queueKey).Result()
	if err == redis.Nil {
		return
	}
	if err != nil {
		w.log.Warn().Err(err).Msg("brpop error")
		return
	}
	if len(result) < 2 {
		return
	}

	var msg Message
	if err := json.Unmarshal([]byte(result[1]), &msg); err != nil {
		w.log.Error().Err(err).Msg("unmarshal notification message")
		return
	}

	var dispatchErr error
	switch msg.Kind {
	case KindEmail:
		dispatchErr = w.sendEmail(msg)
	case KindWhatsApp:
		dispatchErr = w.sendWhatsApp(ctx, msg)
	default:
		w.log.Warn().Str("kind", string(msg.Kind)).Msg("unknown notification kind, dropping")
		return
	}

	if dispatchErr != nil {
		w.log.Error().Err(dispatchErr).Str("id", msg.ID).Str("kind", string(msg.Kind)).
			Int("retries", msg.Retries).Msg("dispatch failed")

		msg.Retries++
		if msg.Retries >= maxRetries {
			w.log.Error().Str("id", msg.ID).Msg("max retries reached, moving to dead letter")
			b, _ := json.Marshal(msg)
			_ = w.rdb.LPush(ctx, deadLetterKey, b).Err()
		} else {
			// Re-queue with exponential backoff via a delayed key
			delay := time.Duration(1<<msg.Retries) * time.Second
			time.Sleep(delay)
			_ = Enqueue(ctx, w.rdb, msg)
		}
	} else {
		w.log.Info().Str("id", msg.ID).Str("kind", string(msg.Kind)).
			Str("to", msg.To).Msg("notification dispatched")
	}
}

func (w *Worker) sendEmail(msg Message) error {
	if w.cfg.SMTPHost == "" {
		return fmt.Errorf("SMTP not configured")
	}

	addr := fmt.Sprintf("%s:%s", w.cfg.SMTPHost, w.cfg.SMTPPort)
	auth := smtp.PlainAuth("", w.cfg.SMTPUser, w.cfg.SMTPPassword, w.cfg.SMTPHost)

	body := fmt.Sprintf("From: %s\r\nTo: %s\r\nSubject: %s\r\nMIME-Version: 1.0\r\nContent-Type: text/html; charset=UTF-8\r\n\r\n%s",
		w.cfg.SMTPUser, msg.To, msg.Subject, msg.Body)

	return smtp.SendMail(addr, auth, w.cfg.SMTPUser, []string{msg.To}, []byte(body))
}

func (w *Worker) sendWhatsApp(ctx context.Context, msg Message) error {
	if w.cfg.WhatsAppAPIURL == "" {
		return fmt.Errorf("WhatsApp API not configured")
	}

	payload := map[string]any{
		"messaging_product": "whatsapp",
		"to":                msg.To,
		"type":              "text",
		"text":              map[string]string{"body": msg.Body},
	}
	b, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, w.cfg.WhatsAppAPIURL, bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+w.cfg.WhatsAppAPIToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("whatsapp API returned status %d", resp.StatusCode)
	}
	return nil
}
