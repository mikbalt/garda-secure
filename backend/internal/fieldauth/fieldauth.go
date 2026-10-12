// Package fieldauth holds the server-side proofs of field activity (D-035):
// short-lived signed guard tokens issued after a server-verified PIN, and
// HMAC-signed checkpoint QR codes.
package fieldauth

import (
	"crypto/hkdf"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// HKDF info strings; changing one invalidates every token or printed QR code.
const (
	GuardTokenInfo = "securepatrol guard token v1"
	QRInfo         = "securepatrol qr v1"
)

// DeriveKey returns secret when it is set; otherwise a 32-byte key derived
// from master (SETTINGS_ENCRYPTION_KEY) with HKDF-SHA256 and info, so each
// purpose gets an independent key.
func DeriveKey(secret, master, info string) []byte {
	if secret != "" {
		return []byte(secret)
	}
	key, err := hkdf.Key(sha256.New, []byte(master), nil, info, 32)
	if err != nil { // only for impossible lengths
		panic("fieldauth: hkdf: " + err.Error())
	}
	return key
}

// ── Guard tokens ─────────────────────────────────────────────────────────────

// ErrInvalidToken is a token that is malformed, not signed with our key, or
// not HS256.
var ErrInvalidToken = errors.New("invalid guard token")

// GuardClaims are the claims of a guard token.
type GuardClaims struct {
	GuardID   string // sub
	DeviceID  string // dev
	SiteID    string // site
	IssuedAt  time.Time
	ExpiresAt time.Time
}

// GuardTokens issues and checks guard tokens: compact JWS (HS256) with the
// claims {sub: guard, dev: device, site, iat, exp}.
type GuardTokens struct {
	key   []byte
	ttl   time.Duration
	grace time.Duration
}

// Defaults when the configuration leaves a value zero.
const (
	DefaultSessionTTL = 12 * time.Hour
	DefaultGrace      = 7 * 24 * time.Hour
)

// NewGuardTokens creates the token issuer. ttl is the token lifetime; grace
// is how long after expiry a token still proves identity for records whose
// event time is before the expiry (offline queue).
func NewGuardTokens(key []byte, ttl, grace time.Duration) *GuardTokens {
	if ttl <= 0 {
		ttl = DefaultSessionTTL
	}
	if grace <= 0 {
		grace = DefaultGrace
	}
	return &GuardTokens{key: key, ttl: ttl, grace: grace}
}

// TTL returns the token lifetime.
func (t *GuardTokens) TTL() time.Duration { return t.ttl }

// Issue signs a token for a guard on a device at now.
func (t *GuardTokens) Issue(guardID, deviceID, siteID string, now time.Time) (string, time.Time, error) {
	exp := now.Add(t.ttl).Truncate(time.Second)
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub": guardID, "dev": deviceID, "site": siteID,
		"iat": now.Unix(), "exp": exp.Unix(),
	})
	s, err := tok.SignedString(t.key)
	if err != nil {
		return "", time.Time{}, err
	}
	return s, exp.UTC(), nil
}

// Parse checks the signature and reads the claims. Expiry is not checked
// here (see Auth).
func (t *GuardTokens) Parse(token string) (*GuardClaims, error) {
	if len(token) > 2048 {
		return nil, ErrInvalidToken
	}
	parsed, err := jwt.Parse(token, func(*jwt.Token) (any, error) { return t.key, nil },
		jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}), jwt.WithoutClaimsValidation())
	if err != nil || !parsed.Valid {
		return nil, ErrInvalidToken
	}
	mc, ok := parsed.Claims.(jwt.MapClaims)
	if !ok {
		return nil, ErrInvalidToken
	}
	str := func(k string) string { v, _ := mc[k].(string); return v }
	num := func(k string) (time.Time, bool) {
		v, ok := mc[k].(float64)
		if !ok {
			return time.Time{}, false
		}
		return time.Unix(int64(v), 0).UTC(), true
	}
	c := &GuardClaims{GuardID: str("sub"), DeviceID: str("dev"), SiteID: str("site")}
	var okIat, okExp bool
	c.IssuedAt, okIat = num("iat")
	c.ExpiresAt, okExp = num("exp")
	if c.GuardID == "" || c.DeviceID == "" || c.SiteID == "" || !okIat || !okExp {
		return nil, ErrInvalidToken
	}
	return c, nil
}

// Auth evaluates a correctly signed token at now.
func (t *GuardTokens) Auth(c *GuardClaims, now time.Time) *GuardAuth {
	return &GuardAuth{
		Present:   true,
		Usable:    !now.After(c.ExpiresAt.Add(t.grace)),
		ExpiresAt: c.ExpiresAt,
	}
}

// GuardAuth is what a request proved about the acting guard.
type GuardAuth struct {
	// Present: a correctly signed token for this guard and device was sent.
	Present bool
	// Usable: the token is not expired by more than the grace period.
	Usable    bool
	ExpiresAt time.Time
}

// VerifiedAt reports whether the guard counts as verified for a record with
// the given event time: the token is still valid now, or (offline queue) it
// was valid at the event time and is within the grace period. A nil
// GuardAuth is unverified.
func (a *GuardAuth) VerifiedAt(event, now time.Time) bool {
	if a == nil || !a.Present || !a.Usable {
		return false
	}
	return !now.After(a.ExpiresAt) || !event.After(a.ExpiresAt)
}

// ── Signed checkpoint QR codes ───────────────────────────────────────────────

// QRPrefix starts every signed checkpoint payload: SP1:<checkpoint id>:<sig>.
const QRPrefix = "SP1:"

// qrSigLen is the length of the printed signature (22 base64url characters
// = 132 bits of the HMAC).
const qrSigLen = 22

// QRSigner signs and verifies checkpoint QR payloads.
type QRSigner struct{ key []byte }

// NewQRSigner creates a signer.
func NewQRSigner(key []byte) *QRSigner { return &QRSigner{key: key} }

// Sig is base64url_nopad(HMAC-SHA256(key, "SP1:" + id + ":" + version))[:22].
func (s *QRSigner) Sig(checkpointID string, version int) string {
	m := hmac.New(sha256.New, s.key)
	m.Write([]byte(QRPrefix + checkpointID + ":" + strconv.Itoa(version)))
	return base64.RawURLEncoding.EncodeToString(m.Sum(nil))[:qrSigLen]
}

// Payload is the text printed in the checkpoint's QR code.
func (s *QRSigner) Payload(checkpointID string, version int) string {
	return QRPrefix + checkpointID + ":" + s.Sig(checkpointID, version)
}

// Valid reports whether sig is the signature of the checkpoint's current
// version (constant-time comparison).
func (s *QRSigner) Valid(checkpointID string, version int, sig string) bool {
	return hmac.Equal([]byte(sig), []byte(s.Sig(checkpointID, version)))
}

// ParseQR splits a scanned "SP1:<id>:<sig>" payload. ok is false for any
// other text (e.g. a legacy plain code).
func ParseQR(payload string) (checkpointID, sig string, ok bool) {
	rest, found := strings.CutPrefix(strings.TrimSpace(payload), QRPrefix)
	if !found {
		return "", "", false
	}
	i := strings.LastIndexByte(rest, ':')
	if i <= 0 || i == len(rest)-1 {
		return "", "", false
	}
	return rest[:i], rest[i+1:], true
}
