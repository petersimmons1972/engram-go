// Package netutil provides network-level security utilities for Engram.
package netutil

import (
	"net"
)

// privateRanges lists IP ranges that must not be dialed by user-supplied URLs.
// Initialized once at startup via init(); safe for concurrent reads.
var privateRanges []*net.IPNet

func init() {
	cidrs := []string{
		"0.0.0.0/8",      // "this" network (RFC 1122)
		"10.0.0.0/8",     // RFC1918
		"100.64.0.0/10",  // Carrier-grade NAT (RFC 6598)
		"127.0.0.0/8",    // IPv4 loopback
		"169.254.0.0/16", // link-local / AWS metadata endpoint
		"172.16.0.0/12",  // RFC1918
		"192.168.0.0/16", // RFC1918
		"198.18.0.0/15",  // benchmark testing (RFC 2544)
		"240.0.0.0/4",    // reserved (RFC 1112)
		"::1/128",        // IPv6 loopback
		"fc00::/7",       // IPv6 ULA
		"fe80::/10",      // IPv6 link-local
	}
	for _, cidr := range cidrs {
		_, ipNet, _ := net.ParseCIDR(cidr)
		privateRanges = append(privateRanges, ipNet)
	}
}

// IsPrivateIP reports whether ipStr is an IP address that falls within a
// private, loopback, link-local, or reserved range. Only literal IP addresses
// are checked; hostnames must be resolved before calling this function.
//
// IPv4-mapped IPv6 addresses (::ffff:x.x.x.x) are normalized to their IPv4
// form before the range check, preventing bypass via alternate notation.
func IsPrivateIP(ipStr string) bool {
	ip := net.ParseIP(ipStr)
	if ip == nil {
		return false
	}
	// Normalize IPv4-mapped IPv6 addresses to their IPv4 form so they are
	// checked against the same IPv4 private ranges.
	if ip4 := ip.To4(); ip4 != nil {
		ip = ip4
	}
	for _, r := range privateRanges {
		if r.Contains(ip) {
			return true
		}
	}
	return false
}
