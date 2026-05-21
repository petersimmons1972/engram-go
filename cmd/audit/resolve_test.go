package main

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// writeMCPConfig writes a synthetic mcp_servers.json under tmpDir/.claude/ and
// returns tmpDir for use as $HOME.
func writeMCPConfig(t *testing.T, url, token string) string {
	t.Helper()
	tmpDir := t.TempDir()
	claudeDir := filepath.Join(tmpDir, ".claude")
	if err := os.MkdirAll(claudeDir, 0755); err != nil {
		t.Fatal(err)
	}
	cfg := map[string]any{
		"mcpServers": map[string]any{
			"engram": map[string]any{
				"url": url,
				"headers": map[string]string{
					"Authorization": "Bearer " + token,
				},
			},
		},
	}
	b, _ := json.Marshal(cfg)
	if err := os.WriteFile(filepath.Join(claudeDir, "mcp_servers.json"), b, 0600); err != nil {
		t.Fatal(err)
	}
	return tmpDir
}

// clearEngramEnv unsets ENGRAM_URL and ENGRAM_TOKEN for the duration of the test.
func clearEngramEnv(t *testing.T) {
	t.Helper()
	t.Setenv("ENGRAM_URL", "")
	t.Setenv("ENGRAM_TOKEN", "")
}

// ---- existing tests (kept, updated to clear env to avoid interference) ----

func TestResolveEngramFlagOverride(t *testing.T) {
	clearEngramEnv(t)
	base, token, err := resolveEngram("http://custom-engram.test", "my-token")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if base != "http://custom-engram.test" {
		t.Errorf("base: want %q, got %q", "http://custom-engram.test", base)
	}
	if token != "my-token" {
		t.Errorf("token: want %q, got %q", "my-token", token)
	}
}

func TestResolveEngramFromConfigFile(t *testing.T) {
	clearEngramEnv(t)
	tmpDir := writeMCPConfig(t, "http://engram.test/sse", "tok-from-config")
	t.Setenv("HOME", tmpDir)

	base, token, err := resolveEngram("", "")
	if err != nil {
		t.Fatalf("resolveEngram from config: %v", err)
	}
	// URL should have /sse stripped.
	if base != "http://engram.test" {
		t.Errorf("base: want %q, got %q", "http://engram.test", base)
	}
	if token != "tok-from-config" {
		t.Errorf("token: want %q, got %q", "tok-from-config", token)
	}
}

func TestResolveEngramMissingConfig(t *testing.T) {
	clearEngramEnv(t)
	// HOME points to a dir without .claude/mcp_servers.json.
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	_, _, err := resolveEngram("", "")
	if err == nil {
		t.Error("expected error when config file is missing, got nil")
	}
	if !errors.Is(err, errNoEngram) {
		t.Errorf("want errNoEngram, got %v", err)
	}
}

// ---- new tests for env-var resolution path ----

func TestResolveEngramCreds_FromFlags(t *testing.T) {
	clearEngramEnv(t)
	base, token, err := resolveEngram("http://flag-engram.test", "flag-token")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if base != "http://flag-engram.test" {
		t.Errorf("base: want %q, got %q", "http://flag-engram.test", base)
	}
	if token != "flag-token" {
		t.Errorf("token: want %q, got %q", "flag-token", token)
	}
}

func TestResolveEngramCreds_FromEnv(t *testing.T) {
	t.Setenv("ENGRAM_URL", "http://env-engram.test")
	t.Setenv("ENGRAM_TOKEN", "env-token")
	// Point HOME at an empty dir so the file path is not accidentally hit.
	t.Setenv("HOME", t.TempDir())

	base, token, err := resolveEngram("", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if base != "http://env-engram.test" {
		t.Errorf("base: want %q, got %q", "http://env-engram.test", base)
	}
	if token != "env-token" {
		t.Errorf("token: want %q, got %q", "env-token", token)
	}
}

func TestResolveEngramCreds_NoneSet_ReturnsError(t *testing.T) {
	clearEngramEnv(t)
	// HOME has no mcp_servers.json.
	t.Setenv("HOME", t.TempDir())

	_, _, err := resolveEngram("", "")
	if err == nil {
		t.Fatal("expected error when no credentials are available, got nil")
	}
	if !errors.Is(err, errNoEngram) {
		t.Errorf("want errNoEngram, got %v", err)
	}
}

func TestResolveEngramCreds_FlagsBeatEnv(t *testing.T) {
	t.Setenv("ENGRAM_URL", "http://env-engram.test")
	t.Setenv("ENGRAM_TOKEN", "env-token")

	base, token, err := resolveEngram("http://flag-engram.test", "flag-token")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if base != "http://flag-engram.test" {
		t.Errorf("flags must win: base want %q, got %q", "http://flag-engram.test", base)
	}
	if token != "flag-token" {
		t.Errorf("flags must win: token want %q, got %q", "flag-token", token)
	}
}
