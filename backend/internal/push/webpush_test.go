package push

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdh"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/hkdf"

	"github.com/securepatrol/backend/internal/config"
)

var b64 = base64.RawURLEncoding

func TestValidateEndpoint(t *testing.T) {
	hosts := config.DefaultWebPushHosts
	for _, ok := range []string{
		"https://fcm.googleapis.com/fcm/send/abc:def",
		"https://updates.push.services.mozilla.com/wpush/v2/gAAA",
		"https://web.push.apple.com/QGZ",
		"https://wns2-par02p.notify.windows.com/w/?token=AQE",
		"https://fcm.googleapis.com:443/fcm/send/x",
	} {
		if _, err := ValidateEndpoint(ok, hosts); err != nil {
			t.Errorf("%s rejected: %v", ok, err)
		}
	}
	for _, bad := range []string{
		"", "http://fcm.googleapis.com/fcm/send/x", "https://evil.example/fcm.googleapis.com",
		"https://fcm.googleapis.com.evil.example/x", "https://user:pw@fcm.googleapis.com/x",
		"https://fcm.googleapis.com:8443/x", "https://127.0.0.1/x", "https://10.0.0.5/x", "https://[::1]/x",
		"https://8.8.8.8/x", "https://notfcm.googleapis.com.example/x", "ftp://push.apple.com/x",
		"https://push.apple.com/" + strings.Repeat("a", MaxEndpointLen),
	} {
		if _, err := ValidateEndpoint(bad, hosts); !errors.Is(err, ErrInvalidSubscription) {
			t.Errorf("%q accepted (%v)", bad, err)
		}
	}
	if _, err := ValidateEndpoint("https://push.example.net/x", append(hosts, "example.net")); err != nil {
		t.Errorf("extra allowed host rejected: %v", err)
	}
}

func TestValidateKeys(t *testing.T) {
	ua, _ := ecdh.P256().GenerateKey(rand.Reader)
	auth := make([]byte, 16)
	_, _ = rand.Read(auth)
	if err := ValidateKeys(b64.EncodeToString(ua.PublicKey().Bytes()), b64.EncodeToString(auth)); err != nil {
		t.Fatalf("valid keys: %v", err)
	}
	if err := ValidateKeys(base64.URLEncoding.EncodeToString(ua.PublicKey().Bytes()), base64.URLEncoding.EncodeToString(auth)); err != nil {
		t.Fatalf("padded keys: %v", err)
	}
	for name, c := range map[string][2]string{
		"short auth":   {b64.EncodeToString(ua.PublicKey().Bytes()), b64.EncodeToString(auth[:8])},
		"bad p256dh":   {"nope!", b64.EncodeToString(auth)},
		"off curve":    {b64.EncodeToString(append([]byte{4}, make([]byte, 64)...)), b64.EncodeToString(auth)},
		"missing auth": {b64.EncodeToString(ua.PublicKey().Bytes()), ""},
	} {
		if err := ValidateKeys(c[0], c[1]); !errors.Is(err, ErrInvalidSubscription) {
			t.Errorf("%s accepted", name)
		}
	}
}

func TestWebOptionsFor(t *testing.T) {
	if o := WebOptionsFor(WebPayload{Event: "sos", Severity: SeverityCritical}); o.TTL != 2*time.Minute || !o.Urgent {
		t.Errorf("sos options = %+v", o)
	}
	if o := WebOptionsFor(WebPayload{Event: "absent", Severity: SeverityHigh}); o.TTL != time.Hour || o.Urgent {
		t.Errorf("absent options = %+v", o)
	}
}

// fakePushService is a TLS push service at "push.test" that records what it
// received and answers with status.
type fakePushService struct {
	srv    *httptest.Server
	mu     sync.Mutex
	status int
	reqs   []*http.Request
	bodies [][]byte
}

func newFakePushService(t *testing.T, status int) *fakePushService {
	f := &fakePushService{status: status}
	f.srv = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		f.mu.Lock()
		f.reqs, f.bodies = append(f.reqs, r), append(f.bodies, b)
		st := f.status
		f.mu.Unlock()
		w.WriteHeader(st)
	}))
	t.Cleanup(f.srv.Close)
	return f
}

// client dials the fake service whatever host the URL names.
func (f *fakePushService) client() *http.Client {
	tr := f.srv.Client().Transport.(*http.Transport).Clone()
	addr := f.srv.Listener.Addr().String()
	tr.DialContext = func(ctx context.Context, network, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, network, addr)
	}
	tr.TLSClientConfig.InsecureSkipVerify = true // #nosec G402 -- test server
	return &http.Client{Transport: tr, Timeout: 5 * time.Second}
}

func testWebPush(t *testing.T, f *fakePushService) (*WebPush, string) {
	t.Helper()
	k, _ := ecdh.P256().GenerateKey(rand.Reader)
	pub, priv := b64.EncodeToString(k.PublicKey().Bytes()), b64.EncodeToString(k.Bytes())
	w := NewWebPush(config.Push{VAPIDPublicKey: pub, VAPIDPrivateKey: priv, Subject: "mailto:ops@garda.example"})
	w.UseHTTPClient(f.client(), "push.test")
	return w, pub
}

func TestWebPushSendEncryptsAndSignsVAPID(t *testing.T) {
	f := newFakePushService(t, http.StatusCreated)
	w, vapidPub := testWebPush(t, f)
	ua, _ := ecdh.P256().GenerateKey(rand.Reader)
	auth := make([]byte, 16)
	_, _ = rand.Read(auth)
	sub := Subscription{Endpoint: "https://push.test/send/abc", P256dh: b64.EncodeToString(ua.PublicKey().Bytes()), Auth: b64.EncodeToString(auth)}
	payload, _ := WebPayload{Title: "[SOS] Budi", Body: "SOS!", URL: "/incidents?tab=sos", Tag: "sos-1", Event: "sos", Severity: SeverityCritical, SiteID: "s1"}.Marshal()

	if err := w.Send(context.Background(), sub, payload, WebOptionsFor(WebPayload{Event: "sos", Severity: SeverityCritical})); err != nil {
		t.Fatal(err)
	}
	if len(f.reqs) != 1 {
		t.Fatalf("requests = %d", len(f.reqs))
	}
	r := f.reqs[0]
	if r.URL.Path != "/send/abc" || r.Header.Get("TTL") != "120" || r.Header.Get("Urgency") != "high" ||
		r.Header.Get("Content-Encoding") != "aes128gcm" {
		t.Errorf("headers = %v path %s", r.Header, r.URL.Path)
	}
	claims := verifyVAPID(t, r.Header.Get("Authorization"), vapidPub)
	if claims["sub"] != "mailto:ops@garda.example" || claims["aud"] != "https://push.test" {
		t.Errorf("VAPID claims = %v", claims)
	}
	plain := decryptRFC8291(t, f.bodies[0], ua, auth)
	var got WebPayload
	if err := json.Unmarshal(plain, &got); err != nil || got.Title != "[SOS] Budi" || got.URL != "/incidents?tab=sos" || got.Severity != "critical" {
		t.Fatalf("decrypted payload = %s (%v)", plain, err)
	}
}

func TestWebPushSendStatuses(t *testing.T) {
	ua, _ := ecdh.P256().GenerateKey(rand.Reader)
	sub := Subscription{Endpoint: "https://push.test/x", P256dh: b64.EncodeToString(ua.PublicKey().Bytes()), Auth: b64.EncodeToString(make([]byte, 16))}
	for status, want := range map[int]error{http.StatusGone: ErrGone, http.StatusNotFound: ErrGone} {
		f := newFakePushService(t, status)
		w, _ := testWebPush(t, f)
		if err := w.Send(context.Background(), sub, []byte(`{}`), WebOptions{TTL: time.Hour}); !errors.Is(err, want) {
			t.Errorf("status %d: %v, want %v", status, err, want)
		}
	}
	f := newFakePushService(t, http.StatusTooManyRequests)
	w, _ := testWebPush(t, f)
	err := w.Send(context.Background(), sub, []byte(`{}`), WebOptions{TTL: time.Hour})
	var se *StatusError
	if !errors.As(err, &se) || se.Status != 429 || errors.Is(err, ErrGone) {
		t.Errorf("429: %v", err)
	}
	if f.reqs[0].Header.Get("Urgency") != "normal" || f.reqs[0].Header.Get("TTL") != "3600" {
		t.Errorf("normal headers = %v", f.reqs[0].Header)
	}
	// An endpoint whose host is no longer allowed is dropped without a request.
	if err := w.Send(context.Background(), Subscription{Endpoint: "https://elsewhere.test/x", P256dh: sub.P256dh, Auth: sub.Auth},
		[]byte(`{}`), WebOptions{}); !errors.Is(err, ErrGone) {
		t.Errorf("disallowed host: %v", err)
	}
	var nilPush *WebPush
	if err := nilPush.Send(context.Background(), sub, nil, WebOptions{}); !errors.Is(err, ErrDisabled) {
		t.Errorf("nil transport: %v", err)
	}
}

func TestGuardedClientRefusesPrivateAddresses(t *testing.T) {
	k, _ := ecdh.P256().GenerateKey(rand.Reader)
	w := NewWebPush(config.Push{VAPIDPublicKey: b64.EncodeToString(k.PublicKey().Bytes()), VAPIDPrivateKey: b64.EncodeToString(k.Bytes()), Subject: "mailto:a@b.c"})
	w.hosts = []string{"localhost"} // allowed by name, but resolves to loopback
	ua, _ := ecdh.P256().GenerateKey(rand.Reader)
	err := w.Send(context.Background(), Subscription{Endpoint: "https://localhost/x", P256dh: b64.EncodeToString(ua.PublicKey().Bytes()), Auth: b64.EncodeToString(make([]byte, 16))}, []byte(`{}`), WebOptions{})
	if err == nil || !strings.Contains(err.Error(), "not allowed") {
		t.Fatalf("loopback push service reached: %v", err)
	}
}

func TestPayloadMarshalClips(t *testing.T) {
	b, err := WebPayload{Title: strings.Repeat("t", 500), Body: strings.Repeat("b", 5000)}.Marshal()
	if err != nil || len(b) > 3000 {
		t.Fatalf("payload %d bytes (%v)", len(b), err)
	}
}

// verifyVAPID checks "vapid t=<jwt>, k=<key>" and returns the JWT claims.
func verifyVAPID(t *testing.T, header, wantKey string) jwt.MapClaims {
	t.Helper()
	parts := strings.Split(strings.TrimPrefix(header, "vapid "), ", ")
	if len(parts) != 2 || !strings.HasPrefix(parts[0], "t=") || parts[1] != "k="+wantKey {
		t.Fatalf("Authorization = %q", header)
	}
	raw, _ := b64.DecodeString(wantKey)
	pub, err := ecdsa.ParseUncompressedPublicKey(elliptic.P256(), raw)
	if err != nil {
		t.Fatal(err)
	}
	tok, err := jwt.Parse(strings.TrimPrefix(parts[0], "t="), func(*jwt.Token) (any, error) { return pub, nil },
		jwt.WithValidMethods([]string{"ES256"}))
	if err != nil {
		t.Fatalf("VAPID JWT: %v", err)
	}
	return tok.Claims.(jwt.MapClaims)
}

// decryptRFC8291 is the browser side of aes128gcm Web Push encryption.
func decryptRFC8291(t *testing.T, body []byte, ua *ecdh.PrivateKey, auth []byte) []byte {
	t.Helper()
	salt, rs, idlen := body[:16], binary.BigEndian.Uint32(body[16:20]), int(body[20])
	asPub := body[21 : 21+idlen]
	ct := body[21+idlen:]
	if rs != 4096 || idlen != 65 {
		t.Fatalf("header rs=%d idlen=%d", rs, idlen)
	}
	as, err := ecdh.P256().NewPublicKey(asPub)
	if err != nil {
		t.Fatal(err)
	}
	shared, err := ua.ECDH(as)
	if err != nil {
		t.Fatal(err)
	}
	read := func(r io.Reader, n int) []byte {
		b := make([]byte, n)
		if _, err := io.ReadFull(r, b); err != nil {
			t.Fatal(err)
		}
		return b
	}
	info := append(append([]byte("WebPush: info\x00"), ua.PublicKey().Bytes()...), asPub...)
	ikm := read(hkdf.New(sha256.New, shared, auth, info), 32)
	cek := read(hkdf.New(sha256.New, ikm, salt, []byte("Content-Encoding: aes128gcm\x00")), 16)
	nonce := read(hkdf.New(sha256.New, ikm, salt, []byte("Content-Encoding: nonce\x00")), 12)
	block, _ := aes.NewCipher(cek)
	gcm, _ := cipher.NewGCM(block)
	plain, err := gcm.Open(nil, nonce, ct, nil)
	if err != nil {
		t.Fatalf("decrypt: %v", err)
	}
	end := len(plain) - 1
	for end >= 0 && plain[end] == 0 {
		end--
	}
	if end < 0 || plain[end] != 2 {
		t.Fatal("missing padding delimiter")
	}
	return plain[:end]
}
