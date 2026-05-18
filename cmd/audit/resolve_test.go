package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestResolveEngramFlagOverride(t *testing.T) {
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
	// Write a synthetic mcp_servers.json to a temp dir and point HOME at it.
	tmpDir := t.TempDir()
	claudeDir := filepath.Join(tmpDir, ".claude")
	if err := os.MkdirAll(claudeDir, 0755); err != nil {
		t.Fatal(err)
	}

	cfg := map[string]any{
		"mcpServers": map[string]any{
			"engram": map[string]any{
				"url": "http://engram.test/sse",
				"headers": map[string]string{
					"Authorization": "Bearer tok-from-config",
				},
			},
		},
	}
	b, _ := json.Marshal(cfg)
	if err := os.WriteFile(filepath.Join(claudeDir, "mcp_servers.json"), b, 0600); err != nil {
		t.Fatal(err)
	}

	origHome := os.Getenv("HOME")
	t.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", origHome)

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
	// HOME points to a dir without .claude/mcp_servers.json.
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	t.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", origHome)

	_, _, err := resolveEngram("", "")
	if err == nil {
		t.Error("expected error when config file is missing, got nil")
	}
}
