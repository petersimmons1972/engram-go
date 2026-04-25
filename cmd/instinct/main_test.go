package main

import (
	"os"
	"path/filepath"
	"strings"
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
	// Use t.Setenv so Go restores original values after test — prevents cross-test contamination
	for _, k := range []string{"INSTINCT_BUFFER", "INSTINCT_MIN_EVENTS", "ENGRAM_BASE_URL", "ENGRAM_API_KEY", "ANTHROPIC_API_KEY"} {
		t.Setenv(k, "")
	}
	cfg, err := loadConfig()
	if err != nil {
		t.Logf("loadConfig warning (acceptable if mcp_servers.json absent): %v", err)
	}
	home, _ := os.UserHomeDir()
	want := home + "/.local/state/instinct/buffer.jsonl"
	if cfg.bufferPath != want {
		t.Errorf("default bufferPath = %q, want %q", cfg.bufferPath, want)
	}
	if cfg.minEvents != 20 {
		t.Errorf("default minEvents = %d, want 20", cfg.minEvents)
	}
}

func TestLoadAndRotate_BelowMinEvents(t *testing.T) {
	dir := t.TempDir()
	buf := filepath.Join(dir, "buffer.jsonl")
	lines := strings.Repeat(`{"session_id":"s1","project_id":"p1","tool_name":"Bash","tool_input_hash":"abc","tool_output_summary":"ok","exit_status":0,"schema_version":1,"timestamp":"2026-01-01T00:00:00Z"}`+"\n", 3)
	os.WriteFile(buf, []byte(lines), 0600)

	events, processedPath := loadAndRotate(buf, 5)
	if len(events) != 0 {
		t.Errorf("want 0 events, got %d", len(events))
	}
	if processedPath != "" {
		t.Errorf("want empty processedPath, got %q", processedPath)
	}
	if _, err := os.Stat(buf); err != nil {
		t.Errorf("buffer file should still exist: %v", err)
	}
}

func TestLoadAndRotate_SkipsMalformed(t *testing.T) {
	dir := t.TempDir()
	buf := filepath.Join(dir, "buffer.jsonl")
	valid := `{"session_id":"s1","project_id":"p1","tool_name":"Bash","tool_input_hash":"abc","tool_output_summary":"ok","exit_status":0,"schema_version":1,"timestamp":"2026-01-01T00:00:00Z"}`
	content := strings.Repeat(valid+"\n", 4) + "not-json\n" + valid + "\n"
	os.WriteFile(buf, []byte(content), 0600)

	events, _ := loadAndRotate(buf, 5)
	if len(events) != 5 {
		t.Errorf("want 5 valid events, got %d", len(events))
	}
}

func TestLoadAndRotate_Rotates(t *testing.T) {
	dir := t.TempDir()
	buf := filepath.Join(dir, "buffer.jsonl")
	valid := `{"session_id":"s1","project_id":"p1","tool_name":"Bash","tool_input_hash":"abc","tool_output_summary":"ok","exit_status":0,"schema_version":1,"timestamp":"2026-01-01T00:00:00Z"}`
	content := strings.Repeat(valid+"\n", 5)
	os.WriteFile(buf, []byte(content), 0600)

	_, processedPath := loadAndRotate(buf, 5)
	if processedPath == "" {
		t.Fatal("want non-empty processedPath")
	}
	if _, err := os.Stat(buf); !os.IsNotExist(err) {
		t.Errorf("original buffer should be renamed, stat err: %v", err)
	}
	if _, err := os.Stat(processedPath); err != nil {
		t.Errorf("processed file should exist: %v", err)
	}
	if !strings.Contains(processedPath, ".processed") {
		t.Errorf("processedPath %q should contain .processed", processedPath)
	}
}

func TestLoadAndRotate_Missing(t *testing.T) {
	events, path := loadAndRotate("/nonexistent/buffer.jsonl", 20)
	if len(events) != 0 || path != "" {
		t.Errorf("want empty result for missing file, got %d events, path=%q", len(events), path)
	}
}

func TestGroupBySession(t *testing.T) {
	events := []Event{
		{SessionID: "sess1", ProjectID: "proj1"},
		{SessionID: "sess1", ProjectID: "proj1"},
		{SessionID: "sess2", ProjectID: "proj1"},
		{SessionID: "sess1", ProjectID: "proj2"},
	}
	groups := groupBySession(events)
	if len(groups) != 3 {
		t.Errorf("want 3 groups, got %d", len(groups))
	}
	k1 := sessionKey{"sess1", "proj1"}
	if len(groups[k1]) != 2 {
		t.Errorf("want 2 events for sess1/proj1, got %d", len(groups[k1]))
	}
	k2 := sessionKey{"sess2", "proj1"}
	if len(groups[k2]) != 1 {
		t.Errorf("want 1 event for sess2/proj1, got %d", len(groups[k2]))
	}
	k3 := sessionKey{"sess1", "proj2"}
	if len(groups[k3]) != 1 {
		t.Errorf("want 1 event for sess1/proj2, got %d", len(groups[k3]))
	}
}
