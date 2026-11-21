//go:build integration

package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/rs/zerolog"

	"github.com/securepatrol/backend/internal/config"
	"github.com/securepatrol/backend/internal/middleware"
	"github.com/securepatrol/backend/internal/notification"
	"github.com/securepatrol/backend/internal/repository"
	"github.com/securepatrol/backend/internal/settings"
)

// The worker claims deliveries from Postgres, so SOS notifications go out
// even when Redis is down (BE-R1/R5), and failures follow the SOS retry
// schedule (BE-R2).
func TestWorkerDispatchesFromPostgresWithoutRedis(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	// Park deliveries left by other tests so only this test's rows are sent.
	if _, err := e.pool.Exec(ctx, `UPDATE notification_log SET status = 'failed' WHERE status = 'queued'`); err != nil {
		t.Fatal(err)
	}

	var fail atomic.Bool
	var calls atomic.Int32
	wa := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		calls.Add(1)
		if fail.Load() {
			http.Error(w, `{"error":"rate limited"}`, http.StatusInternalServerError)
			return
		}
		_, _ = w.Write([]byte(`{"messages":[{"id":"wamid.1"}]}`))
	}))
	defer wa.Close()

	admin := e.web(realm1, "admin-worker", "admin")
	siteID := e.expect(e.do("POST", "/api/v1/sites", admin, "", map[string]any{"name": "Worker Site"}), 201, "site").str("id")
	e.expect(e.do("POST", "/api/v1/notifications/rules", admin, "", map[string]any{
		"site_id": siteID, "event_type": "sos", "channel": "whatsapp", "recipient": "62811110000",
	}), 201, "sos rule")
	site, err := repository.GetSite(ctx, e.pool, siteID)
	if err != nil {
		t.Fatal(err)
	}

	cfg := &config.Config{SettingsEncryptionKey: "test-key", WhatsAppAPIURL: wa.URL,
		WhatsAppAPIToken: "tok", WhatsAppPhoneNumberID: "123"}
	st, err := settings.NewStore(e.pool, cfg)
	if err != nil {
		t.Fatal(err)
	}
	deadRedis := redis.NewClient(&redis.Options{Addr: "127.0.0.1:1", MaxRetries: -1})
	notifier := notification.NewNotifier(e.pool, deadRedis, zerolog.Nop())

	if n, err := notifier.Publish(ctx, notification.Event{Type: notification.EventSOS, Site: site,
		DedupeKey: "sos:worker-ok", Params: map[string]string{"guard": "Budi"}}); err != nil || n != 1 {
		t.Fatalf("publish with redis down: %d, %v", n, err)
	}

	w := notification.NewWorker(e.pool, deadRedis, st, notifier, 2, zerolog.Nop())
	wctx, stop := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() { w.Run(wctx); close(done) }()
	defer func() {
		stop()
		select {
		case <-done:
		case <-time.After(10 * time.Second):
			t.Error("worker did not stop")
		}
	}()

	e.waitCount(t, 10*time.Second, 1,
		`SELECT COUNT(*) FROM notification_log WHERE dedupe_key LIKE 'sos:worker-ok|%' AND status = 'sent' AND attempts = 1`)

	fail.Store(true)
	if _, err := notifier.Publish(ctx, notification.Event{Type: notification.EventSOS, Site: site,
		DedupeKey: "sos:worker-retry", Params: map[string]string{"guard": "Budi"}}); err != nil {
		t.Fatal(err)
	}
	// First failure: still queued, next attempt ~10 s later (SOS schedule).
	e.waitCount(t, 10*time.Second, 1, `SELECT COUNT(*) FROM notification_log
		WHERE dedupe_key LIKE 'sos:worker-retry|%' AND status = 'queued' AND attempts = 1
		  AND last_error LIKE '%status 500%' AND next_attempt_at > now() + interval '5 seconds'`)
	if w.Heartbeat().Before(time.Now().Add(-5 * time.Second)) {
		t.Error("dispatcher heartbeat is stale while running")
	}
}

// An SOS delivery that gives up commits together with its ops alert: when the
// alert cannot be staged the delivery stays queued and is attempted again, so
// an SOS never ends "failed" without anybody being told.
func TestUndeliveredSOSFailsOnlyWithItsOpsAlert(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := e.pool.Exec(ctx, sql, args...); err != nil {
			t.Fatal(err)
		}
	}
	exec(`UPDATE notification_log SET status = 'failed' WHERE status = 'queued'`)

	var calls atomic.Int32
	wa := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		calls.Add(1)
		http.Error(w, `{"error":"down"}`, http.StatusInternalServerError)
	}))
	defer wa.Close()

	admin := e.web(realm1, "admin-giveup", "admin")
	siteID := e.expect(e.do("POST", "/api/v1/sites", admin, "", map[string]any{"name": "Give-up Site"}), 201, "site").str("id")
	e.expect(e.do("POST", "/api/v1/notifications/rules", admin, "", map[string]any{
		"site_id": siteID, "event_type": "sos", "channel": "whatsapp", "recipient": "62811110001",
	}), 201, "sos rule")
	exec(`INSERT INTO ops_alert_rules (event_type, channel, recipient, enabled, created_by)
		VALUES ('ops_sos_undelivered', 'whatsapp', '62811119999', true, 'test')`)
	t.Cleanup(func() {
		_, _ = e.pool.Exec(ctx, `DELETE FROM ops_alert_rules WHERE recipient = '62811119999'`)
		_, _ = e.pool.Exec(ctx, `UPDATE notification_log SET status = 'failed' WHERE status = 'queued'`)
	})
	site, err := repository.GetSite(ctx, e.pool, siteID)
	if err != nil {
		t.Fatal(err)
	}

	cfg := &config.Config{SettingsEncryptionKey: "test-key", WhatsAppAPIURL: wa.URL,
		WhatsAppAPIToken: "tok", WhatsAppPhoneNumberID: "123"}
	st, err := settings.NewStore(e.pool, cfg)
	if err != nil {
		t.Fatal(err)
	}
	notifier := notification.NewNotifier(e.pool, nil, zerolog.Nop())
	if n, err := notifier.Publish(ctx, notification.Event{Type: notification.EventSOS, Site: site,
		DedupeKey: "sos:giveup", Params: map[string]string{"guard": "Budi"}}); err != nil || n != 1 {
		t.Fatalf("publish: %d, %v", n, err)
	}
	// One attempt left before the SOS schedule gives up.
	last := 0
	for a := 1; ; a++ {
		if _, ok := notification.RetryDelay(notification.LaneSOS, a); !ok {
			last = a
			break
		}
	}
	exec(`UPDATE notification_log SET attempts = $1 WHERE dedupe_key LIKE 'sos:giveup|%'`, last-1)

	// The ops alert cannot be staged.
	exec(`CREATE FUNCTION test_block_ops_alert() RETURNS trigger AS $$
		BEGIN RAISE EXCEPTION 'ops alert blocked by test'; END $$ LANGUAGE plpgsql`)
	exec(`CREATE TRIGGER test_block_ops_alert BEFORE INSERT ON notification_log FOR EACH ROW
		WHEN (NEW.event_type = 'ops_sos_undelivered') EXECUTE FUNCTION test_block_ops_alert()`)
	blocked := true
	unblock := func() {
		if blocked {
			blocked = false
			_, _ = e.pool.Exec(ctx, `DROP TRIGGER IF EXISTS test_block_ops_alert ON notification_log`)
			_, _ = e.pool.Exec(ctx, `DROP FUNCTION IF EXISTS test_block_ops_alert()`)
		}
	}
	t.Cleanup(unblock)

	w := notification.NewWorker(e.pool, nil, st, notifier, 2, zerolog.Nop())
	wctx, stop := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() { w.Run(wctx); close(done) }()
	defer func() {
		stop()
		select {
		case <-done:
		case <-time.After(10 * time.Second):
			t.Error("worker did not stop")
		}
	}()

	deadline := time.Now().Add(10 * time.Second)
	for calls.Load() == 0 {
		if time.Now().After(deadline) {
			t.Fatal("the last SOS attempt was never sent")
		}
		time.Sleep(50 * time.Millisecond)
	}
	time.Sleep(1500 * time.Millisecond) // bookkeeping of that attempt
	if n := e.count(`SELECT COUNT(*) FROM notification_log WHERE dedupe_key LIKE 'sos:giveup|%' AND status = 'queued'`); n != 1 {
		t.Fatalf("SOS delivery gave up although its ops alert could not be staged (queued rows = %d)", n)
	}

	// The alert can be staged again; the lease runs out; the delivery is attempted again.
	unblock()
	exec(`UPDATE notification_log SET locked_until = NULL WHERE dedupe_key LIKE 'sos:giveup|%'`)
	e.waitCount(t, 10*time.Second, 1, `SELECT COUNT(*) FROM notification_log WHERE dedupe_key LIKE 'sos:giveup|%' AND status = 'failed'`)
	if n := e.count(`SELECT COUNT(*) FROM notification_log
		WHERE event_type = 'ops_sos_undelivered' AND dedupe_key LIKE 'ops:sos_failed:%' AND recipient = '62811119999'`); n != 1 {
		t.Fatalf("ops alerts for the undelivered SOS = %d, want 1", n)
	}
}

// Identical clock-in retries racing each other return the stored record
// instead of a 409 (BE-R6).
func TestConcurrentClockInRetries(t *testing.T) {
	e := newEnv(t)
	admin := e.web(realm1, "admin-race", "admin")
	siteID := e.expect(e.do("POST", "/api/v1/sites", admin, "", map[string]any{"name": "Race Site"}), 201, "site").str("id")
	guard := e.expect(e.do("POST", "/api/v1/guards", admin, "", map[string]any{
		"name": "Racer", "site_id": siteID, "role": "guard",
	}), 201, "guard").str("id")
	tablet := e.token(realm1, "device-race", "securepatrol-tablet", "device")
	dev := e.expect(e.do("POST", "/api/v1/device/register", tablet, "", map[string]any{"name": "Race"}), 200, "register").str("id")
	e.expect(e.do("PUT", "/api/v1/devices/"+dev, admin, "", map[string]any{"status": "active", "site_id": siteID}), 200, "approve")
	now := time.Now().UTC()
	e.expect(e.do("POST", "/api/v1/shifts", admin, "", map[string]any{
		"site_id": siteID, "guard_id": guard, "start_time": rfc(now.Add(-2 * time.Minute)), "end_time": rfc(now.Add(8 * time.Hour)),
	}), 201, "shift")

	id := uuid.NewString()
	const n = 16
	statuses := make([]int, n)
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			statuses[i] = e.doConcurrent("POST", "/api/v1/attendance/clock-in", tablet, guard, map[string]any{"id": id})
		}(i)
	}
	close(start)
	wg.Wait()
	created := 0
	for _, s := range statuses {
		switch s {
		case 201:
			created++
		case 200:
		default:
			t.Fatalf("concurrent identical clock-ins: statuses %v", statuses)
		}
	}
	if created != 1 {
		t.Fatalf("created %d records, want 1 (statuses %v)", created, statuses)
	}
	if c := e.count(`SELECT COUNT(*) FROM attendance_records WHERE guard_id = $1`, guard); c != 1 {
		t.Fatalf("attendance records = %d", c)
	}
}

// waitCount polls a COUNT query until it returns want.
func (e *env) waitCount(t *testing.T, timeout time.Duration, want int, sql string, args ...any) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		var n int
		if err := e.pool.QueryRow(context.Background(), sql, args...).Scan(&n); err != nil {
			t.Fatalf("count %q: %v", sql, err)
		}
		if n == want {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out: %q = %d, want %d", sql, n, want)
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func (e *env) doRaw(method, path, token, guardID string, body any) *http.Response {
	b, _ := json.Marshal(body)
	req := httptest.NewRequest(method, path, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	if guardID != "" {
		req.Header.Set(middleware.GuardHeader, guardID)
	}
	res, err := e.app.Test(req, 30_000)
	if err != nil {
		return nil
	}
	return res
}

// doConcurrent is e.do for goroutines: it returns the status instead of
// failing the test from another goroutine.
func (e *env) doConcurrent(method, path, token, guardID string, body any) int {
	r := e.doRaw(method, path, token, guardID, body)
	if r == nil {
		return -1
	}
	defer r.Body.Close()
	_, _ = io.Copy(io.Discard, r.Body)
	return r.StatusCode
}
