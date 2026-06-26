// Package netutil provides network-level security utilities for Engram.
package netutil

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"time"
)

// IPAddrResolver resolves a hostname into IP addresses.
type IPAddrResolver interface {
	LookupIPAddr(ctx context.Context, host string) ([]net.IPAddr, error)
}

// ContextDialer dials a network address using a context-aware call.
type ContextDialer interface {
	DialContext(ctx context.Context, network, addr string) (net.Conn, error)
}

// SafeDialOptions configures the dial-time DNS re-resolution guard.
type SafeDialOptions struct {
	Resolver                   IPAddrResolver
	Dialer                     ContextDialer
	AllowPrivateConfiguredHost bool
	ErrorPrefix                string
}

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
// 4. Return error if any resolved IP is private/reserved.
func ValidateUpstreamURL(urlStr string) error {
	return validateUpstreamURLWithResolver(urlStr, net.DefaultResolver)
}

func validateUpstreamURLWithResolver(urlStr string, resolver IPAddrResolver) error {
	if resolver == nil {
		resolver = net.DefaultResolver
	}

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
	resolveCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	ips, err := resolver.LookupIPAddr(resolveCtx, hostname)
	if err != nil {
		return fmt.Errorf("failed to resolve hostname %q: %w", hostname, err)
	}
	if len(ips) == 0 {
		return fmt.Errorf("hostname %q resolved to no IP addresses", hostname)
	}

	for _, ip := range ips {
		if IsPrivateIP(ip.IP.String()) {
			return fmt.Errorf("upstream URL hostname %q resolves to private IP %q (#549)", hostname, ip.IP)
		}
	}

	return nil
}

// NewUpstreamDialContext returns a DialContext hook that re-resolves the target
// hostname at dial time, rejects private/reserved IPs when configured to do so,
// and dials the resolved literal IP directly to avoid a second OS resolver hop.
func NewUpstreamDialContext(baseURL string, opts SafeDialOptions) func(ctx context.Context, network, addr string) (net.Conn, error) {
	var configuredHost string
	if u, err := url.Parse(baseURL); err == nil {
		configuredHost = u.Hostname()
	}

	resolver := opts.Resolver
	if resolver == nil {
		resolver = net.DefaultResolver
	}

	dialer := opts.Dialer
	if dialer == nil {
		dialer = &net.Dialer{}
	}

	errorPrefix := opts.ErrorPrefix
	if errorPrefix == "" {
		errorPrefix = "upstream URL"
	}

	return func(ctx context.Context, network, addr string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(addr)
		if err != nil {
			return nil, err
		}

		ips, err := resolver.LookupIPAddr(ctx, host)
		if err != nil {
			return nil, fmt.Errorf("DNS resolution failed for %q: %w", host, err)
		}
		if len(ips) == 0 {
			return nil, fmt.Errorf("hostname %q resolved to no IP addresses", host)
		}

		if !opts.AllowPrivateConfiguredHost || host != configuredHost {
			for _, ipAddr := range ips {
				if ipAddr.IP == nil {
					continue
				}
				if IsPrivateIP(ipAddr.IP.String()) {
					return nil, fmt.Errorf("%s resolved to private IP %q", errorPrefix, ipAddr.IP)
				}
			}
		}

		for _, ipAddr := range ips {
			if ipAddr.IP == nil {
				continue
			}
			return dialer.DialContext(ctx, network, net.JoinHostPort(ipAddr.IP.String(), port))
		}

		return nil, fmt.Errorf("hostname %q resolved to no valid IP addresses", host)
	}
}
