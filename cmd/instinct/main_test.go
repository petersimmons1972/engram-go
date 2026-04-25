package main

import (
	"os"
	"testing"
)

func TestLoadConfig_EnvVars(t *testing.T) {
	t.Setenv("INSTINCT_BUFFER", "/tmp/test-buffer.jsonl")
	t.Setenv("INSTINCT_MIN_EVENTS", "5")
	t.Setenv("ENGRAM_BASE_URL", "http://localhost:9999")
	t.Setenv("ENGRAM_API_KEY", "test-key-abc")
	t.Setenv("ANTHROPIC_API_KEY", "sk-ant-test")

	cfg, err := loadConfig()
	if err != nil {
		t.Fatalf("loadConfig() error: %v", err)
	}
	if cfg.bufferPath != "/tmp/test-buffer.jsonl" {
		t.Errorf("bufferPath = %q, want /tmp/test-buffer.jsonl", cfg.bufferPath)
	}
	if cfg.minEvents != 5 {
		t.Errorf("minEvents = %d, want 5", cfg.minEvents)
	}
	if cfg.engramURL != "http://localhost:9999" {
		t.Errorf("engramURL = %q, want http://localhost:9999", cfg.engramURL)
	}
	if cfg.engramToken != "test-key-abc" {
		t.Errorf("engramToken = %q, want test-key-abc", cfg.engramToken)
	}
	if cfg.anthropicKey != "sk-ant-test" {
		t.Errorf("anthropicKey = %q, want sk-ant-test", cfg.anthropicKey)
	}
}

func TestLoadConfig_Defaults(t *testing.T) {
	// Unset all env vars so we get defaults
	for _, k := range []string{"INSTINCT_BUFFER", "INSTINCT_MIN_EVENTS", "ENGRAM_BASE_URL", "ENGRAM_API_KEY", "ANTHROPIC_API_KEY"} {
		os.Unsetenv(k)
	}
	cfg, err := loadConfig()
	// err is acceptable here if mcp_servers.json is absent — we only test the defaults
	// that are populated before the fallback is attempted
	_ = err
	home, _ := os.UserHomeDir()
	want := home + "/.local/state/instinct/buffer.jsonl"
	if cfg.bufferPath != want {
		t.Errorf("default bufferPath = %q, want %q", cfg.bufferPath, want)
	}
	if cfg.minEvents != 20 {
		t.Errorf("default minEvents = %d, want 20", cfg.minEvents)
	}
}
