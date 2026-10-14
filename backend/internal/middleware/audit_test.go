package middleware

import (
	"strings"
	"testing"
)

func TestRedactJSON(t *testing.T) {
	got := string(RedactJSON([]byte(`{"smtp":{"host":"h","password":"p1"},"whatsapp":{"token":"t1"},"pin":"4826","name":"Budi","list":[{"api_key":"k"}]}`), 4096))
	for _, secret := range []string{"p1", "t1", "4826", `"k"`} {
		if strings.Contains(got, secret) {
			t.Errorf("secret %s kept: %s", secret, got)
		}
	}
	if !strings.Contains(got, "Budi") || !strings.Contains(got, `"host":"h"`) {
		t.Errorf("non-secret values lost: %s", got)
	}
	big := string(RedactJSON([]byte(`{"description":"`+strings.Repeat("x", 5000)+`"}`), 4096))
	if !strings.Contains(big, "_truncated_bytes") || len(big) > 200 {
		t.Errorf("oversized body not reduced: %.200s", big)
	}
	if nj := string(RedactJSON([]byte("pin=1234"), 4096)); strings.Contains(nj, "1234") {
		t.Errorf("non-JSON body kept: %s", nj)
	}
	sub := string(RedactJSON([]byte(`{"kind":"webpush","endpoint":"https://fcm.googleapis.com/x","keys":{"p256dh":"P","auth":"A"},"label":"Chrome","author":"Sari"}`), 4096))
	for _, secret := range []string{"fcm.googleapis.com", `"P"`, `"A"`} {
		if strings.Contains(sub, secret) {
			t.Errorf("push secret %s kept: %s", secret, sub)
		}
	}
	if !strings.Contains(sub, "Chrome") || !strings.Contains(sub, "Sari") {
		t.Errorf("push label or unrelated auth* key lost: %s", sub)
	}
}

func TestResourceType(t *testing.T) {
	for route, want := range map[string]string{
		"/api/v1/patrol/sessions/:id/scans":           "patrol.sessions",
		"/api/v1/guards/:id/pin":                      "guards",
		"/api/v1/attendance/shifts/:shiftId/override": "attendance.shifts",
		"/api/v1/sites":                               "sites",
		"/api/v1/settings/channels/test":              "settings.channels",
	} {
		if got := resourceType(route); got != want {
			t.Errorf("resourceType(%s) = %q, want %q", route, got, want)
		}
	}
}
