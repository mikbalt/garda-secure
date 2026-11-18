package handler

import (
	"time"

	"github.com/gofiber/contrib/websocket"
	"github.com/gofiber/fiber/v2"

	"github.com/securepatrol/backend/internal/livehub"
)

const (
	wsPingEvery = 25 * time.Second
	wsReadLimit = 60 * time.Second // a pong (or any frame) must arrive within this
	wsSubKey    = "live_sub"
)

// POST /ws/ticket {site_id} — a 30 s single-use ticket for GET /ws/patrol.
// Browsers cannot send an Authorization header on a WebSocket, and a JWT in
// the URL would end up in proxy logs, so the socket is opened with this
// ticket instead. The site/realm check happens here (h.siteFor).
func (h *Handler) wsTicket(c *fiber.Ctx) error {
	var b struct {
		SiteID string `json:"site_id"`
	}
	if err := parse(c, &b); err != nil {
		return h.fail(c, err)
	}
	site, err := h.siteFor(c, b.SiteID)
	if err != nil {
		return h.fail(c, err)
	}
	return c.JSON(fiber.Map{"ticket": h.Live.IssueTicket(site.ID), "expires_in": int(livehub.TicketTTL / time.Second)})
}

// wsPatrol is GET /ws/patrol?ticket= — it pushes {"type":"patrol_changed",
// "site_id"} whenever a patrol of the ticket's site changes. Registered
// outside the JWT chain: the ticket is the credential.
func (h *Handler) wsPatrol() []fiber.Handler {
	gate := func(c *fiber.Ctx) error {
		if !websocket.IsWebSocketUpgrade(c) {
			return c.Status(fiber.StatusUpgradeRequired).JSON(fiber.Map{"error": "websocket upgrade required", "code": "upgrade_required"})
		}
		siteID, ok := h.Live.Redeem(c.Query("ticket"))
		if !ok {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "invalid or expired ticket", "code": "invalid_ticket"})
		}
		sub, ok := h.Live.Subscribe(siteID)
		if !ok {
			return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{"error": "too many live connections for this site", "code": "ws_site_full"})
		}
		c.Locals(wsSubKey, sub)
		return c.Next()
	}
	serve := websocket.New(func(conn *websocket.Conn) {
		sub, _ := conn.Locals(wsSubKey).(*livehub.Sub)
		if sub == nil {
			return
		}
		defer h.Live.Unsubscribe(sub)

		_ = conn.SetReadDeadline(time.Now().Add(wsReadLimit))
		conn.SetPongHandler(func(string) error {
			return conn.SetReadDeadline(time.Now().Add(wsReadLimit))
		})
		// The client sends nothing we care about; reading detects close/timeout.
		gone := make(chan struct{})
		go func() {
			defer close(gone)
			for {
				if _, _, err := conn.ReadMessage(); err != nil {
					return
				}
			}
		}()

		tick := time.NewTicker(wsPingEvery)
		defer tick.Stop()
		for {
			select {
			case msg, ok := <-sub.C:
				if !ok { // dropped for being too slow
					return
				}
				if err := conn.WriteMessage(websocket.TextMessage, msg); err != nil {
					return
				}
			case <-tick.C:
				if err := conn.WriteControl(websocket.PingMessage, nil, time.Now().Add(5*time.Second)); err != nil {
					return
				}
			case <-gone:
				return
			}
		}
	})
	return []fiber.Handler{gate, serve}
}

// publishPatrol nudges the live patrol subscribers of a site. It never
// blocks and never fails the request.
func (h *Handler) publishPatrol(siteID string) { h.Live.Publish(siteID) }
