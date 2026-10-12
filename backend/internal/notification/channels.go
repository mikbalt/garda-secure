package notification

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"net/mail"
	"net/smtp"
	"strings"
	"time"

	"github.com/securepatrol/backend/internal/netguard"
	"github.com/securepatrol/backend/internal/settings"
)

// Errors callers can classify (e.g. the settings test endpoint).
var (
	ErrNotConfigured    = errors.New("channel is not configured")
	ErrInvalidRecipient = errors.New("invalid recipient")
)

// StatusError is a non-2xx answer of an HTTP channel API.
type StatusError struct {
	Status int
	Detail string // response excerpt, for the delivery log only
}

func (e *StatusError) Error() string {
	return fmt.Sprintf("API status %d: %s", e.Status, e.Detail)
}

// Channel delivers one message to one recipient. New channels (LINE,
// Telegram in v2) implement this and register in Worker.channels.
type Channel interface {
	Send(ctx context.Context, cfg settings.Channels, recipient, subject, body string) error
}

// ── E-mail ───────────────────────────────────────────────────────────────────

type emailChannel struct{}

func (emailChannel) Send(ctx context.Context, cfg settings.Channels, to, subject, body string) error {
	s := cfg.SMTP
	if s.Host == "" {
		return fmt.Errorf("%w: SMTP host missing", ErrNotConfigured)
	}
	toAddr, err := mail.ParseAddress(to)
	if err != nil {
		return fmt.Errorf("%w: e-mail %q", ErrInvalidRecipient, to)
	}
	from := s.From
	if from == "" {
		from = s.User
	}
	fromAddr, err := mail.ParseAddress(from)
	if err != nil {
		return fmt.Errorf("invalid SMTP from address %q", from)
	}

	contentType := "text/plain; charset=UTF-8"
	if strings.HasPrefix(strings.TrimSpace(body), "<") {
		contentType = "text/html; charset=UTF-8"
	}
	var msg bytes.Buffer
	fmt.Fprintf(&msg, "From: %s\r\n", fromAddr.String())
	fmt.Fprintf(&msg, "To: %s\r\n", toAddr.String())
	fmt.Fprintf(&msg, "Subject: %s\r\n", mime.QEncoding.Encode("UTF-8", oneLine(subject)))
	fmt.Fprintf(&msg, "Date: %s\r\n", time.Now().Format(time.RFC1123Z))
	fmt.Fprintf(&msg, "MIME-Version: 1.0\r\nContent-Type: %s\r\nContent-Transfer-Encoding: 8bit\r\n\r\n", contentType)
	msg.WriteString(strings.ReplaceAll(body, "\n", "\r\n"))

	port := s.Port
	if port == "" {
		port = "587"
	}
	var auth smtp.Auth
	if s.User != "" {
		auth = smtp.PlainAuth("", s.User, s.Password, s.Host)
	}
	// Port 465 is implicit TLS; anything else upgrades with STARTTLS when the
	// server offers it.
	return sendSMTP(ctx, s.Guard, net.JoinHostPort(s.Host, port), s.Host, port == "465", auth,
		fromAddr.Address, toAddr.Address, msg.Bytes())
}

// smtpDialTimeout bounds the TCP connect; the whole exchange is bounded by
// the context deadline (audit BE-R2: a hung SMTP server must not stall the
// worker).
const smtpDialTimeout = 15 * time.Second

// sendSMTP delivers one message with every network step bounded by ctx:
// dial, TLS handshake (implicit or STARTTLS), SMTP dialogue and data.
//
// The dial goes through guard: relays entered in the UI may not resolve to
// internal addresses (checked on the connected IP, so DNS rebinding cannot
// bypass the check made when the settings were saved; audit BE-S).
func sendSMTP(ctx context.Context, guard netguard.Policy, addr, host string, implicitTLS bool, auth smtp.Auth, from, to string, msg []byte) error {
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, sendTimeout)
		defer cancel()
	}
	tlsCfg := &tls.Config{ServerName: host, MinVersion: tls.VersionTLS12}
	dialer := guard.Dialer(smtpDialTimeout)
	var conn net.Conn
	var err error
	if implicitTLS {
		conn, err = (&tls.Dialer{NetDialer: dialer, Config: tlsCfg}).DialContext(ctx, "tcp", addr)
	} else {
		conn, err = dialer.DialContext(ctx, "tcp", addr)
	}
	if err != nil {
		return fmt.Errorf("smtp dial: %w", err)
	}
	defer conn.Close()
	// Every read/write fails once the deadline passes; cancelling ctx closes
	// the connection so a blocked call returns immediately.
	deadline, _ := ctx.Deadline()
	if err := conn.SetDeadline(deadline); err != nil {
		return fmt.Errorf("smtp deadline: %w", err)
	}
	stop := context.AfterFunc(ctx, func() { _ = conn.Close() })
	defer stop()

	c, err := smtp.NewClient(conn, host)
	if err != nil {
		return fmt.Errorf("smtp greeting: %w", ctxErr(ctx, err))
	}
	defer c.Close()
	if err := c.Hello("localhost"); err != nil {
		return fmt.Errorf("smtp hello: %w", ctxErr(ctx, err))
	}
	if !implicitTLS {
		if ok, _ := c.Extension("STARTTLS"); ok {
			// The TLS client wraps conn, so its deadline still applies.
			if err := c.StartTLS(tlsCfg); err != nil {
				return fmt.Errorf("smtp starttls: %w", ctxErr(ctx, err))
			}
		}
	}
	if auth != nil {
		if ok, _ := c.Extension("AUTH"); !ok {
			return errors.New("smtp: server doesn't support AUTH")
		}
		if err := c.Auth(auth); err != nil {
			return fmt.Errorf("smtp auth: %w", ctxErr(ctx, err))
		}
	}
	if err := c.Mail(from); err != nil {
		return fmt.Errorf("smtp mail from: %w", ctxErr(ctx, err))
	}
	if err := c.Rcpt(to); err != nil {
		return fmt.Errorf("smtp rcpt to: %w", ctxErr(ctx, err))
	}
	w, err := c.Data()
	if err != nil {
		return fmt.Errorf("smtp data: %w", ctxErr(ctx, err))
	}
	if _, err := w.Write(msg); err != nil {
		return fmt.Errorf("smtp write: %w", ctxErr(ctx, err))
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("smtp end of data: %w", ctxErr(ctx, err))
	}
	// The message was accepted; a failed QUIT is not a delivery failure.
	_ = c.Quit()
	return nil
}

// ctxErr prefers the context error (timeout) over the I/O error it caused.
func ctxErr(ctx context.Context, err error) error {
	cerr := ctx.Err()
	if cerr == nil {
		// The connection deadline equals the context deadline and may fire
		// a moment before the context notices.
		if dl, ok := ctx.Deadline(); ok && !time.Now().Before(dl) {
			cerr = context.DeadlineExceeded
		}
	}
	if cerr != nil {
		return fmt.Errorf("%w (%v)", cerr, err)
	}
	return err
}

// ── WhatsApp Business (Cloud API) ────────────────────────────────────────────

type whatsAppChannel struct {
	client *http.Client
}

func (w whatsAppChannel) Send(ctx context.Context, cfg settings.Channels, to, subject, body string) error {
	s := cfg.WhatsApp
	if s.Token == "" || s.PhoneNumberID == "" {
		return fmt.Errorf("%w: WhatsApp token and phone number ID are required", ErrNotConfigured)
	}
	phone := digitsOnly(to)
	if len(phone) < 8 {
		return fmt.Errorf("%w: WhatsApp %q", ErrInvalidRecipient, to)
	}
	text := subject
	if body != "" && !strings.HasPrefix(strings.TrimSpace(body), "<") {
		text = subject + "\n" + body
	}

	payload := map[string]any{"messaging_product": "whatsapp", "to": phone}
	if s.TemplateName != "" {
		// Business-initiated messages must use an approved template; ours has a
		// single body parameter, which may not contain newlines or tabs.
		lang := s.TemplateLang
		if lang == "" {
			lang = "id"
		}
		payload["type"] = "template"
		payload["template"] = map[string]any{
			"name":     s.TemplateName,
			"language": map[string]string{"code": lang},
			"components": []any{map[string]any{
				"type":       "body",
				"parameters": []any{map[string]string{"type": "text", "text": truncate(oneLine(text), 1000)}},
			}},
		}
	} else {
		payload["type"] = "text"
		payload["text"] = map[string]string{"body": truncate(text, 4000)}
	}
	b, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	url := strings.TrimRight(s.APIURL, "/") + "/" + s.PhoneNumberID + "/messages"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+s.Token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := w.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		detail, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return &StatusError{Status: resp.StatusCode, Detail: strings.TrimSpace(string(detail))}
	}
	return nil
}

func oneLine(s string) string {
	s = strings.NewReplacer("\r\n", " | ", "\n", " | ", "\r", " ", "\t", " ").Replace(s)
	for strings.Contains(s, "     ") {
		s = strings.ReplaceAll(s, "     ", " ")
	}
	return s
}

func digitsOnly(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}
