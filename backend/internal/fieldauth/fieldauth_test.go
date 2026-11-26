package fieldauth

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func TestDeriveKey(t *testing.T) {
	a := DeriveKey("", "master", GuardTokenInfo)
	b := DeriveKey("", "master", QRInfo)
	if len(a) != 32 || len(b) != 32 || bytes.Equal(a, b) {
		t.Fatalf("derived keys must be 32 bytes and independent per purpose")
	}
	if !bytes.Equal(a, DeriveKey("", "master", GuardTokenInfo)) {
		t.Fatal("derivation is not deterministic")
	}
	if got := DeriveKey("explicit-secret-explicit-secret-xx", "master", QRInfo); string(got) != "explicit-secret-explicit-secret-xx" {
		t.Fatalf("explicit secret not used: %q", got)
	}
}

func TestGuardTokenRoundTrip(t *testing.T) {
	g := NewGuardTokens(DeriveKey("", "k", GuardTokenInfo), time.Hour, 24*time.Hour)
	now := time.Date(2026, 10, 12, 8, 0, 0, 0, time.UTC)
	tok, exp, err := g.Issue("guard-1", "dev-1", "site-1", now)
	if err != nil {
		t.Fatal(err)
	}
	if !exp.Equal(now.Add(time.Hour)) {
		t.Fatalf("exp = %v", exp)
	}
	c, err := g.Parse(tok)
	if err != nil {
		t.Fatal(err)
	}
	if c.GuardID != "guard-1" || c.DeviceID != "dev-1" || c.SiteID != "site-1" || !c.IssuedAt.Equal(now) || !c.ExpiresAt.Equal(exp) {
		t.Fatalf("claims = %+v", c)
	}
}

func TestGuardTokenRejectsForgery(t *testing.T) {
	g := NewGuardTokens([]byte("key-one-key-one-key-one-key-one!"), time.Hour, time.Hour)
	other := NewGuardTokens([]byte("key-two-key-two-key-two-key-two!"), time.Hour, time.Hour)
	tok, _, _ := other.Issue("g", "d", "s", time.Now())
	if _, err := g.Parse(tok); err == nil {
		t.Fatal("token of another key accepted")
	}
	good, _, _ := g.Issue("g", "d", "s", time.Now())
	parts := strings.Split(good, ".")
	if _, err := g.Parse(parts[0] + "." + parts[1] + ".AAAA"); err == nil {
		t.Fatal("bad signature accepted")
	}
	none := jwt.NewWithClaims(jwt.SigningMethodNone, jwt.MapClaims{"sub": "g", "dev": "d", "site": "s", "iat": 1, "exp": 2})
	raw, _ := none.SignedString(jwt.UnsafeAllowNoneSignatureType)
	if _, err := g.Parse(raw); err == nil {
		t.Fatal("alg none accepted")
	}
	missing := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{"sub": "g", "iat": 1, "exp": 2})
	raw, _ = missing.SignedString([]byte("key-one-key-one-key-one-key-one!"))
	if _, err := g.Parse(raw); err == nil {
		t.Fatal("token without dev/site accepted")
	}
	for _, s := range []string{"", "x", "a.b.c", strings.Repeat("a", 3000)} {
		if _, err := g.Parse(s); err == nil {
			t.Fatalf("garbage %q accepted", s[:min(len(s), 10)])
		}
	}
}

func TestGuardAuthExpiryAndGrace(t *testing.T) {
	g := NewGuardTokens([]byte("0123456789abcdef0123456789abcdef"), 12*time.Hour, 7*24*time.Hour)
	issued := time.Date(2026, 10, 1, 6, 0, 0, 0, time.UTC)
	tok, exp, _ := g.Issue("g", "d", "s", issued)
	c, err := g.Parse(tok)
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name       string
		now, event time.Time
		want       bool
	}{
		{"valid now", issued.Add(time.Hour), issued.Add(time.Hour), true},
		{"valid now, earlier event", issued.Add(11 * time.Hour), issued.Add(-time.Hour), true},
		{"expired, queued event before expiry", exp.Add(48 * time.Hour), exp.Add(-time.Minute), true},
		{"expired, event at expiry", exp.Add(time.Hour), exp, true},
		{"expired, event after expiry", exp.Add(time.Hour), exp.Add(time.Minute), false},
		{"beyond grace", exp.Add(7*24*time.Hour + time.Second), exp.Add(-time.Hour), false},
	}
	for _, tc := range cases {
		if got := g.Auth(c, tc.now).VerifiedAt(tc.event, tc.now); got != tc.want {
			t.Errorf("%s: verified = %v, want %v", tc.name, got, tc.want)
		}
	}
	var none *GuardAuth
	if none.VerifiedAt(issued, issued) {
		t.Error("nil auth verified")
	}
	if (&GuardAuth{}).VerifiedAt(issued, issued) {
		t.Error("absent token verified")
	}
}

func TestQRSignature(t *testing.T) {
	s := NewQRSigner(DeriveKey("", "master", QRInfo))
	id := "6f1c2c1e-8a8e-4d4a-9f43-0f2a7e1d9b10"
	p := s.Payload(id, 1)
	if !strings.HasPrefix(p, "SP1:"+id+":") || len(p) != len("SP1:")+len(id)+1+22 {
		t.Fatalf("payload = %q", p)
	}
	cp, sig, ok := ParseQR(p)
	if !ok || cp != id {
		t.Fatalf("parse = %q %q %v", cp, sig, ok)
	}
	if !s.Valid(id, 1, sig) {
		t.Fatal("own signature rejected")
	}
	if s.Valid(id, 2, sig) {
		t.Fatal("signature of an old version accepted")
	}
	if s.Valid("6f1c2c1e-8a8e-4d4a-9f43-0f2a7e1d9b11", 1, sig) {
		t.Fatal("signature valid for another checkpoint")
	}
	if NewQRSigner([]byte("other")).Valid(id, 1, sig) {
		t.Fatal("signature valid under another key")
	}
	if again := s.Sig(id, 1); again != sig || strings.ContainsAny(sig, "+/=") {
		t.Fatalf("signature not stable base64url: %q", sig)
	}
	for _, bad := range []string{"QR-GATE", "SP1:", "SP1:abc", "SP1:abc:", "SP1::sig", "SP2:a:b"} {
		if _, _, ok := ParseQR(bad); ok {
			t.Errorf("ParseQR(%q) ok", bad)
		}
	}
	if cp, _, ok := ParseQR("  SP1:a:b:c  "); !ok || cp != "a:b" {
		t.Errorf("ParseQR with colon in id = %q %v", cp, ok)
	}
}

func TestQRWindowedPayload(t *testing.T) {
	s := NewQRSigner(DeriveKey("", "master", QRInfo)).WithWindow(48 * time.Hour)
	id := "6f1c2c1e-8a8e-4d4a-9f43-0f2a7e1d9b10"
	now := time.Date(2026, 11, 26, 8, 0, 0, 0, time.UTC)
	p := s.WindowedPayload(id, 3, now)
	if !strings.HasPrefix(p, "SP2:"+id+":") {
		t.Fatalf("payload = %q", p)
	}
	gotID, w, sig, ok := ParseWindowedQR("  " + p + " ")
	if !ok || gotID != id || !w.NotBefore.Equal(now) || !w.Expires.Equal(now.Add(48*time.Hour)) {
		t.Fatalf("parse = %q %+v %v", gotID, w, ok)
	}
	if !s.ValidWindowed(id, 3, w, sig) {
		t.Fatal("own windowed signature rejected")
	}
	if s.ValidWindowed(id, 4, w, sig) {
		t.Fatal("signature of an old version accepted")
	}
	// A longer window with the old signature = tampering.
	longer := QRWindow{NotBefore: w.NotBefore, Expires: w.Expires.Add(24 * time.Hour)}
	if s.ValidWindowed(id, 3, longer, sig) {
		t.Fatal("tampered expiry accepted")
	}
	later := QRWindow{NotBefore: w.NotBefore.Add(24 * time.Hour), Expires: w.Expires.Add(24 * time.Hour)}
	if s.ValidWindowed(id, 3, later, sig) {
		t.Fatal("shifted window accepted")
	}
	if NewQRSigner([]byte("other")).ValidWindowed(id, 3, w, sig) {
		t.Fatal("valid under another key")
	}
	// An SP1 signature cannot be replayed as an SP2 one.
	if s.ValidWindowed(id, 3, w, s.Sig(id, 3)) {
		t.Fatal("SP1 signature accepted as SP2")
	}
	for _, bad := range []string{"SP1:" + id + ":sig", "SP2:", "SP2:" + id, "SP2:" + id + ":1:2", "SP2:" + id + ":x:2:sig",
		"SP2:" + id + ":5:2:sig", "SP2::1:2:sig", "SP2:" + id + ":1:2:"} {
		if _, _, _, ok := ParseWindowedQR(bad); ok {
			t.Errorf("ParseWindowedQR(%q) ok", bad)
		}
	}
	if _, _, ok := ParseQR(p); ok {
		t.Error("ParseQR must stay SP1-only")
	}
}

func TestQRWindowContains(t *testing.T) {
	from := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	w := QRWindow{NotBefore: from, Expires: from.Add(30 * 24 * time.Hour)}
	cases := []struct {
		name string
		at   time.Time
		want bool
	}{
		{"inside", from.Add(24 * time.Hour), true},
		{"at start", from, true},
		{"just before start, within skew", from.Add(-QRSkew), true},
		{"before start beyond skew", from.Add(-QRSkew - time.Second), false},
		{"just after expiry, within skew", w.Expires.Add(QRSkew), true},
		{"after expiry beyond skew", w.Expires.Add(QRSkew + time.Second), false},
		{"long after (reused photo)", w.Expires.Add(90 * 24 * time.Hour), false},
	}
	for _, tc := range cases {
		if got := w.Contains(tc.at); got != tc.want {
			t.Errorf("%s: Contains = %v, want %v", tc.name, got, tc.want)
		}
	}
	if d := NewQRSigner([]byte("k")).Window(); d != DefaultQRWindow {
		t.Errorf("default window = %v", d)
	}
}

// The event time is client-supplied, so it alone cannot prove a scan is
// fresh: the receipt time must also be near the window (QROfflineGrace).
func TestQRWindowAccepts(t *testing.T) {
	from := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	w := QRWindow{NotBefore: from, Expires: from.Add(30 * 24 * time.Hour)}
	lastReceipt := w.Expires.Add(QRSkew + QROfflineGrace)
	cases := []struct {
		name         string
		at, received time.Time
		want         bool
	}{
		{"online, mid-window", from.Add(time.Hour), from.Add(time.Hour), true},
		{"offline scan queued 2 h ago, inside window", from.Add(24 * time.Hour), from.Add(26 * time.Hour), true},
		{"offline scan made in window, synced just after expiry", w.Expires.Add(-time.Hour), w.Expires.Add(5 * time.Hour), true},
		{"receipt on the last accepted second", w.Expires.Add(-time.Hour), lastReceipt, true},
		{"receipt one second later", w.Expires.Add(-time.Hour), lastReceipt.Add(time.Second), false},
		{"photo of a 3-day-expired QR, scanned_at backdated 1 day", w.Expires.Add(48 * time.Hour), w.Expires.Add(72 * time.Hour), false},
		{"event backdated into the window, receipt 3 days after expiry", w.Expires.Add(-time.Hour), w.Expires.Add(72 * time.Hour), false},
		{"event after expiry, receipt after expiry", w.Expires.Add(time.Hour), w.Expires.Add(2 * time.Hour), false},
		{"event before the window", from.Add(-QRSkew - time.Second), from, false},
	}
	for _, tc := range cases {
		if got := w.Accepts(tc.at, tc.received); got != tc.want {
			t.Errorf("%s: Accepts = %v, want %v", tc.name, got, tc.want)
		}
	}
}
