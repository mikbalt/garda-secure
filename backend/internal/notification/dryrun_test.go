package notification

// TAQ-7 dry run: the real e-mail and WhatsApp channels, templates and worker
// retry logic exercised against local stand-ins (an in-process SMTP catcher and
// an httptest WhatsApp Cloud API). No Docker, no network, no credentials.
// Live delivery still needs real accounts - see docs/runbook.md, "Live
// delivery test checklist".

import (
	"bufio"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/securepatrol/backend/internal/netguard"
	"github.com/securepatrol/backend/internal/settings"
)

// catcher is a tiny SMTP server that records messages (like Mailpit).
type catcher struct {
	addr string
	// behaviour
	authFail  bool   // answer 535 to AUTH
	rcptReply string // reply to RCPT (default "250 OK")
	dataReply string // reply after DATA (default "250 queued")

	mu    sync.Mutex
	mails []caught
}

type caught struct {
	from, rcpt, auth, data string
}

func (c *catcher) all() []caught {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]caught(nil), c.mails...)
}

func newCatcher(t *testing.T, mutate func(*catcher)) *catcher {
	t.Helper()
	c := &catcher{rcptReply: "250 OK", dataReply: "250 queued"}
	if mutate != nil {
		mutate(c)
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	c.addr = ln.Addr().String()
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go c.serve(conn)
		}
	}()
	return c
}

func (c *catcher) serve(conn net.Conn) {
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(10 * time.Second))
	r := bufio.NewReader(conn)
	w := func(s string) { _, _ = conn.Write([]byte(s + "\r\n")) }
	w("220 catcher ESMTP")
	var m caught
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return
		}
		line = strings.TrimSpace(line)
		up := strings.ToUpper(line)
		switch {
		case strings.HasPrefix(up, "EHLO"):
			w("250-catcher")
			w("250-AUTH PLAIN")
			w("250 8BITMIME")
		case strings.HasPrefix(up, "AUTH PLAIN"):
			if c.authFail {
				w("535 5.7.8 authentication failed")
				continue
			}
			m.auth = strings.TrimSpace(line[len("AUTH PLAIN"):])
			w("235 ok")
		case strings.HasPrefix(up, "MAIL FROM:"):
			m.from, _, _ = strings.Cut(strings.TrimPrefix(line[len("MAIL FROM:"):], "<"), ">")
			w("250 OK")
		case strings.HasPrefix(up, "RCPT TO:"):
			m.rcpt, _, _ = strings.Cut(strings.TrimPrefix(line[len("RCPT TO:"):], "<"), ">")
			w(c.rcptReply)
		case up == "DATA":
			w("354 go")
			var b strings.Builder
			for {
				l, err := r.ReadString('\n')
				if err != nil {
					return
				}
				if l == ".\r\n" {
					break
				}
				b.WriteString(l)
			}
			w(c.dataReply)
			if strings.HasPrefix(c.dataReply, "250") {
				m.data = b.String()
				c.mu.Lock()
				c.mails = append(c.mails, m)
				c.mu.Unlock()
			}
		case up == "QUIT":
			w("221 bye")
			return
		default:
			w("250 OK")
		}
	}
}

func smtpCfg(addr string) settings.Channels {
	host, port, _ := net.SplitHostPort(addr)
	var cfg settings.Channels
	cfg.SMTP = settings.SMTP{Host: host, Port: port, User: "garda@example.com", Password: "pw-dry-run",
		From: "GARDA <garda@example.com>", Guard: netguard.AllowAll}
	return cfg
}

func sendCtx() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), 5*time.Second)
}

func TestDryRunEmailRendersEveryEventInBothLanguages(t *testing.T) {
	params := map[string]string{"guard": "Budi Çelik", "site": "Plant 1", "time": "08:00", "location": "Gate A",
		"severity": "high", "type": "theft", "description": "d", "shift": "Pagi", "minutes": "7", "route": "R1",
		"checkpoint": "CP1", "due": "08:30", "defect": "lamp", "month": "2026-10", "date": "2026-10-01",
		"grace_until": "08:45", "escalation": "1", "reason": "r", "device": "Tab-1", "actor": "Sup",
		"status": "ok", "backup": "b1", "trigger": "cron", "error": "e", "last": "x", "hour": "2",
		"recipient": "x@y.z", "job": "j", "failures": "3"}
	cat := newCatcher(t, nil)
	cfg := smtpCfg(cat.addr)
	n := 0
	for lang := range templates {
		for ev := range templates[lang] {
			subj, body := Render(ev, lang, params)
			if strings.Contains(subj+body, "{") || strings.Contains(subj+body, "}") {
				t.Errorf("%s/%s: unfilled placeholder in %q / %q", lang, ev, subj, body)
			}
			ctx, cancel := sendCtx()
			err := (emailChannel{}).Send(ctx, cfg, "ops@example.com", subj, body)
			cancel()
			if err != nil {
				t.Fatalf("%s/%s: %v", lang, ev, err)
			}
			n++
		}
	}
	mails := cat.all()
	if len(mails) != n {
		t.Fatalf("caught %d mails, sent %d", len(mails), n)
	}
}

func TestDryRunEmailMessageShape(t *testing.T) {
	cat := newCatcher(t, nil)
	cfg := smtpCfg(cat.addr)
	subj, body := Render(EventSOS, "id", map[string]string{"guard": "Budi", "site": "Pabrik Ä", "time": "08:00", "location": "Gate"})
	ctx, cancel := sendCtx()
	defer cancel()
	if err := (emailChannel{}).Send(ctx, cfg, "ops@example.com", subj, body); err != nil {
		t.Fatal(err)
	}
	m := cat.all()[0]
	if m.from != "garda@example.com" || m.rcpt != "ops@example.com" {
		t.Errorf("envelope from=%q rcpt=%q", m.from, m.rcpt)
	}
	wantAuth := base64.StdEncoding.EncodeToString([]byte("\x00garda@example.com\x00pw-dry-run"))
	if m.auth != wantAuth {
		t.Errorf("AUTH PLAIN credentials not sent from settings")
	}
	hdr, rest, _ := strings.Cut(m.data, "\r\n\r\n")
	for _, want := range []string{"From: \"GARDA\" <garda@example.com>", "To: <ops@example.com>",
		"MIME-Version: 1.0", "Content-Type: text/plain; charset=UTF-8"} {
		if !strings.Contains(hdr, want) {
			t.Errorf("header missing %q in:\n%s", want, hdr)
		}
	}
	var gotSubj string
	for _, l := range strings.Split(hdr, "\r\n") {
		if v, ok := strings.CutPrefix(l, "Subject: "); ok {
			gotSubj, _ = new(mime.WordDecoder).DecodeHeader(v)
		}
	}
	if gotSubj != subj {
		t.Errorf("subject decodes to %q, want %q", gotSubj, subj)
	}
	if !strings.Contains(rest, "SOS! Budi") {
		t.Errorf("body = %q", rest)
	}
}

func TestDryRunEmailHTMLAndHeaderInjection(t *testing.T) {
	cat := newCatcher(t, nil)
	cfg := smtpCfg(cat.addr)
	ctx, cancel := sendCtx()
	defer cancel()
	// A newline in the subject must not start a new header.
	if err := (emailChannel{}).Send(ctx, cfg, "ops@example.com", "hi\r\nBcc: evil@example.com", "<p>report</p>"); err != nil {
		t.Fatal(err)
	}
	m := cat.all()[0]
	hdr, _, _ := strings.Cut(m.data, "\r\n\r\n")
	if !strings.Contains(hdr, "Content-Type: text/html; charset=UTF-8") {
		t.Errorf("html body not flagged text/html:\n%s", hdr)
	}
	if strings.Contains(hdr, "\r\nBcc:") {
		t.Errorf("header injection through subject:\n%s", hdr)
	}
	// A recipient that tries to add an address is rejected before any dial.
	err := (emailChannel{}).Send(ctx, cfg, "a@example.com\r\nRCPT TO:<x@y.z>", "s", "b")
	if !errors.Is(err, ErrInvalidRecipient) {
		t.Errorf("injected recipient: %v", err)
	}
}

func TestDryRunEmailConfigErrors(t *testing.T) {
	ctx, cancel := sendCtx()
	defer cancel()
	if err := (emailChannel{}).Send(ctx, settings.Channels{}, "a@example.com", "s", "b"); !errors.Is(err, ErrNotConfigured) {
		t.Errorf("no host: %v", err)
	}
	cfg := smtpCfg("127.0.0.1:1")
	if err := (emailChannel{}).Send(ctx, cfg, "not-an-address", "s", "b"); !errors.Is(err, ErrInvalidRecipient) {
		t.Errorf("bad recipient: %v", err)
	}
	cfg.SMTP.From, cfg.SMTP.User = "", ""
	if err := (emailChannel{}).Send(ctx, cfg, "a@example.com", "s", "b"); err == nil || !strings.Contains(err.Error(), "from address") {
		t.Errorf("missing from: %v", err)
	}
	// Connection refused is an ordinary (retryable) error.
	cfg = smtpCfg("127.0.0.1:1")
	if err := (emailChannel{}).Send(ctx, cfg, "a@example.com", "s", "b"); err == nil || !strings.Contains(err.Error(), "smtp dial") {
		t.Errorf("refused: %v", err)
	}
}

func TestDryRunEmailServerFailures(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*catcher)
		want   string
	}{
		{"auth rejected", func(c *catcher) { c.authFail = true }, "smtp auth"},
		{"recipient rejected", func(c *catcher) { c.rcptReply = "550 5.1.1 no such user" }, "smtp rcpt to"},
		{"greylisted at DATA", func(c *catcher) { c.dataReply = "451 4.7.1 try later" }, "smtp end of data"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cat := newCatcher(t, tc.mutate)
			ctx, cancel := sendCtx()
			defer cancel()
			err := (emailChannel{}).Send(ctx, smtpCfg(cat.addr), "ops@example.com", "s", "b")
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want %q", err, tc.want)
			}
			if n := len(cat.all()); n != 0 {
				t.Errorf("failed send stored %d mails", n)
			}
		})
	}
}

// ── WhatsApp ─────────────────────────────────────────────────────────────────

type waCall struct {
	path, auth, ctype string
	payload           map[string]any
}

// fakeWhatsApp stands in for the Cloud API: it records calls and answers with
// the given status/body.
func fakeWhatsApp(t *testing.T, status int, reply string) (*httptest.Server, func() []waCall) {
	t.Helper()
	var mu sync.Mutex
	var calls []waCall
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		var p map[string]any
		_ = json.Unmarshal(b, &p)
		mu.Lock()
		calls = append(calls, waCall{r.URL.Path, r.Header.Get("Authorization"), r.Header.Get("Content-Type"), p})
		mu.Unlock()
		w.WriteHeader(status)
		_, _ = io.WriteString(w, reply)
	}))
	t.Cleanup(srv.Close)
	return srv, func() []waCall { mu.Lock(); defer mu.Unlock(); return append([]waCall(nil), calls...) }
}

func waCfg(url string) settings.Channels {
	var cfg settings.Channels
	cfg.WhatsApp = settings.WhatsApp{APIURL: url + "/v18.0/", Token: "tok-dry-run", PhoneNumberID: "1234567890",
		TemplateName: "garda_alert", TemplateLang: "id"}
	return cfg
}

func waChannel() Channel { return Channels["whatsapp"].New() }

func TestDryRunWhatsAppTemplatePayload(t *testing.T) {
	srv, calls := fakeWhatsApp(t, 200, `{"messages":[{"id":"wamid.X"}]}`)
	to, err := NormalizeRecipient("whatsapp", "0812-3456-789")
	if err != nil {
		t.Fatal(err)
	}
	subj, body := Render(EventIncidentHigh, "id", map[string]string{"severity": "high", "type": "theft", "site": "Plant",
		"guard": "Budi", "time": "08:00", "location": "Gate A", "description": "line1\nline2\ttab"})
	ctx, cancel := sendCtx()
	defer cancel()
	if err := waChannel().Send(ctx, waCfg(srv.URL), to, subj, body); err != nil {
		t.Fatal(err)
	}
	c := calls()
	if len(c) != 1 {
		t.Fatalf("%d API calls", len(c))
	}
	if c[0].path != "/v18.0/1234567890/messages" || c[0].auth != "Bearer tok-dry-run" || c[0].ctype != "application/json" {
		t.Errorf("request = %+v", c[0])
	}
	p := c[0].payload
	if p["messaging_product"] != "whatsapp" || p["to"] != "628123456789" || p["type"] != "template" {
		t.Errorf("payload = %v", p)
	}
	tpl := p["template"].(map[string]any)
	if tpl["name"] != "garda_alert" || tpl["language"].(map[string]any)["code"] != "id" {
		t.Errorf("template = %v", tpl)
	}
	param := tpl["components"].([]any)[0].(map[string]any)["parameters"].([]any)[0].(map[string]any)
	text := param["text"].(string)
	if strings.ContainsAny(text, "\n\r\t") {
		t.Errorf("template parameter has newline/tab (Meta rejects it): %q", text)
	}
	if !strings.Contains(text, subj) || !strings.Contains(text, "line1 | line2") {
		t.Errorf("parameter text = %q", text)
	}
}

func TestDryRunWhatsAppPlainTextAndTruncation(t *testing.T) {
	srv, calls := fakeWhatsApp(t, 200, `{}`)
	cfg := waCfg(srv.URL)
	cfg.WhatsApp.TemplateName = "" // session (non-template) message
	ctx, cancel := sendCtx()
	defer cancel()
	if err := waChannel().Send(ctx, cfg, "628123456789", "subj", "body\nline2"); err != nil {
		t.Fatal(err)
	}
	cfg.WhatsApp.TemplateName = "garda_alert"
	if err := waChannel().Send(ctx, cfg, "628123456789", strings.Repeat("x", 3000), ""); err != nil {
		t.Fatal(err)
	}
	c := calls()
	if c[0].payload["type"] != "text" || c[0].payload["text"].(map[string]any)["body"] != "subj\nbody\nline2" {
		t.Errorf("text payload = %v", c[0].payload)
	}
	param := c[1].payload["template"].(map[string]any)["components"].([]any)[0].(map[string]any)["parameters"].([]any)[0].(map[string]any)
	if n := len([]rune(param["text"].(string))); n != 1000 {
		t.Errorf("template parameter length %d, want 1000", n)
	}
}

func TestDryRunWhatsAppFailures(t *testing.T) {
	ctx, cancel := sendCtx()
	defer cancel()
	for _, tc := range []struct {
		status int
		reply  string
	}{
		{401, `{"error":{"message":"Invalid OAuth access token","code":190}}`},
		{400, `{"error":{"message":"Template name does not exist in the translation","code":132001}}`},
		{429, `{"error":{"message":"rate limit hit","code":80007}}`},
		{500, `oops`},
		{302, ``}, // a redirect must not be followed (the token would leak)
	} {
		srv, calls := fakeWhatsApp(t, tc.status, tc.reply)
		err := waChannel().Send(ctx, waCfg(srv.URL), "628123456789", "s", "b")
		var se *StatusError
		if !errors.As(err, &se) || se.Status != tc.status {
			t.Fatalf("status %d: err = %v", tc.status, err)
		}
		if len(tc.reply) > 10 && !strings.Contains(se.Detail, tc.reply[:10]) {
			t.Errorf("status %d: detail %q lost the API reason", tc.status, se.Detail)
		}
		if len(calls()) != 1 {
			t.Errorf("status %d: %d calls", tc.status, len(calls()))
		}
	}
	// Network failure (server gone) is an ordinary error, retried by the worker.
	srv, _ := fakeWhatsApp(t, 200, `{}`)
	cfg := waCfg(srv.URL)
	srv.Close()
	if err := waChannel().Send(ctx, cfg, "628123456789", "s", "b"); err == nil {
		t.Error("closed server: expected an error")
	}
}

func TestDryRunWhatsAppConfigErrors(t *testing.T) {
	ctx, cancel := sendCtx()
	defer cancel()
	if err := waChannel().Send(ctx, settings.Channels{}, "628123456789", "s", "b"); !errors.Is(err, ErrNotConfigured) {
		t.Errorf("no token: %v", err)
	}
	if err := waChannel().Send(ctx, waCfg("http://127.0.0.1:1"), "123", "s", "b"); !errors.Is(err, ErrInvalidRecipient) {
		t.Errorf("short number: %v", err)
	}
}

// ── Worker: retries and give-up with the real channels ───────────────────────

func TestDryRunWorkerRetriesRealChannelsAndGivesUp(t *testing.T) {
	cat := newCatcher(t, func(c *catcher) { c.dataReply = "451 4.7.1 try later" })
	srv, calls := fakeWhatsApp(t, 503, `{"error":{"message":"service unavailable"}}`)
	cfg := smtpCfg(cat.addr)
	cfg.WhatsApp = waCfg(srv.URL).WhatsApp

	store := newFakeStore(
		delivery("mail-first", LaneNormal, "email", EventAbsent, 0),
		delivery("wa-first", LaneNormal, "whatsapp", EventAbsent, 0),
		delivery("mail-last", LaneNormal, "email", EventAbsent, len(normalDelays)),
		delivery("wa-last", LaneNormal, "whatsapp", EventAbsent, len(normalDelays)),
		delivery("wa-sos", LaneSOS, "whatsapp", EventSOS, 2),
	)
	for _, d := range store.rows {
		if d.Channel == "email" {
			d.Recipient = "ops@example.com"
		} else {
			d.Recipient = "628123456789"
		}
		d.Subject, d.Body = "s", "b"
	}
	w := testWorker(store, map[string]Channel{"email": emailChannel{}, "whatsapp": waChannel()}, 2)
	w.config = func(context.Context) (settings.Channels, error) { return cfg, nil }
	for w.claimAndSend(context.Background(), []string{LaneSOS, LaneNormal}) {
	}
	if len(calls()) != 3 {
		t.Errorf("WhatsApp API saw %d calls, want 3", len(calls()))
	}
	for _, id := range []string{"mail-first", "wa-first"} {
		if next, ok := store.retried[id]; !ok || time.Until(next) < 25*time.Second || time.Until(next) > 35*time.Second {
			t.Errorf("%s: retry at %v (ok=%v), want ~30 s", id, time.Until(next), ok)
		}
	}
	if next, ok := store.retried["wa-sos"]; !ok || time.Until(next) > 31*time.Second {
		t.Errorf("SOS retry should be quick, got %v (ok=%v)", time.Until(next), ok)
	}
	if m := store.failed["mail-last"]; !strings.Contains(m, "451") {
		t.Errorf("mail-last failed = %q", m)
	}
	if m := store.failed["wa-last"]; !strings.Contains(m, "503") || !strings.Contains(m, "service unavailable") {
		t.Errorf("wa-last failed = %q", m)
	}
	if len(store.sent) != 0 {
		t.Errorf("nothing should be marked sent: %v", store.sent)
	}
}

func TestDryRunWorkerMarksSentOnSuccess(t *testing.T) {
	cat := newCatcher(t, nil)
	srv, _ := fakeWhatsApp(t, 200, `{}`)
	cfg := smtpCfg(cat.addr)
	cfg.WhatsApp = waCfg(srv.URL).WhatsApp
	store := newFakeStore(delivery("m", LaneNormal, "email", EventAbsent, 0), delivery("w", LaneNormal, "whatsapp", EventAbsent, 0))
	store.rows[0].Recipient, store.rows[1].Recipient = "ops@example.com", "628123456789"
	w := testWorker(store, map[string]Channel{"email": emailChannel{}, "whatsapp": waChannel()}, 2)
	w.config = func(context.Context) (settings.Channels, error) { return cfg, nil }
	for w.claimAndSend(context.Background(), []string{LaneNormal}) {
	}
	for _, id := range []string{"m", "w"} {
		if _, ok := store.sentAt(id); !ok {
			t.Errorf("%s not marked sent (failed=%v retried=%v)", id, store.failed, store.retried)
		}
	}
	if len(cat.all()) != 1 {
		t.Errorf("catcher got %d mails", len(cat.all()))
	}
}
