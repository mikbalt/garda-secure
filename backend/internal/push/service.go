package push

import (
	"crypto/ecdh"
	"crypto/rand"
	"encoding/base64"
	"net/http"

	"github.com/securepatrol/backend/internal/config"
)

// GenerateVAPIDKeys creates a P-256 key pair for WEBPUSH_VAPID_PUBLIC_KEY /
// WEBPUSH_VAPID_PRIVATE_KEY (base64url without padding).
func GenerateVAPIDKeys() (public, private string, err error) {
	k, err := ecdh.P256().GenerateKey(rand.Reader)
	if err != nil {
		return "", "", err
	}
	enc := base64.RawURLEncoding
	return enc.EncodeToString(k.PublicKey().Bytes()), enc.EncodeToString(k.Bytes()), nil
}

// UseHTTPClient replaces the guarded client and the allowed endpoint hosts,
// so tests can point the transport at a fake push service.
func (w *WebPush) UseHTTPClient(c *http.Client, hosts ...string) {
	w.client = c
	if len(hosts) > 0 {
		w.hosts = hosts
	}
}

// UseEndpoints points FCM at other token and send URLs with client (tests).
func (f *FCM) UseEndpoints(tokenURL, sendURL string, c *http.Client) {
	f.tokenURL, f.sendURL, f.client = tokenURL, sendURL, c
}

// Service bundles the configured transports. Either may be nil (disabled).
type Service struct {
	Web *WebPush
	FCM *FCM
}

// New builds the transports from the configuration. An unreadable or
// invalid FCM credentials file returns the error with FCM disabled, so the
// caller can log it and keep running with web push only.
func New(cfg config.Push) (*Service, error) {
	s := &Service{Web: NewWebPush(cfg)}
	f, err := LoadFCM(cfg.FCMCredentialsFile)
	if err != nil {
		return s, err
	}
	s.FCM = f
	return s, nil
}

// WebEnabled reports whether browser push is configured.
func (s *Service) WebEnabled() bool { return s != nil && s.Web != nil }

// FCMEnabled reports whether tablet push is configured.
func (s *Service) FCMEnabled() bool { return s != nil && s.FCM != nil }
