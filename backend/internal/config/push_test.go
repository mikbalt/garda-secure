package config

import (
	"crypto/ecdh"
	"crypto/rand"
	"encoding/base64"
	"strings"
	"testing"
	"time"
)

func vapidPair(t *testing.T) (string, string) {
	t.Helper()
	k, err := ecdh.P256().GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return base64.RawURLEncoding.EncodeToString(k.PublicKey().Bytes()), base64.RawURLEncoding.EncodeToString(k.Bytes())
}

func TestValidateVAPIDKeys(t *testing.T) {
	pub, priv := vapidPair(t)
	if err := ValidateVAPIDKeys(pub, priv); err != nil {
		t.Fatalf("valid pair: %v", err)
	}
	// Padded base64url is accepted too.
	padded := base64.URLEncoding.EncodeToString(mustDecode(t, pub))
	if err := ValidateVAPIDKeys(padded, priv); err != nil {
		t.Fatalf("padded public key: %v", err)
	}
	otherPub, otherPriv := vapidPair(t)
	for name, c := range map[string][2]string{
		"mismatched pair":    {otherPub, priv},
		"missing private":    {pub, ""},
		"missing public":     {"", priv},
		"public not base64":  {"!!!", priv},
		"private not base64": {pub, "***"},
		"short public":       {base64.RawURLEncoding.EncodeToString([]byte{4, 1, 2}), priv},
		"compressed public":  {base64.RawURLEncoding.EncodeToString(append([]byte{2}, make([]byte, 32)...)), priv},
		"short private":      {pub, base64.RawURLEncoding.EncodeToString(make([]byte, 16))},
		"public as private":  {otherPub, otherPub},
		"not on curve":       {base64.RawURLEncoding.EncodeToString(append([]byte{4}, make([]byte, 64)...)), otherPriv},
	} {
		if err := ValidateVAPIDKeys(c[0], c[1]); err == nil {
			t.Errorf("%s accepted", name)
		}
	}
}

func mustDecode(t *testing.T, s string) []byte {
	t.Helper()
	b, err := DecodeBase64URL(s)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestValidateVAPIDSubject(t *testing.T) {
	for _, ok := range []string{"mailto:ops@garda.example", "https://garda.example/contact"} {
		if err := ValidateVAPIDSubject(ok); err != nil {
			t.Errorf("%q rejected: %v", ok, err)
		}
	}
	for _, bad := range []string{"", "ops@garda.example", "mailto:", "mailto:no-at", "http://garda.example", "mailto:a@b <x>", "https://"} {
		if err := ValidateVAPIDSubject(bad); err == nil {
			t.Errorf("%q accepted", bad)
		}
	}
}

func TestLoadPush(t *testing.T) {
	c := &Config{}
	if err := c.loadPush(); err != nil {
		t.Fatal(err)
	}
	if c.Push.WebPushEnabled() || c.Push.SOSEscalateAfter != 3*time.Minute || c.Push.SOSEscalateEvery != 5*time.Minute ||
		c.Push.AudienceMaxAge != 30*24*time.Hour {
		t.Fatalf("defaults = %+v", c.Push)
	}

	pub, priv := vapidPair(t)
	t.Setenv("WEBPUSH_VAPID_PUBLIC_KEY", pub)
	t.Setenv("WEBPUSH_VAPID_PRIVATE_KEY", priv)
	if err := (&Config{}).loadPush(); err == nil || !strings.Contains(err.Error(), "WEBPUSH_SUBJECT") {
		t.Fatalf("keys without subject: %v", err)
	}
	t.Setenv("WEBPUSH_SUBJECT", "mailto:ops@garda.example")
	t.Setenv("WEBPUSH_ALLOWED_HOSTS", "push.example.net, .Web.Push.Example")
	c = &Config{}
	if err := c.loadPush(); err != nil {
		t.Fatal(err)
	}
	if !c.Push.WebPushEnabled() || strings.Join(c.Push.AllowedHosts, ",") != "push.example.net,web.push.example" {
		t.Fatalf("push = %+v", c.Push)
	}

	t.Setenv("WEBPUSH_ALLOWED_HOSTS", "bad_host")
	if err := (&Config{}).loadPush(); err == nil {
		t.Fatal("invalid allowed host accepted")
	}
	t.Setenv("WEBPUSH_ALLOWED_HOSTS", "")
	t.Setenv("WEBPUSH_VAPID_PRIVATE_KEY", "")
	if err := (&Config{}).loadPush(); err == nil || !strings.Contains(err.Error(), "WEBPUSH_VAPID") {
		t.Fatalf("half a key pair: %v", err)
	}
	t.Setenv("WEBPUSH_VAPID_PUBLIC_KEY", "")
	t.Setenv("SOS_ESCALATE_AFTER", "30s")
	if err := (&Config{}).loadPush(); err == nil {
		t.Fatal("escalation below 1m accepted")
	}
	t.Setenv("SOS_ESCALATE_AFTER", "2m")
	t.Setenv("SOS_ESCALATE_EVERY", "2h")
	if err := (&Config{}).loadPush(); err == nil {
		t.Fatal("escalation interval above 1h accepted")
	}
	t.Setenv("SOS_ESCALATE_EVERY", "10m")
	t.Setenv("PUSH_AUDIENCE_MAX_AGE", "10m")
	if err := (&Config{}).loadPush(); err == nil {
		t.Fatal("audience max age below 1h accepted")
	}
	t.Setenv("PUSH_AUDIENCE_MAX_AGE", "168h")
	c = &Config{}
	if err := c.loadPush(); err != nil || c.Push.SOSEscalateAfter != 2*time.Minute || c.Push.SOSEscalateEvery != 10*time.Minute {
		t.Fatalf("custom escalation: %+v %v", c.Push, err)
	}
}
