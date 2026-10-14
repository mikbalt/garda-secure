// Package push implements the push transports: Web Push (RFC 8030 with
// RFC 8291 payload encryption and RFC 8292 VAPID) to browsers and Firebase
// Cloud Messaging (HTTP v1) to tablets. Who receives what is decided by the
// notification package; this package only sends one message to one target.
package push

import (
	"context"
	"crypto/ecdh"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"time"

	webpush "github.com/SherClockHolmes/webpush-go"

	"github.com/securepatrol/backend/internal/config"
	"github.com/securepatrol/backend/internal/netguard"
)

// Errors callers classify.
var (
	// ErrGone means the target no longer exists (HTTP 404/410 from the push
	// service, FCM UNREGISTERED): delete the subscription / token, do not retry.
	ErrGone = errors.New("push target is gone")
	// ErrDisabled means the transport is not configured.
	ErrDisabled = errors.New("push transport is not configured")
	// ErrInvalidSubscription is a malformed browser subscription.
	ErrInvalidSubscription = errors.New("invalid push subscription")
)

// StatusError is a non-2xx answer of a push service.
type StatusError struct {
	Status int
	Detail string // response excerpt, for logs only
}

func (e *StatusError) Error() string {
	return fmt.Sprintf("push service status %d: %s", e.Status, e.Detail)
}

// sendTimeout bounds one push request.
const sendTimeout = 20 * time.Second

// MaxEndpointLen bounds a subscription endpoint URL.
const MaxEndpointLen = 1024

// Severities of a web push payload.
const (
	SeverityCritical = "critical"
	SeverityHigh     = "high"
	SeverityNormal   = "normal"
)

// WebPayload is the JSON a browser's service worker receives (encrypted
// end to end; the push service only sees its size).
type WebPayload struct {
	Title    string `json:"title"`
	Body     string `json:"body"`
	URL      string `json:"url"`
	Tag      string `json:"tag"`
	Event    string `json:"event"`
	Severity string `json:"severity"`
	SiteID   string `json:"site_id"`
}

// Subscription is a browser PushSubscription (endpoint + keys).
type Subscription struct {
	Endpoint string
	P256dh   string
	Auth     string
}

// WebPush sends encrypted messages to browser push services.
type WebPush struct {
	publicKey, privateKey, subject string
	hosts                          []string
	client                         *http.Client
}

// NewWebPush returns the browser transport, or nil when VAPID keys are not
// configured. The keys were validated by config.Load.
func NewWebPush(cfg config.Push) *WebPush {
	if !cfg.WebPushEnabled() {
		return nil
	}
	return &WebPush{
		publicKey: cfg.VAPIDPublicKey, privateKey: cfg.VAPIDPrivateKey,
		// webpush-go prefixes "mailto:" itself unless the subject is https.
		subject: strings.TrimPrefix(cfg.Subject, "mailto:"),
		hosts:   append(append([]string{}, config.DefaultWebPushHosts...), cfg.AllowedHosts...),
		client:  guardedClient(netguard.Policy{}),
	}
}

// guardedClient is an HTTP client for push services: public addresses only
// (checked on the connected IP), no redirects, bounded time.
func guardedClient(guard netguard.Policy) *http.Client {
	return &http.Client{
		Timeout: sendTimeout,
		Transport: &http.Transport{
			Proxy:               nil,
			DialContext:         guard.Dialer(10 * time.Second).DialContext,
			TLSHandshakeTimeout: 10 * time.Second,
			MaxIdleConnsPerHost: 4,
			IdleConnTimeout:     90 * time.Second,
		},
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
}

// PublicKey is the VAPID public key browsers subscribe with.
func (w *WebPush) PublicKey() string {
	if w == nil {
		return ""
	}
	return w.publicKey
}

// Hosts lists the push-service host suffixes endpoints may use.
func (w *WebPush) Hosts() []string { return w.hosts }

// ValidateEndpoint checks a subscription endpoint against SSRF: https, no
// credentials, default port, and a host that is a known push service (an IP
// literal must at least be public). The dial-time guard re-checks the
// connected address on every send.
func ValidateEndpoint(raw string, hosts []string) (string, error) {
	if raw == "" || len(raw) > MaxEndpointLen {
		return "", fmt.Errorf("%w: endpoint is required (max %d characters)", ErrInvalidSubscription, MaxEndpointLen)
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.Fragment != "" || u.Opaque != "" {
		return "", fmt.Errorf("%w: endpoint must be an https URL", ErrInvalidSubscription)
	}
	if p := u.Port(); p != "" && p != "443" {
		return "", fmt.Errorf("%w: endpoint must use the default https port", ErrInvalidSubscription)
	}
	host := strings.ToLower(strings.TrimSuffix(u.Hostname(), "."))
	if ip, err := netip.ParseAddr(host); err == nil {
		if !(netguard.Policy{}).Allowed(ip) {
			return "", fmt.Errorf("%w: endpoint host is not public", ErrInvalidSubscription)
		}
		return "", fmt.Errorf("%w: endpoint host is not a known push service", ErrInvalidSubscription)
	}
	for _, h := range hosts {
		if host == h || strings.HasSuffix(host, "."+h) {
			return u.String(), nil
		}
	}
	return "", fmt.Errorf("%w: endpoint host %s is not a known push service", ErrInvalidSubscription, host)
}

// ValidateKeys checks the subscription keys: p256dh a 65-byte uncompressed
// P-256 point, auth a 16-byte secret, both base64url.
func ValidateKeys(p256dh, auth string) error {
	pub, err := config.DecodeBase64URL(p256dh)
	if err != nil || len(pub) != 65 || pub[0] != 4 {
		return fmt.Errorf("%w: keys.p256dh must be a base64url P-256 public key", ErrInvalidSubscription)
	}
	if _, err := ecdh.P256().NewPublicKey(pub); err != nil {
		return fmt.Errorf("%w: keys.p256dh is not a valid P-256 point", ErrInvalidSubscription)
	}
	a, err := config.DecodeBase64URL(auth)
	if err != nil || len(a) != 16 {
		return fmt.Errorf("%w: keys.auth must be a base64url 16-byte secret", ErrInvalidSubscription)
	}
	return nil
}

// WebOptions are the per-message delivery hints.
type WebOptions struct {
	TTL    time.Duration // how long the push service keeps an undelivered message
	Urgent bool          // Urgency: high
	Topic  string        // optional collapse key (<= 32 base64url characters)
}

// WebOptionsFor derives TTL and urgency from a payload: SOS reminders expire
// after 2 minutes, everything else after an hour; critical is urgent.
func WebOptionsFor(p WebPayload) WebOptions {
	o := WebOptions{TTL: time.Hour, Urgent: p.Severity == SeverityCritical}
	if p.Event == "sos" || p.Event == "sos_unacknowledged" {
		o.TTL = 2 * time.Minute
	}
	return o
}

// Send encrypts payload for sub and posts it to the push service. A 404 or
// 410 answer wraps ErrGone.
func (w *WebPush) Send(ctx context.Context, sub Subscription, payload []byte, o WebOptions) error {
	if w == nil {
		return ErrDisabled
	}
	if _, err := ValidateEndpoint(sub.Endpoint, w.hosts); err != nil {
		return fmt.Errorf("%w (%v)", ErrGone, err) // host no longer allowed: drop it
	}
	opts := &webpush.Options{
		HTTPClient: w.client, Subscriber: w.subject,
		VAPIDPublicKey: w.publicKey, VAPIDPrivateKey: w.privateKey,
		TTL: int(o.TTL / time.Second), Urgency: webpush.UrgencyNormal, Topic: o.Topic,
	}
	if o.Urgent {
		opts.Urgency = webpush.UrgencyHigh
	}
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, sendTimeout)
		defer cancel()
	}
	resp, err := webpush.SendNotificationWithContext(ctx, payload,
		&webpush.Subscription{Endpoint: sub.Endpoint, Keys: webpush.Keys{P256dh: sub.P256dh, Auth: sub.Auth}}, opts)
	if err != nil {
		return redactURL(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		return nil
	}
	detail, _ := io.ReadAll(io.LimitReader(resp.Body, 256))
	se := &StatusError{Status: resp.StatusCode, Detail: strings.TrimSpace(string(detail))}
	if resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusGone {
		return fmt.Errorf("%w: %w", ErrGone, se)
	}
	return se
}

// redactURL drops the endpoint URL (a capability) from transport errors.
func redactURL(err error) error {
	var ue *url.Error
	if errors.As(err, &ue) {
		return fmt.Errorf("push request failed: %w", ue.Err)
	}
	return err
}

// Marshal encodes a payload; the web push record limit is ~4 KB, so the body
// is shortened first.
func (p WebPayload) Marshal() ([]byte, error) {
	p.Title = clip(p.Title, 120)
	p.Body = clip(p.Body, 600)
	return json.Marshal(p)
}

func clip(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}
