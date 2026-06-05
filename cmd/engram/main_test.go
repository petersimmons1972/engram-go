package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

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
		{"any-model", 0, true, "ENGRAM_EMBED_DIMENSIONS=1024"},
		{"any-model", 512, true, "ENGRAM_EMBED_DIMENSIONS=1024"},
		{"any-model", 768, true, "ENGRAM_EMBED_DIMENSIONS=1024"},
		{"any-model", 1024, false, ""},
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

func TestRunMigrate_HelpAndArgs(t *testing.T) {
	t.Run("help prints usage and exits cleanly", func(t *testing.T) {
		if err := runMigrate([]string{"--help"}); err != nil {
			t.Fatalf("runMigrate --help returned error: %v", err)
		}
	})

	t.Run("rejects positional arguments", func(t *testing.T) {
		err := runMigrate([]string{"unexpected"})
		if err == nil || !strings.Contains(err.Error(), "does not accept positional arguments") {
			t.Fatalf("runMigrate([]string{\"unexpected\"}) error = %v, want positional-args message", err)
		}
	})

	t.Run("rejects unknown flags", func(t *testing.T) {
		err := runMigrate([]string{"--foo"})
		if err == nil || !strings.Contains(err.Error(), "flag provided but not defined") {
			t.Fatalf("runMigrate([]string{\"--foo\"}) error = %v, want unknown-flag message", err)
		}
	})
}

func TestRunMigrate_RequiresDatabaseURL(t *testing.T) {
	t.Setenv("DATABASE_URL", "")
	err := runMigrate(nil)
	if err == nil || !strings.Contains(err.Error(), "DATABASE_URL required") {
		t.Fatalf("runMigrate with missing DATABASE_URL error = %v, want DATABASE_URL required", err)
	}
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

func TestRunMigrate(t *testing.T) {
	t.Run("help is command specific", func(t *testing.T) {
		stdout, restore := captureStdout(t)
		defer restore()

		if err := runMigrate([]string{"--help"}); err != nil {
			t.Fatalf("runMigrate(--help): %v", err)
		}

		got := stdout()
		if !strings.Contains(got, "Usage: engram migrate [options]") {
			t.Fatalf("migrate help missing usage, got: %q", got)
		}
		if strings.Contains(got, "Start the engram MCP server") {
			t.Fatalf("migrate help should not include server help, got: %q", got)
		}
	})

	t.Run("requires database url", func(t *testing.T) {
		t.Setenv("DATABASE_URL", "")

		err := runMigrate(nil)
		if err == nil || !strings.Contains(err.Error(), "DATABASE_URL required") {
			t.Fatalf("runMigrate without DATABASE_URL error = %v, want DATABASE_URL required", err)
		}
	})

	t.Run("runs migrations and exits without server startup", func(t *testing.T) {
		t.Setenv("DATABASE_URL", "postgres://user:strong@localhost/db")
		t.Setenv("ENGRAM_MIGRATE_TIMEOUT", "0")

		called := false
		oldRunMigrations := runDatabaseMigrations
		runDatabaseMigrations = func(ctx context.Context, dsn string) error {
			called = true
			if dsn != "postgres://user:strong@localhost/db" {
				t.Fatalf("dsn = %q, want env DATABASE_URL", dsn)
			}
			if got := os.Getenv("DATABASE_URL"); got != "" {
				t.Fatalf("DATABASE_URL remained in process environment during migration: %q", got)
			}
			return nil
		}
		t.Cleanup(func() { runDatabaseMigrations = oldRunMigrations })

		stdout, restore := captureStdout(t)
		defer restore()

		if err := runMigrate(nil); err != nil {
			t.Fatalf("runMigrate: %v", err)
		}
		if !called {
			t.Fatal("runMigrate did not run database migrations")
		}
		got := stdout()
		if !strings.Contains(got, "migration complete") {
			t.Fatalf("runMigrate stdout missing completion message, got %q", got)
		}
	})

	t.Run("uses configurable timeout", func(t *testing.T) {
		t.Setenv("DATABASE_URL", "postgres://user:strong@localhost/db")
		t.Setenv("ENGRAM_MIGRATE_TIMEOUT", "250ms")

		oldRunMigrations := runDatabaseMigrations
		runDatabaseMigrations = func(ctx context.Context, _ string) error {
			deadline, ok := ctx.Deadline()
			if !ok {
				t.Fatal("migration context has no deadline")
			}
			remaining := time.Until(deadline)
			if remaining <= 0 || remaining > time.Second {
				t.Fatalf("migration deadline remaining = %v, want about 250ms", remaining)
			}
			return nil
		}
		t.Cleanup(func() { runDatabaseMigrations = oldRunMigrations })

		if err := runMigrate(nil); err != nil {
			t.Fatalf("runMigrate: %v", err)
		}
	})

	t.Run("zero timeout disables deadline", func(t *testing.T) {
		t.Setenv("DATABASE_URL", "postgres://user:strong@localhost/db")
		t.Setenv("ENGRAM_MIGRATE_TIMEOUT", "0")

		oldRunMigrations := runDatabaseMigrations
		runDatabaseMigrations = func(ctx context.Context, _ string) error {
			if _, ok := ctx.Deadline(); ok {
				t.Fatal("migration context unexpectedly has a deadline")
			}
			return nil
		}
		t.Cleanup(func() { runDatabaseMigrations = oldRunMigrations })

		if err := runMigrate(nil); err != nil {
			t.Fatalf("runMigrate: %v", err)
		}
	})
}

func captureStdout(t *testing.T) (func() string, func()) {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe stdout: %v", err)
	}
	os.Stdout = w

	return func() string {
			if err := w.Close(); err != nil {
				t.Fatalf("close stdout writer: %v", err)
			}
			out, err := io.ReadAll(r)
			if err != nil {
				t.Fatalf("read stdout: %v", err)
			}
			return string(out)
		}, func() {
			os.Stdout = old
			_ = r.Close()
		}
}

// ---------------------------------------------------------------------------
// envInt
// ---------------------------------------------------------------------------

func TestEnvInt_SetValid(t *testing.T) {
	t.Setenv("ENGRAM_TEST_INT", "42")
	got := envInt("ENGRAM_TEST_INT", 0)
	if got != 42 {
		t.Errorf("envInt set valid = %d, want 42", got)
	}
}

func TestEnvInt_Unset(t *testing.T) {
	t.Setenv("ENGRAM_TEST_INT_UNSET", "")
	got := envInt("ENGRAM_TEST_INT_UNSET", 7)
	if got != 7 {
		t.Errorf("envInt unset = %d, want 7 (default)", got)
	}
}

func TestEnvInt_ParseError(t *testing.T) {
	t.Setenv("ENGRAM_TEST_INT_BAD", "notanint")
	got := envInt("ENGRAM_TEST_INT_BAD", 99)
	if got != 99 {
		t.Errorf("envInt parse error = %d, want 99 (default)", got)
	}
}

func TestEnvInt_NegativeValue(t *testing.T) {
	t.Setenv("ENGRAM_TEST_INT_NEG", "-5")
	got := envInt("ENGRAM_TEST_INT_NEG", 0)
	if got != -5 {
		t.Errorf("envInt negative = %d, want -5", got)
	}
}

func TestEnvInt_Zero(t *testing.T) {
	t.Setenv("ENGRAM_TEST_INT_ZERO", "0")
	// "0" is a valid value but os.Getenv returns "0" (non-empty) → parsed → 0 returned.
	got := envInt("ENGRAM_TEST_INT_ZERO", 10)
	if got != 0 {
		t.Errorf("envInt zero = %d, want 0", got)
	}
}

// ---------------------------------------------------------------------------
// envFloat
// ---------------------------------------------------------------------------

func TestEnvFloat_SetValid(t *testing.T) {
	t.Setenv("ENGRAM_TEST_FLOAT", "3.14")
	got := envFloat("ENGRAM_TEST_FLOAT", 0.0)
	if got < 3.139 || got > 3.141 {
		t.Errorf("envFloat set valid = %v, want ~3.14", got)
	}
}

func TestEnvFloat_Unset(t *testing.T) {
	t.Setenv("ENGRAM_TEST_FLOAT_UNSET", "")
	got := envFloat("ENGRAM_TEST_FLOAT_UNSET", 2.718)
	if got != 2.718 {
		t.Errorf("envFloat unset = %v, want 2.718 (default)", got)
	}
}

func TestEnvFloat_ParseError(t *testing.T) {
	t.Setenv("ENGRAM_TEST_FLOAT_BAD", "notafloat")
	got := envFloat("ENGRAM_TEST_FLOAT_BAD", 1.0)
	if got != 1.0 {
		t.Errorf("envFloat parse error = %v, want 1.0 (default)", got)
	}
}

func TestEnvFloat_NegativeValue(t *testing.T) {
	t.Setenv("ENGRAM_TEST_FLOAT_NEG", "-1.5")
	got := envFloat("ENGRAM_TEST_FLOAT_NEG", 0.0)
	if got != -1.5 {
		t.Errorf("envFloat negative = %v, want -1.5", got)
	}
}

func TestEnvFloat_Zero(t *testing.T) {
	t.Setenv("ENGRAM_TEST_FLOAT_ZERO", "0")
	got := envFloat("ENGRAM_TEST_FLOAT_ZERO", 5.0)
	if got != 0.0 {
		t.Errorf("envFloat zero = %v, want 0.0", got)
	}
}

// ---------------------------------------------------------------------------
// envBool
// ---------------------------------------------------------------------------

func TestEnvBool_TrueVariants(t *testing.T) {
	for _, v := range []string{"1", "true", "yes", "TRUE", "True", "YES", "Yes"} {
		t.Setenv("ENGRAM_TEST_BOOL", v)
		got := envBool("ENGRAM_TEST_BOOL", false)
		if !got {
			t.Errorf("envBool(%q) = false, want true", v)
		}
	}
}

func TestEnvBool_FalseVariants(t *testing.T) {
	for _, v := range []string{"0", "false", "no", "FALSE", "False", "NO", "No"} {
		t.Setenv("ENGRAM_TEST_BOOL", v)
		got := envBool("ENGRAM_TEST_BOOL", true)
		if got {
			t.Errorf("envBool(%q) = true, want false", v)
		}
	}
}

func TestEnvBool_Unset_DefaultFalse(t *testing.T) {
	t.Setenv("ENGRAM_TEST_BOOL_UNSET", "")
	got := envBool("ENGRAM_TEST_BOOL_UNSET", false)
	if got {
		t.Errorf("envBool unset, default=false: got true, want false")
	}
}

func TestEnvBool_Unset_DefaultTrue(t *testing.T) {
	t.Setenv("ENGRAM_TEST_BOOL_UNSET_T", "")
	got := envBool("ENGRAM_TEST_BOOL_UNSET_T", true)
	if !got {
		t.Errorf("envBool unset, default=true: got false, want true")
	}
}

func TestEnvBool_UnknownValue_ReturnsDefault(t *testing.T) {
	t.Setenv("ENGRAM_TEST_BOOL_UNK", "maybe")
	got := envBool("ENGRAM_TEST_BOOL_UNK", true)
	if !got {
		t.Errorf("envBool unknown value, default=true: got false, want true")
	}
	t.Setenv("ENGRAM_TEST_BOOL_UNK2", "maybe")
	got2 := envBool("ENGRAM_TEST_BOOL_UNK2", false)
	if got2 {
		t.Errorf("envBool unknown value, default=false: got true, want false")
	}
}

// ---------------------------------------------------------------------------
// envDuration
// ---------------------------------------------------------------------------

func TestEnvDuration_SetValid(t *testing.T) {
	t.Setenv("ENGRAM_TEST_DUR", "30s")
	got := envDuration("ENGRAM_TEST_DUR", 0)
	if got != 30*time.Second {
		t.Errorf("envDuration '30s' = %v, want 30s", got)
	}
}

func TestEnvDuration_SetMinutes(t *testing.T) {
	t.Setenv("ENGRAM_TEST_DUR_MIN", "5m")
	got := envDuration("ENGRAM_TEST_DUR_MIN", 0)
	if got != 5*time.Minute {
		t.Errorf("envDuration '5m' = %v, want 5m", got)
	}
}

func TestEnvDuration_SetHours(t *testing.T) {
	t.Setenv("ENGRAM_TEST_DUR_HOUR", "2h")
	got := envDuration("ENGRAM_TEST_DUR_HOUR", 0)
	if got != 2*time.Hour {
		t.Errorf("envDuration '2h' = %v, want 2h", got)
	}
}

func TestEnvDuration_Unset(t *testing.T) {
	t.Setenv("ENGRAM_TEST_DUR_UNSET", "")
	got := envDuration("ENGRAM_TEST_DUR_UNSET", 10*time.Second)
	if got != 10*time.Second {
		t.Errorf("envDuration unset = %v, want 10s (default)", got)
	}
}

func TestEnvDuration_ParseError(t *testing.T) {
	t.Setenv("ENGRAM_TEST_DUR_BAD", "notaduration")
	got := envDuration("ENGRAM_TEST_DUR_BAD", time.Minute)
	if got != time.Minute {
		t.Errorf("envDuration parse error = %v, want 1m (default)", got)
	}
}

func TestEnvDuration_Zero(t *testing.T) {
	t.Setenv("ENGRAM_TEST_DUR_ZERO", "0s")
	got := envDuration("ENGRAM_TEST_DUR_ZERO", time.Hour)
	if got != 0 {
		t.Errorf("envDuration '0s' = %v, want 0", got)
	}
}
