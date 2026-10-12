package notification

import (
	"context"
	"errors"
	"net"
	"testing"

	"github.com/securepatrol/backend/internal/netguard"
	"github.com/securepatrol/backend/internal/settings"
)

// A relay entered in the UI must not reach internal services, even when its
// name only resolves to an internal address at send time (DNS rebinding).
func TestEmailRefusesInternalRelayAtDialTime(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	_, port, _ := net.SplitHostPort(ln.Addr().String())

	cfg := settings.Channels{SMTP: settings.SMTP{Host: "127.0.0.1", Port: port, From: "sp@example.com"}}
	err = emailChannel{}.Send(context.Background(), cfg, "ops@example.com", "s", "b")
	if !errors.Is(err, netguard.ErrBlocked) {
		t.Fatalf("send via loopback relay: %v, want ErrBlocked", err)
	}

	err = emailChannel{}.Send(context.Background(), settings.Channels{}, "ops@example.com", "s", "b")
	if !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("unconfigured SMTP: %v", err)
	}
	cfg.SMTP.Guard = netguard.AllowAll // operator-configured relay
	go func() {
		if c, err := ln.Accept(); err == nil {
			_, _ = c.Write([]byte("554 no service\r\n"))
			_ = c.Close()
		}
	}()
	if err := (emailChannel{}).Send(context.Background(), cfg, "ops@example.com", "s", "b"); errors.Is(err, netguard.ErrBlocked) || err == nil {
		t.Fatalf("operator relay should be dialled (and fail on the 554 greeting): %v", err)
	}
}
