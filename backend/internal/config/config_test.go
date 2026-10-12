package config

import (
	"strings"
	"testing"
	"time"
)

func TestLoadFieldDefaultsAndValidation(t *testing.T) {
	c := &Config{}
	if err := c.loadField(); err != nil {
		t.Fatal(err)
	}
	if c.GuardSessionTTL != 12*time.Hour || c.GuardTokenGrace != 7*24*time.Hour || c.GuardTokenSecret != "" || c.QRSigningSecret != "" {
		t.Fatalf("defaults = %+v", c)
	}

	t.Setenv("GUARD_TOKEN_SECRET", "too-short")
	if err := (&Config{}).loadField(); err == nil || !strings.Contains(err.Error(), "GUARD_TOKEN_SECRET") {
		t.Fatalf("short guard secret: %v", err)
	}
	t.Setenv("GUARD_TOKEN_SECRET", strings.Repeat("g", 32))
	t.Setenv("QR_SIGNING_SECRET", "short")
	if err := (&Config{}).loadField(); err == nil || !strings.Contains(err.Error(), "QR_SIGNING_SECRET") {
		t.Fatalf("short QR secret: %v", err)
	}
	t.Setenv("QR_SIGNING_SECRET", strings.Repeat("q", 40))
	t.Setenv("GUARD_SESSION_TTL", "2m")
	if err := (&Config{}).loadField(); err == nil {
		t.Fatal("TTL below 5m accepted")
	}
	t.Setenv("GUARD_SESSION_TTL", "8h")
	t.Setenv("GUARD_TOKEN_GRACE", "0s")
	if err := (&Config{}).loadField(); err == nil {
		t.Fatal("zero grace accepted")
	}
	t.Setenv("GUARD_TOKEN_GRACE", "72h")
	c = &Config{}
	if err := c.loadField(); err != nil {
		t.Fatal(err)
	}
	if c.GuardSessionTTL != 8*time.Hour || c.GuardTokenGrace != 72*time.Hour || len(c.QRSigningSecret) != 40 {
		t.Fatalf("loaded = %+v", c)
	}
}
