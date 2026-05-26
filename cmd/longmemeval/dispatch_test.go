package main

import (
	"bytes"
	"flag"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestDispatch_Help — #662: `longmemeval help` must exit 0 and print usage.
// Currently the binary prints `unknown subcommand "help"` to stderr and exits 1.
func TestDispatch_Help(t *testing.T) {
	var stdout, stderr bytes.Buffer
	exit := dispatch([]string{"longmemeval", "help"}, &stdout, &stderr)

	if exit != 0 {
		t.Errorf("`longmemeval help` exit = %d, want 0", exit)
	}
	out := stdout.String()
	if !strings.Contains(out, "Usage:") {
		t.Errorf("help output missing 'Usage:' header\n--- stdout ---\n%s\n--- stderr ---\n%s",
			out, stderr.String())
	}
	for _, sub := range []string{"ingest", "run", "score", "all"} {
		if !strings.Contains(out, sub) {
			t.Errorf("help output missing subcommand %q in listing", sub)
		}
	}
	if strings.Contains(stderr.String(), "unknown subcommand") {
		t.Errorf("help should NOT report itself as an unknown subcommand: %s", stderr.String())
	}
}

// TestDispatch_NoArgs — invoking with no subcommand should print usage to stderr
// and exit non-zero (matches existing behaviour for unknown subcommands).
func TestDispatch_NoArgs(t *testing.T) {
	var stdout, stderr bytes.Buffer
	exit := dispatch([]string{"longmemeval"}, &stdout, &stderr)

	if exit == 0 {
		t.Errorf("`longmemeval` with no subcommand must exit non-zero, got 0")
	}
	if !strings.Contains(stderr.String()+stdout.String(), "Usage") {
		t.Errorf("no-args invocation must print usage somewhere; stderr=%q stdout=%q",
			stderr.String(), stdout.String())
	}
}

// TestDispatch_UnknownSubcommand — preserves existing error-on-unknown behaviour.
func TestDispatch_UnknownSubcommand(t *testing.T) {
	var stdout, stderr bytes.Buffer
	exit := dispatch([]string{"longmemeval", "frobnicate"}, &stdout, &stderr)

	if exit == 0 {
		t.Error("unknown subcommand must exit non-zero")
	}
	if !strings.Contains(stderr.String(), "unknown subcommand") {
		t.Errorf("expected 'unknown subcommand' in stderr, got %q", stderr.String())
	}
}

func TestDispatch_APIKeyDefaultsDoNotLeakInHelp(t *testing.T) {
	for _, subcommand := range []string{"ingest", "run"} {
		t.Run(subcommand+"/env", func(t *testing.T) {
			const secret = "env-secret-must-not-print"
			t.Setenv("ENGRAM_API_KEY", secret)
			t.Setenv("HOME", t.TempDir())

			var stdout, stderr bytes.Buffer
			exit := dispatch([]string{"longmemeval", subcommand, "--help"}, &stdout, &stderr)
			if exit != 0 {
				t.Fatalf("%s --help exit = %d, want 0; stderr=%q", subcommand, exit, stderr.String())
			}
			combined := stdout.String() + stderr.String()
			if strings.Contains(combined, secret) {
				t.Fatalf("%s --help leaked ENGRAM_API_KEY in output:\n%s", subcommand, combined)
			}
		})

		t.Run(subcommand+"/mcp", func(t *testing.T) {
			const secret = "mcp-secret-must-not-print"
			home := t.TempDir()
			t.Setenv("ENGRAM_API_KEY", "")
			t.Setenv("HOME", home)
			writeClaudeMCPConfig(t, home, secret)

			var stdout, stderr bytes.Buffer
			exit := dispatch([]string{"longmemeval", subcommand, "--help"}, &stdout, &stderr)
			if exit != 0 {
				t.Fatalf("%s --help exit = %d, want 0; stderr=%q", subcommand, exit, stderr.String())
			}
			combined := stdout.String() + stderr.String()
			if strings.Contains(combined, secret) {
				t.Fatalf("%s --help leaked Claude MCP bearer token in output:\n%s", subcommand, combined)
			}
		})
	}
}

func TestApplySharedDefaultsPreservesExplicitEmptyValues(t *testing.T) {
	t.Setenv("ENGRAM_API_KEY", "env-secret")
	t.Setenv("ENGRAM_URL", "http://env.example.test")
	t.Setenv("HOME", t.TempDir())

	cfg := &Config{}
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	fs.StringVar(&cfg.APIKey, "api-key", "", "Engram API key")
	fs.StringVar(&cfg.ServerURL, "url", "", "Engram server URL")
	if err := fs.Parse([]string{"--api-key=", "--url="}); err != nil {
		t.Fatalf("parse flags: %v", err)
	}

	applySharedDefaults(cfg, fs)

	if cfg.APIKey != "" {
		t.Fatalf("explicit empty --api-key must remain empty, got %q", cfg.APIKey)
	}
	if cfg.ServerURL != "" {
		t.Fatalf("explicit empty --url must remain empty, got %q", cfg.ServerURL)
	}
}

func TestApplySharedDefaultsUsesEnvironmentWhenFlagsOmitted(t *testing.T) {
	t.Setenv("ENGRAM_API_KEY", "env-secret")
	t.Setenv("ENGRAM_URL", "http://env.example.test")
	t.Setenv("HOME", t.TempDir())

	cfg := &Config{}
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	fs.StringVar(&cfg.APIKey, "api-key", "", "Engram API key")
	fs.StringVar(&cfg.ServerURL, "url", "", "Engram server URL")
	if err := fs.Parse(nil); err != nil {
		t.Fatalf("parse flags: %v", err)
	}

	applySharedDefaults(cfg, fs)

	if cfg.APIKey != "env-secret" {
		t.Fatalf("omitted --api-key should use ENGRAM_API_KEY, got %q", cfg.APIKey)
	}
	if cfg.ServerURL != "http://env.example.test" {
		t.Fatalf("omitted --url should use ENGRAM_URL, got %q", cfg.ServerURL)
	}
}

func writeClaudeMCPConfig(t *testing.T, home, token string) {
	t.Helper()
	dir := filepath.Join(home, ".claude")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatalf("create .claude dir: %v", err)
	}
	data := `{"mcpServers":{"engram":{"url":"http://example.test:8788/sse","headers":{"Authorization":"Bearer ` + token + `"}}}}`
	if err := os.WriteFile(filepath.Join(dir, "mcp_servers.json"), []byte(data), 0o600); err != nil {
		t.Fatalf("write mcp config: %v", err)
	}
}

// silenceWriter discards writes — used as a placeholder for tests that don't
// inspect output but need to satisfy a writer parameter.
type silenceWriter struct{}

func (silenceWriter) Write(b []byte) (int, error) { return len(b), nil }

var _ io.Writer = silenceWriter{}
