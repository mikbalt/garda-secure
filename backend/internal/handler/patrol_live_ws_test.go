package handler

import (
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/fasthttp/websocket"
	"github.com/gofiber/fiber/v2"
	"github.com/rs/zerolog"

	"github.com/securepatrol/backend/internal/config"
	"github.com/securepatrol/backend/internal/livehub"
)

// wsServer serves only the socket route (auth is the ticket, so no JWT chain).
func wsServer(t *testing.T, hub *livehub.Hub) (h *Handler, addr string) {
	t.Helper()
	h = &Handler{Live: hub, Log: zerolog.Nop()}
	app := fiber.New(AppConfig(&config.Config{}, zerolog.Nop()))
	app.Get("/api/v1/ws/patrol", h.wsPatrol()...)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go func() { _ = app.Listener(ln) }()
	t.Cleanup(func() { _ = app.Shutdown() })
	return h, ln.Addr().String()
}

func dial(addr, ticket string) (*websocket.Conn, *http.Response, error) {
	return websocket.DefaultDialer.Dial("ws://"+addr+"/api/v1/ws/patrol?ticket="+ticket, nil)
}

func TestWsPatrolRejectsBadTicket(t *testing.T) {
	_, addr := wsServer(t, livehub.New())
	_, res, err := dial(addr, "nope")
	if err == nil {
		t.Fatal("dial with unknown ticket succeeded")
	}
	if res == nil || res.StatusCode != 401 {
		t.Fatalf("response = %v, want 401", res)
	}
}

func TestWsPatrolDeliversNudge(t *testing.T) {
	hub := livehub.New()
	h, addr := wsServer(t, hub)
	conn, _, err := dial(addr, hub.IssueTicket("site-a"))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	// Give the server a moment to subscribe, then nudge.
	time.Sleep(100 * time.Millisecond)
	h.publishPatrol("site-b") // other site: must not arrive
	h.publishPatrol("site-a")
	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, msg, err := conn.ReadMessage()
	if err != nil {
		t.Fatal(err)
	}
	if string(msg) != `{"type":"patrol_changed","site_id":"site-a"}` {
		t.Fatalf("message = %s", msg)
	}
}

func TestWsPatrolTicketNotReusable(t *testing.T) {
	hub := livehub.New()
	_, addr := wsServer(t, hub)
	tk := hub.IssueTicket("site-a")
	c, _, err := dial(addr, tk)
	if err != nil {
		t.Fatal(err)
	}
	c.Close()
	if _, res, err := dial(addr, tk); err == nil || res == nil || res.StatusCode != 401 {
		t.Fatalf("second dial: err=%v res=%v, want 401", err, res)
	}
}

func TestWsPatrolFullSite(t *testing.T) {
	hub := livehub.New()
	_, addr := wsServer(t, hub)
	for i := 0; i < livehub.MaxPerSite; i++ {
		if _, ok := hub.Subscribe("site-a"); !ok {
			t.Fatal("setup subscribe failed")
		}
	}
	_, res, err := dial(addr, hub.IssueTicket("site-a"))
	if err == nil || res == nil || res.StatusCode != 503 {
		t.Fatalf("err=%v res=%v, want 503", err, res)
	}
}

func TestWsPatrolPlainHTTPIsRefused(t *testing.T) {
	_, addr := wsServer(t, livehub.New())
	res, err := http.Get("http://" + addr + "/api/v1/ws/patrol?ticket=x")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != 426 {
		t.Fatalf("status = %d, want 426", res.StatusCode)
	}
}

// Review I1: when the handler exits on its own (slow subscriber dropped) while
// the client keeps sending, the reader goroutine must not outlive the handler
// (the connection wrapper is recycled). Run with -race.
func TestWsPatrolDroppedSubscriberStopsReader(t *testing.T) {
	hub := livehub.New()
	h, addr := wsServer(t, hub)
	conn, _, err := dial(addr, hub.IssueTicket("site-a"))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	stop := make(chan struct{})
	defer close(stop)
	go func() { // chatty client
		for {
			select {
			case <-stop:
				return
			default:
				if conn.WriteMessage(websocket.TextMessage, []byte("x")) != nil {
					return
				}
				time.Sleep(time.Millisecond)
			}
		}
	}()
	time.Sleep(100 * time.Millisecond)
	for i := 0; i < 5000; i++ { // overflow the send buffer: the hub drops the subscriber
		h.publishPatrol("site-a")
	}
	// The server closes the connection once the handler returns.
	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	for {
		if _, _, err := conn.ReadMessage(); err != nil {
			break
		}
	}
	time.Sleep(200 * time.Millisecond) // let a stray reader touch the recycled conn
}
