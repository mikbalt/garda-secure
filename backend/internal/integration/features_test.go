//go:build integration

package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/xuri/excelize/v2"

	"github.com/securepatrol/backend/internal/config"
	"github.com/securepatrol/backend/internal/kcadmin/kcfake"
	"github.com/securepatrol/backend/internal/service"
)

// Tests of the be-features work (audit F1, F3-F10, WEB-3, BE-S9). They use
// realm4 and assert on their own records only (the database is shared).

// webSites is a web token restricted to sites (claim "sites", F4).
func (e *env) webSites(realm, user string, sites []string, roles ...string) string {
	e.t.Helper()
	tok := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims{
		"iss": issuer + realm, "sub": user, "preferred_username": user, "azp": "securepatrol-web", "typ": "Bearer",
		"exp": time.Now().Add(time.Hour).Unix(), "roles": roles, "sites": sites,
	})
	tok.Header["kid"] = "k1"
	s, err := tok.SignedString(e.key)
	if err != nil {
		e.t.Fatal(err)
	}
	return s
}

// raw performs a request and returns status, headers and body.
func (e *env) raw(method, path, token string, body any) (int, http.Header, []byte) {
	e.t.Helper()
	res := e.doRaw(method, path, token, "", body)
	if res == nil {
		e.t.Fatalf("%s %s failed", method, path)
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(res.Body)
	return res.StatusCode, res.Header, b
}

func ids(l []any) []string {
	out := []string{}
	for _, v := range l {
		if m, ok := v.(map[string]any); ok {
			if id, ok := m["id"].(string); ok {
				out = append(out, id)
			}
		}
	}
	return out
}

func TestChannelRegistryAndIncidentFiled(t *testing.T) {
	e := newEnv(t)
	admin := e.web(realm4, "feat-admin", "admin")
	f := e.newFieldSite(realm4, admin, "Feat Notify")

	ch := e.expect(e.do("GET", "/api/v1/notifications/channels", admin, "", nil), 200, "channels").list()
	if len(ch) != 2 || ch[0].(map[string]any)["name"] != "email" {
		t.Fatalf("channels = %v", ch)
	}
	if r := e.do("POST", "/api/v1/notifications/rules", admin, "", map[string]any{
		"site_id": f.site, "event_type": "sos", "channel": "line", "recipient": "x",
	}); r.status != 400 || r.str("code") != "invalid_channel" {
		t.Fatalf("unknown channel: %d %s", r.status, r.raw)
	}
	types := e.expect(e.do("GET", "/api/v1/notifications/event-types", admin, "", nil), 200, "types").obj()
	if !slices.Contains(types["event_types"].([]any), any("incident_filed")) {
		t.Fatalf("event types = %v", types)
	}
	// The DB no longer restricts channel names (F10): validated in code only.
	if _, err := e.pool.Exec(context.Background(),
		`INSERT INTO notification_rules (site_id, event_type, channel, recipient) VALUES ($1, 'sos', 'telegram', 'x')`, f.site); err != nil {
		t.Fatalf("channel check still in the schema: %v", err)
	}
	_, _ = e.pool.Exec(context.Background(), `DELETE FROM notification_rules WHERE site_id = $1 AND channel = 'telegram'`, f.site)

	for _, rule := range []map[string]any{
		{"site_id": f.site, "event_type": "incident_filed", "channel": "email", "recipient": "desk@example.com"},
		{"site_id": f.site, "event_type": "incident_high", "channel": "whatsapp", "recipient": "08123456789"},
	} {
		e.expect(e.do("POST", "/api/v1/notifications/rules", admin, "", rule), 201, "rule")
	}
	logCount := func(ev string) int {
		return e.count(`SELECT count(*) FROM notification_log WHERE site_id = $1 AND event_type = $2`, f.site, ev)
	}
	e.expect(e.do("POST", "/api/v1/incidents", f.tablet, f.guard, map[string]any{"type": "trespass", "severity": "low"}), 201, "low incident")
	if logCount("incident_filed") != 1 || logCount("incident_high") != 0 {
		t.Fatalf("low incident: filed %d, high %d", logCount("incident_filed"), logCount("incident_high"))
	}
	id := uuid.NewString()
	e.expect(e.do("POST", "/api/v1/incidents", f.tablet, f.guard, map[string]any{"id": id, "type": "fire", "severity": "critical"}), 201, "critical")
	e.expect(e.do("POST", "/api/v1/incidents", f.tablet, f.guard, map[string]any{"id": id, "type": "fire", "severity": "critical"}), 200, "replay")
	if logCount("incident_filed") != 2 || logCount("incident_high") != 1 {
		t.Fatalf("critical incident: filed %d, high %d", logCount("incident_filed"), logCount("incident_high"))
	}
}

func TestSiteScopingAndActiveSOS(t *testing.T) {
	e := newEnv(t)
	admin := e.web(realm4, "scope-admin", "admin")
	a := e.newFieldSite(realm4, admin, "Scope A")
	b := e.newFieldSite(realm4, admin, "Scope B")
	sosA := e.expect(e.do("POST", "/api/v1/sos", a.tablet, a.guard, map[string]any{"gps_lat": -6.2, "gps_lng": 106.8}), 201, "sos A").str("id")
	sosB := e.expect(e.do("POST", "/api/v1/sos", b.tablet, b.guard, map[string]any{}), 201, "sos B").str("id")

	client := e.webSites(realm4, "scope-client", []string{a.site}, "client-readonly")
	sites := e.expect(e.do("GET", "/api/v1/sites", client, "", nil), 200, "sites").list()
	if got := ids(sites); !slices.Equal(got, []string{a.site}) {
		t.Fatalf("scoped sites = %v, want only %s", got, a.site)
	}
	me := e.expect(e.do("GET", "/api/v1/me", client, "", nil), 200, "me").obj()
	if me["sites_restricted"] != true || len(me["sites"].([]any)) != 1 {
		t.Fatalf("me = %v", me)
	}
	for _, p := range []string{
		"/api/v1/sites/" + b.site, "/api/v1/incidents?site_id=" + b.site, "/api/v1/dashboard/summary?site_id=" + b.site,
		"/api/v1/reports/incidents?site_id=" + b.site, "/api/v1/patrol/live?site_id=" + b.site,
	} {
		e.expect(e.do("GET", p, client, "", nil), 404, "other site "+p)
	}
	for _, p := range []string{"/api/v1/sites/" + a.site, "/api/v1/incidents?site_id=" + a.site, "/api/v1/dashboard/summary?site_id=" + a.site} {
		e.expect(e.do("GET", p, client, "", nil), 200, "own site "+p)
	}
	hr := e.webSites(realm4, "scope-hr", []string{a.site}, "hr")
	for _, g := range e.expect(e.do("GET", "/api/v1/guards", hr, "", nil), 200, "guards").list() {
		if g.(map[string]any)["site_id"] != a.site {
			t.Fatalf("scoped guard list leaks %v", g)
		}
	}
	if l := e.expect(e.do("GET", "/api/v1/sites", e.webSites(realm4, "scope-bad", []string{"nope"}, "client-readonly"), "", nil), 200, "bad claim").list(); len(l) != 0 {
		t.Fatalf("invalid sites claim must fail closed: %v", l)
	}

	// WEB-3: realm-wide active SOS, narrowed for scoped users.
	all := e.expect(e.do("GET", "/api/v1/sos/active", admin, "", nil), 200, "active sos").list()
	got := ids(all)
	if !slices.Contains(got, sosA) || !slices.Contains(got, sosB) {
		t.Fatalf("active sos = %v", got)
	}
	for _, s := range all {
		m := s.(map[string]any)
		if m["id"] == sosA && (m["site_name"] != "Scope A" || m["guard_name"] != "Scope A G1" || m["gps_lat"] == nil) {
			t.Fatalf("active sos row = %v", m)
		}
	}
	sup := e.webSites(realm4, "scope-sup", []string{a.site}, "supervisor")
	for _, s := range e.expect(e.do("GET", "/api/v1/sos/active", sup, "", nil), 200, "scoped sos").list() {
		if s.(map[string]any)["site_id"] != a.site {
			t.Fatalf("scoped active sos leaks %v", s)
		}
	}
	e.expect(e.do("GET", "/api/v1/sos/active", client, "", nil), 403, "client active sos")
	e.expect(e.do("POST", "/api/v1/sos/"+sosB+"/acknowledge", sup, "", nil), 404, "scoped ack other site")
	e.expect(e.do("POST", "/api/v1/sos/"+sosA+"/acknowledge", sup, "", nil), 200, "ack")
	if got := ids(e.do("GET", "/api/v1/sos/active", admin, "", nil).list()); slices.Contains(got, sosA) || !slices.Contains(got, sosB) {
		t.Fatalf("after ack = %v", got)
	}
}

func TestEffectiveIncidentsExportsAndPaging(t *testing.T) {
	e := newEnv(t)
	admin := e.web(realm4, "exp-admin", "admin")
	sup := e.web(realm4, "exp-sup", "supervisor")
	f := e.newFieldSite(realm4, admin, "Export Site")
	inc := e.expect(e.do("POST", "/api/v1/incidents", f.tablet, f.guard, map[string]any{
		"type": "theft", "severity": "low", "description": "=cmd|' /C calc'!A0", "location": "Gudang",
	}), 201, "incident").str("id")
	e.expect(e.do("POST", "/api/v1/incidents/"+inc+"/amendments", sup, "", map[string]any{
		"changes": map[string]string{"severity": "high"}, "reason": "CCTV shows forced entry",
	}), 201, "amend")

	list := func(q string) []any {
		return e.expect(e.do("GET", "/api/v1/incidents?site_id="+f.site+q, sup, "", nil), 200, "list "+q).list()
	}
	if got := ids(list("&severity=high")); !slices.Equal(got, []string{inc}) {
		t.Fatalf("effective severity filter = %v", got)
	}
	if got := ids(list("&severity=low")); len(got) != 0 {
		t.Fatalf("original severity still matches: %v", got)
	}
	row := list("")[0].(map[string]any)
	if row["severity"] != "high" || row["amended"] != true || row["original_severity"] != "low" || row["original_type"] != nil {
		t.Fatalf("effective row = %v", row)
	}
	dash := e.expect(e.do("GET", "/api/v1/dashboard/summary?site_id="+f.site, sup, "", nil), 200, "dash").obj()
	if bySev := dash["incidents_by_severity"].(map[string]any); bySev["high"] != float64(1) || bySev["low"] != nil {
		t.Fatalf("dashboard severity = %v", bySev)
	}

	// Paging headers.
	for i := 0; i < 3; i++ {
		e.expect(e.do("POST", "/api/v1/incidents", f.tablet, f.guard, map[string]any{"type": "noise", "severity": "low"}), 201, "more")
	}
	status, h, body := e.raw("GET", "/api/v1/incidents?site_id="+f.site+"&limit=2", sup, nil)
	if status != 200 || h.Get("X-Total-Count") != "4" || h.Get("X-Truncated") != "true" || strings.Count(string(body), `"id"`) != 2 {
		t.Fatalf("paging: %d %v %s", status, h, body)
	}
	_, h, _ = e.raw("GET", "/api/v1/incidents?site_id="+f.site+"&limit=2&offset=2", sup, nil)
	if h.Get("X-Truncated") != "" {
		t.Fatal("last page must not be truncated")
	}
	e.expect(e.do("GET", "/api/v1/incidents?site_id="+f.site+"&limit=5000", sup, "", nil), 400, "limit too big")
	e.expect(e.do("GET", "/api/v1/defects?site_id="+f.site+"&limit=1", sup, "", nil), 200, "defects paging")
	e.expect(e.do("GET", "/api/v1/visitors?site_id="+f.site+"&offset=1", sup, "", nil), 200, "visitors paging")

	// Exports (F5).
	status, _, body = e.raw("GET", "/api/v1/reports/incidents?site_id="+f.site+"&format=csv", sup, nil)
	if status != 200 || !strings.Contains(string(body), "Amended") || !strings.Contains(string(body), "severity: low") ||
		!strings.Contains(string(body), `'=cmd`) {
		t.Fatalf("csv: %d %s", status, body)
	}
	status, h, body = e.raw("GET", "/api/v1/reports/incidents?site_id="+f.site+"&format=xlsx", sup, nil)
	if status != 200 || !strings.Contains(h.Get("Content-Type"), "spreadsheetml") {
		t.Fatalf("xlsx: %d %v", status, h)
	}
	x, err := excelize.OpenReader(bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	rows, _ := x.GetRows("Report")
	if len(rows) != 5 || rows[0][0] != "Occurred" {
		t.Fatalf("xlsx rows = %v", rows)
	}
	_ = x.Close()
	for _, p := range []string{
		"/api/v1/reports/incidents?site_id=" + f.site + "&format=pdf",
		"/api/v1/incidents/" + inc + "/pdf",
	} {
		status, h, body = e.raw("GET", p, sup, nil)
		if status != 200 || h.Get("Content-Type") != "application/pdf" || !bytes.HasPrefix(body, []byte("%PDF-")) {
			t.Fatalf("%s: %d %v", p, status, h)
		}
	}
	for _, p := range []string{"/attendance/daily", "/attendance/monthly", "/patrol/sessions", "/guards/kpi"} {
		status, _, body = e.raw("GET", "/api/v1/reports"+p+"?site_id="+f.site+"&format=xlsx", admin, nil)
		if status != 200 || !bytes.HasPrefix(body, []byte("PK")) {
			t.Fatalf("%s xlsx: %d %s", p, status, body)
		}
	}
	if r := e.do("GET", "/api/v1/reports/guards/kpi?site_id="+f.site+"&format=pdf", admin, "", nil); r.status != 400 || r.str("code") != "invalid_format" {
		t.Fatalf("pdf only for the incident log: %d %s", r.status, r.raw)
	}
	if _, err := e.pool.Exec(context.Background(), `
		INSERT INTO incidents (site_id, guard_id, type, severity, occurred_at)
		SELECT $1, $2, 'bulk', 'low', now() - interval '1 minute' FROM generate_series(1, 2001)`, f.site, f.guard); err != nil {
		t.Fatal(err)
	}
	if r := e.do("GET", "/api/v1/reports/incidents?site_id="+f.site+"&format=pdf", sup, "", nil); r.status != 400 || r.str("code") != "export_too_large" {
		t.Fatalf("too large pdf: %d %s", r.status, r.raw)
	}
	_, h, _ = e.raw("GET", "/api/v1/incidents?site_id="+f.site, sup, nil)
	if h.Get("X-Truncated") != "true" || h.Get("X-Total-Count") != "2005" {
		t.Fatalf("default page must flag truncation: %v", h)
	}
}

func TestAttendanceOverrideHistory(t *testing.T) {
	e := newEnv(t)
	admin := e.web(realm4, "ovr-admin", "admin")
	f := e.newFieldSite(realm4, admin, "Override Site")
	now := time.Now().UTC()
	shift := e.expect(e.do("POST", "/api/v1/shifts", admin, "", map[string]any{
		"site_id": f.site, "guard_id": f.guard, "start_time": rfc(now.Add(-2 * time.Hour)), "end_time": rfc(now.Add(6 * time.Hour)),
	}), 201, "shift").str("id")
	in := now.Add(-2 * time.Hour).Format(time.RFC3339)
	rec := e.expect(e.do("PUT", "/api/v1/attendance/shifts/"+shift+"/override", admin, "", map[string]any{
		"status": "present", "reason": "Tablet broken", "clock_in_at": in,
	}), 200, "override 1").str("id")
	e.expect(e.do("PUT", "/api/v1/attendance/shifts/"+shift+"/override", admin, "", map[string]any{
		"status": "late", "reason": "CCTV shows 20 min late",
	}), 200, "override 2")

	for _, p := range []string{"/api/v1/attendance/shifts/" + shift + "/history", "/api/v1/attendance/" + rec + "/history"} {
		h := e.expect(e.do("GET", p, e.web(realm4, "ovr-hr", "hr"), "", nil), 200, "history").obj()
		o := h["overrides"].([]any)
		if len(o) != 2 {
			t.Fatalf("%s: %v", p, h)
		}
		first, second := o[0].(map[string]any), o[1].(map[string]any)
		if first["before_status"] != nil || first["after_status"] != "present" || first["reason"] != "Tablet broken" ||
			second["before_status"] != "present" || second["after_status"] != "late" || second["before_reason"] != "Tablet broken" ||
			second["actor"] != realm4+"/ovr-admin" || second["after_clock_in_at"] == nil {
			t.Fatalf("history = %v", o)
		}
	}
	e.expect(e.do("GET", "/api/v1/attendance/"+rec+"/history", e.web(realm4, "ovr-client", "client-readonly"), "", nil), 403, "client")
	e.expect(e.do("GET", "/api/v1/attendance/"+rec+"/history", e.web(realm1, "ovr-other", "admin"), "", nil), 404, "other realm")
}

// uploadFloorplan posts a multipart floor plan image.
func (e *env) uploadFloorplan(token, site, name string, data []byte) resp {
	e.t.Helper()
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	_ = mw.WriteField("name", name)
	fw, _ := mw.CreateFormFile("file", "plan.jpg")
	_, _ = fw.Write(data)
	_ = mw.Close()
	req := httptest.NewRequest("POST", "/api/v1/sites/"+site+"/floorplans", &body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+token)
	res, err := e.app.Test(req, 30_000)
	if err != nil {
		e.t.Fatal(err)
	}
	raw, _ := io.ReadAll(res.Body)
	r := resp{status: res.StatusCode, raw: raw}
	_ = json.Unmarshal(raw, &r.body)
	return r
}

func TestFloorplansLiveAndRouteSnapshots(t *testing.T) {
	s3 := &fakeS3{objs: map[string]fakeObj{}}
	srv := httptest.NewServer(s3)
	defer srv.Close()
	e := newEnv(t, func(c *config.Config) { c.MinioEndpoint = strings.TrimPrefix(srv.URL, "http://") })
	admin := e.web(realm4, "plan-admin", "admin")
	client := e.web(realm4, "plan-client", "client-readonly")
	f := e.newFieldSite(realm4, admin, "Plan Site")

	e.expect(e.uploadFloorplan(client, f.site, "Ground", jpegWithGPS(t, 5)), 403, "client upload")
	if r := e.uploadFloorplan(admin, f.site, "Ground", []byte("not an image")); r.status != 400 {
		t.Fatalf("garbage upload: %d %s", r.status, r.raw)
	}
	fp := e.expect(e.uploadFloorplan(admin, f.site, "Ground floor", jpegWithGPS(t, 5)), 201, "upload").obj()
	fpID, key := fp["id"].(string), fp["object_key"].(string)
	if fp["width"] != float64(8) || fp["height"] != float64(8) || !strings.HasPrefix(key, "sites/"+f.site+"/floorplan/") {
		t.Fatalf("floor plan = %v", fp)
	}
	if bytes.Contains(s3.objs["/photos/"+key].body, []byte("GPS-")) {
		t.Fatal("floor plan metadata not stripped")
	}
	if l := e.expect(e.do("GET", "/api/v1/sites/"+f.site+"/floorplans", client, "", nil), 200, "list").list(); len(l) != 1 || l[0].(map[string]any)["url"] == "" {
		t.Fatalf("floor plans = %v", l)
	}
	e.expect(e.do("GET", "/api/v1/media/url?key="+key, client, "", nil), 200, "client reads floor plan")

	cps := func(fid string, x float64) []map[string]any {
		return []map[string]any{
			{"name": "Gate", "qr_code": "PLAN-1", "order_index": 0, "sla_minutes": 30, "floorplan_id": fid, "plan_x": x, "plan_y": 0.5},
			{"name": "Roof", "qr_code": "PLAN-2", "order_index": 1},
		}
	}
	if r := e.do("POST", "/api/v1/patrol/routes", admin, "", map[string]any{"site_id": f.site, "name": "P", "checkpoints": cps(uuid.NewString(), 0.5)}); r.str("code") != "unknown_floorplan" {
		t.Fatalf("unknown floor plan: %d %s", r.status, r.raw)
	}
	if r := e.do("POST", "/api/v1/patrol/routes", admin, "", map[string]any{"site_id": f.site, "name": "P", "checkpoints": cps(fpID, 1.5)}); r.str("code") != "invalid_plan_position" {
		t.Fatalf("bad position: %d %s", r.status, r.raw)
	}
	route := e.expect(e.do("POST", "/api/v1/patrol/routes", admin, "", map[string]any{"site_id": f.site, "name": "Plan route", "checkpoints": cps(fpID, 0.25)}), 201, "route").obj()
	routeID := route["id"].(string)
	if r := e.do("DELETE", "/api/v1/sites/"+f.site+"/floorplans/"+fpID, admin, "", nil); r.status != 409 || r.str("code") != "floorplan_in_use" {
		t.Fatalf("delete used plan: %d %s", r.status, r.raw)
	}

	sess := e.expect(e.do("POST", "/api/v1/patrol/sessions", f.tablet, f.guard, map[string]any{"route_id": routeID}), 201, "session").str("id")
	e.expect(e.do("POST", "/api/v1/patrol/sessions/"+sess+"/scans", f.tablet, f.guard, map[string]any{"qr_code": "PLAN-1"}), 201, "scan")
	// Editing the route does not change the running/past session (F9).
	e.expect(e.do("PUT", "/api/v1/patrol/routes/"+routeID, admin, "", map[string]any{"name": "Plan route", "checkpoints": []map[string]any{
		{"name": "Gate", "qr_code": "PLAN-1", "order_index": 0}, {"name": "Lobby", "order_index": 1}, {"name": "Car park", "order_index": 2},
	}}), 200, "edit route")
	prog := e.expect(e.do("GET", "/api/v1/patrol/sessions/"+sess, admin, "", nil), 200, "progress").obj()
	if prog["total"] != float64(2) || prog["checkpoints"].([]any)[1].(map[string]any)["name"] != "Roof" {
		t.Fatalf("session progress must use the snapshot: %v", prog)
	}
	live := e.expect(e.do("GET", "/api/v1/patrol/live?site_id="+f.site, client, "", nil), 200, "live").list()
	if len(live) != 1 {
		t.Fatalf("live = %v", live)
	}
	l0 := live[0].(map[string]any)
	cp0 := l0["progress"].(map[string]any)["checkpoints"].([]any)[0].(map[string]any)
	if l0["last_scan_at"] == nil || l0["guard_name"] != "Plan Site G1" || cp0["plan_x"] != 0.25 || cp0["floorplan_id"] != fpID || cp0["status"] != "on_time" {
		t.Fatalf("live row = %v", l0)
	}
	rep := e.expect(e.do("GET", "/api/v1/reports/patrol/sessions?site_id="+f.site, admin, "", nil), 200, "report").obj()
	if r0 := rep["rows"].([]any)[0].(map[string]any); r0["total_checkpoints"] != float64(2) || r0["scanned_count"] != float64(1) {
		t.Fatalf("summary must use the snapshot: %v", r0)
	}

	spare := e.expect(e.uploadFloorplan(admin, f.site, "Spare", jpegWithGPS(t, 9)), 201, "upload 2").obj()
	e.expect(e.do("DELETE", "/api/v1/sites/"+f.site+"/floorplans/"+spare["id"].(string), admin, "", nil), 204, "delete unused")
	if _, ok := s3.objs["/photos/"+spare["object_key"].(string)]; ok {
		t.Fatal("deleted floor plan image still stored")
	}
	e.expect(e.do("DELETE", "/api/v1/sites/"+f.site+"/floorplans/"+fpID, e.web(realm1, "plan-other", "admin"), "", nil), 404, "other realm")
}

func TestRetentionAndVisitorPII(t *testing.T) {
	s3 := &fakeS3{objs: map[string]fakeObj{}}
	srv := httptest.NewServer(s3)
	defer srv.Close()
	e := newEnv(t, func(c *config.Config) {
		c.MinioEndpoint = strings.TrimPrefix(srv.URL, "http://")
		c.Retention = config.Retention{VisitorDays: 365, SelfieDays: 365, GPSDays: 365, NotificationDays: 180}
	})
	ctx := context.Background()
	admin := e.web(realm4, "ret-admin", "admin")
	sup := e.web(realm4, "ret-sup", "supervisor")
	f := e.newFieldSite(realm4, admin, "Retention Site")

	v := e.expect(e.do("POST", "/api/v1/visitors", f.tablet, f.guard, map[string]any{
		"visitor_name": "Andi", "id_type": "KTP", "id_number": "3201234567890001",
	}), 201, "visitor").obj()
	if v["id_number"] != "3201********0001" {
		t.Fatalf("tablet response must be masked: %v", v["id_number"])
	}
	vid := v["id"].(string)
	var stored string
	_ = e.pool.QueryRow(ctx, `SELECT id_number FROM visitor_logs WHERE id = $1`, vid).Scan(&stored)
	if !strings.HasPrefix(stored, "pii1:") || strings.Contains(stored, "3201") {
		t.Fatalf("id_number stored in plaintext: %q", stored)
	}
	for _, r := range e.expect(e.do("GET", "/api/v1/visitors?site_id="+f.site, sup, "", nil), 200, "visitors").list() {
		if m := r.(map[string]any); m["id"] == vid && m["id_number"] != "3201********0001" {
			t.Fatalf("list must be masked: %v", m)
		}
	}
	det := e.expect(e.do("GET", "/api/v1/visitors/"+vid, sup, "", nil), 200, "detail").obj()
	if det["visitor"].(map[string]any)["id_number"] != "3201234567890001" {
		t.Fatalf("detail = %v", det)
	}
	e.expect(e.do("GET", "/api/v1/visitors/"+vid, e.web(realm4, "ret-hr", "hr"), "", nil), 403, "hr detail")

	// Legacy plaintext gets encrypted by the job.
	legacy := e.expect(e.do("POST", "/api/v1/visitors", f.tablet, f.guard, map[string]any{"visitor_name": "Lama"}), 201, "legacy").str("id")
	_, _ = e.pool.Exec(ctx, `UPDATE visitor_logs SET id_number = '1234567890' WHERE id = $1`, legacy)

	// Expired data.
	photoKey := service.ObjectKey(f.site, "visitor", uuid.NewString(), "jpg")
	selfieKey := service.ObjectKey(f.site, "selfie", uuid.NewString(), "jpg")
	s3.objs["/photos/"+photoKey] = fakeObj{size: 3, body: []byte("abc")}
	s3.objs["/photos/"+selfieKey] = fakeObj{size: 3, body: []byte("abc")}
	var oldVisitor, oldAtt, oldLog string
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(e.pool.QueryRow(ctx, `INSERT INTO visitor_logs (site_id, guard_id, visitor_name, photo_object_key, created_at)
		VALUES ($1, $2, 'Old visitor', $3, now() - interval '400 days') RETURNING id`, f.site, f.guard, photoKey).Scan(&oldVisitor))
	must(e.pool.QueryRow(ctx, `INSERT INTO attendance_records (guard_id, site_id, status, selfie_object_key, gps_lat, gps_lng, created_at)
		VALUES ($1, $2, 'present', $3, -6.2, 106.8, now() - interval '400 days') RETURNING id`, f.guard, f.site, selfieKey).Scan(&oldAtt))
	must(e.pool.QueryRow(ctx, `INSERT INTO notification_log (site_id, event_type, channel, recipient, subject, body, status, dedupe_key, created_at)
		VALUES ($1, 'sos', 'email', 'boss@example.com', '[SOS] Budi', 'secret body', 'sent', $2, now() - interval '200 days') RETURNING id`,
		f.site, "ret:"+uuid.NewString()).Scan(&oldLog))

	if err := e.sched.Retention(ctx); err != nil {
		t.Fatalf("retention: %v", err)
	}
	if e.count(`SELECT count(*) FROM visitor_logs WHERE id = $1`, oldVisitor) != 0 {
		t.Fatal("expired visitor log kept")
	}
	if e.count(`SELECT count(*) FROM visitor_logs WHERE id = $1`, vid) != 1 {
		t.Fatal("recent visitor log deleted")
	}
	if _, ok := s3.objs["/photos/"+photoKey]; ok {
		t.Fatal("expired visitor photo kept")
	}
	if _, ok := s3.objs["/photos/"+selfieKey]; ok {
		t.Fatal("expired selfie kept")
	}
	if e.count(`SELECT count(*) FROM attendance_records WHERE id = $1 AND selfie_object_key IS NULL AND gps_lat IS NULL AND gps_lng IS NULL`, oldAtt) != 1 {
		t.Fatal("selfie key / GPS not cleared")
	}
	if e.count(`SELECT count(*) FROM notification_log WHERE id = $1 AND body = '' AND recipient = '[purged]' AND dedupe_key IS NULL AND subject = 'sos'`, oldLog) != 1 {
		t.Fatal("notification log not purged")
	}
	_ = e.pool.QueryRow(ctx, `SELECT id_number FROM visitor_logs WHERE id = $1`, legacy).Scan(&stored)
	if !strings.HasPrefix(stored, "pii1:") {
		t.Fatalf("legacy id number not encrypted: %q", stored)
	}
	if det := e.expect(e.do("GET", "/api/v1/visitors/"+legacy, admin, "", nil), 200, "legacy detail").obj(); det["visitor"].(map[string]any)["id_number"] != "1234567890" {
		t.Fatalf("legacy detail = %v", det)
	}
	if err := e.sched.Retention(ctx); err != nil { // idempotent
		t.Fatalf("second run: %v", err)
	}
}

func TestUserManagementViaKeycloak(t *testing.T) {
	kc := kcfake.New("securepatrol-admin-api", map[string]string{realm4: "kc-secret"})
	defer kc.Close()
	e := newEnv(t, func(c *config.Config) {
		c.KeycloakAdmin = config.KeycloakAdmin{InternalURL: kc.URL, ClientID: "securepatrol-admin-api",
			Secrets: map[string]string{realm4: "kc-secret"}, Timeout: 5 * time.Second}
		c.PlatformAdminRealms = nil // platform admins of any realm (realm4 here)
	})
	setupAdmin := e.web(realm4, "um-setup", "admin")
	a := e.newFieldSite(realm4, setupAdmin, "Users A")
	b := e.newFieldSite(realm4, setupAdmin, "Users B")
	other := e.newFieldSite(realm1, e.web(realm1, "um-r1", "admin"), "Users R1")

	me := kc.AddUser(realm4, kcfake.User{Username: "um-admin", Enabled: true, Roles: []string{"admin"}, Password: "x"})
	admin := e.token(realm4, me.ID, "securepatrol-web", "admin")

	// Create with a generated temporary password.
	r := e.expect(e.do("POST", "/api/v1/users", admin, "", map[string]any{
		"username": "Siti", "email": "siti@example.com", "first_name": "Siti", "roles": []string{"supervisor"}, "sites": []string{a.site},
	}), 201, "create user").obj()
	pw, _ := r["temporary_password"].(string)
	user := r["user"].(map[string]any)
	sitiID := user["id"].(string)
	if len(pw) < 12 || user["username"] != "siti" || user["roles"].([]any)[0] != "supervisor" || user["sites"].([]any)[0] != a.site {
		t.Fatalf("created = %v", r)
	}
	if s := kc.Get(realm4, sitiID); !s.TempPass || s.Password != pw || !slices.Contains(s.RequiredActions, "CONFIGURE_TOTP") ||
		!slices.Contains(s.RequiredActions, "UPDATE_PASSWORD") || s.Attributes["sites"][0] != a.site {
		t.Fatalf("keycloak user = %+v", s)
	}
	if n := e.count(`SELECT count(*) FROM audit_log WHERE route = '/api/v1/users' AND method = 'POST' AND realm = $1
		AND after_state->>'username' = 'siti' AND position($2 in coalesce(after_state::text, '') || coalesce(summary::text, '')) = 0`, realm4, pw); n != 1 {
		t.Fatalf("audit of user creation (without password) = %d rows", n)
	}

	for _, c := range []struct {
		body map[string]any
		code string
	}{
		{map[string]any{"username": "siti", "roles": []string{"hr"}}, "username_taken"},
		{map[string]any{"username": "other", "email": "SITI@example.com", "roles": []string{"hr"}}, "email_taken"},
		{map[string]any{"username": "plat", "roles": []string{"platform-admin"}}, "role_not_allowed"},
		{map[string]any{"username": "dev", "roles": []string{"device"}}, "role_not_allowed"},
		{map[string]any{"username": "device-x", "roles": []string{"hr"}}, "invalid_username"},
		{map[string]any{"username": "foreign", "roles": []string{"hr"}, "sites": []string{other.site}}, "invalid_site"},
		{map[string]any{"username": "noroles"}, "roles_required"},
		{map[string]any{"username": "invite", "roles": []string{"hr"}, "send_invite": true}, "email_required"},
		{map[string]any{"username": "weakpw", "roles": []string{"hr"}, "temporary_password": "short"}, "password_policy"},
	} {
		if r := e.do("POST", "/api/v1/users", admin, "", c.body); r.str("code") != c.code {
			t.Errorf("%v: %d %s, want %s", c.body, r.status, r.raw, c.code)
		}
	}
	if kc.ByUsername(realm4, "weakpw") != nil {
		t.Fatal("half-created user not rolled back")
	}
	inv := e.expect(e.do("POST", "/api/v1/users", admin, "", map[string]any{
		"username": "budi", "email": "budi@example.com", "roles": []string{"hr"}, "send_invite": true,
	}), 201, "invite").obj()
	if inv["invite_sent"] != true || inv["temporary_password"] != nil || len(kc.ByUsername(realm4, "budi").EmailedActs) == 0 {
		t.Fatalf("invite = %v", inv)
	}

	// List + filters.
	status, h, body := e.raw("GET", "/api/v1/users?role=supervisor", admin, nil)
	if status != 200 || !strings.Contains(string(body), sitiID) || strings.Contains(string(body), "budi") || h.Get("X-Total-Count") != "1" {
		t.Fatalf("list by role: %d %v %s", status, h, body)
	}
	if l := e.expect(e.do("GET", "/api/v1/users?search=BUD&enabled=true", admin, "", nil), 200, "search").list(); len(l) != 1 {
		t.Fatalf("search = %v", l)
	}

	// Update, self-protection, platform-admin rule.
	upd := e.expect(e.do("PUT", "/api/v1/users/"+sitiID, admin, "", map[string]any{"roles": []string{"hr"}, "enabled": false, "last_name": "Rahma"}), 200, "update").obj()
	if s := kc.Get(realm4, sitiID); s.Enabled || !slices.Equal(s.Roles, []string{"hr"}) || s.LoggedOut != 1 || s.LastName != "Rahma" || upd["enabled"] != false {
		t.Fatalf("after update: %+v / %v", s, upd)
	}
	if r := e.do("PUT", "/api/v1/users/"+me.ID, admin, "", map[string]any{"roles": []string{"hr"}}); r.str("code") != "cannot_modify_self" {
		t.Fatalf("self demotion: %d %s", r.status, r.raw)
	}
	if r := e.do("PUT", "/api/v1/users/"+me.ID, admin, "", map[string]any{"enabled": false}); r.str("code") != "cannot_modify_self" {
		t.Fatalf("self disable: %d %s", r.status, r.raw)
	}
	op := kc.AddUser(realm4, kcfake.User{Username: "operator", Enabled: true, Roles: []string{"admin", "platform-admin"}})
	if r := e.do("PUT", "/api/v1/users/"+op.ID, admin, "", map[string]any{"enabled": false}); r.str("code") != "role_not_allowed" {
		t.Fatalf("realm admin edits platform admin: %d %s", r.status, r.raw)
	}
	platform := e.token(realm4, op.ID, "securepatrol-web", "admin", "platform-admin")
	e.expect(e.do("PUT", "/api/v1/users/"+sitiID, platform, "", map[string]any{"roles": []string{"hr", "platform-admin"}}), 200, "platform grants")

	// Password / OTP / sessions.
	if r := e.expect(e.do("POST", "/api/v1/users/"+sitiID+"/reset-password", platform, "", map[string]any{}), 200, "reset pw").obj(); r["temporary_password"] == "" {
		t.Fatalf("reset = %v", r)
	}
	e.expect(e.do("POST", "/api/v1/users/"+sitiID+"/reset-password", platform, "", map[string]any{"send_email": true}), 200, "reset by mail")
	if r := e.do("POST", "/api/v1/users/"+me.ID+"/reset-password", admin, "", nil); r.str("code") != "cannot_modify_self" {
		t.Fatalf("self reset: %d %s", r.status, r.raw)
	}
	totp := kc.AddUser(realm4, kcfake.User{Username: "lost-phone", Enabled: true, Totp: true, Roles: []string{"hr"}})
	e.expect(e.do("POST", "/api/v1/users/"+totp.ID+"/reset-otp", admin, "", nil), 200, "reset otp")
	if s := kc.Get(realm4, totp.ID); s.Totp || s.LoggedOut != 1 || !slices.Contains(s.RequiredActions, "CONFIGURE_TOTP") {
		t.Fatalf("after otp reset: %+v", s)
	}
	e.expect(e.do("POST", "/api/v1/users/"+totp.ID+"/logout", admin, "", nil), 200, "logout")

	// Site-scoped admin.
	scoped := kc.AddUser(realm4, kcfake.User{Username: "scoped-admin", Enabled: true, Roles: []string{"admin"},
		Attributes: map[string][]string{"sites": {a.site}}})
	scopedTok := e.webSites(realm4, scoped.ID, []string{a.site}, "admin")
	if l := e.expect(e.do("GET", "/api/v1/users", scopedTok, "", nil), 200, "scoped list").list(); len(l) != 2 { // siti + itself
		t.Fatalf("scoped admin sees %v", ids(l))
	}
	if r := e.do("POST", "/api/v1/users", scopedTok, "", map[string]any{"username": "wide", "roles": []string{"hr"}}); r.str("code") != "sites_required" {
		t.Fatalf("scoped admin creates unscoped user: %d %s", r.status, r.raw)
	}
	if r := e.do("POST", "/api/v1/users", scopedTok, "", map[string]any{"username": "wide", "roles": []string{"hr"}, "sites": []string{b.site}}); r.str("code") != "invalid_site" {
		t.Fatalf("scoped admin grants other site: %d %s", r.status, r.raw)
	}
	e.expect(e.do("GET", "/api/v1/users/"+totp.ID, scopedTok, "", nil), 404, "scoped admin reads unscoped user")

	// Device accounts.
	d := e.expect(e.do("POST", "/api/v1/device-accounts", admin, "", map[string]any{"name": "Pos Utara"}), 201, "device account").obj()
	acc := d["account"].(map[string]any)
	accID := acc["id"].(string)
	if acc["username"] != "device-pos-utara" || len(d["password"].(string)) != 24 {
		t.Fatalf("device account = %v", d)
	}
	if s := kc.Get(realm4, accID); !slices.Equal(s.Roles, []string{"device"}) || s.TempPass || len(s.RequiredActions) != 0 || s.Email == "" {
		t.Fatalf("keycloak device user = %+v", s)
	}
	e.expect(e.do("POST", "/api/v1/device-accounts", admin, "", map[string]any{"name": "pos utara"}), 409, "duplicate device")
	tablet := e.token(realm4, accID, "securepatrol-tablet", "device")
	e.expect(e.do("POST", "/api/v1/device/register", tablet, "", map[string]any{"name": "Tab Utara"}), 200, "register")
	l := e.expect(e.do("GET", "/api/v1/device-accounts", admin, "", nil), 200, "device accounts").list()
	if len(l) != 1 || l[0].(map[string]any)["device"].(map[string]any)["name"] != "Tab Utara" {
		t.Fatalf("device accounts = %v", l)
	}
	if strings.Contains(string(e.do("GET", "/api/v1/users", admin, "", nil).raw), accID) {
		t.Fatal("device account listed as a user")
	}
	e.expect(e.do("GET", "/api/v1/users/"+accID, admin, "", nil), 404, "device via /users")
	e.expect(e.do("POST", "/api/v1/device-accounts/"+sitiID+"/reset-password", admin, "", nil), 404, "person via /device-accounts")
	rp := e.expect(e.do("POST", "/api/v1/device-accounts/"+accID+"/reset-password", admin, "", nil), 200, "device reset").obj()
	if rp["password"] == d["password"] || kc.Get(realm4, accID).Password != rp["password"] {
		t.Fatalf("device reset = %v", rp)
	}
	e.expect(e.do("PUT", "/api/v1/device-accounts/"+accID, admin, "", map[string]any{"enabled": false}), 200, "disable device")
	if kc.Get(realm4, accID).Enabled {
		t.Fatal("device account still enabled")
	}

	// Roles and availability.
	e.expect(e.do("GET", "/api/v1/users", e.web(realm4, "um-sup", "supervisor"), "", nil), 403, "supervisor")
	if r := e.do("GET", "/api/v1/users", e.web(realm3, "um-r3", "admin"), "", nil); r.status != 503 || r.str("code") != "kc_unavailable" {
		t.Fatalf("realm without secret: %d %s", r.status, r.raw)
	}
	kc.Down = true
	if r := e.do("GET", "/api/v1/users", admin, "", nil); r.status != 503 || r.str("code") != "kc_unavailable" {
		t.Fatalf("keycloak down: %d %s", r.status, r.raw)
	}
}
