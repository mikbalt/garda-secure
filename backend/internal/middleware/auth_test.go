package middleware

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/rs/zerolog"
)

type testIssuer struct {
	key    *rsa.PrivateKey
	server *httptest.Server
}

func newTestIssuer(t *testing.T) *testIssuer {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	ti := &testIssuer{key: key}
	ti.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"keys": []map[string]string{{
			"kid": "k1", "kty": "RSA", "use": "sig",
			"n": base64.RawURLEncoding.EncodeToString(key.N.Bytes()),
			"e": base64.RawURLEncoding.EncodeToString(big.NewInt(int64(key.E)).Bytes()),
		}}})
	}))
	t.Cleanup(ti.server.Close)
	return ti
}

func (ti *testIssuer) token(t *testing.T, claims jwt.MapClaims) string {
	t.Helper()
	tok := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	tok.Header["kid"] = "k1"
	s, err := tok.SignedString(ti.key)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestValidatorMultiRealm(t *testing.T) {
	ti := newTestIssuer(t)
	v := NewValidator([]RealmKeys{
		{Realm: "client1", Issuer: "https://sp/auth/realms/client1", JWKSURL: ti.server.URL},
		{Realm: "client2", Issuer: "https://sp/auth/realms/client2", JWKSURL: ti.server.URL},
	}, []string{"securepatrol-web", "securepatrol-tablet"}, zerolog.Nop())

	base := func() jwt.MapClaims {
		return jwt.MapClaims{
			"iss": "https://sp/auth/realms/client2", "sub": "u1", "preferred_username": "ani",
			"azp": "securepatrol-web", "exp": time.Now().Add(time.Hour).Unix(),
			"roles":        []string{"supervisor"},
			"realm_access": map[string]any{"roles": []string{"hr", "supervisor"}},
		}
	}

	p, err := v.Parse(ti.token(t, base()))
	if err != nil {
		t.Fatalf("valid token rejected: %v", err)
	}
	if p.Realm != "client2" || p.Username != "ani" {
		t.Errorf("principal = %+v", p)
	}
	slices.Sort(p.Roles)
	if !slices.Equal(p.Roles, []string{"hr", "supervisor"}) {
		t.Errorf("roles = %v", p.Roles)
	}

	cases := map[string]func(jwt.MapClaims){
		"untrusted issuer": func(c jwt.MapClaims) { c["iss"] = "https://evil/realms/client1" },
		"foreign client":   func(c jwt.MapClaims) { c["azp"] = "account-console" },
		"expired":          func(c jwt.MapClaims) { c["exp"] = time.Now().Add(-time.Hour).Unix() },
		"no expiry":        func(c jwt.MapClaims) { delete(c, "exp") },
		"no subject":       func(c jwt.MapClaims) { delete(c, "sub") },
	}
	for name, mutate := range cases {
		c := base()
		mutate(c)
		if _, err := v.Parse(ti.token(t, c)); err == nil {
			t.Errorf("%s: token accepted", name)
		}
	}

	other := newTestIssuer(t) // same kid, different key → bad signature
	if _, err := v.Parse(other.token(t, base())); err == nil {
		t.Error("token signed by another key accepted")
	}
}
