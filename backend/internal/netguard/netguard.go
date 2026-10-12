// Package netguard stops admin-configurable outbound connections (SMTP
// relays) from reaching internal services: loopback, private, link-local and
// other non-public addresses are refused unless explicitly allowed. The check
// runs on the resolved address at dial time, so DNS rebinding cannot bypass
// a check done when the settings were saved.
package netguard

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"strings"
	"syscall"
	"time"
)

// ErrBlocked is returned (wrapped) when a destination is not allowed.
var ErrBlocked = errors.New("destination address not allowed")

// nonPublic lists special-purpose ranges not covered by netip's predicates.
var nonPublic = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"),
	netip.MustParsePrefix("100.64.0.0/10"), // carrier-grade NAT
	netip.MustParsePrefix("192.0.0.0/24"),
	netip.MustParsePrefix("192.0.2.0/24"),
	netip.MustParsePrefix("198.18.0.0/15"),
	netip.MustParsePrefix("198.51.100.0/24"),
	netip.MustParsePrefix("203.0.113.0/24"),
	netip.MustParsePrefix("240.0.0.0/4"),
	netip.MustParsePrefix("64:ff9b::/96"), // NAT64 can map to private IPv4
	netip.MustParsePrefix("2001:db8::/32"),
}

// Policy decides which addresses outbound connections may reach. The zero
// value allows public addresses only.
type Policy struct {
	allow    []netip.Prefix
	allowAll bool
}

// AllowAll is a policy without restrictions, for operator-configured
// (environment) destinations.
var AllowAll = Policy{allowAll: true}

// NewPolicy builds a policy that additionally allows the given CIDRs or
// single IPs (e.g. an on-prem SMTP relay).
func NewPolicy(allowed []string) (Policy, error) {
	var p Policy
	for _, a := range allowed {
		a = strings.TrimSpace(a)
		if a == "" {
			continue
		}
		if !strings.Contains(a, "/") {
			ip, err := netip.ParseAddr(a)
			if err != nil {
				return p, fmt.Errorf("invalid IP or CIDR %q", a)
			}
			p.allow = append(p.allow, netip.PrefixFrom(ip.Unmap(), ip.Unmap().BitLen()))
			continue
		}
		pfx, err := netip.ParsePrefix(a)
		if err != nil {
			return p, fmt.Errorf("invalid IP or CIDR %q", a)
		}
		p.allow = append(p.allow, pfx.Masked())
	}
	return p, nil
}

// Allowed reports whether ip may be connected to.
func (p Policy) Allowed(ip netip.Addr) bool {
	if p.allowAll {
		return true
	}
	ip = ip.Unmap()
	for _, a := range p.allow {
		if a.Contains(ip) {
			return true
		}
	}
	if !ip.IsValid() || ip.IsLoopback() || ip.IsPrivate() || ip.IsUnspecified() ||
		ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsInterfaceLocalMulticast() || ip.IsMulticast() {
		return false
	}
	for _, n := range nonPublic {
		if n.Contains(ip) {
			return false
		}
	}
	return true
}

// CheckHost resolves host and fails unless every address is allowed.
func (p Policy) CheckHost(ctx context.Context, host string) error {
	if p.allowAll {
		return nil
	}
	if ip, err := netip.ParseAddr(host); err == nil {
		if !p.Allowed(ip) {
			return fmt.Errorf("%w: %s", ErrBlocked, host)
		}
		return nil
	}
	addrs, err := net.DefaultResolver.LookupNetIP(ctx, "ip", host)
	if err != nil {
		return err
	}
	if len(addrs) == 0 {
		return fmt.Errorf("no addresses for %s", host)
	}
	for _, a := range addrs {
		if !p.Allowed(a) {
			return fmt.Errorf("%w: %s resolves to a non-public address", ErrBlocked, host)
		}
	}
	return nil
}

// Dialer returns a dialer that refuses connections to disallowed addresses,
// checked on the address actually being connected to.
func (p Policy) Dialer(timeout time.Duration) *net.Dialer {
	return &net.Dialer{Timeout: timeout, Control: p.control}
}

func (p Policy) control(_, address string, _ syscall.RawConn) error {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return fmt.Errorf("%w: %s", ErrBlocked, address)
	}
	ip, err := netip.ParseAddr(host)
	if err != nil || !p.Allowed(ip) {
		return fmt.Errorf("%w: %s", ErrBlocked, host)
	}
	return nil
}
