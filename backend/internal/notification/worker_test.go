package notification

import (
	"bufio"
	"context"
	"errors"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/rs/zerolog"

	"github.com/securepatrol/backend/internal/model"
	"github.com/securepatrol/backend/internal/repository"
	"github.com/securepatrol/backend/internal/settings"
)

// ── Retry schedule ───────────────────────────────────────────────────────────

func TestRetryDelaySOSKeepsTryingForThirtyMinutes(t *testing.T) {
	var total time.Duration
	attempts := 0
	for {
		attempts++
		d, ok := RetryDelay(LaneSOS, attempts)
		if !ok {
			break
		}
		total += d
		if attempts > 1000 {
			t.Fatal("SOS never gives up")
		}
	}
	if total < 30*time.Minute {
		t.Errorf("SOS retries span %s, want at least 30 min", total)
	}
	for i, want := range []time.Duration{10 * time.Second, 20 * time.Second, 30 * time.Second, time.Minute, time.Minute} {
		if d, _ := RetryDelay(LaneSOS, i+1); d != want {
			t.Errorf("SOS delay after attempt %d = %s, want %s", i+1, d, want)
		}
	}
	if attempts != sosMaxAttempts {
		t.Errorf("SOS gives up after %d attempts, want %d", attempts, sosMaxAttempts)
	}
}

func TestRetryDelayNormalBacksOff(t *testing.T) {
	want := []time.Duration{30 * time.Second, 2 * time.Minute, 10 * time.Minute, 30 * time.Minute}
	for i, w := range want {
		d, ok := RetryDelay(LaneNormal, i+1)
		if !ok || d != w {
			t.Errorf("attempt %d: %s %v, want %s", i+1, d, ok, w)
		}
	}
	if _, ok := RetryDelay(LaneNormal, len(want)+1); ok {
		t.Error("normal deliveries should give up after 5 attempts")
	}
	// An admin retry of a failed delivery gets exactly one more attempt.
	if _, ok := RetryDelay(LaneSOS, sosMaxAttempts+3); ok {
		t.Error("retried failed SOS should not be rescheduled")
	}
}

func TestLaneFor(t *testing.T) {
	if LaneFor(EventSOS) != LaneSOS || LaneFor(EventOpsSOSUndelivered) != LaneSOS {
		t.Error("SOS events must use the sos lane")
	}
	if LaneFor(EventMonthlyRecap) != LaneNormal || LaneFor(EventOpsBackupFailed) != LaneNormal {
		t.Error("other events use the normal lane")
	}
}

// ── Dispatch with a fake store ───────────────────────────────────────────────

type fakeStore struct {
	mu      sync.Mutex
	rows    []*repository.Delivery
	claimed map[string]bool
	claims  []string // lanes in claim order
	sent    map[string]time.Time
	retried map[string]time.Time
	failed  map[string]string
}

func newFakeStore(rows ...*repository.Delivery) *fakeStore {
	return &fakeStore{rows: rows, claimed: map[string]bool{}, sent: map[string]time.Time{},
		retried: map[string]time.Time{}, failed: map[string]string{}}
}

func (s *fakeStore) add(d *repository.Delivery) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.rows = append(s.rows, d)
}

func (s *fakeStore) Claim(_ context.Context, lane string, _ time.Duration) (*repository.Delivery, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, d := range s.rows {
		if d.Lane == lane && !s.claimed[d.ID] {
			s.claimed[d.ID] = true
			s.claims = append(s.claims, lane)
			return d, nil
		}
	}
	return nil, repository.ErrNotFound
}

func (s *fakeStore) MarkSent(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sent[id] = time.Now()
	return nil
}

func (s *fakeStore) MarkRetry(_ context.Context, id, _ string, next time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.retried[id] = next
	return nil
}

func (s *fakeStore) MarkFailed(_ context.Context, id, msg string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.failed[id] = msg
	return nil
}

func (s *fakeStore) sentAt(id string) (time.Time, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	t, ok := s.sent[id]
	return t, ok
}

// fakeChannel succeeds, fails, or blocks until released / ctx expires.
type fakeChannel struct {
	block   chan struct{} // nil = do not block
	fail    error
	mu      sync.Mutex
	started int
}

func (c *fakeChannel) Send(ctx context.Context, _ settings.Channels, _, _, _ string) error {
	c.mu.Lock()
	c.started++
	c.mu.Unlock()
	if c.block != nil {
		select {
		case <-c.block:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return c.fail
}

func delivery(id, lane, channel, event string, attempts int) *repository.Delivery {
	return &repository.Delivery{
		NotificationLog: model.NotificationLog{ID: id, EventType: event, Channel: channel, Recipient: "r",
			Status: "queued", Attempts: attempts, CreatedAt: time.Now()},
		Lane: lane,
	}
}

func testWorker(store deliveryStore, channels map[string]Channel, workers int) *Worker {
	return &Worker{
		store: store, channels: channels, workers: workers, log: zerolog.Nop(),
		config: func(context.Context) (settings.Channels, error) { return settings.Channels{}, nil },
		poll:   20 * time.Millisecond, wakeCh: make(chan struct{}),
	}
}

func TestSOSIsNotStuckBehindHungEmail(t *testing.T) {
	email := &fakeChannel{block: make(chan struct{})}
	defer close(email.block)
	wa := &fakeChannel{}
	store := newFakeStore(
		delivery("mail1", LaneNormal, "email", EventMonthlyRecap, 0),
		delivery("mail2", LaneNormal, "email", EventAbsent, 0),
		delivery("mail3", LaneNormal, "email", EventAbsent, 0),
	)
	w := testWorker(store, map[string]Channel{"email": email, "whatsapp": wa}, 2)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { w.Run(ctx); close(done) }()

	// Wait until the general sender is stuck on an e-mail.
	waitFor(t, func() bool { email.mu.Lock(); defer email.mu.Unlock(); return email.started >= 1 })
	start := time.Now()
	store.add(delivery("sos1", LaneSOS, "whatsapp", EventSOS, 0))
	waitFor(t, func() bool { _, ok := store.sentAt("sos1"); return ok })
	if d := time.Since(start); d > time.Second {
		t.Errorf("SOS took %s behind a hung e-mail", d)
	}
	if _, ok := store.sentAt("mail2"); ok {
		t.Error("an e-mail was sent although every e-mail send hangs")
	}
	cancel()
	// Shutdown waits for in-flight sends; release them.
	email.block <- struct{}{}
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("worker did not stop after shutdown")
	}
}

func TestGeneralSenderClaimsSOSFirst(t *testing.T) {
	store := newFakeStore(
		delivery("mail", LaneNormal, "email", EventAbsent, 0),
		delivery("sos", LaneSOS, "email", EventSOS, 0),
	)
	w := testWorker(store, map[string]Channel{"email": &fakeChannel{}}, 2)
	if !w.claimAndSend(context.Background(), []string{LaneSOS, LaneNormal}) {
		t.Fatal("nothing claimed")
	}
	if _, ok := store.sentAt("sos"); !ok {
		t.Fatal("SOS was not sent first")
	}
	if !w.claimAndSend(context.Background(), []string{LaneSOS, LaneNormal}) {
		t.Fatal("e-mail not claimed second")
	}
	if w.claimAndSend(context.Background(), []string{LaneSOS}) {
		t.Fatal("SOS-only sender claimed a normal delivery")
	}
}

func TestDeliverRetriesThenFails(t *testing.T) {
	ch := &fakeChannel{fail: errors.New("smtp 451 try later")}
	store := newFakeStore(
		delivery("first", LaneNormal, "email", EventAbsent, 0),
		delivery("last", LaneNormal, "email", EventAbsent, len(normalDelays)),
		delivery("sos", LaneSOS, "email", EventSOS, 3),
	)
	w := testWorker(store, map[string]Channel{"email": ch}, 2)
	for w.claimAndSend(context.Background(), []string{LaneSOS, LaneNormal}) {
	}
	if next, ok := store.retried["first"]; !ok || time.Until(next) < 25*time.Second || time.Until(next) > 35*time.Second {
		t.Errorf("first failure: retry at %v (ok=%v), want ~30 s", time.Until(next), ok)
	}
	if msg := store.failed["last"]; !strings.Contains(msg, "451") {
		t.Errorf("last attempt should fail permanently with the error, got %q", msg)
	}
	if next, ok := store.retried["sos"]; !ok || time.Until(next) > 61*time.Second {
		t.Errorf("SOS after 4 attempts should retry within a minute, got %v (ok=%v)", time.Until(next), ok)
	}
}

func TestUnknownChannelIsAFailure(t *testing.T) {
	store := newFakeStore(delivery("x", LaneNormal, "pigeon", EventAbsent, len(normalDelays)))
	w := testWorker(store, map[string]Channel{}, 2)
	w.claimAndSend(context.Background(), []string{LaneNormal})
	if !strings.Contains(store.failed["x"], "unsupported channel") {
		t.Errorf("failed = %v", store.failed)
	}
}

func TestWakeSignalBroadcasts(t *testing.T) {
	w := testWorker(newFakeStore(), nil, 2)
	a, b := w.wakeSignal(), w.wakeSignal()
	w.broadcast()
	for _, ch := range []<-chan struct{}{a, b} {
		select {
		case <-ch:
		default:
			t.Fatal("waiter not woken")
		}
	}
	select {
	case <-w.wakeSignal():
		t.Fatal("new signal already closed")
	default:
	}
}

func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatal("condition not met in time")
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// ── SMTP timeouts ────────────────────────────────────────────────────────────

// silentServer accepts connections and never answers (a hung SMTP server).
func silentServer(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	var conns []net.Conn
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			mu.Lock()
			conns = append(conns, c)
			mu.Unlock()
		}
	}()
	t.Cleanup(func() {
		_ = ln.Close()
		mu.Lock()
		defer mu.Unlock()
		for _, c := range conns {
			_ = c.Close()
		}
	})
	return ln.Addr().String()
}

func TestSMTPTimesOutOnSilentServer(t *testing.T) {
	addr := silentServer(t)
	for _, implicitTLS := range []bool{false, true} {
		ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
		start := time.Now()
		err := sendSMTP(ctx, addr, "localhost", implicitTLS, nil, "a@example.com", "b@example.com", []byte("x"))
		cancel()
		if err == nil {
			t.Fatalf("implicitTLS=%v: send to a silent server succeeded", implicitTLS)
		}
		if d := time.Since(start); d > 2*time.Second {
			t.Errorf("implicitTLS=%v: send took %s, want ~300 ms", implicitTLS, d)
		}
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Errorf("implicitTLS=%v: error %v should wrap context.DeadlineExceeded", implicitTLS, err)
		}
	}
}

func TestEmailChannelTimesOutViaSettingsPort(t *testing.T) {
	addr := silentServer(t)
	host, port, _ := net.SplitHostPort(addr)
	cfg := settings.Channels{}
	cfg.SMTP.Host, cfg.SMTP.Port, cfg.SMTP.From = host, port, "sp@example.com"
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	start := time.Now()
	if err := (emailChannel{}).Send(ctx, cfg, "ops@example.com", "s", "b"); err == nil {
		t.Fatal("expected a timeout")
	}
	if d := time.Since(start); d > 2*time.Second {
		t.Errorf("e-mail send took %s", d)
	}
}

// fakeSMTP is a minimal SMTP server that accepts one message.
func fakeSMTP(t *testing.T) (string, <-chan string) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	got := make(chan string, 1)
	go func() {
		c, err := ln.Accept()
		if err != nil {
			return
		}
		defer c.Close()
		r := bufio.NewReader(c)
		write := func(s string) { _, _ = c.Write([]byte(s + "\r\n")) }
		write("220 localhost ESMTP test")
		var data strings.Builder
		for {
			line, err := r.ReadString('\n')
			if err != nil {
				return
			}
			cmd := strings.ToUpper(strings.TrimSpace(line))
			switch {
			case strings.HasPrefix(cmd, "EHLO"):
				write("250-localhost")
				write("250 8BITMIME")
			case strings.HasPrefix(cmd, "MAIL"), strings.HasPrefix(cmd, "RCPT"):
				write("250 OK")
			case cmd == "DATA":
				write("354 go ahead")
				for {
					l, err := r.ReadString('\n')
					if err != nil {
						return
					}
					if l == ".\r\n" {
						break
					}
					data.WriteString(l)
				}
				write("250 queued")
				got <- data.String()
			case cmd == "QUIT":
				write("221 bye")
				return
			default:
				write("502 unknown")
			}
		}
	}()
	return ln.Addr().String(), got
}

func TestSMTPDeliversMessage(t *testing.T) {
	addr, got := fakeSMTP(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	msg := []byte("Subject: hi\r\n\r\nhello\r\n")
	if err := sendSMTP(ctx, addr, "localhost", false, nil, "a@example.com", "b@example.com", msg); err != nil {
		t.Fatal(err)
	}
	select {
	case d := <-got:
		if !strings.Contains(d, "hello") {
			t.Errorf("server received %q", d)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("server got no message")
	}
}
