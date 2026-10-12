//go:build integration

package integration

import (
	"strings"
	"testing"
	"time"
)

func TestDeviceTelemetryAndIncidentGPS(t *testing.T) {
	e := newEnv(t)
	admin := e.web(realm4, "tel-admin", "admin")
	f := e.newFieldSite(realm4, admin, "Telemetry Site")
	beacon := map[string]any{
		"app_version": "1.4.0", "app_build": "42", "platform": "android", "os_version": "14",
		"outbox_pending": 3, "outbox_failed": 1, "sos_pending": 0, "clock_offset_ms": -1200,
		"private_ca": true, "errors": []string{"sync: timeout"}, "sent_at": rfc(time.Now()),
	}
	e.expect(e.do("POST", "/api/v1/device/telemetry", f.tablet, "", beacon), 204, "telemetry")
	if r := e.do("POST", "/api/v1/device/telemetry", f.tablet, "", beacon); r.status != 429 {
		t.Fatalf("second beacon within a minute: %d %s", r.status, r.raw)
	}
	e.mr.FlushAll()
	beacon["errors"] = make([]string, 11)
	if r := e.do("POST", "/api/v1/device/telemetry", f.tablet, "", beacon); r.str("code") != "too_many_errors" {
		t.Fatalf("too many errors: %d %s", r.status, r.raw)
	}
	e.expect(e.do("POST", "/api/v1/device/telemetry", admin, "", beacon), 403, "web user")
	found := false
	for _, d := range e.expect(e.do("GET", "/api/v1/devices", admin, "", nil), 200, "devices").list() {
		m := d.(map[string]any)
		if hh, ok := m["health"].(map[string]any); ok && m["name"] == "Telemetry Site" {
			found = hh["outbox_pending"] == float64(3) && hh["app_version"] == "1.4.0" && m["health_at"] != nil && hh["received_at"] != nil
		}
	}
	if !found {
		t.Fatal("device health not exposed in GET /devices")
	}

	if r := e.do("POST", "/api/v1/incidents", f.tablet, f.guard, map[string]any{"type": "fire", "gps_lat": 95.0, "gps_lng": 106.8}); r.str("code") != "invalid_gps" {
		t.Fatalf("bad gps: %d %s", r.status, r.raw)
	}
	if r := e.do("POST", "/api/v1/incidents", f.tablet, f.guard, map[string]any{"type": "fire", "gps_lat": -6.2}); r.str("code") != "invalid_gps" {
		t.Fatalf("half gps: %d %s", r.status, r.raw)
	}
	inc := e.expect(e.do("POST", "/api/v1/incidents", f.tablet, f.guard, map[string]any{"type": "fire", "gps_lat": -6.2, "gps_lng": 106.8}), 201, "gps incident").obj()
	if inc["gps_lat"] != -6.2 || inc["gps_lng"] != 106.8 {
		t.Fatalf("incident gps = %v", inc)
	}
	det := e.expect(e.do("GET", "/api/v1/incidents/"+inc["id"].(string), admin, "", nil), 200, "detail").obj()
	if det["incident"].(map[string]any)["gps_lng"] != 106.8 {
		t.Fatalf("detail gps = %v", det)
	}
	_, _, csv := e.raw("GET", "/api/v1/reports/incidents?site_id="+f.site+"&format=csv", admin, nil)
	if !strings.Contains(string(csv), "-6.200000,106.800000") {
		t.Fatalf("csv lacks gps: %s", csv)
	}
}
