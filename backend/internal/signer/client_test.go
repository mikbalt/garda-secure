package signer

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

const site = "11111111-2222-4333-8444-555555555555"

func TestClientOutcomes(t *testing.T) {
	var status atomic.Int32
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.Method != http.MethodPost || r.URL.Query().Get("site_id") != site {
			t.Errorf("request %s %s", r.Method, r.URL)
		}
		switch status.Load() {
		case 200:
			_, _ = w.Write([]byte(`{"seq": 7}`))
		default:
			http.Error(w, "signer: head is lower than an already signed head", int(status.Load()))
		}
	}))
	defer srv.Close()
	now := time.Now()
	c := NewClient(srv.URL, time.Second)
	c.now = func() time.Time { return now }
	ctx := context.Background()

	status.Store(409)
	if o := c.SignNow(ctx, site); o.Status != OutcomeRefused || o.Signed() || o.Reason == "" {
		t.Fatalf("refused: %+v", o)
	}
	status.Store(500)
	if o := c.SignNow(ctx, site); o.Status != OutcomeUnreachable || o.Signed() {
		t.Fatalf("server error: %+v", o)
	}
	status.Store(200)
	if o := c.SignNow(ctx, site); o.Status != OutcomeSigned || o.Seq != 7 || !o.Signed() {
		t.Fatalf("signed: %+v", o)
	}
	// within the window the head is reused: no second request
	n := calls.Load()
	now = now.Add(ReuseWindow / 2)
	if o := c.SignNow(ctx, site); o.Status != OutcomeReused || o.Seq != 7 || calls.Load() != n {
		t.Fatalf("reused: %+v, %d new calls", o, calls.Load()-n)
	}
	now = now.Add(ReuseWindow)
	if c.SignNow(ctx, site); calls.Load() != n+1 {
		t.Fatalf("after the window the signer is asked again")
	}
	if o := c.SignNow(ctx, "not-a-uuid"); o.Status != OutcomeRefused {
		t.Fatalf("bad site: %+v", o)
	}

	srv.Close()
	now = now.Add(time.Hour)
	if o := c.SignNow(ctx, site); o.Status != OutcomeUnreachable {
		t.Fatalf("down: %+v", o)
	}
}
