package main

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	mcpmcp "github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
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

// ── Engram client tests ───────────────────────────────────────────────────────

func newTestEngramServer(t *testing.T) (engramAPI, func()) {
	t.Helper()

	mcpServer := server.NewMCPServer("test-engram", "1.0.0",
		server.WithToolCapabilities(true),
	)

	mcpServer.AddTool(mcpmcp.NewTool("memory_episode_start"), func(ctx context.Context, req mcpmcp.CallToolRequest) (*mcpmcp.CallToolResult, error) {
		return &mcpmcp.CallToolResult{Content: []mcpmcp.Content{
			mcpmcp.TextContent{Type: "text", Text: `{"episode_id":"ep-test-123"}`},
		}}, nil
	})
	mcpServer.AddTool(mcpmcp.NewTool("memory_ingest"), func(ctx context.Context, req mcpmcp.CallToolRequest) (*mcpmcp.CallToolResult, error) {
		return &mcpmcp.CallToolResult{Content: []mcpmcp.Content{
			mcpmcp.TextContent{Type: "text", Text: `{}`},
		}}, nil
	})
	mcpServer.AddTool(mcpmcp.NewTool("memory_episode_end"), func(ctx context.Context, req mcpmcp.CallToolRequest) (*mcpmcp.CallToolResult, error) {
		return &mcpmcp.CallToolResult{Content: []mcpmcp.Content{
			mcpmcp.TextContent{Type: "text", Text: `{}`},
		}}, nil
	})
	mcpServer.AddTool(mcpmcp.NewTool("memory_store"), func(ctx context.Context, req mcpmcp.CallToolRequest) (*mcpmcp.CallToolResult, error) {
		return &mcpmcp.CallToolResult{Content: []mcpmcp.Content{
			mcpmcp.TextContent{Type: "text", Text: `{"id":"mem-abc"}`},
		}}, nil
	})
	mcpServer.AddTool(mcpmcp.NewTool("memory_recall"), func(ctx context.Context, req mcpmcp.CallToolRequest) (*mcpmcp.CallToolResult, error) {
		return &mcpmcp.CallToolResult{Content: []mcpmcp.Content{
			mcpmcp.TextContent{Type: "text", Text: `{"memories":[{"id":"mem-abc","importance":0.5,"tags":["instinct","sig-test"]}]}`},
		}}, nil
	})
	mcpServer.AddTool(mcpmcp.NewTool("memory_correct"), func(ctx context.Context, req mcpmcp.CallToolRequest) (*mcpmcp.CallToolResult, error) {
		return &mcpmcp.CallToolResult{Content: []mcpmcp.Content{
			mcpmcp.TextContent{Type: "text", Text: `{}`},
		}}, nil
	})

	ts := server.NewTestServer(mcpServer)
	e, err := newSSEEngram(ts.URL, "")
	if err != nil {
		t.Fatalf("newSSEEngram: %v", err)
	}
	ctx := context.Background()
	if err := e.connect(ctx); err != nil {
		t.Fatalf("connect: %v", err)
	}
	return e, func() {
		e.close()
		ts.Close()
	}
}

func TestEngramClient_EpisodeStartAndEnd(t *testing.T) {
	e, cleanup := newTestEngramServer(t)
	defer cleanup()

	epID, err := e.episodeStart(context.Background(), "sess1", "proj1")
	if err != nil {
		t.Fatalf("episodeStart: %v", err)
	}
	if epID != "ep-test-123" {
		t.Errorf("episodeID = %q, want ep-test-123", epID)
	}
	if err := e.episodeEnd(context.Background(), epID); err != nil {
		t.Errorf("episodeEnd: %v", err)
	}
}

func TestEngramClient_Ingest(t *testing.T) {
	e, cleanup := newTestEngramServer(t)
	defer cleanup()

	ev := Event{SessionID: "s1", ProjectID: "p1", ToolName: "Bash", ToolOutputSummary: "done", ExitStatus: 0}
	if err := e.ingest(context.Background(), ev, "p1", "s1"); err != nil {
		t.Fatalf("ingest: %v", err)
	}
}

func TestEngramClient_StoreAndRecall(t *testing.T) {
	e, cleanup := newTestEngramServer(t)
	defer cleanup()

	p := Pattern{Type: "workflow", Description: "test", Domain: "git", Evidence: "seen 2x", TagSignature: "sig-test"}
	id, err := e.store(context.Background(), p, 0.3, "proj1")
	if err != nil {
		t.Fatalf("store: %v", err)
	}
	if id != "mem-abc" {
		t.Errorf("stored id = %q, want mem-abc", id)
	}

	r, err := e.recall(context.Background(), "sig-test", "proj1")
	if err != nil {
		t.Fatalf("recall: %v", err)
	}
	if r == nil {
		t.Fatal("recall returned nil, want match")
	}
	if r.id != "mem-abc" {
		t.Errorf("recall id = %q, want mem-abc", r.id)
	}
}

func TestEngramClient_Correct(t *testing.T) {
	e, cleanup := newTestEngramServer(t)
	defer cleanup()

	if err := e.correct(context.Background(), "mem-abc", 0.7); err != nil {
		t.Fatalf("correct: %v", err)
	}
}

// ── Haiku client tests ────────────────────────────────────────────────────────

func TestCallHaiku_ValidPatterns(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"content":[{"type":"text","text":"[{\"type\":\"workflow\",\"description\":\"User runs tests after edits\",\"domain\":\"testing\",\"evidence\":\"edit then test 3x\",\"tag_signature\":\"sig-edit-test\"}]"}],"usage":{"input_tokens":10,"output_tokens":50}}`)
	}))
	defer srv.Close()

	events := []Event{{ToolName: "Edit", ToolOutputSummary: "saved", ExitStatus: 0}}
	patterns := callHaiku(context.Background(), "sk-ant-fake", events, srv.URL+"/v1/messages")
	if len(patterns) != 1 {
		t.Fatalf("want 1 pattern, got %d", len(patterns))
	}
	if patterns[0].TagSignature != "sig-edit-test" {
		t.Errorf("tag_signature = %q, want sig-edit-test", patterns[0].TagSignature)
	}
}

func TestCallHaiku_SkipsInvalidPatterns(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"content":[{"type":"text","text":"[{\"type\":\"workflow\",\"description\":\"ok\",\"domain\":\"git\",\"evidence\":\"x\",\"tag_signature\":\"sig-ok\"},{\"type\":\"correction\",\"description\":\"bad\",\"domain\":\"bash\",\"evidence\":\"y\"}]"}],"usage":{"input_tokens":5,"output_tokens":20}}`)
	}))
	defer srv.Close()

	events := []Event{{ToolName: "Bash", ToolOutputSummary: "err", ExitStatus: 1}}
	patterns := callHaiku(context.Background(), "sk-ant-fake", events, srv.URL+"/v1/messages")
	if len(patterns) != 1 {
		t.Fatalf("want 1 valid pattern (invalid skipped), got %d", len(patterns))
	}
	if patterns[0].TagSignature != "sig-ok" {
		t.Errorf("tag_signature = %q, want sig-ok", patterns[0].TagSignature)
	}
}

func TestCallHaiku_APIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
	}))
	defer srv.Close()

	events := []Event{{ToolName: "Bash", ToolOutputSummary: "ok", ExitStatus: 0}}
	patterns := callHaiku(context.Background(), "sk-ant-fake", events, srv.URL+"/v1/messages")
	if len(patterns) != 0 {
		t.Errorf("want 0 patterns on API error, got %d", len(patterns))
	}
}

func TestCallHaiku_EmptyResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"content":[{"type":"text","text":"[]"}],"usage":{"input_tokens":5,"output_tokens":2}}`)
	}))
	defer srv.Close()

	patterns := callHaiku(context.Background(), "sk-ant-fake", []Event{}, srv.URL+"/v1/messages")
	if len(patterns) != 0 {
		t.Errorf("want 0 patterns for empty response, got %d", len(patterns))
	}
}
