package netutil

import (
	"net/netip"
	"strings"
)

// NormalizeRateLimitKey canonicalizes IP keys for rate limiting. IPv4 remains
// exact, native IPv6 addresses share a /64 bucket, and other keys are trimmed.
func NormalizeRateLimitKey(raw string) string {
	key := strings.TrimSpace(raw)
	address, err := netip.ParseAddr(key)
	if err != nil {
		return key
	}
	address = address.Unmap()
	if !address.Is6() {
		return address.String()
	}
	address = address.WithZone("")
	return netip.PrefixFrom(address, 64).Masked().Addr().String()
}
