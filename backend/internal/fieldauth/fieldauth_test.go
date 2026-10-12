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
