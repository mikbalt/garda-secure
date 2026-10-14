package push

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// serviceAccountJSON returns a service-account key file for project p.
func serviceAccountJSON(t *testing.T, key *rsa.PrivateKey, tokenURI string) []byte {
	t.Helper()
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(map[string]string{
		"type": "service_account", "project_id": "garda-test", "private_key_id": "kid1",
		"private_key":  string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})),
		"client_email": "fcm@garda-test.iam.gserviceaccount.com", "token_uri": tokenURI,
	})
	return b
}

func TestLoadFCM(t *testing.T) {
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	dir := t.TempDir()
	if f, err := LoadFCM(""); f != nil || err != nil {
		t.Fatalf("no path: %v %v", f, err)
	}
	if f, err := LoadFCM(filepath.Join(dir, "missing.json")); f != nil || err != nil {
		t.Fatalf("missing file: %v %v", f, err)
	}
	empty := filepath.Join(dir, "empty.json")
	_ = os.WriteFile(empty, []byte("\n"), 0o600)
	if f, err := LoadFCM(empty); f != nil || err != nil {
		t.Fatalf("empty placeholder: %v %v", f, err)
	}
	good := filepath.Join(dir, "sa.json")
	_ = os.WriteFile(good, serviceAccountJSON(t, key, googleTokenURL), 0o600)
	f, err := LoadFCM(good)
	if err != nil || f.ProjectID() != "garda-test" || !strings.HasSuffix(f.sendURL, "/projects/garda-test/messages:send") {
		t.Fatalf("valid file: %v %+v", err, f)
	}
	for name, data := range map[string][]byte{
		"not json":         []byte("{"),
		"wrong type":       []byte(`{"type":"authorized_user","project_id":"p","client_email":"a","private_key":"k"}`),
		"foreign tokenuri": serviceAccountJSON(t, key, "https://evil.example/token"),
		"bad key":          []byte(`{"type":"service_account","project_id":"p","client_email":"a","private_key":"nope"}`),
		"bad project":      []byte(`{"type":"service_account","project_id":"../x","client_email":"a","private_key":"k"}`),
	} {
		if _, err := ParseFCMCredentials(data); err == nil {
			t.Errorf("%s accepted", name)
		}
	}
}

// fakeFCM serves the OAuth2 token exchange and the v1 send endpoint.
type fakeFCM struct {
	srv        *httptest.Server
	key        *rsa.PrivateKey
	mu         sync.Mutex
	tokenCalls int
	sends      []map[string]any
	auth       []string
	sendStatus int
	sendBody   string
}

func newFakeFCM(t *testing.T, key *rsa.PrivateKey) *fakeFCM {
	f := &fakeFCM{key: key, sendStatus: 200, sendBody: `{"name":"projects/garda-test/messages/1"}`}
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		switch r.URL.Path {
		case "/token":
			f.tokenCalls++
			_ = r.ParseForm()
			tok, err := jwt.Parse(r.PostForm.Get("assertion"), func(*jwt.Token) (any, error) { return &f.key.PublicKey, nil },
				jwt.WithValidMethods([]string{"RS256"}), jwt.WithAudience(f.srv.URL+"/token"))
			c, _ := tok.Claims.(jwt.MapClaims)
			if err != nil || r.PostForm.Get("grant_type") != "urn:ietf:params:oauth:grant-type:jwt-bearer" ||
				c["iss"] != "fcm@garda-test.iam.gserviceaccount.com" || c["scope"] != fcmScope || tok.Header["kid"] != "kid1" {
				w.WriteHeader(400)
				return
			}
			_, _ = io.WriteString(w, `{"access_token":"ya29.test","expires_in":3600,"token_type":"Bearer"}`)
		case "/send":
			var m map[string]any
			_ = json.NewDecoder(r.Body).Decode(&m)
			f.sends = append(f.sends, m)
			f.auth = append(f.auth, r.Header.Get("Authorization"))
			w.WriteHeader(f.sendStatus)
			_, _ = io.WriteString(w, f.sendBody)
		default:
			w.WriteHeader(404)
		}
	}))
	t.Cleanup(f.srv.Close)
	return f
}

func testFCM(t *testing.T) (*FCM, *fakeFCM) {
	t.Helper()
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	fc, err := ParseFCMCredentials(serviceAccountJSON(t, key, ""))
	if err != nil {
		t.Fatal(err)
	}
	fake := newFakeFCM(t, key)
	fc.UseEndpoints(fake.srv.URL+"/token", fake.srv.URL+"/send", fake.srv.Client())
	return fc, fake
}

func TestFCMSendMessageShape(t *testing.T) {
	fc, fake := testFCM(t)
	msg := TabletMessage{Type: TabletSOS, SiteID: "site-1", RefID: "sos-1", Title: "[SOS] Budi", Body: "SOS!"}
	if err := fc.Send(context.Background(), "tok-A", msg); err != nil {
		t.Fatal(err)
	}
	if err := fc.Send(context.Background(), "tok-B", TabletMessage{Type: TabletSync, SiteID: "site-1"}); err != nil {
		t.Fatal(err)
	}
	if fake.tokenCalls != 1 {
		t.Errorf("token exchanged %d times, want 1 (cached)", fake.tokenCalls)
	}
	if fake.auth[0] != "Bearer ya29.test" {
		t.Errorf("Authorization = %q", fake.auth[0])
	}
	m := fake.sends[0]["message"].(map[string]any)
	data := m["data"].(map[string]any)
	if m["token"] != "tok-A" || data["type"] != "sos" || data["site_id"] != "site-1" || data["ref_id"] != "sos-1" ||
		data["title"] != "[SOS] Budi" || data["body"] != "SOS!" || len(data) != 5 {
		t.Errorf("message = %v", m)
	}
	if _, hasNotification := m["notification"]; hasNotification {
		t.Error("tablet messages must be data-only")
	}
	android := m["android"].(map[string]any)
	if android["priority"] != "high" || android["ttl"] != "60s" || android["collapse_key"] != "sos" {
		t.Errorf("android = %v", android)
	}
	apns := m["apns"].(map[string]any)
	if h := apns["headers"].(map[string]any); h["apns-push-type"] != "background" || h["apns-priority"] != "5" {
		t.Errorf("apns = %v", apns)
	}
	sync := fake.sends[1]["message"].(map[string]any)
	if sync["android"].(map[string]any)["ttl"] != "3600s" || sync["data"].(map[string]any)["title"] != "" {
		t.Errorf("sync message = %v", sync)
	}
}

func TestFCMErrors(t *testing.T) {
	fc, fake := testFCM(t)
	fake.sendStatus, fake.sendBody = 404, `{"error":{"code":404,"status":"NOT_FOUND","details":[{"@type":"type.googleapis.com/google.firebase.fcm.v1.FcmError","errorCode":"UNREGISTERED"}]}}`
	if err := fc.Send(context.Background(), "gone", TabletMessage{Type: TabletSync}); !errors.Is(err, ErrGone) {
		t.Errorf("unregistered token: %v", err)
	}
	fake.sendStatus, fake.sendBody = 400, `{"error":{"code":400,"details":[{"errorCode":"UNREGISTERED"}]}}`
	if err := fc.Send(context.Background(), "gone2", TabletMessage{Type: TabletSync}); !errors.Is(err, ErrGone) {
		t.Errorf("400 UNREGISTERED: %v", err)
	}
	fake.sendStatus, fake.sendBody = 503, `{"error":{"code":503,"status":"UNAVAILABLE"}}`
	err := fc.Send(context.Background(), "t", TabletMessage{Type: TabletSync})
	var se *StatusError
	if !errors.As(err, &se) || se.Status != 503 || errors.Is(err, ErrGone) {
		t.Errorf("503: %v", err)
	}
	// 401: the cached token is dropped and exchanged again on the retry.
	fake.sendStatus = 401
	_ = fc.Send(context.Background(), "t", TabletMessage{Type: TabletSync})
	fake.sendStatus = 200
	if err := fc.Send(context.Background(), "t", TabletMessage{Type: TabletSync}); err != nil {
		t.Fatal(err)
	}
	if fake.tokenCalls != 2 {
		t.Errorf("token exchanges = %d, want 2 after a 401", fake.tokenCalls)
	}
	var none *FCM
	if err := none.Send(context.Background(), "t", TabletMessage{}); !errors.Is(err, ErrDisabled) {
		t.Errorf("nil FCM: %v", err)
	}
}

func TestFCMTokenFailure(t *testing.T) {
	fc, fake := testFCM(t)
	fc.tokenURL = fake.srv.URL + "/nowhere"
	if err := fc.Send(context.Background(), "t", TabletMessage{Type: TabletSync}); err == nil || !strings.Contains(err.Error(), "fcm token") {
		t.Fatalf("token failure: %v", err)
	}
	if len(fake.sends) != 0 {
		t.Error("message sent without an access token")
	}
}

func TestTabletTTL(t *testing.T) {
	for typ, want := range map[string]time.Duration{
		TabletSOS: time.Minute, TabletSOSAck: 5 * time.Minute, TabletPatrolNotStarted: 15 * time.Minute,
		TabletDeviceStatus: 24 * time.Hour, TabletSync: time.Hour,
	} {
		if got := TabletTTL(typ); got != want {
			t.Errorf("TTL %s = %s, want %s", typ, got, want)
		}
	}
	req := BuildFCMRequest("x", TabletMessage{Type: TabletDeviceStatus}, time.Unix(1000, 0)).(fcmRequest)
	if req.Message.APNS.Headers["apns-expiration"] != "87400" {
		t.Errorf("apns-expiration = %s", req.Message.APNS.Headers["apns-expiration"])
	}
}
