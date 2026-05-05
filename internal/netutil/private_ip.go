// Package netutil provides network-level security utilities for Engram.
package netutil

import (
	"fmt"
	"net"
	"net/url" // used by ValidateUpstreamURL
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

// ValidateUpstreamURL checks that urlStr is a valid HTTP(S) URL whose hostname
// does not resolve to a private/reserved IP address. This prevents SSRF attacks
// where environment variables point to internal services (e.g. 127.0.0.1:9200) (#549).
//
// Validation steps:
// 1. Parse the URL and check scheme is http or https
// 2. Extract hostname and check if it's a literal IP (reject if private)
// 3. If hostname, resolve via net.LookupIP and check all resolved IPs
// 4. Return error if any resolved IP is private/reserved
func ValidateUpstreamURL(urlStr string) error {
	u, err := url.Parse(urlStr)
	if err != nil {
		return fmt.Errorf("invalid URL: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("invalid scheme %q — only http and https allowed", u.Scheme)
	}
	if u.Hostname() == "" {
		return fmt.Errorf("URL has no hostname")
	}

	hostname := u.Hostname()

	// If hostname is a literal IP, check it immediately.
	if ip := net.ParseIP(hostname); ip != nil {
		if IsPrivateIP(hostname) {
			return fmt.Errorf("upstream URL resolves to private IP %q (#549)", hostname)
		}
		return nil
	}

	// Otherwise, resolve the hostname and check all resolved IPs.
	ips, err := net.LookupIP(hostname)
	if err != nil {
		return fmt.Errorf("failed to resolve hostname %q: %w", hostname, err)
	}
	if len(ips) == 0 {
		return fmt.Errorf("hostname %q resolved to no IP addresses", hostname)
	}

	for _, ip := range ips {
		if IsPrivateIP(ip.String()) {
			return fmt.Errorf("upstream URL hostname %q resolves to private IP %q (#549)", hostname, ip)
		}
	}

	return nil
}
