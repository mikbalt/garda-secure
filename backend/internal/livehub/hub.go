// Package livehub is the in-process fan-out behind the live patrol WebSocket
// (D-014 follow-up). It carries "something changed" nudges only; clients
// refetch the REST endpoint. It is single-instance by design: to scale the
// API out, replace Publish/Subscribe with Redis pub/sub behind the same API.
package livehub

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"sync"
	"time"
)

const (
	// TicketTTL is how long a WebSocket ticket can be redeemed.
	TicketTTL = 30 * time.Second
	// MaxPerSite caps concurrent subscribers per site.
	MaxPerSite = 50
	// SendBuffer is the per-subscriber queue; a subscriber that falls
	// behind is dropped rather than blocking publishers.
	SendBuffer = 8
)

// Sub is one subscriber. C is closed when the subscriber is dropped.
type Sub struct {
	C    <-chan []byte
	ch   chan []byte
	site string
}

type ticket struct {
	site string
	exp  time.Time
}

// Hub holds subscribers per site and single-use tickets.
type Hub struct {
	mu      sync.Mutex
	subs    map[string]map[*Sub]struct{}
	tickets map[string]ticket
	now     func() time.Time
}

// New returns an empty hub.
func New() *Hub {
	return &Hub{
		subs:    map[string]map[*Sub]struct{}{},
		tickets: map[string]ticket{},
		now:     time.Now,
	}
}

// IssueTicket returns a single-use ticket bound to siteID.
func (h *Hub) IssueTicket(siteID string) string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic("livehub: crypto/rand failed: " + err.Error())
	}
	tk := hex.EncodeToString(b)
	h.mu.Lock()
	defer h.mu.Unlock()
	now := h.now()
	for k, t := range h.tickets {
		if now.After(t.exp) {
			delete(h.tickets, k)
		}
	}
	h.tickets[tk] = ticket{site: siteID, exp: now.Add(TicketTTL)}
	return tk
}

// Redeem consumes a ticket and returns the site it was issued for.
func (h *Hub) Redeem(tk string) (string, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	t, ok := h.tickets[tk]
	if !ok {
		return "", false
	}
	delete(h.tickets, tk)
	if h.now().After(t.exp) {
		return "", false
	}
	return t.site, true
}

// Subscribe registers a subscriber for siteID; ok is false at the per-site cap.
func (h *Hub) Subscribe(siteID string) (*Sub, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	set := h.subs[siteID]
	if len(set) >= MaxPerSite {
		return nil, false
	}
	if set == nil {
		set = map[*Sub]struct{}{}
		h.subs[siteID] = set
	}
	ch := make(chan []byte, SendBuffer)
	s := &Sub{C: ch, ch: ch, site: siteID}
	set[s] = struct{}{}
	return s, true
}

// Unsubscribe removes s and closes its channel. Safe to call twice.
func (h *Hub) Unsubscribe(s *Sub) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.dropLocked(s)
}

func (h *Hub) dropLocked(s *Sub) {
	set := h.subs[s.site]
	if _, ok := set[s]; !ok {
		return
	}
	delete(set, s)
	if len(set) == 0 {
		delete(h.subs, s.site)
	}
	close(s.ch)
}

// Publish nudges every subscriber of siteID. It never blocks and is a no-op
// on a nil hub.
func (h *Hub) Publish(siteID string) {
	if h == nil {
		return
	}
	msg, _ := json.Marshal(struct {
		Type   string `json:"type"`
		SiteID string `json:"site_id"`
	}{"patrol_changed", siteID})
	h.mu.Lock()
	defer h.mu.Unlock()
	for s := range h.subs[siteID] {
		select {
		case s.ch <- msg:
		default:
			h.dropLocked(s)
		}
	}
}
