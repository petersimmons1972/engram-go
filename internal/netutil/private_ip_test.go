package netutil

import (
	"testing"
)

func TestIsPrivateIP(t *testing.T) {
	cases := []struct {
		name string
		ip   string
		want bool
	}{
		// IPv4 loopback
		{"ipv4 loopback 127.0.0.1", "127.0.0.1", true},
		{"ipv4 loopback 127.255.255.255", "127.255.255.255", true},

		// RFC1918
		{"rfc1918 10.0.0.1", "10.0.0.1", true},
		{"rfc1918 10.255.255.255", "10.255.255.255", true},
		{"rfc1918 172.16.0.1", "172.16.0.1", true},
		{"rfc1918 172.31.255.255", "172.31.255.255", true},
		{"rfc1918 192.168.1.1", "192.168.1.1", true},
		{"rfc1918 192.168.255.255", "192.168.255.255", true},

		// Link-local
		{"link-local ipv4 169.254.1.1", "169.254.1.1", true},

		// Carrier-grade NAT
		{"cgnat 100.64.0.1", "100.64.0.1", true},

		// Benchmark range (RFC 2544)
		{"benchmark 198.18.0.1", "198.18.0.1", true},

		// Reserved
		{"reserved 240.0.0.1", "240.0.0.1", true},

		// "This" network
		{"this-network 0.0.0.1", "0.0.0.1", true},

		// Public IPv4 — must be false
		{"public 8.8.8.8", "8.8.8.8", false},
		{"public 1.1.1.1", "1.1.1.1", false},
		{"public 203.0.113.1", "203.0.113.1", false},

		// IPv6 loopback
		{"ipv6 loopback ::1", "::1", true},

		// IPv6 link-local
		{"ipv6 link-local fe80::1", "fe80::1", true},
		{"ipv6 link-local fe80::dead:beef", "fe80::dead:beef", true},

		// IPv6 ULA
		{"ipv6 ULA fc00::1", "fc00::1", true},
		{"ipv6 ULA fd00::1", "fd00::1", true},

		// Public IPv6 — must be false
		{"public ipv6 2001:db8::1", "2001:db8::1", false},
		{"public ipv6 2606:4700::1", "2606:4700::1", false},

		// IPv4-mapped IPv6 — must match their IPv4 classification
		{"ipv4-mapped loopback ::ffff:127.0.0.1", "::ffff:127.0.0.1", true},
		{"ipv4-mapped rfc1918 ::ffff:192.168.1.1", "::ffff:192.168.1.1", true},
		{"ipv4-mapped public ::ffff:8.8.8.8", "::ffff:8.8.8.8", false},

		// Invalid / non-IP input
		{"empty string", "", false},
		{"hostname", "example.com", false},
		{"garbage", "not-an-ip", false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := IsPrivateIP(tc.ip)
			if got != tc.want {
				t.Errorf("IsPrivateIP(%q) = %v, want %v", tc.ip, got, tc.want)
			}
		})
	}
}
