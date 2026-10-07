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

	"github.com/securepatrol/backend/internal/settings"
)

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
		return errors.New("SMTP is not configured")
	}
	toAddr, err := mail.ParseAddress(to)
	if err != nil {
		return fmt.Errorf("invalid e-mail recipient %q", to)
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
	addr := net.JoinHostPort(s.Host, port)
	var auth smtp.Auth
	if s.User != "" {
		auth = smtp.PlainAuth("", s.User, s.Password, s.Host)
	}

	if port != "465" {
		// smtp.SendMail upgrades with STARTTLS when the server offers it.
		return smtp.SendMail(addr, auth, fromAddr.Address, []string{toAddr.Address}, msg.Bytes())
	}

	// Port 465: implicit TLS.
	dialer := &net.Dialer{Timeout: 15 * time.Second}
	conn, err := tls.DialWithDialer(dialer, "tcp", addr, &tls.Config{ServerName: s.Host, MinVersion: tls.VersionTLS12})
	if err != nil {
		return fmt.Errorf("smtp dial: %w", err)
	}
	c, err := smtp.NewClient(conn, s.Host)
	if err != nil {
		return fmt.Errorf("smtp client: %w", err)
	}
	defer c.Close()
	if auth != nil {
		if err := c.Auth(auth); err != nil {
			return fmt.Errorf("smtp auth: %w", err)
		}
	}
	if err := c.Mail(fromAddr.Address); err != nil {
		return err
	}
	if err := c.Rcpt(toAddr.Address); err != nil {
		return err
	}
	w, err := c.Data()
	if err != nil {
		return err
	}
	if _, err := w.Write(msg.Bytes()); err != nil {
		return err
	}
	if err := w.Close(); err != nil {
		return err
	}
	return c.Quit()
}

// ── WhatsApp Business (Cloud API) ────────────────────────────────────────────

type whatsAppChannel struct {
	client *http.Client
}

func (w whatsAppChannel) Send(ctx context.Context, cfg settings.Channels, to, subject, body string) error {
	s := cfg.WhatsApp
	if s.Token == "" || s.PhoneNumberID == "" {
		return errors.New("WhatsApp is not configured (token and phone number ID are required)")
	}
	phone := digitsOnly(to)
	if len(phone) < 8 {
		return fmt.Errorf("invalid WhatsApp recipient %q", to)
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
		return fmt.Errorf("whatsapp API status %d: %s", resp.StatusCode, strings.TrimSpace(string(detail)))
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
