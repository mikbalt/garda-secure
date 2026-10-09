//go:build integration

// Package integration exercises the HTTP API end to end against a real
// Postgres 16 (embedded, no Docker) and an in-memory Redis.
//
//	go test -tags integration ./internal/integration/
package integration

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	embeddedpostgres "github.com/fergusstrange/embedded-postgres"
	"github.com/gofiber/fiber/v2"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	"github.com/rs/zerolog"

	"github.com/securepatrol/backend/internal/config"
	"github.com/securepatrol/backend/internal/db"
	"github.com/securepatrol/backend/internal/handler"
	"github.com/securepatrol/backend/internal/jobs"
	"github.com/securepatrol/backend/internal/middleware"
	"github.com/securepatrol/backend/internal/notification"
	"github.com/securepatrol/backend/internal/service"
	"github.com/securepatrol/backend/internal/settings"
	"github.com/securepatrol/backend/internal/storage"
)

const pgPort = 54329

var dsn = fmt.Sprintf("postgres://postgres:postgres@localhost:%d/postgres?sslmode=disable", pgPort)

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "sp-pg-*")
	if err != nil {
		panic(err)
	}
	cache := filepath.Join(os.TempDir(), "sp-embedded-pg-cache")
	pg := embeddedpostgres.NewDatabase(embeddedpostgres.DefaultConfig().
		Version(embeddedpostgres.V16).Port(pgPort).Locale("C").Encoding("UTF8").
		RuntimePath(filepath.Join(dir, "rt")).DataPath(filepath.Join(dir, "data")).CachePath(cache).
		Logger(io.Discard))
	if err := pg.Start(); err != nil {
		fmt.Fprintln(os.Stderr, "embedded postgres:", err)
		os.Exit(1)
	}
	if err := db.RunMigrations(dsn, "../../migrations"); err != nil {
		_ = pg.Stop()
		fmt.Fprintln(os.Stderr, "migrations:", err)
		os.Exit(1)
	}
	code := m.Run()
	_ = pg.Stop()
	_ = os.RemoveAll(dir)
	os.Exit(code)
}

// ── Test environment ─────────────────────────────────────────────────────────

type env struct {
	t        *testing.T
	app      *fiber.App
	pool     *pgxpool.Pool
	mr       *miniredis.Miniredis
	key      *rsa.PrivateKey
	sched    *jobs.Scheduler
	settings *settings.Store
}

const (
	realm1 = "securepatrol-client1"
	realm2 = "securepatrol-client2"
	issuer = "https://sp.test/auth/realms/"
)

func newEnv(t *testing.T) *env {
	t.Helper()
	ctx := context.Background()
	pool, err := db.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)

	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	jwks := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"keys": []map[string]string{{
			"kid": "k1", "kty": "RSA", "use": "sig",
			"n": base64.RawURLEncoding.EncodeToString(key.N.Bytes()),
			"e": base64.RawURLEncoding.EncodeToString(big.NewInt(int64(key.E)).Bytes()),
		}}})
	}))
	t.Cleanup(jwks.Close)

	cfg := &config.Config{
		DatabaseURL: dsn, MinioBucketPhotos: "photos", MinioBucketBackups: "backups",
		SettingsEncryptionKey: "test-key", SMTPPort: "587", WhatsAppAPIURL: "https://graph.example",
		KeycloakRealms: []string{realm1, realm2},
	}
	stg, err := storage.New(storage.Options{Endpoint: "127.0.0.1:1", AccessKey: "x", SecretKey: "y"})
	if err != nil {
		t.Fatal(err)
	}
	st, err := settings.NewStore(pool, cfg)
	if err != nil {
		t.Fatal(err)
	}
	log := zerolog.Nop()
	notifier := notification.NewNotifier(pool, rdb, log)
	validator := middleware.NewValidator([]middleware.RealmKeys{
		{Realm: realm1, Issuer: issuer + realm1, JWKSURL: jwks.URL},
		{Realm: realm2, Issuer: issuer + realm2, JWKSURL: jwks.URL},
	}, []string{"securepatrol-web", "securepatrol-tablet"}, log)

	h := &handler.Handler{DB: pool, Redis: rdb, Storage: stg, Notifier: notifier, Settings: st, Cfg: cfg, Log: log}
	app := fiber.New(fiber.Config{BodyLimit: handler.MaxPhotoBytes + 1<<20})
	api := app.Group("/api/v1", middleware.JWT(validator), middleware.LoadDevice(pool), middleware.Audit(pool))
	h.Register(api)

	return &env{t: t, app: app, pool: pool, mr: mr, key: key, settings: st,
		sched: jobs.NewScheduler(pool, rdb, notifier, stg, cfg, log)}
}

func (e *env) token(realm, sub, client string, roles ...string) string {
	tok := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims{
		"iss": issuer + realm, "sub": sub, "preferred_username": sub, "azp": client,
		"exp": time.Now().Add(time.Hour).Unix(), "roles": roles,
	})
	tok.Header["kid"] = "k1"
	s, err := tok.SignedString(e.key)
	if err != nil {
		e.t.Fatal(err)
	}
	return s
}

func (e *env) web(realm, user string, roles ...string) string {
	return e.token(realm, user, "securepatrol-web", roles...)
}

type resp struct {
	status int
	raw    []byte
	body   any
}

func (r resp) obj() map[string]any {
	m, _ := r.body.(map[string]any)
	return m
}

func (r resp) list() []any {
	l, _ := r.body.([]any)
	return l
}

func (r resp) str(key string) string {
	v, _ := r.obj()[key].(string)
	return v
}

func (e *env) do(method, path, token, guardID string, body any) resp {
	e.t.Helper()
	var rdr io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rdr = bytes.NewReader(b)
	}
	req := httptest.NewRequest(method, path, rdr)
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if guardID != "" {
		req.Header.Set(middleware.GuardHeader, guardID)
	}
	res, err := e.app.Test(req, 30_000)
	if err != nil {
		e.t.Fatalf("%s %s: %v", method, path, err)
	}
	raw, _ := io.ReadAll(res.Body)
	r := resp{status: res.StatusCode, raw: raw}
	_ = json.Unmarshal(raw, &r.body)
	return r
}

func (e *env) expect(r resp, status int, what string) resp {
	e.t.Helper()
	if r.status != status {
		e.t.Fatalf("%s: status %d, want %d — %s", what, r.status, status, r.raw)
	}
	return r
}

func (e *env) count(sql string, args ...any) int {
	e.t.Helper()
	var n int
	if err := e.pool.QueryRow(context.Background(), sql, args...).Scan(&n); err != nil {
		e.t.Fatalf("count %q: %v", sql, err)
	}
	return n
}

func rfc(t time.Time) string { return t.UTC().Format(time.RFC3339) }

// ── The scenario ─────────────────────────────────────────────────────────────

func TestAPIEndToEnd(t *testing.T) {
	e := newEnv(t)
	admin := e.web(realm1, "admin1", "admin")
	sup := e.web(realm1, "sup1", "supervisor")
	hr := e.web(realm1, "hr1", "hr")
	client := e.web(realm1, "owner1", "client-readonly")
	otherAdmin := e.web(realm2, "admin2", "admin")
	now := time.Now().UTC()

	// ── Sites & tenancy ──────────────────────────────────────────────────────
	siteA := e.expect(e.do("POST", "/api/v1/sites", admin, "", map[string]any{
		"name": "Gudang A", "timezone": "Asia/Jakarta",
		"config_json": map[string]any{"late_grace_minutes": 5, "absent_after_minutes": 30},
	}), 201, "create site").str("id")
	siteB := e.expect(e.do("POST", "/api/v1/sites", admin, "", map[string]any{"name": "Kantor B"}), 201, "create site B").str("id")
	e.expect(e.do("POST", "/api/v1/sites", sup, "", map[string]any{"name": "x"}), 403, "supervisor creates site")
	e.expect(e.do("GET", "/api/v1/sites/"+siteA, otherAdmin, "", nil), 404, "other realm reads site")
	if l := e.do("GET", "/api/v1/sites", otherAdmin, "", nil).list(); len(l) != 0 {
		t.Fatalf("other realm sees %d sites", len(l))
	}
	e.expect(e.do("GET", "/api/v1/sites/not-a-uuid", admin, "", nil), 404, "malformed id")
	e.expect(e.do("GET", "/api/v1/sites", "", "", nil), 401, "no token")

	// ── Guards & PINs ────────────────────────────────────────────────────────
	mkGuard := func(name, site, role string) string {
		return e.expect(e.do("POST", "/api/v1/guards", admin, "", map[string]any{
			"name": name, "site_id": site, "role": role,
		}), 201, "create guard "+name).str("id")
	}
	budi := mkGuard("Budi", siteA, "guard")
	sari := mkGuard("Sari", siteA, "supervisor")
	joko := mkGuard("Joko", siteA, "guard")
	dewi := mkGuard("Dewi", siteB, "guard")
	e.expect(e.do("PUT", "/api/v1/guards/"+budi+"/pin", admin, "", map[string]any{"pin": "1234"}), 400, "weak pin")
	e.expect(e.do("PUT", "/api/v1/guards/"+budi+"/pin", admin, "", map[string]any{"pin": "4826"}), 200, "set pin")
	e.expect(e.do("GET", "/api/v1/guards?site_id="+siteA, hr, "", nil), 200, "hr lists guards")
	e.expect(e.do("GET", "/api/v1/guards", client, "", nil), 403, "client lists guards")
	e.expect(e.do("POST", "/api/v1/guards", otherAdmin, "", map[string]any{"name": "X", "site_id": siteA}), 404, "other realm assigns guard to site")
	e.expect(e.do("POST", "/api/v1/guards", admin, "", map[string]any{"name": "No Site"}), 400, "guard without site")
	e.expect(e.do("GET", "/api/v1/guards/"+budi, otherAdmin, "", nil), 404, "other realm reads guard")
	if l := e.do("GET", "/api/v1/guards", otherAdmin, "", nil).list(); len(l) != 0 {
		t.Fatalf("other realm sees %d guards", len(l))
	}

	// ── Device registration & approval ───────────────────────────────────────
	tablet := e.token(realm1, "device-user-1", "securepatrol-tablet", "device")
	deviceID := e.expect(e.do("POST", "/api/v1/device/register", tablet, "", map[string]any{"name": "Pos 1"}), 200, "register").str("id")
	r := e.do("POST", "/api/v1/attendance/clock-in", tablet, budi, map[string]any{})
	if r.status != 403 || r.str("code") != "device_pending" {
		t.Fatalf("pending device: %d %s", r.status, r.raw)
	}
	e.expect(e.do("PUT", "/api/v1/devices/"+deviceID, admin, "", map[string]any{"status": "active"}), 400, "activate without site")
	e.expect(e.do("PUT", "/api/v1/devices/"+deviceID, otherAdmin, "", map[string]any{"status": "active", "site_id": siteA}), 404, "other realm approves")
	e.expect(e.do("PUT", "/api/v1/devices/"+deviceID, admin, "", map[string]any{"status": "active", "site_id": siteA}), 200, "approve")
	roster := e.expect(e.do("GET", "/api/v1/device/roster", tablet, "", nil), 200, "roster").obj()["guards"].([]any)
	if len(roster) != 3 {
		t.Fatalf("roster has %d guards, want 3 (site A only)", len(roster))
	}
	for _, g := range roster {
		gm := g.(map[string]any)
		if gm["id"] == budi && gm["pin_hash"] == nil {
			t.Error("roster lacks Budi's PIN hash")
		}
	}
	e.expect(e.do("GET", "/api/v1/sites", tablet, "", nil), 403, "device on web route")

	// ── Shifts & attendance ──────────────────────────────────────────────────
	mkShift := func(guard string, start, end time.Time) string {
		return e.expect(e.do("POST", "/api/v1/shifts", admin, "", map[string]any{
			"site_id": siteA, "guard_id": guard, "start_time": rfc(start), "end_time": rfc(end),
		}), 201, "create shift").str("id")
	}
	mkShift(budi, now.Add(-3*time.Minute), now.Add(8*time.Hour))
	mkShift(joko, now.Add(-20*time.Minute), now.Add(8*time.Hour))
	sariShift := mkShift(sari, now.Add(-40*time.Minute), now.Add(7*time.Hour))
	e.expect(e.do("POST", "/api/v1/shifts", admin, "", map[string]any{
		"site_id": siteA, "guard_id": dewi, "start_time": rfc(now), "end_time": rfc(now.Add(time.Hour)),
	}), 400, "shift for guard of another site")

	for _, ev := range []string{"late_clock_in", "absent", "incident_high", "sla_missed"} {
		e.expect(e.do("POST", "/api/v1/notifications/rules", admin, "", map[string]any{
			"site_id": siteA, "event_type": ev, "channel": "email", "recipient": "ops@example.com",
		}), 201, "rule "+ev)
	}
	e.expect(e.do("POST", "/api/v1/notifications/rules", admin, "", map[string]any{
		"site_id": siteA, "event_type": "sos", "channel": "whatsapp", "recipient": "+62 812-3456-7890",
	}), 201, "rule sos")
	local := e.expect(e.do("POST", "/api/v1/notifications/rules", admin, "", map[string]any{
		"site_id": siteA, "event_type": "defect_opened", "channel": "whatsapp", "recipient": "0812-1111-2222",
	}), 201, "rule with local phone number")
	if local.str("recipient") != "6281211112222" {
		t.Fatalf("local number not normalised: %s", local.raw)
	}

	clockID := uuid.NewString()
	rec := e.expect(e.do("POST", "/api/v1/attendance/clock-in", tablet, budi, map[string]any{"id": clockID}), 201, "clock in")
	if rec.str("status") != "present" || rec.str("id") != clockID {
		t.Fatalf("clock-in = %s", rec.raw)
	}
	e.expect(e.do("POST", "/api/v1/attendance/clock-in", tablet, budi, map[string]any{"id": clockID}), 200, "clock-in retry")
	e.expect(e.do("POST", "/api/v1/attendance/clock-in", tablet, budi, map[string]any{"id": uuid.NewString()}), 409, "second clock-in")
	if r := e.do("POST", "/api/v1/attendance/clock-in", tablet, dewi, map[string]any{}); r.status != 403 || r.str("code") != "guard_not_allowed" {
		t.Fatalf("guard of other site: %d %s", r.status, r.raw)
	}
	e.expect(e.do("POST", "/api/v1/attendance/clock-in", tablet, "", map[string]any{}), 400, "missing guard header")
	e.expect(e.do("POST", "/api/v1/attendance/clock-in", tablet, budi, map[string]any{
		"clock_in_at": rfc(now.Add(-10 * 24 * time.Hour)),
	}), 400, "timestamp outside sync window")

	late := e.expect(e.do("POST", "/api/v1/attendance/clock-in", tablet, joko, map[string]any{}), 201, "late clock in")
	if late.str("status") != "late" || late.obj()["late_minutes"].(float64) < 19 {
		t.Fatalf("late clock-in = %s", late.raw)
	}
	if n := e.count(`SELECT COUNT(*) FROM notification_log WHERE event_type = 'late_clock_in'`); n != 1 {
		t.Fatalf("late notifications = %d", n)
	}

	ctx := context.Background()
	if err := e.sched.FlagAbsent(ctx); err != nil {
		t.Fatal(err)
	}
	if err := e.sched.FlagAbsent(ctx); err != nil {
		t.Fatal(err)
	}
	if n := e.count(`SELECT COUNT(*) FROM attendance_records WHERE status = 'absent'`); n != 1 {
		t.Fatalf("absent records = %d, want 1 (Sari)", n)
	}
	if n := e.count(`SELECT COUNT(*) FROM notification_log WHERE event_type = 'absent'`); n != 1 {
		t.Fatalf("absent notifications = %d, want exactly 1", n)
	}

	// Sari's tablet was offline: her clock-in arrives after the absent flag.
	sync := e.expect(e.do("POST", "/api/v1/attendance/clock-in", tablet, sari, map[string]any{
		"clock_in_at": rfc(now.Add(-38 * time.Minute)),
	}), 201, "offline clock-in after absent")
	if sync.obj()["needs_review"] != true || sync.str("status") != "present" { // 2 min after start: within grace
		t.Fatalf("offline clock-in = %s", sync.raw)
	}
	queue := e.expect(e.do("GET", "/api/v1/attendance/review?site_id="+siteA, sup, "", nil), 200, "review queue").list()
	if len(queue) != 1 {
		t.Fatalf("review queue = %d", len(queue))
	}
	e.expect(e.do("POST", "/api/v1/attendance/"+sync.str("id")+"/review", sup, "", map[string]any{"note": "ok"}), 200, "review")
	if q := e.do("GET", "/api/v1/attendance/review?site_id="+siteA, sup, "", nil).list(); len(q) != 0 {
		t.Fatalf("review queue after review = %d", len(q))
	}

	out := e.expect(e.do("POST", "/api/v1/attendance/clock-out", tablet, budi, map[string]any{}), 200, "clock out")
	if out.obj()["early_out_minutes"].(float64) < 400 {
		t.Fatalf("early-out minutes = %v", out.obj()["early_out_minutes"])
	}
	e.expect(e.do("POST", "/api/v1/attendance/clock-out", tablet, budi, map[string]any{}), 409, "clock out twice")
	e.expect(e.do("POST", "/api/v1/attendance/clock-out", tablet, budi, map[string]any{"attendance_id": clockID}), 200, "clock-out retry by id")

	daily := e.expect(e.do("GET", "/api/v1/attendance/daily?site_id="+siteA, client, "", nil), 200, "daily").obj()["rows"].([]any)
	if len(daily) != 3 {
		t.Fatalf("daily rows = %d", len(daily))
	}
	csv := e.expect(e.do("GET", "/api/v1/reports/attendance/daily?site_id="+siteA+"&format=csv", hr, "", nil), 200, "daily csv")
	if !bytes.HasPrefix(csv.raw, []byte("\xEF\xBB\xBFGuard,")) || !strings.Contains(string(csv.raw), "Budi") {
		t.Fatalf("csv = %q", csv.raw)
	}

	e.expect(e.do("PUT", "/api/v1/attendance/shifts/"+sariShift+"/override", admin, "", map[string]any{"status": "present"}), 400, "override without reason")
	e.expect(e.do("PUT", "/api/v1/attendance/shifts/"+sariShift+"/override", sup, "", map[string]any{"status": "present", "reason": "x"}), 403, "supervisor override")
	ov := e.expect(e.do("PUT", "/api/v1/attendance/shifts/"+sariShift+"/override", admin, "", map[string]any{
		"status": "present", "reason": "Traffic accident on toll road, approved by ops",
	}), 200, "override")
	if ov.str("status") != "present" || ov.str("overridden_by") == "" {
		t.Fatalf("override = %s", ov.raw)
	}

	// ── Shift templates ──────────────────────────────────────────────────────
	today := now.In(mustLoc(t, "Asia/Jakarta")).Format("2006-01-02")
	tpl := e.expect(e.do("POST", "/api/v1/shift-templates", admin, "", map[string]any{
		"site_id": siteA, "guard_id": joko, "start_time": "23:30", "end_time": "07:30",
		"weekdays": []int{1, 2, 3, 4, 5, 6, 7}, "valid_from": today,
	}), 201, "create template").obj()
	tplID := tpl["template"].(map[string]any)["id"].(string)
	if g := tpl["generated_shifts"].(float64); g < 14 {
		t.Fatalf("generated %v shifts, want ≥ 14", g)
	}
	if err := e.sched.GenerateShifts(ctx); err != nil {
		t.Fatal(err)
	}
	before := e.count(`SELECT COUNT(*) FROM shifts WHERE template_id = $1`, tplID)
	e.expect(e.do("PUT", "/api/v1/shift-templates/"+tplID, admin, "", map[string]any{
		"guard_id": joko, "start_time": "22:00", "end_time": "06:00", "weekdays": []int{1, 3, 5}, "valid_from": today,
	}), 200, "update template")
	after := e.count(`SELECT COUNT(*) FROM shifts WHERE template_id = $1`, tplID)
	if after >= before {
		t.Fatalf("regeneration: %d → %d shifts", before, after)
	}
	if n := e.count(`SELECT COUNT(*) FROM shifts WHERE template_id = $1 AND to_char(start_time AT TIME ZONE 'Asia/Jakarta', 'HH24:MI') <> '22:00'`, tplID); n != 0 {
		t.Fatalf("%d regenerated shifts have the old start time", n)
	}
	e.expect(e.do("DELETE", "/api/v1/shift-templates/"+tplID, admin, "", nil), 204, "delete template")
	if n := e.count(`SELECT COUNT(*) FROM shifts WHERE start_time > now() AND guard_id = $1 AND start_time > now() + interval '9 hours'`, joko); n != 0 {
		t.Fatalf("%d future template shifts left after delete", n)
	}

	// ── Patrol & SLA ─────────────────────────────────────────────────────────
	route := e.expect(e.do("POST", "/api/v1/patrol/routes", admin, "", map[string]any{
		"site_id": siteA, "name": "Patroli Malam",
		"checkpoints": []map[string]any{
			{"name": "Gerbang", "qr_code": "QR-GATE", "sla_minutes": 5, "order_index": 0},
			{"name": "Gudang", "qr_code": "QR-WH", "sla_minutes": 1, "order_index": 1},
			{"name": "Parkir", "qr_code": "QR-PARK", "order_index": 2},
		},
	}), 201, "create route")
	routeID := route.str("id")
	if l := e.expect(e.do("GET", "/api/v1/device/routes", tablet, "", nil), 200, "device routes").list(); len(l) != 1 {
		t.Fatalf("device routes = %d", len(l))
	}
	sessID := uuid.NewString()
	e.expect(e.do("POST", "/api/v1/patrol/sessions", tablet, budi, map[string]any{
		"id": sessID, "route_id": routeID, "started_at": rfc(now.Add(-10 * time.Minute)),
	}), 201, "start session")
	e.expect(e.do("POST", "/api/v1/patrol/sessions", tablet, budi, map[string]any{"id": sessID, "route_id": routeID}), 200, "start retry")
	st := e.expect(e.do("GET", "/api/v1/device/status", tablet, budi, nil), 200, "device status").obj()
	if st["session"] == nil || st["shift"] == nil {
		t.Fatalf("device status lacks session/shift: %v", st)
	}
	if sess := st["session"].(map[string]any)["session"].(map[string]any); sess["id"] != sessID {
		t.Fatalf("device status session = %v", sess["id"])
	}
	e.expect(e.do("POST", "/api/v1/patrol/sessions/"+sessID+"/scans", tablet, budi, map[string]any{
		"qr_code": "QR-GATE", "scanned_at": rfc(now.Add(-8 * time.Minute)),
	}), 201, "scan gate")
	e.expect(e.do("POST", "/api/v1/patrol/sessions/"+sessID+"/scans", tablet, budi, map[string]any{
		"qr_code": "QR-PARK", "method": "manual",
	}), 400, "manual without photo")
	e.expect(e.do("POST", "/api/v1/patrol/sessions/"+sessID+"/scans", tablet, budi, map[string]any{"qr_code": "QR-NOPE"}), 400, "unknown QR")

	if err := e.sched.CheckSLA(ctx); err != nil {
		t.Fatal(err)
	}
	if err := e.sched.CheckSLA(ctx); err != nil {
		t.Fatal(err)
	}
	if n := e.count(`SELECT COUNT(*) FROM sla_breaches WHERE session_id = $1`, sessID); n != 1 {
		t.Fatalf("SLA breaches = %d, want 1 (Gudang)", n)
	}
	if n := e.count(`SELECT COUNT(*) FROM notification_log WHERE event_type = 'sla_missed'`); n != 1 {
		t.Fatalf("SLA notifications = %d", n)
	}
	live := e.expect(e.do("GET", "/api/v1/patrol/live?site_id="+siteA, client, "", nil), 200, "live").list()
	if len(live) != 1 {
		t.Fatalf("live patrols = %d", len(live))
	}
	prog := e.expect(e.do("GET", "/api/v1/patrol/sessions/"+sessID, tablet, budi, nil), 200, "session via tablet").obj()
	cps := prog["checkpoints"].([]any)
	if cps[0].(map[string]any)["status"] != "on_time" || cps[1].(map[string]any)["status"] != "overdue" {
		t.Fatalf("progress = %v", cps)
	}
	e.expect(e.do("POST", "/api/v1/patrol/sessions/"+sessID+"/end", tablet, budi, nil), 200, "end session")
	sessions := e.expect(e.do("GET", "/api/v1/reports/patrol/sessions?site_id="+siteA, sup, "", nil), 200, "patrol report").obj()["rows"].([]any)
	if s := sessions[0].(map[string]any); s["scanned_count"].(float64) != 1 || s["sla_breaches"].(float64) != 1 || s["status"] != "completed" {
		t.Fatalf("session summary = %v", s)
	}

	// ── Incidents ────────────────────────────────────────────────────────────
	photo := service.ObjectKey(siteA, "incident", uuid.NewString(), "jpg")
	incID := uuid.NewString()
	incBody := map[string]any{
		"id": incID, "type": "Theft", "severity": "critical", "description": "Pallet missing",
		"location": "Dock 3", "photo_object_keys": []string{photo},
	}
	e.expect(e.do("POST", "/api/v1/incidents", tablet, budi, incBody), 201, "create incident")
	e.expect(e.do("POST", "/api/v1/incidents", tablet, budi, incBody), 200, "incident retry")
	e.expect(e.do("POST", "/api/v1/incidents", tablet, budi, map[string]any{
		"type": "theft", "photo_object_keys": []string{photo, photo, photo, photo, photo, photo},
	}), 400, "six photos")
	e.expect(e.do("POST", "/api/v1/incidents", tablet, budi, map[string]any{
		"type": "theft", "photo_object_keys": []string{service.ObjectKey(siteB, "incident", uuid.NewString(), "jpg")},
	}), 400, "photo key of another site")
	if n := e.count(`SELECT COUNT(*) FROM notification_log WHERE event_type = 'incident_high'`); n != 1 {
		t.Fatalf("incident notifications = %d", n)
	}
	if l := e.expect(e.do("GET", "/api/v1/incidents?site_id="+siteA, client, "", nil), 200, "client incidents").list(); len(l) != 1 {
		t.Fatalf("incidents = %d", len(l))
	}
	det := e.expect(e.do("GET", "/api/v1/incidents/"+incID, sup, "", nil), 200, "incident detail").obj()
	if p := det["photos"].([]any)[0].(map[string]any); p["uploaded"] != false {
		t.Fatalf("photo should be pending: %v", p)
	}
	e.expect(e.do("POST", "/api/v1/incidents/"+incID+"/amendments", client, "", map[string]any{
		"changes": map[string]string{"description": "x"}, "reason": "y",
	}), 403, "client amends")
	e.expect(e.do("POST", "/api/v1/incidents/"+incID+"/amendments", sup, "", map[string]any{
		"changes": map[string]string{"description": "Two pallets missing"}, "reason": "Recount by supervisor",
	}), 201, "amend")
	det = e.do("GET", "/api/v1/incidents/"+incID, sup, "", nil).obj()
	if det["incident"].(map[string]any)["description"] != "Pallet missing" || len(det["amendments"].([]any)) != 1 {
		t.Fatalf("original incident changed or amendment missing: %v", det)
	}
	e.expect(e.do("PUT", "/api/v1/incidents/"+incID+"/status", sup, "", map[string]any{"status": "in_progress"}), 200, "incident status")
	if l := e.do("GET", "/api/v1/incidents?site_id="+siteA, client, "", nil).list(); l[0].(map[string]any)["guard_name"] != "Budi" {
		t.Fatalf("incident list lacks guard_name: %v", l[0])
	}
	if p := e.expect(e.do("GET", "/api/v1/media/url?key="+photo, client, "", nil), 200, "media url").obj(); p["key"] != photo || p["uploaded"] != false {
		t.Fatalf("media url = %v", p)
	}
	e.expect(e.do("GET", "/api/v1/media/url?key="+photo, otherAdmin, "", nil), 404, "media url other realm")
	e.expect(e.do("GET", "/api/v1/media/url?key=sites/x/../../etc", client, "", nil), 400, "media url bad key")

	// ── SOS ──────────────────────────────────────────────────────────────────
	sosID := uuid.NewString()
	e.expect(e.do("POST", "/api/v1/sos", tablet, budi, map[string]any{"id": sosID, "gps_lat": -6.2, "gps_lng": 106.8}), 201, "sos")
	e.expect(e.do("POST", "/api/v1/sos", tablet, budi, map[string]any{"id": sosID}), 200, "sos retry")
	if l := e.expect(e.do("GET", "/api/v1/device/sos", tablet, "", nil), 200, "device sos").list(); len(l) != 1 {
		t.Fatalf("device sos = %d", len(l))
	}
	if n := e.count(`SELECT COUNT(*) FROM sos_alerts`); n != 1 {
		t.Fatalf("sos alerts = %d", n)
	}
	if n := e.count(`SELECT COUNT(*) FROM notification_log WHERE event_type = 'sos' AND recipient = '6281234567890'`); n != 1 {
		t.Fatalf("sos notifications = %d", n)
	}
	if n, _ := e.mr.List("notifications:queue"); len(n) == 0 {
		t.Fatal("nothing enqueued in redis")
	}
	e.expect(e.do("POST", "/api/v1/sos/"+sosID+"/acknowledge", tablet, budi, nil), 403, "guard acks sos")
	e.expect(e.do("POST", "/api/v1/sos/"+sosID+"/acknowledge", client, "", nil), 403, "client acks sos")
	ack := e.expect(e.do("POST", "/api/v1/sos/"+sosID+"/acknowledge", tablet, sari, nil), 200, "supervisor guard acks")
	if ack.str("acknowledged_by") != "guard:"+sari {
		t.Fatalf("ack = %s", ack.raw)
	}

	// ── Defects & visitors ───────────────────────────────────────────────────
	def := e.expect(e.do("POST", "/api/v1/defects", tablet, joko, map[string]any{"defect_type": "CCTV", "location": "Gate"}), 201, "defect")
	e.expect(e.do("PUT", "/api/v1/defects/"+def.str("id")+"/status", sup, "", map[string]any{"status": "resolved"}), 200, "defect status")
	vis := e.expect(e.do("POST", "/api/v1/visitors", tablet, joko, map[string]any{"visitor_name": "Andi", "plate": "B 1234 XY"}), 201, "visitor")
	e.expect(e.do("POST", "/api/v1/visitors/"+vis.str("id")+"/exit", tablet, joko, nil), 200, "visitor exit")
	if l := e.do("GET", "/api/v1/visitors?site_id="+siteA, sup, "", nil).list(); len(l) != 1 {
		t.Fatalf("visitors = %d", len(l))
	}

	// ── Reports & dashboard ──────────────────────────────────────────────────
	dash := e.expect(e.do("GET", "/api/v1/dashboard/summary?site_id="+siteA, client, "", nil), 200, "dashboard").obj()
	if dash["shifts_today"].(float64) < 3 || dash["open_incidents"].(float64) != 1 {
		t.Fatalf("dashboard = %v", dash)
	}
	recap := e.expect(e.do("GET", "/api/v1/reports/attendance/monthly?site_id="+siteA, hr, "", nil), 200, "monthly").obj()["rows"].([]any)
	if len(recap) < 3 {
		t.Fatalf("monthly recap rows = %d", len(recap))
	}
	e.expect(e.do("GET", "/api/v1/reports/guards/kpi?site_id="+siteA, client, "", nil), 403, "client KPI")
	kpi := e.expect(e.do("GET", "/api/v1/reports/guards/kpi?site_id="+siteA, sup, "", nil), 200, "kpi").obj()["rows"].([]any)
	if len(kpi) == 0 {
		t.Fatal("kpi empty")
	}
	e.expect(e.do("GET", "/api/v1/reports/incidents?site_id="+siteA+"&format=csv", client, "", nil), 200, "incident csv")

	// ── Monthly recap job publishes once per site and month ─────────────────
	orig := service.Now
	service.Now = func() time.Time { return time.Date(2026, 11, 1, 1, 0, 0, 0, time.UTC) } // 08:00 WIB on the 1st
	e.expect(e.do("POST", "/api/v1/notifications/rules", admin, "", map[string]any{
		"site_id": siteA, "event_type": "monthly_recap", "channel": "email", "recipient": "hr@example.com",
	}), 201, "recap rule")
	if err := e.sched.MonthlyRecap(ctx); err != nil {
		t.Fatal(err)
	}
	if err := e.sched.MonthlyRecap(ctx); err != nil {
		t.Fatal(err)
	}
	service.Now = orig
	if n := e.count(`SELECT COUNT(*) FROM notification_log WHERE event_type = 'monthly_recap' AND body LIKE '<h2>%'`); n != 1 {
		t.Fatalf("monthly recap notifications = %d", n)
	}

	// ── Settings, notification log, backups ──────────────────────────────────
	e.expect(e.do("PUT", "/api/v1/settings/channels", admin, "", map[string]any{
		"smtp": map[string]any{"host": "smtp.example.com", "port": "587", "password": "s3cret", "from": "SP <sp@example.com>"},
	}), 200, "save channels")
	got := e.expect(e.do("GET", "/api/v1/settings/channels", admin, "", nil), 200, "get channels").obj()
	if got["smtp"].(map[string]any)["password"] != "********" {
		t.Fatalf("secret not masked: %v", got)
	}
	if n := e.count(`SELECT COUNT(*) FROM system_settings WHERE value->'smtp'->>'password' LIKE 'enc:v1:%'`); n != 1 {
		t.Fatal("secret not encrypted at rest")
	}
	e.expect(e.do("PUT", "/api/v1/settings/channels", admin, "", map[string]any{
		"smtp": map[string]any{"host": "smtp2.example.com", "password": "********"},
	}), 200, "save channels keeping secret")
	if ch, err := e.settings.Channels(ctx); err != nil || ch.SMTP.Password != "s3cret" || ch.SMTP.Host != "smtp2.example.com" {
		t.Fatalf("channels after placeholder save = %+v, %v", ch.SMTP, err)
	}
	e.expect(e.do("GET", "/api/v1/settings/channels", sup, "", nil), 403, "supervisor reads settings")

	logRows := e.expect(e.do("GET", "/api/v1/notifications/log?site_id="+siteA, admin, "", nil), 200, "notification log").list()
	if len(logRows) < 5 {
		t.Fatalf("notification log rows = %d", len(logRows))
	}
	failedID := logRows[0].(map[string]any)["id"].(string)
	e.expect(e.do("POST", "/api/v1/notifications/log/"+failedID+"/retry", admin, "", nil), 409, "retry non-failed")
	if _, err := e.pool.Exec(ctx, `UPDATE notification_log SET status = 'failed', attempts = 3 WHERE id = $1`, failedID); err != nil {
		t.Fatal(err)
	}
	e.expect(e.do("POST", "/api/v1/notifications/log/"+failedID+"/retry", admin, "", nil), 200, "retry failed")

	e.expect(e.do("POST", "/api/v1/backups", admin, "", nil), 202, "request backup")
	if l := e.do("GET", "/api/v1/backups", admin, "", nil).list(); len(l) != 1 || l[0].(map[string]any)["status"] != "pending" {
		t.Fatalf("backups = %v", l)
	}

	// ── Revocation & audit ───────────────────────────────────────────────────
	e.expect(e.do("PUT", "/api/v1/devices/"+deviceID, admin, "", map[string]any{"status": "revoked", "site_id": siteA}), 200, "revoke")
	e.expect(e.do("POST", "/api/v1/sos", tablet, budi, map[string]any{}), 403, "revoked device")
	time.Sleep(200 * time.Millisecond) // audit writes are asynchronous
	if n := e.count(`SELECT COUNT(*) FROM audit_log WHERE user_id LIKE $1`, realm1+"/admin1%"); n == 0 {
		t.Fatal("no audit entries for admin")
	}
}

func mustLoc(t *testing.T, name string) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation(name)
	if err != nil {
		t.Fatal(err)
	}
	return loc
}
