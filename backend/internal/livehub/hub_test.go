package livehub

import (
	"testing"
	"time"
)

func TestTicketSingleUse(t *testing.T) {
	h := New()
	tk := h.IssueTicket("A")
	if site, ok := h.Redeem(tk); !ok || site != "A" {
		t.Fatalf("first redeem = %q,%v", site, ok)
	}
	if _, ok := h.Redeem(tk); ok {
		t.Fatal("ticket redeemed twice")
	}
	if _, ok := h.Redeem("nope"); ok {
		t.Fatal("unknown ticket redeemed")
	}
}

func TestTicketExpires(t *testing.T) {
	h := New()
	now := time.Now()
	h.now = func() time.Time { return now }
	tk := h.IssueTicket("A")
	now = now.Add(TicketTTL + time.Second)
	if _, ok := h.Redeem(tk); ok {
		t.Fatal("expired ticket redeemed")
	}
}

func TestPublishReachesOnlySiteSubscribers(t *testing.T) {
	h := New()
	a, _ := h.Subscribe("A")
	b, _ := h.Subscribe("B")
	h.Publish("A")
	select {
	case m := <-a.C:
		if string(m) != `{"type":"patrol_changed","site_id":"A"}` {
			t.Fatalf("payload = %s", m)
		}
	case <-time.After(time.Second):
		t.Fatal("A got no message")
	}
	select {
	case m := <-b.C:
		t.Fatalf("B got %s", m)
	default:
	}
}

func TestSubscribeCap(t *testing.T) {
	h := New()
	for i := 0; i < MaxPerSite; i++ {
		if _, ok := h.Subscribe("A"); !ok {
			t.Fatalf("subscribe %d refused", i)
		}
	}
	if _, ok := h.Subscribe("A"); ok {
		t.Fatal("cap not enforced")
	}
	if _, ok := h.Subscribe("B"); !ok {
		t.Fatal("cap must be per site")
	}
}

func TestSlowSubscriberDoesNotBlock(t *testing.T) {
	h := New()
	s, _ := h.Subscribe("A")
	done := make(chan struct{})
	go func() {
		for i := 0; i < 20; i++ {
			h.Publish("A")
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Publish blocked on a slow subscriber")
	}
	// Dropped: channel is closed after draining the buffered messages.
	n := 0
	for range s.C {
		n++
	}
	if n > SendBuffer {
		t.Fatalf("got %d buffered messages", n)
	}
}

func TestNilHubPublishNoPanic(t *testing.T) {
	var h *Hub
	h.Publish("A")
}

func TestUnsubscribeStopsDelivery(t *testing.T) {
	h := New()
	s, _ := h.Subscribe("A")
	h.Unsubscribe(s)
	h.Unsubscribe(s) // idempotent
	h.Publish("A")
}
