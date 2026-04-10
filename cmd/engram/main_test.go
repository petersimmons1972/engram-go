package main

import (
	"testing"
)

func TestIsPrivateIP(t *testing.T) {
	cases := []struct {
		ip      string
		private bool
	}{
		// Private / reserved ranges that must be blocked
		{"169.254.169.254", true},  // AWS metadata
		{"10.0.0.1", true},         // RFC-1918
		{"10.255.255.255", true},   // RFC-1918
		{"172.16.0.1", true},       // RFC-1918
		{"172.31.255.255", true},   // RFC-1918
		{"192.168.1.1", true},      // RFC-1918
		{"127.0.0.1", true},        // loopback
		{"127.255.0.1", true},      // loopback range
		{"::1", true},              // IPv6 loopback
		{"fc00::1", true},          // IPv6 ULA
		{"fe80::1", true},          // IPv6 link-local

		// Public addresses that must pass
		{"8.8.8.8", false},
		{"1.1.1.1", false},
		{"208.67.222.222", false},
		{"2001:4860:4860::8888", false}, // Google public DNS IPv6

		// Not an IP at all — must return false (not a private IP)
		{"", false},
		{"not-an-ip", false},
	}

	for _, tc := range cases {
		got := isPrivateIP(tc.ip)
		if got != tc.private {
			t.Errorf("isPrivateIP(%q) = %v, want %v", tc.ip, got, tc.private)
		}
	}
}
