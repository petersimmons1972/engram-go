package main

import (
	"testing"
)

func TestRecallDefaultModeDefault(t *testing.T) {
	const key = "ENGRAM_RECALL_DEFAULT_MODE"

	// Case 1: env var not set — should return the default "handle".
	t.Setenv(key, "")
	if got := envOr(key, "handle"); got != "handle" {
		t.Errorf("envOr with unset var = %q, want %q", got, "handle")
	}

	// Case 2: env var set to "full" — should return "full".
	t.Setenv(key, "full")
	if got := envOr(key, "handle"); got != "full" {
		t.Errorf("envOr with var=full = %q, want %q", got, "full")
	}

	// Case 3: env var set to "" (empty string) — envOr treats empty as unset,
	// so the default "handle" must be returned.
	t.Setenv(key, "")
	if got := envOr(key, "handle"); got != "handle" {
		t.Errorf("envOr with empty var = %q, want %q (default)", got, "handle")
	}
}

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
		// Previously missing ranges (fixes #68)
		{"0.0.0.1", true},          // this-network (RFC 1122)
		{"100.64.0.1", true},       // CGNAT (RFC 6598)
		{"100.127.255.255", true},  // CGNAT top
		{"198.18.0.1", true},       // benchmark (RFC 2544)
		{"198.19.255.255", true},   // benchmark top
		{"240.0.0.1", true},        // reserved (RFC 1112)
		{"::ffff:192.168.1.1", true}, // IPv4-mapped IPv6

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
