package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/petersimmons1972/engram/internal/longmemeval"
)

// newTestEngram builds a stub MCP server for cmd-level tests.
func newTestEngram(t *testing.T, handlers map[string]func(mcp.CallToolRequest) (*mcp.CallToolResult, error)) string {
	t.Helper()
	mcpServer := server.NewMCPServer("stub-engram", "1.0.0", server.WithToolCapabilities(true))
	for name, h := range handlers {
		toolName := name
		handler := h
		mcpServer.AddTool(mcp.NewTool(toolName), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return handler(req)
		})
	}
	ts := server.NewTestServer(mcpServer)
	t.Cleanup(ts.Close)
	return ts.URL
}

// TestScoreOne_HappyPath verifies that scoreOne returns a done entry on success.
func TestScoreOne_HappyPath(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"choices":[{"message":{"content":"CORRECT\nExact match."}}]}`)
	}))
	defer srv.Close()

	cfg := &Config{LLMBaseURL: srv.URL, LLMModel: "test", Retries: 0}
	item := longmemeval.Item{
		QuestionID:   "q-score",
		QuestionType: "single-session-user",
		Question:     "Who was there?",
		Answer:       "Alice",
	}
	run := longmemeval.RunEntry{QuestionID: "q-score", Hypothesis: "Alice", Status: "done"}

	entry := scoreOne(context.Background(), cfg, item, run)
	if entry.Status != "done" {
		t.Errorf("scoreOne status = %q, want done", entry.Status)
	}
	if entry.ScoreLabel != "CORRECT" {
		t.Errorf("scoreOne ScoreLabel = %q, want CORRECT", entry.ScoreLabel)
	}
}

// TestScoreOne_LLMError_ReturnsErrorEntry verifies error propagation from scoreOne.
func TestScoreOne_LLMError_ReturnsErrorEntry(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	cfg := &Config{LLMBaseURL: srv.URL, LLMModel: "test", Retries: 0}
	item := longmemeval.Item{QuestionID: "q-fail", Question: "?", Answer: "X"}
	run := longmemeval.RunEntry{QuestionID: "q-fail", Hypothesis: "Y", Status: "done"}

	entry := scoreOne(context.Background(), cfg, item, run)
	if entry.Status != "error" {
		t.Errorf("scoreOne status = %q, want error on LLM failure", entry.Status)
	}
	if entry.Error == "" {
		t.Error("scoreOne error field should not be empty on failure")
	}
}

// TestRunOne_RecallError_ReturnsErrorEntry verifies recall errors set error= field.
func TestRunOne_RecallError_ReturnsErrorEntry(t *testing.T) {
	url := newTestEngram(t, map[string]func(mcp.CallToolRequest) (*mcp.CallToolResult, error){
		"memory_recall": func(req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return nil, fmt.Errorf("recall: connection refused")
		},
	})
	ctx := context.Background()
	c, err := longmemeval.Connect(ctx, url, "")
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer c.Close()

	item := longmemeval.Item{QuestionID: "q-recall-fail", Question: "What happened?"}
	ingest := longmemeval.IngestEntry{QuestionID: "q-recall-fail", Project: "lme-r-q-recall-fail"}

	entry := runOne(ctx, &Config{Retries: 0}, c, item, ingest)
	if entry.Status != "error" {
		t.Errorf("runOne status = %q, want error", entry.Status)
	}
	if !strings.Contains(entry.Error, "recall") {
		t.Errorf("error field should mention recall: %q", entry.Error)
	}
}

// TestRunOne_GenerateError_ReturnsErrorEntry verifies generate errors set error= field.
func TestRunOne_GenerateError_ReturnsErrorEntry(t *testing.T) {
	// Recall returns empty results, generate fails.
	url := newTestEngram(t, map[string]func(mcp.CallToolRequest) (*mcp.CallToolResult, error){
		"memory_recall": func(req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			resp, _ := json.Marshal(map[string]any{"results": []any{}})
			return &mcp.CallToolResult{
				Content: []mcp.Content{mcp.TextContent{Type: "text", Text: string(resp)}},
			}, nil
		},
	})
	// LLM server that always returns 500.
	llmSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer llmSrv.Close()

	ctx := context.Background()
	c, err := longmemeval.Connect(ctx, url, "")
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer c.Close()

	item := longmemeval.Item{QuestionID: "q-gen-fail", Question: "?", QuestionDate: "2024-01-01"}
	ingest := longmemeval.IngestEntry{QuestionID: "q-gen-fail", Project: "lme-r-q-gen-fail"}
	cfg := &Config{LLMBaseURL: llmSrv.URL, LLMModel: "test", Retries: 0}

	entry := runOne(ctx, cfg, c, item, ingest)
	if entry.Status != "error" {
		t.Errorf("runOne status = %q, want error on generate failure", entry.Status)
	}
	if !strings.Contains(entry.Error, "generate") {
		t.Errorf("error field should mention generate: %q", entry.Error)
	}
}

// TestRunOne_HappyPath verifies the full runOne path returns a hypothesis.
func TestRunOne_HappyPath(t *testing.T) {
	url := newTestEngram(t, map[string]func(mcp.CallToolRequest) (*mcp.CallToolResult, error){
		"memory_recall": func(req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			resp, _ := json.Marshal(map[string]any{
				"results": []map[string]any{{"memory": map[string]any{"id": "m1"}, "score": 0.9}},
			})
			return &mcp.CallToolResult{
				Content: []mcp.Content{mcp.TextContent{Type: "text", Text: string(resp)}},
			}, nil
		},
		"memory_fetch": func(req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			resp, _ := json.Marshal(map[string]any{
				"memory": map[string]any{"content": "Alice was there."},
			})
			return &mcp.CallToolResult{
				Content: []mcp.Content{mcp.TextContent{Type: "text", Text: string(resp)}},
			}, nil
		},
	})

	llmSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"choices":[{"message":{"content":"Alice"}}]}`)
	}))
	defer llmSrv.Close()

	ctx := context.Background()
	c, err := longmemeval.Connect(ctx, url, "")
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer c.Close()

	item := longmemeval.Item{QuestionID: "q-ok", Question: "Who was there?", QuestionDate: "2024-01-15"}
	ingest := longmemeval.IngestEntry{
		QuestionID: "q-ok",
		Project:    "lme-r-q-ok",
		MemoryMap:  map[string]string{"m1": "sid-1"},
	}
	cfg := &Config{LLMBaseURL: llmSrv.URL, LLMModel: "test", Retries: 0}

	entry := runOne(ctx, cfg, c, item, ingest)
	if entry.Status != "done" {
		t.Errorf("runOne status = %q error=%q, want done", entry.Status, entry.Error)
	}
	if entry.Hypothesis == "" {
		t.Error("hypothesis should not be empty on success")
	}
}

// TestRunEntryLogLine_BothStatuses verifies Bug #643 fix: error cause appears in log.
func TestRunEntryLogLine_BothStatuses(t *testing.T) {
	errEntry := longmemeval.RunEntry{
		QuestionID: "q-err",
		Status:     "error",
		Error:      "recall: connection refused",
	}
	line := runEntryLogLine(errEntry)
	if !strings.Contains(line, "status=error") {
		t.Errorf("log line missing status=error: %q", line)
	}
	if !strings.Contains(line, "recall: connection refused") {
		t.Errorf("log line missing error cause (#643 regression): %q", line)
	}

	doneEntry := longmemeval.RunEntry{
		QuestionID: "q-done",
		Hypothesis: "The answer.",
		Status:     "done",
	}
	line = runEntryLogLine(doneEntry)
	if !strings.Contains(line, "status=done") {
		t.Errorf("log line missing status=done: %q", line)
	}
	if strings.Contains(line, "error=") {
		t.Errorf("log line should not contain error= on success: %q", line)
	}
}
