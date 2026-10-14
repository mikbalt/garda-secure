//go:build integration

package integration

import (
	"context"
	"crypto/ecdh"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog"

	"github.com/securepatrol/backend/internal/config"
	"github.com/securepatrol/backend/internal/notification"
	"github.com/securepatrol/backend/internal/push"
)

// Push notification tests (web push + FCM). They use realm2 and assert on
// their own subscriptions, tablets and sites only (the database is shared).

// fakeWebPush is a TLS browser push service at "push.test": 201 for every
// endpoint, except paths starting with /gone (410).
type fakeWebPush struct {
	srv   *httptest.Server
	mu    sync.Mutex
	paths []string
}

func newFakeWebPush(t *testing.T) *fakeWebPush {
	f := &fakeWebPush{}
	f.srv = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		f.mu.Lock()
		f.paths = append(f.paths, r.URL.Path)
		f.mu.Unlock()
		if strings.HasPrefix(r.URL.Path, "/gone") {
			w.WriteHeader(http.StatusGone)
			return
		}
		w.WriteHeader(http.StatusCreated)
	}))
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fakeWebPush) client() *http.Client {
	tr := f.srv.Client().Transport.(*http.Transport).Clone()
	addr := f.srv.Listener.Addr().String()
	tr.DialContext = func(ctx context.Context, network, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, network, addr)
	}
	tr.TLSClientConfig.InsecureSkipVerify = true // #nosec G402 -- test server
	return &http.Client{Transport: tr, Timeout: 5 * time.Second}
}

func (f *fakeWebPush) got(path string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Contains(f.paths, path)
}

// fakeFCMServer answers the OAuth2 token exchange and records data messages.
type fakeFCMServer struct {
	srv  *httptest.Server
	mu   sync.Mutex
	msgs []map[string]any // message objects
}

func newFakeFCMServer(t *testing.T) *fakeFCMServer {
	f := &fakeFCMServer{}
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/token" {
			_, _ = io.WriteString(w, `{"access_token":"ya29.it","expires_in":3600}`)
			return
		}
		var body struct {
			Message map[string]any `json:"message"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		f.mu.Lock()
		f.msgs = append(f.msgs, body.Message)
		f.mu.Unlock()
		_, _ = io.WriteString(w, `{"name":"projects/p/messages/1"}`)
	}))
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fakeFCMServer) find(token, typ string) map[string]any {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, m := range f.msgs {
		if d, _ := m["data"].(map[string]any); m["token"] == token && d["type"] == typ {
			return m
		}
	}
	return nil
}

func serviceAccountFile(t *testing.T) string {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	der, _ := x509.MarshalPKCS8PrivateKey(key)
	b, _ := json.Marshal(map[string]string{
		"type": "service_account", "project_id": "garda-it", "private_key_id": "k",
		"private_key":  string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})),
		"client_email": "fcm@garda-it.iam.gserviceaccount.com",
	})
	path := filepath.Join(t.TempDir(), "fcm.json")
	if err := os.WriteFile(path, b, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// subscribeBody is a browser subscription with fresh keys.
func subscribeBody(endpoint string) map[string]any {
	k, _ := ecdh.P256().GenerateKey(rand.Reader)
	auth := make([]byte, 16)
	_, _ = rand.Read(auth)
	return map[string]any{"kind": "webpush", "endpoint": endpoint, "label": "Chrome on Windows",
		"keys": map[string]string{"p256dh": base64.RawURLEncoding.EncodeToString(k.PublicKey().Bytes()), "auth": base64.RawURLEncoding.EncodeToString(auth)}}
}

// recipients lists the recipients of deliveries whose dedupe key starts with
// prefix on a channel.
func (e *env) recipients(prefix, channel string) []string {
	e.t.Helper()
	rows, err := e.pool.Query(context.Background(),
		`SELECT recipient FROM notification_log WHERE dedupe_key LIKE $1 AND channel = $2 ORDER BY recipient`, prefix+"%", channel)
	if err != nil {
		e.t.Fatal(err)
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var r string
		_ = rows.Scan(&r)
		out = append(out, r)
	}
	return out
}

func sameSet(got []string, want ...string) bool {
	g, w := slices.Clone(got), slices.Clone(want)
	slices.Sort(g)
	slices.Sort(w)
	return slices.Equal(g, w)
}

func TestPushNotifications(t *testing.T) {
	pub, priv, err := push.GenerateVAPIDKeys()
	if err != nil {
		t.Fatal(err)
	}
	e := newEnv(t, func(c *config.Config) {
		c.Push = config.Push{VAPIDPublicKey: pub, VAPIDPrivateKey: priv, Subject: "mailto:ops@garda.test",
			FCMCredentialsFile: serviceAccountFile(t)}
	})
	wp, fcm := newFakeWebPush(t), newFakeFCMServer(t)
	e.push.Web.UseHTTPClient(wp.client(), "push.test")
	e.push.FCM.UseEndpoints(fcm.srv.URL+"/token", fcm.srv.URL+"/send", fcm.srv.Client())
	ctx := context.Background()
	run := uuid.NewString()[:8]
	ep := func(name string) string { return "https://push.test/" + name + "-" + run }

	admin := e.web(realm2, "push-admin-"+run, "admin")
	sup := e.web(realm2, "push-sup-"+run, "supervisor")
	hr := e.web(realm2, "push-hr-"+run, "hr")
	client := e.web(realm2, "push-client-"+run, "client-readonly")
	otherRealm := e.web(realm3, "push-other-"+run, "admin")

	// ── config ───────────────────────────────────────────────────────────────
	cfg := e.expect(e.do("GET", "/api/v1/push/config", client, "", nil), 200, "push config").obj()
	if wpc := cfg["webpush"].(map[string]any); wpc["enabled"] != true || wpc["vapid_public_key"] != pub ||
		cfg["fcm"].(map[string]any)["enabled"] != true {
		t.Fatalf("push config = %v", cfg)
	}

	// ── sites, tablets ───────────────────────────────────────────────────────
	a := e.newFieldSite(realm2, admin, "Push A "+run)
	b := e.newFieldSite(realm2, admin, "Push B "+run)
	supB := e.webSites(realm2, "push-supb-"+run, []string{b.site}, "supervisor")

	// ── subscriptions ────────────────────────────────────────────────────────
	sub := func(tok, endpoint string, want int) string {
		return e.expect(e.do("POST", "/api/v1/push/subscriptions", tok, "", subscribeBody(endpoint)), want, "subscribe "+endpoint).str("id")
	}
	adminSub := sub(admin, ep("admin"), 201)
	supSub := sub(sup, ep("sup"), 201)
	supBSub := sub(supB, "https://push.test/gone-supb-"+run, 201)
	hrSub := sub(hr, "https://push.test/gone-hr-"+run, 201)
	clientSub := sub(client, ep("client"), 201)
	otherSub := sub(otherRealm, ep("other"), 201)
	if again := sub(admin, ep("admin"), 200); again != adminSub {
		t.Fatalf("re-subscribe returned %s, want %s", again, adminSub)
	}
	// The same browser profile used by another account moves to it.
	shared := sub(sup, ep("shared"), 201)
	if moved := sub(hr, ep("shared"), 201); moved != shared {
		t.Fatalf("rebound subscription id %s, want %s", moved, shared)
	}
	if l := e.expect(e.do("GET", "/api/v1/push/subscriptions", sup, "", nil), 200, "list").list(); len(l) != 1 ||
		l[0].(map[string]any)["id"] != supSub || l[0].(map[string]any)["endpoint"] != ep("sup") || l[0].(map[string]any)["p256dh"] != nil {
		t.Fatalf("sup subscriptions = %v", l)
	}
	e.expect(e.do("DELETE", "/api/v1/push/subscriptions/"+shared, sup, "", nil), 404, "delete a moved subscription")
	e.expect(e.do("DELETE", "/api/v1/push/subscriptions/"+shared, hr, "", nil), 204, "delete own")
	e.expect(e.do("DELETE", "/api/v1/push/subscriptions/not-a-uuid", hr, "", nil), 404, "malformed id")
	for name, body := range map[string]map[string]any{
		"http endpoint": subscribeBody("http://push.test/x"),
		"unknown host":  subscribeBody("https://evil.example/x"),
		"private ip":    subscribeBody("https://10.0.0.1/x"),
		"metadata ip":   subscribeBody("https://169.254.169.254/x"),
		"missing keys":  {"kind": "webpush", "endpoint": ep("x")},
		"wrong kind":    {"kind": "fcm", "endpoint": ep("x")},
		"invalid json":  nil,
		"long label":    func() map[string]any { m := subscribeBody(ep("x")); m["label"] = strings.Repeat("l", 101); return m }(),
		"bad auth bytes": func() map[string]any {
			m := subscribeBody(ep("x"))
			m["keys"].(map[string]string)["auth"] = "AAAA"
			return m
		}(),
	} {
		if r := e.do("POST", "/api/v1/push/subscriptions", admin, "", body); r.status != 400 {
			t.Errorf("%s: %d %s", name, r.status, r.raw)
		}
	}
	tablet := a.tablet
	e.expect(e.do("POST", "/api/v1/push/subscriptions", tablet, "", subscribeBody(ep("tab"))), 403, "tablet subscribes as web user")

	// ── preferences ──────────────────────────────────────────────────────────
	prefs := e.expect(e.do("GET", "/api/v1/push/preferences", sup, "", nil), 200, "prefs").obj()
	if av := prefs["available"].([]any); len(av) != 10 || prefs["events"].(map[string]any)["incident_filed"] != false ||
		prefs["events"].(map[string]any)["sos"] != true {
		t.Fatalf("supervisor prefs = %v", prefs)
	}
	prefs = e.expect(e.do("PUT", "/api/v1/push/preferences", sup, "", map[string]any{"events": map[string]bool{"incident_filed": true}}), 200, "opt in").obj()
	if ev := prefs["events"].(map[string]any); ev["incident_filed"] != true || ev["sos"] != true {
		t.Fatalf("after opt-in = %v", prefs)
	}
	if r := e.do("PUT", "/api/v1/push/preferences", hr, "", map[string]any{"events": map[string]bool{"sos": true}}); r.str("code") != "invalid_event" {
		t.Fatalf("hr opts into sos: %d %s", r.status, r.raw)
	}
	cp := e.expect(e.do("GET", "/api/v1/push/preferences", client, "", nil), 200, "client prefs").obj()
	if av := cp["available"].([]any); len(av) != 2 || cp["events"].(map[string]any)["incident_high"] != false {
		t.Fatalf("client prefs = %v", cp)
	}
	e.expect(e.do("PUT", "/api/v1/push/preferences", client, "", map[string]any{"events": map[string]bool{"incident_high": true}}), 200, "client opt in")

	// ── tablet tokens ────────────────────────────────────────────────────────
	e.expect(e.do("PUT", "/api/v1/device/push-token", a.tablet, "", map[string]any{"token": "fcm-a1-" + run, "platform": "android"}), 204, "token A1")
	e.expect(e.do("PUT", "/api/v1/device/push-token", b.tablet, "", map[string]any{"token": "fcm-b1-" + run, "platform": "ios"}), 204, "token B1")
	if r := e.do("PUT", "/api/v1/device/push-token", a.tablet, "", map[string]any{"token": "x", "platform": "windows"}); r.str("code") != "invalid_platform" {
		t.Fatalf("bad platform: %d %s", r.status, r.raw)
	}
	if r := e.do("PUT", "/api/v1/device/push-token", a.tablet, "", map[string]any{"token": "has space", "platform": "android"}); r.str("code") != "invalid_token" {
		t.Fatalf("bad token: %d %s", r.status, r.raw)
	}
	e.expect(e.do("PUT", "/api/v1/device/push-token", admin, "", map[string]any{"token": "t", "platform": "android"}), 403, "web user sets a tablet token")
	// A pending tablet registers its token, then hears its approval.
	tabA2 := e.token(realm2, "tablet-"+uuid.NewString(), "securepatrol-tablet", "device")
	devA2 := e.expect(e.do("POST", "/api/v1/device/register", tabA2, "", map[string]any{"name": "Pos A2 " + run}), 200, "register A2").str("id")
	if got := e.recipients("device_pending:"+devA2, "webpush"); !sameSet(got, adminSub) {
		t.Fatalf("device_pending push to %v, want only the admin", got)
	}
	e.expect(e.do("POST", "/api/v1/device/register", tabA2, "", map[string]any{"name": "Pos A2 " + run}), 200, "register again")
	if n := e.count(`SELECT COUNT(*) FROM notification_log WHERE dedupe_key LIKE $1`, "device_pending:"+devA2+"%"); n != 1 {
		t.Fatalf("re-registration staged %d device_pending deliveries", n)
	}
	e.expect(e.do("PUT", "/api/v1/device/push-token", tabA2, "", map[string]any{"token": "fcm-a2-" + run, "platform": "android"}), 204, "pending token")
	e.expect(e.do("PUT", "/api/v1/devices/"+devA2, admin, "", map[string]any{"status": "active", "site_id": a.site}), 200, "approve A2")
	if got := e.recipients("device_status:"+devA2, "fcm"); !sameSet(got, "fcm-a2-"+run) {
		t.Fatalf("device_status to %v", got)
	}
	devs := e.expect(e.do("GET", "/api/v1/devices", admin, "", nil), 200, "devices").list()
	for _, d := range devs {
		if m := d.(map[string]any); m["id"] == devA2 && m["push_enabled"] != true {
			t.Fatalf("A2 push_enabled = %v", m["push_enabled"])
		}
	}

	// ── SOS: audience by role and site, tablets except the source ───────────
	sosA := e.expect(e.do("POST", "/api/v1/sos", a.tablet, a.guard, map[string]any{}), 201, "SOS A").str("id")
	if got := e.recipients("sos:"+sosA, "webpush"); !sameSet(got, adminSub, supSub) {
		t.Fatalf("SOS A web push to %v, want admin+sup (not site-B supervisor, hr, client, other realm)", got)
	}
	if got := e.recipients("sos:"+sosA, "fcm"); !sameSet(got, "fcm-a2-"+run) {
		t.Fatalf("SOS A tablet push to %v, want only the other tablet of site A", got)
	}
	sosB := e.expect(e.do("POST", "/api/v1/sos", b.tablet, b.guard, map[string]any{}), 201, "SOS B").str("id")
	if got := e.recipients("sos:"+sosB, "webpush"); !sameSet(got, adminSub, supSub, supBSub) {
		t.Fatalf("SOS B web push to %v", got)
	}
	if got := e.recipients("sos:"+sosB, "fcm"); len(got) != 0 {
		t.Fatalf("SOS B pushed its own tablet: %v", got)
	}
	var payload, lane string
	if err := e.pool.QueryRow(ctx, `SELECT body, lane FROM notification_log WHERE dedupe_key = $1`,
		"sos:"+sosA+"|webpush|"+adminSub).Scan(&payload, &lane); err != nil {
		t.Fatal(err)
	}
	var p map[string]any
	_ = json.Unmarshal([]byte(payload), &p)
	if p["url"] != "/incidents?tab=sos" || p["severity"] != "critical" || p["tag"] != "sos-"+sosA || p["site_id"] != a.site ||
		!strings.Contains(p["title"].(string), "SOS") || lane != "sos" {
		t.Fatalf("SOS payload = %v lane %s", p, lane)
	}
	for _, l := range e.expect(e.do("GET", "/api/v1/notifications/log", admin, "", nil), 200, "log").list() {
		if ch := l.(map[string]any)["channel"]; ch == "webpush" || ch == "fcm" {
			t.Fatalf("push delivery in the rule log: %v", l)
		}
	}

	// ── delivery by the worker: web push + FCM, 410 removes the subscription ─
	w := notification.NewWorker(e.pool, nil, e.settings, e.notifier, 2, zerolog.Nop())
	w.UsePush(e.push)
	wctx, stop := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() { w.Run(wctx); close(done) }()
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) && (!wp.got("/admin-"+run) || fcm.find("fcm-a2-"+run, "sos") == nil ||
		e.count(`SELECT COUNT(*) FROM push_subscriptions WHERE id = $1`, supBSub) != 0) {
		time.Sleep(200 * time.Millisecond)
	}
	stop()
	<-done
	if !wp.got("/admin-"+run) || !wp.got("/sup-"+run) {
		t.Fatalf("web push not delivered: %v", wp.paths)
	}
	m := fcm.find("fcm-a2-"+run, "sos")
	if m == nil {
		t.Fatal("FCM SOS not delivered to tablet A2")
	}
	if d := m["data"].(map[string]any); d["ref_id"] != sosA || d["site_id"] != a.site || m["android"].(map[string]any)["priority"] != "high" {
		t.Fatalf("FCM SOS message = %v", m)
	}
	if n := e.count(`SELECT COUNT(*) FROM push_subscriptions WHERE id = $1`, supBSub); n != 0 {
		t.Fatal("410 Gone did not remove the subscription")
	}
	var lastSuccess *time.Time
	_ = e.pool.QueryRow(ctx, `SELECT last_success_at FROM push_subscriptions WHERE id = $1`, adminSub).Scan(&lastSuccess)
	if lastSuccess == nil {
		t.Error("delivered subscription has no last_success_at")
	}

	// ── sos_unacknowledged: reminders, then acknowledgement ─────────────────
	e.expect(e.do("POST", "/api/v1/notifications/rules", admin, "", map[string]any{
		"site_id": a.site, "event_type": "sos_unacknowledged", "channel": "email", "recipient": "duty@garda.test",
	}), 201, "sos_unacknowledged rule")
	if _, err := e.pool.Exec(ctx, `UPDATE sos_alerts SET received_at = now() - interval '4 minutes' WHERE id = $1`, sosA); err != nil {
		t.Fatal(err)
	}
	if err := e.sched.EscalateSOS(ctx); err != nil {
		t.Fatal(err)
	}
	if got := e.recipients("sos_unack:"+sosA+":1", "webpush"); !sameSet(got, adminSub, supSub) {
		t.Fatalf("reminder 1 web push to %v", got)
	}
	if got := e.recipients("sos_unack:"+sosA+":1", "email"); !sameSet(got, "duty@garda.test") {
		t.Fatalf("reminder 1 e-mail to %v", got)
	}
	if err := e.sched.EscalateSOS(ctx); err != nil {
		t.Fatal(err)
	}
	if n := e.count(`SELECT COUNT(*) FROM notification_log WHERE dedupe_key LIKE $1`, "sos_unack:"+sosA+"%"); n != 3 {
		t.Fatalf("second run staged again: %d deliveries", n)
	}
	if _, err := e.pool.Exec(ctx, `UPDATE sos_alerts SET received_at = now() - interval '9 minutes' WHERE id = $1`, sosA); err != nil {
		t.Fatal(err)
	}
	_ = e.sched.EscalateSOS(ctx)
	if n := e.count(`SELECT escalations FROM sos_alerts WHERE id = $1`, sosA); n != 2 {
		t.Fatalf("escalations = %d, want 2", n)
	}
	e.expect(e.do("POST", "/api/v1/sos/"+sosA+"/acknowledge", sup, "", nil), 200, "acknowledge")
	if got := e.recipients("sos_ack:"+sosA, "fcm"); !sameSet(got, "fcm-a1-"+run, "fcm-a2-"+run) {
		t.Fatalf("sos_ack to %v", got)
	}
	e.expect(e.do("POST", "/api/v1/sos/"+sosA+"/acknowledge", admin, "", nil), 200, "acknowledge again")
	if n := e.count(`SELECT COUNT(*) FROM notification_log WHERE dedupe_key LIKE $1`, "sos_ack:"+sosA+"%"); n != 2 {
		t.Fatalf("second acknowledge staged again: %d", n)
	}
	if _, err := e.pool.Exec(ctx, `UPDATE sos_alerts SET received_at = now() - interval '14 minutes' WHERE id = $1`, sosA); err != nil {
		t.Fatal(err)
	}
	_ = e.sched.EscalateSOS(ctx)
	if n := e.count(`SELECT COUNT(*) FROM notification_log WHERE dedupe_key LIKE $1`, "sos_unack:"+sosA+":3%"); n != 0 {
		t.Fatal("acknowledged SOS escalated")
	}

	// ── incidents: preferences and the client role ──────────────────────────
	inc := e.expect(e.do("POST", "/api/v1/incidents", a.tablet, a.guard, map[string]any{"type": "intrusion", "severity": "high"}), 201, "incident").str("id")
	if got := e.recipients("incident:"+inc, "webpush"); !sameSet(got, adminSub, supSub, clientSub) {
		t.Fatalf("incident_high to %v", got)
	}
	if got := e.recipients("incident_filed:"+inc, "webpush"); !sameSet(got, supSub) {
		t.Fatalf("incident_filed to %v (default off, sup opted in)", got)
	}
	_ = e.pool.QueryRow(ctx, `SELECT body FROM notification_log WHERE dedupe_key = $1`, "incident:"+inc+"|webpush|"+adminSub).Scan(&payload)
	if !strings.Contains(payload, `"url":"/incidents?id=`+inc+`"`) {
		t.Fatalf("incident payload = %s", payload)
	}
	// A user whose snapshot is too old (no request for 30 days) gets nothing.
	if _, err := e.pool.Exec(ctx, `UPDATE push_users SET seen_at = now() - interval '31 days' WHERE keycloak_user_id = $1`, "push-client-"+run); err != nil {
		t.Fatal(err)
	}
	inc2 := e.expect(e.do("POST", "/api/v1/incidents", a.tablet, a.guard, map[string]any{"type": "fire", "severity": "critical"}), 201, "incident 2").str("id")
	if got := e.recipients("incident:"+inc2, "webpush"); slices.Contains(got, clientSub) {
		t.Fatal("stale audience snapshot still receives pushes")
	}

	// ── attendance review (push only) ────────────────────────────────────────
	now := time.Now().UTC()
	e.expect(e.do("POST", "/api/v1/shifts", admin, "", map[string]any{
		"site_id": a.site, "guard_id": a.guard2, "start_time": rfc(now.Add(-time.Minute)), "end_time": rfc(now.Add(8 * time.Hour)),
	}), 201, "shift")
	rec := e.expect(e.doH("POST", "/api/v1/attendance/clock-in", a.tablet, a.guard2, map[string]any{},
		map[string]string{"X-Guard-Token": ""}), 201, "unverified clock-in").obj()
	if rec["needs_review"] != true {
		t.Fatalf("unverified clock-in not queued for review: %v", rec)
	}
	if got := e.recipients("attendance_review:"+rec["id"].(string), "webpush"); !sameSet(got, adminSub, supSub, hrSub) {
		t.Fatalf("attendance_review to %v", got)
	}

	// ── tablet sync, debounced per site ──────────────────────────────────────
	e.expect(e.do("PUT", "/api/v1/guards/"+a.guard2, admin, "", map[string]any{"name": "Renamed " + run, "site_id": a.site, "role": "guard", "status": "active"}), 200, "rename guard")
	e.expect(e.do("PUT", "/api/v1/guards/"+a.guard2, admin, "", map[string]any{"name": "Renamed2 " + run, "site_id": a.site, "role": "guard", "status": "active"}), 200, "rename again")
	syncs := e.count(`SELECT COUNT(*) FROM notification_log WHERE dedupe_key LIKE $1 AND next_attempt_at > now() AND next_attempt_at <= now() + interval '31 seconds'`, "sync:"+a.site+"%")
	if syncs != 2 && syncs != 4 { // two tablets; a window boundary between the two edits may add one more
		t.Fatalf("sync deliveries = %d", syncs)
	}
	if got := e.recipients("sync:"+b.site, "fcm"); len(got) != 0 {
		t.Fatalf("site B tablets told to sync: %v", got)
	}

	// ── tablet health (push only, admins) ────────────────────────────────────
	e.mr.FlushAll()
	e.expect(e.do("POST", "/api/v1/device/telemetry", a.tablet, "", map[string]any{
		"app_version": "1.0", "platform": "android", "outbox_pending": 0, "clock_offset_ms": 300000, "errors": []string{},
	}), 204, "telemetry")
	if err := e.sched.CheckDeviceHealth(ctx); err != nil {
		t.Fatal(err)
	}
	if err := e.sched.CheckDeviceHealth(ctx); err != nil {
		t.Fatal(err)
	}
	if n := e.count(`SELECT COUNT(*) FROM notification_log WHERE event_type = 'device_unhealthy' AND channel = 'webpush' AND recipient = $1`, adminSub); n != 1 {
		t.Fatalf("device_unhealthy pushes to admin = %d, want 1 (debounced)", n)
	}
	if n := e.count(`SELECT COUNT(*) FROM notification_log WHERE event_type = 'device_unhealthy' AND recipient = $1`, supSub); n != 0 {
		t.Fatal("device_unhealthy reached a supervisor")
	}

	// ── test push: 410 removes, other subscriptions count as sent ───────────
	res := e.expect(e.do("POST", "/api/v1/push/test", admin, "", map[string]any{"lang": "en"}), 200, "test push").obj()
	if res["sent"] != float64(1) || res["failed"] != float64(0) {
		t.Fatalf("admin test push = %v", res)
	}
	res = e.expect(e.do("POST", "/api/v1/push/test", hr, "", nil), 200, "test push hr").obj()
	if res["sent"] != float64(0) || res["failed"] != float64(1) {
		t.Fatalf("hr test push = %v", res)
	}
	if l := e.expect(e.do("GET", "/api/v1/push/subscriptions", hr, "", nil), 200, "hr subs").list(); len(l) != 0 {
		t.Fatalf("gone subscription kept: %v", l)
	}

	// ── revoke clears the token after telling the tablet ────────────────────
	e.expect(e.do("PUT", "/api/v1/devices/"+devA2, admin, "", map[string]any{"status": "revoked", "site_id": a.site}), 200, "revoke A2")
	if n := e.count(`SELECT COUNT(*) FROM notification_log WHERE dedupe_key LIKE $1 AND channel = 'fcm' AND recipient = $2`,
		"device_status:"+devA2+"%", "fcm-a2-"+run); n != 2 {
		t.Fatalf("device_status deliveries to A2 = %d, want approval + revocation", n)
	}
	if n := e.count(`SELECT COUNT(*) FROM devices WHERE id = $1 AND push_token IS NULL`, devA2); n != 1 {
		t.Fatal("revoked tablet kept its push token")
	}
	e.expect(e.do("PUT", "/api/v1/device/push-token", tabA2, "", map[string]any{"token": "fcm-a2b-" + run, "platform": "android"}), 403, "revoked tablet token")
	e.expect(e.do("DELETE", "/api/v1/device/push-token", a.tablet, "", nil), 204, "tablet removes its token")
	if n := e.count(`SELECT COUNT(*) FROM devices WHERE push_token = $1`, "fcm-a1-"+run); n != 0 {
		t.Fatal("token not cleared")
	}
	_ = otherSub
}

func TestPushDisabledByDefault(t *testing.T) {
	e := newEnv(t)
	admin := e.web(realm2, "push-off-admin", "admin")
	cfg := e.expect(e.do("GET", "/api/v1/push/config", admin, "", nil), 200, "config").obj()
	if cfg["webpush"].(map[string]any)["enabled"] != false || cfg["webpush"].(map[string]any)["vapid_public_key"] != "" ||
		cfg["fcm"].(map[string]any)["enabled"] != false {
		t.Fatalf("config = %v", cfg)
	}
	if r := e.do("POST", "/api/v1/push/subscriptions", admin, "", subscribeBody("https://fcm.googleapis.com/fcm/send/x")); r.str("code") != "push_disabled" {
		t.Fatalf("subscribe while disabled: %d %s", r.status, r.raw)
	}
	if r := e.do("POST", "/api/v1/push/test", admin, "", nil); r.str("code") != "push_disabled" {
		t.Fatalf("test while disabled: %d %s", r.status, r.raw)
	}
	// Preferences work without a transport (the page shows them anyway).
	e.expect(e.do("PUT", "/api/v1/push/preferences", admin, "", map[string]any{"events": map[string]bool{"late_clock_in": true}}), 200, "prefs")
	// A platform-admin-only operator may open the page; no events are available.
	op := e.web(realm1, "push-operator", "platform-admin")
	if av := e.expect(e.do("GET", "/api/v1/push/preferences", op, "", nil), 200, "operator prefs").obj()["available"].([]any); len(av) != 0 {
		t.Fatalf("operator available = %v", av)
	}
}
