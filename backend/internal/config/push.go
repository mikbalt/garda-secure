package config

import (
	"crypto/ecdh"
	"encoding/base64"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"
	"time"
)

// Push configures push notifications to browsers (Web Push with VAPID) and
// tablets (Firebase Cloud Messaging). Both are optional; an unconfigured
// transport is disabled and GET /push/config says so.
type Push struct {
	// VAPIDPublicKey / VAPIDPrivateKey: P-256 key pair, base64url
	// (WEBPUSH_VAPID_PUBLIC_KEY: 65-byte uncompressed point,
	// WEBPUSH_VAPID_PRIVATE_KEY: 32-byte scalar). Generate with
	// `securepatrol vapid-keys` or scripts/gen-env.sh.
	VAPIDPublicKey  string
	VAPIDPrivateKey string
	// Subject is WEBPUSH_SUBJECT, the operator contact push services see
	// (mailto:ops@example.com or an https: URL). Required with the keys.
	Subject string
	// AllowedHosts are extra push-service host suffixes a browser
	// subscription endpoint may point to (WEBPUSH_ALLOWED_HOSTS), on top of
	// DefaultWebPushHosts.
	AllowedHosts []string
	// FCMCredentialsFile is the Firebase service-account JSON
	// (FCM_CREDENTIALS_FILE, a compose secret). Missing or empty = FCM off.
	FCMCredentialsFile string
	// SOSEscalateAfter / SOSEscalateEvery: an SOS still unacknowledged after
	// SOS_ESCALATE_AFTER (3m) raises sos_unacknowledged, repeated every
	// SOS_ESCALATE_EVERY (5m) up to SOSEscalations times.
	SOSEscalateAfter time.Duration
	SOSEscalateEvery time.Duration
	// AudienceMaxAge stops pushes to users who made no authenticated request
	// for this long (PUSH_AUDIENCE_MAX_AGE, default 720h = 30 days): their
	// role/site snapshot is too old to trust.
	AudienceMaxAge time.Duration
}

// SOSEscalations is how many sos_unacknowledged reminders an SOS gets.
const SOSEscalations = 3

// DefaultWebPushHosts are the host suffixes of the browser push services
// (Chrome/Edge-Chromium/Opera/Samsung via FCM, Firefox, Safari, Edge legacy).
var DefaultWebPushHosts = []string{
	"fcm.googleapis.com", "android.googleapis.com", "push.services.mozilla.com",
	"push.apple.com", "notify.windows.com",
}

// WebPushEnabled reports whether browser push is configured.
func (p Push) WebPushEnabled() bool { return p.VAPIDPublicKey != "" && p.VAPIDPrivateKey != "" }

// loadPush reads and validates the push settings.
func (c *Config) loadPush() error {
	p := &c.Push
	p.VAPIDPublicKey = strings.TrimSpace(os.Getenv("WEBPUSH_VAPID_PUBLIC_KEY"))
	p.VAPIDPrivateKey = strings.TrimSpace(os.Getenv("WEBPUSH_VAPID_PRIVATE_KEY"))
	p.Subject = strings.TrimSpace(os.Getenv("WEBPUSH_SUBJECT"))
	p.FCMCredentialsFile = strings.TrimSpace(os.Getenv("FCM_CREDENTIALS_FILE"))
	for _, h := range splitList(os.Getenv("WEBPUSH_ALLOWED_HOSTS")) {
		h = strings.ToLower(strings.TrimPrefix(h, "."))
		if !validHostSuffix(h) {
			return fmt.Errorf("WEBPUSH_ALLOWED_HOSTS: %q is not a host name", h)
		}
		p.AllowedHosts = append(p.AllowedHosts, h)
	}
	if p.VAPIDPublicKey != "" || p.VAPIDPrivateKey != "" {
		if err := ValidateVAPIDKeys(p.VAPIDPublicKey, p.VAPIDPrivateKey); err != nil {
			return fmt.Errorf("WEBPUSH_VAPID_*: %w", err)
		}
		if err := ValidateVAPIDSubject(p.Subject); err != nil {
			return fmt.Errorf("WEBPUSH_SUBJECT: %w", err)
		}
	}
	var err error
	if p.SOSEscalateAfter, err = envDuration("SOS_ESCALATE_AFTER", 3*time.Minute); err != nil {
		return err
	}
	if p.SOSEscalateEvery, err = envDuration("SOS_ESCALATE_EVERY", 5*time.Minute); err != nil {
		return err
	}
	for name, d := range map[string]time.Duration{"SOS_ESCALATE_AFTER": p.SOSEscalateAfter, "SOS_ESCALATE_EVERY": p.SOSEscalateEvery} {
		if d < time.Minute || d > time.Hour {
			return fmt.Errorf("%s must be between 1m and 1h", name)
		}
	}
	if p.AudienceMaxAge, err = envDuration("PUSH_AUDIENCE_MAX_AGE", 30*24*time.Hour); err != nil {
		return err
	}
	if p.AudienceMaxAge < time.Hour || p.AudienceMaxAge > 365*24*time.Hour {
		return errors.New("PUSH_AUDIENCE_MAX_AGE must be between 1h and 8760h")
	}
	return nil
}

// ValidateVAPIDKeys checks a VAPID key pair: base64url (padding optional),
// a 65-byte uncompressed P-256 public point and the 32-byte private scalar
// it belongs to.
func ValidateVAPIDKeys(public, private string) error {
	if public == "" || private == "" {
		return errors.New("both the public and the private key are required")
	}
	pub, err := DecodeBase64URL(public)
	if err != nil {
		return errors.New("public key is not base64url")
	}
	priv, err := DecodeBase64URL(private)
	if err != nil {
		return errors.New("private key is not base64url")
	}
	if len(pub) != 65 || pub[0] != 4 {
		return fmt.Errorf("public key must be a 65-byte uncompressed P-256 point, got %d bytes", len(pub))
	}
	if _, err := ecdh.P256().NewPublicKey(pub); err != nil {
		return errors.New("public key is not a valid P-256 point")
	}
	if len(priv) != 32 {
		return fmt.Errorf("private key must be 32 bytes, got %d", len(priv))
	}
	k, err := ecdh.P256().NewPrivateKey(priv)
	if err != nil {
		return errors.New("private key is not a valid P-256 scalar")
	}
	if string(k.PublicKey().Bytes()) != string(pub) {
		return errors.New("public key does not belong to the private key")
	}
	return nil
}

// ValidateVAPIDSubject checks the VAPID "sub": a mailto: address or an
// https: URL (RFC 8292).
func ValidateVAPIDSubject(s string) error {
	switch {
	case s == "":
		return errors.New("required when VAPID keys are set (mailto:ops@example.com or https://...)")
	case strings.HasPrefix(s, "mailto:"):
		addr := strings.TrimPrefix(s, "mailto:")
		if len(addr) < 3 || !strings.Contains(addr, "@") || strings.ContainsAny(addr, " <>\r\n") {
			return fmt.Errorf("%q is not a mailto: address", s)
		}
	case strings.HasPrefix(s, "https://"):
		u, err := url.Parse(s)
		if err != nil || u.Host == "" {
			return fmt.Errorf("%q is not an https URL", s)
		}
	default:
		return fmt.Errorf("%q must start with mailto: or https://", s)
	}
	return nil
}

// DecodeBase64URL decodes base64url with or without padding.
func DecodeBase64URL(s string) ([]byte, error) {
	return base64.RawURLEncoding.DecodeString(strings.TrimRight(s, "="))
}

func validHostSuffix(h string) bool {
	if h == "" || len(h) > 253 || !strings.Contains(h, ".") {
		return false
	}
	for _, label := range strings.Split(h, ".") {
		if label == "" || len(label) > 63 {
			return false
		}
		for _, r := range label {
			if (r < 'a' || r > 'z') && (r < '0' || r > '9') && r != '-' {
				return false
			}
		}
	}
	return true
}
