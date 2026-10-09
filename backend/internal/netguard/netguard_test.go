package netguard

import (
	"context"
	"errors"
	"net"
	"net/netip"
	"testing"
	"time"
)

func TestPolicyAllowed(t *testing.T) {
	p, err := NewPolicy([]string{"10.20.0.0/16", "192.168.1.25"})
	if err != nil {
		t.Fatal(err)
	}
	cases := map[string]bool{
		"8.8.8.8":          true,
		"2a03:2880::1":     true,
		"127.0.0.1":        false,
		"::1":              false,
		"10.0.0.5":         false,
		"10.20.3.4":        true, // allow-listed relay network
		"192.168.1.25":     true,
		"192.168.1.26":     false,
		"172.18.0.3":       false, // docker network
		"169.254.169.254":  false, // cloud metadata
		"100.64.1.1":       false,
		"0.0.0.0":          false,
		"::ffff:127.0.0.1": false,
		"fd00::1":          false,
		"fe80::1":          false,
	}
	for ip, want := range cases {
		if got := p.Allowed(netip.MustParseAddr(ip)); got != want {
			t.Errorf("Allowed(%s) = %v, want %v", ip, got, want)
		}
	}
	if _, err := NewPolicy([]string{"not-an-ip"}); err == nil {
		t.Error("invalid allow-list entry accepted")
	}
	if !AllowAll.Allowed(netip.MustParseAddr("127.0.0.1")) {
		t.Error("AllowAll blocks loopback")
	}
}

func TestDialerBlocksAtConnectTime(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	var p Policy
	_, err = p.Dialer(time.Second).DialContext(context.Background(), "tcp", ln.Addr().String())
	if !errors.Is(err, ErrBlocked) {
		t.Fatalf("dial to loopback: %v, want ErrBlocked", err)
	}
	if err := p.CheckHost(context.Background(), "127.0.0.1"); !errors.Is(err, ErrBlocked) {
		t.Fatalf("CheckHost(127.0.0.1) = %v", err)
	}
	if err := p.CheckHost(context.Background(), "localhost"); !errors.Is(err, ErrBlocked) {
		t.Fatalf("CheckHost(localhost) = %v", err)
	}
	allow, _ := NewPolicy([]string{"127.0.0.1"})
	conn, err := allow.Dialer(time.Second).DialContext(context.Background(), "tcp", ln.Addr().String())
	if err != nil {
		t.Fatalf("allow-listed dial: %v", err)
	}
	conn.Close()
}
