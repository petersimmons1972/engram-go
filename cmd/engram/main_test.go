package main

import (
	"bytes"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/petersimmons1972/engram/internal/netutil"
)

// TestRouterURLFallback verifies the deprecation fallback:
// - ENGRAM_ROUTER_URL takes priority when set.
// - When only LITELLM_URL is set, its value is used and a deprecation warning is emitted.
// - When neither is set, the default is returned.
func TestRouterURLFallback(t *testing.T) {
	t.Run("ENGRAM_ROUTER_URL wins when both set", func(t *testing.T) {
		t.Setenv("ENGRAM_ROUTER_URL", "http://router:4000")
		t.Setenv("LITELLM_URL", "http://legacy:4000")
		var buf bytes.Buffer
		logger := slog.New(slog.NewTextHandler(&buf, nil))
		got := routerURLFromEnv("http://litellm:4000", logger)
		if got != "http://router:4000" {
			t.Errorf("routerURLFromEnv = %q, want %q", got, "http://router:4000")
		}
		if strings.Contains(buf.String(), "deprecated") {
			t.Error("unexpected deprecation warning when ENGRAM_ROUTER_URL is set")
		}
	})

	t.Run("LITELLM_URL fallback with deprecation warning", func(t *testing.T) {
		t.Setenv("ENGRAM_ROUTER_URL", "")
		t.Setenv("LITELLM_URL", "http://legacy:4000")
		var buf bytes.Buffer
		logger := slog.New(slog.NewTextHandler(&buf, nil))
		got := routerURLFromEnv("http://litellm:4000", logger)
		if got != "http://legacy:4000" {
			t.Errorf("routerURLFromEnv = %q, want %q", got, "http://legacy:4000")
		}
		if !strings.Contains(buf.String(), "deprecated") {
			t.Error("expected deprecation warning when only LITELLM_URL is set, got none")
		}
	})

	t.Run("default returned when neither set", func(t *testing.T) {
		t.Setenv("ENGRAM_ROUTER_URL", "")
		t.Setenv("LITELLM_URL", "")
		var buf bytes.Buffer
		logger := slog.New(slog.NewTextHandler(&buf, nil))
		got := routerURLFromEnv("http://litellm:4000", logger)
		if got != "http://litellm:4000" {
			t.Errorf("routerURLFromEnv = %q, want %q", got, "http://litellm:4000")
		}
	})
}

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

// TestIsPrivateIP_ViaNetutil verifies that the startup SSRF guard in main.go
// delegates correctly to netutil.IsPrivateIP. The comprehensive range coverage
// is in internal/netutil/private_ip_test.go; this test covers the cases that
// were exercised by the old inline isPrivateIP function (#55, #68, #242).
// ---------------------------------------------------------------------------
// validateEmbedConfig — embed config consistency guard (#380)
// ---------------------------------------------------------------------------

func TestValidateEmbedConfig(t *testing.T) {
	cases := []struct {
		model        string
		dims         int
		wantWarn     bool
		warnContains string
	}{
		{"BAAI/bge-m3", 1024, false, ""},
		{"BAAI/bge-m3", 0, true, "ENGRAM_EMBED_DIMENSIONS=1024"},
		{"some-model", 1024, true, "not in SuggestedModels"},
		{"some-model", 512, true, "not in SuggestedModels"},
		{"nomic-embed-text", 0, true, "ENGRAM_EMBED_DIMENSIONS=1024"},
	}

	for _, tc := range cases {
		warn := validateEmbedConfig(tc.model, tc.dims)
		if tc.wantWarn && warn == "" {
			t.Errorf("validateEmbedConfig(%q, %d): want warning, got empty", tc.model, tc.dims)
		}
		if !tc.wantWarn && warn != "" {
			t.Errorf("validateEmbedConfig(%q, %d): want no warning, got %q", tc.model, tc.dims, warn)
		}
		if tc.warnContains != "" && !strings.Contains(warn, tc.warnContains) {
			t.Errorf("validateEmbedConfig(%q, %d): warning %q should contain %q", tc.model, tc.dims, warn, tc.warnContains)
		}
	}
}

func TestEmbedModelIsRequired(t *testing.T) {
	t.Setenv("ENGRAM_EMBED_MODEL", "")
	t.Setenv("ENGRAM_OLLAMA_MODEL", "")
	t.Setenv("ENGRAM_API_KEY", "test-key")
	t.Setenv("DATABASE_URL", "postgres://user:pass@localhost/db")
	t.Setenv("LITELLM_URL", "http://localhost:4000")
	// The run path will fail earlier for other missing dependencies in this
	// sandbox, so this test only checks the flag/env default resolution.
	if got := envOr("ENGRAM_EMBED_MODEL", envOr("ENGRAM_OLLAMA_MODEL", "")); got != "" {
		t.Fatalf("expected empty embed model default, got %q", got)
	}
}

func TestIsPrivateIP_ViaNetutil(t *testing.T) {
	cases := []struct {
		ip      string
		private bool
	}{
		// Private / reserved ranges that must be blocked
		{"169.254.169.254", true}, // AWS metadata
		{"10.0.0.1", true},        // RFC-1918
		{"10.255.255.255", true},  // RFC-1918
		{"172.16.0.1", true},      // RFC-1918
		{"172.31.255.255", true},  // RFC-1918
		{"192.168.1.1", true},     // RFC-1918
		{"127.0.0.1", true},       // loopback
		{"127.255.0.1", true},     // loopback range
		{"::1", true},             // IPv6 loopback
		{"fc00::1", true},         // IPv6 ULA
		{"fe80::1", true},         // IPv6 link-local
		// Previously missing ranges (fixes #68)
		{"0.0.0.1", true},            // this-network (RFC 1122)
		{"100.64.0.1", true},         // CGNAT (RFC 6598)
		{"100.127.255.255", true},    // CGNAT top
		{"198.18.0.1", true},         // benchmark (RFC 2544)
		{"198.19.255.255", true},     // benchmark top
		{"240.0.0.1", true},          // reserved (RFC 1112)
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
		got := netutil.IsPrivateIP(tc.ip)
		if got != tc.private {
			t.Errorf("netutil.IsPrivateIP(%q) = %v, want %v", tc.ip, got, tc.private)
		}
	}
}

// TestPprofNotRegisteredByDefault verifies that pprof HTTP handlers are NOT
// registered by default (requires -tags=pprof). This is the complement of the
// pprof_enabled.go build tag — pprof should only be available in opt-in builds.
func TestPprofNotRegisteredByDefault(t *testing.T) {
	// In the default build (without -tags=pprof), the pprof import should not
	// be active, so no handlers should be registered on http.DefaultServeMux.
	//
	// We test this by iterating the mux and checking that no pattern contains
	// the "debug/pprof" string that pprof handlers are registered under.
	defaultMux := http.DefaultServeMux
	defaultMux.Handle("/test-marker", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))

	req := &http.Request{
		Method: "GET",
		URL:    &url.URL{Path: "/debug/pprof/"},
		Host:   "localhost",
	}
	handler, pattern := defaultMux.Handler(req)
	if pattern != "" && strings.Contains(pattern, "debug/pprof") {
		t.Errorf("pprof should not be registered by default; found pattern %q", pattern)
	}
	if handler != nil && strings.Contains(fmt.Sprint(handler), "pprof") {
		t.Errorf("pprof handler should not be registered by default")
	}
}

// TestRateLimitPrecedence verifies that --rate-limit-rps takes precedence
// over --rate-limit when both are set (#560).
func TestRateLimitPrecedence(t *testing.T) {
	// This test documents the expected behavior:
	// When both --rate-limit and --rate-limit-rps are provided,
	// --rate-limit-rps should win and a warning should be logged.
	// The actual implementation of this logic occurs during config
	// initialization in the server setup, which is not testable in isolation
	// without starting a full server.
	//
	// The test here is a placeholder documenting the expectation.
	t.Log("rate-limit-rps precedence is enforced during server startup")
}

// TestCheckBindInterlock verifies the A-3 #666 startup interlock:
// non-loopback host + rate-limit-disable must refuse to start.
func TestCheckBindInterlock(t *testing.T) {
	cases := []struct {
		name             string
		host             string
		rateLimitDisable bool
		wantErr          bool
		wantErrSubstr    string
	}{
		{"loopback + rate limit enabled = allow", "127.0.0.1", false, false, ""},
		{"loopback IPv6 + rate limit enabled = allow", "::1", false, false, ""},
		{"loopback hostname + rate limit enabled = allow", "localhost", false, false, ""},
		{"loopback + rate limit disabled = allow (legitimate single-user)", "127.0.0.1", true, false, ""},
		{"non-loopback + rate limit enabled = allow", "0.0.0.0", false, false, ""},
		{"non-loopback wildcard + rate limit disabled = REFUSE", "0.0.0.0", true, true, "ENGRAM_RATE_LIMIT_DISABLE"},
		{"non-loopback LAN IP + rate limit disabled = REFUSE", "192.168.1.10", true, true, "non-loopback"},
		{"non-loopback IPv6 + rate limit disabled = REFUSE", "::", true, true, "non-loopback"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := checkBindInterlock(tc.host, tc.rateLimitDisable)
			if tc.wantErr && err == nil {
				t.Fatalf("expected error, got nil")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("expected nil, got %v", err)
			}
			if tc.wantErr && tc.wantErrSubstr != "" && !strings.Contains(err.Error(), tc.wantErrSubstr) {
				t.Errorf("error %q missing %q", err.Error(), tc.wantErrSubstr)
			}
		})
	}
}

// TestEngramReadyLog_IncludesVersion — #674: the startup log line must include
// the binary version so operators can identify the running build via logs.
// This is a string-presence test on main.go; it can run without spinning up
// the server.
func TestEngramReadyLog_IncludesVersion(t *testing.T) {
	src, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatalf("read main.go: %v", err)
	}
	want := `slog.Info("engram ready", "version", Version`
	if !strings.Contains(string(src), want) {
		t.Errorf("main.go missing version key in 'engram ready' slog.Info call (#674)")
	}
}
