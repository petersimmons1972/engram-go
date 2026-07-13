package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
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
	ts := server.NewTestStreamableHTTPServer(mcpServer)
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

func TestGenerateHypothesis_VLLMDefaultPathIsByteForByteUnchanged(t *testing.T) {
	type call struct {
		prompt  string
		baseURL string
		model   string
		retries int
		opts    longmemeval.OAIOptions
	}
	calls := make([]call, 0, 2)
	deps := generatorDeps{
		generateOAI: func(
			_ context.Context,
			prompt string,
			baseURL string,
			model string,
			retries int,
			opts longmemeval.OAIOptions,
		) (string, error) {
			calls = append(calls, call{
				prompt:  prompt,
				baseURL: baseURL,
				model:   model,
				retries: retries,
				opts:    opts,
			})
			return "answer", nil
		},
		generateClaude: func(context.Context, string, string, int) (string, error) {
			t.Fatal("default vLLM path called Claude generator")
			return "", nil
		},
		generateCodex: func(context.Context, string, string) (string, error) {
			t.Fatal("default vLLM path called Codex generator")
			return "", nil
		},
	}

	base := Config{
		LLMBaseURL:     "http://vllm.test/v1",
		LLMModel:       "test-model",
		Retries:        2,
		EnableThinking: true,
		LLMMaxTokens:   4096,
		LLMApiKey:      "test-key",
	}
	for _, generator := range []string{"", "vllm"} {
		cfg := base
		cfg.Generator = generator
		got, err := generateHypothesisWithDeps(context.Background(), &cfg, "same prompt bytes", deps)
		if err != nil {
			t.Fatalf("generateHypothesis(generator=%q) error = %v", generator, err)
		}
		if got != "answer" {
			t.Fatalf("generateHypothesis(generator=%q) = %q, want answer", generator, got)
		}
	}

	if len(calls) != 2 {
		t.Fatalf("vLLM generator call count = %d, want 2", len(calls))
	}
	if !reflect.DeepEqual(calls[0], calls[1]) {
		t.Fatalf("legacy/default vLLM call changed:\nlegacy: %#v\nexplicit: %#v", calls[0], calls[1])
	}
	if calls[0].prompt != "same prompt bytes" {
		t.Fatalf("vLLM prompt = %q, want byte-identical input", calls[0].prompt)
	}
}

func TestGenerateHypothesis_CodexRoutesIdenticalPrompt(t *testing.T) {
	binDir := t.TempDir()
	promptPath := filepath.Join(t.TempDir(), "prompt")
	// Prompt now arrives on stdin (argv would exceed ARG_MAX with full context).
	script := "#!/bin/sh\ncat > \"$CODEX_PROMPT_FILE\"\nprintf 'codex\\nfrontier answer\\ntokens used\\n1\\n'\n"
	if err := os.WriteFile(filepath.Join(binDir, "codex"), []byte(script), 0o700); err != nil {
		t.Fatalf("write fake codex: %v", err)
	}
	// binDir first shadows any real codex; system PATH kept so `cat` resolves.
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("CODEX_PROMPT_FILE", promptPath)

	const prompt = "same assembled prompt\nwith context"
	cfg := &Config{
		Generator:      "codex",
		GeneratorModel: "gpt-5.6-sol",
		LLMBaseURL:     "http://127.0.0.1:1",
	}
	got, err := generateHypothesis(context.Background(), cfg, prompt)
	if err != nil {
		t.Fatalf("generateHypothesis() error = %v", err)
	}
	if got != "frontier answer" {
		t.Fatalf("generateHypothesis() = %q, want frontier answer", got)
	}
	forwarded, err := os.ReadFile(promptPath)
	if err != nil {
		t.Fatalf("read forwarded prompt: %v", err)
	}
	if string(forwarded) != prompt {
		t.Fatalf("codex prompt = %q, want identical %q", forwarded, prompt)
	}
}

func TestRunOneOracle_UsesSelectedAnswerGenerator(t *testing.T) {
	src, err := os.ReadFile("atom_oracle.go")
	if err != nil {
		t.Fatalf("read atom_oracle.go: %v", err)
	}
	text := string(src)
	if !strings.Contains(text, "generateHypothesis(ctx, cfg, prompt)") {
		t.Fatal("atom-oracle final answer bypasses --generator selection")
	}
}

func TestShouldLockGeneratorBackend(t *testing.T) {
	tests := []struct {
		name string
		cfg  Config
		want bool
	}{
		{
			name: "default vLLM with URL locks",
			cfg:  Config{LLMBaseURL: "http://vllm.test/v1"},
			want: true,
		},
		{
			name: "explicit vLLM with URL locks",
			cfg:  Config{Generator: "vllm", LLMBaseURL: "http://vllm.test/v1"},
			want: true,
		},
		{
			name: "Codex without oracle does not lock unused vLLM",
			cfg:  Config{Generator: "codex", LLMBaseURL: "http://vllm.test/v1"},
			want: false,
		},
		{
			name: "Codex oracle locks vLLM extraction backend",
			cfg:  Config{Generator: "codex", AtomOracle: true, LLMBaseURL: "http://vllm.test/v1"},
			want: true,
		},
		{
			name: "missing URL never locks",
			cfg:  Config{Generator: "vllm"},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := shouldLockGeneratorBackend(&tt.cfg); got != tt.want {
				t.Fatalf("shouldLockGeneratorBackend() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRunOne_DualPreferenceRecall_OnlyRunsForInferredPreferenceQuestions(t *testing.T) {
	var queries []string
	url := newTestEngram(t, map[string]func(mcp.CallToolRequest) (*mcp.CallToolResult, error){
		"memory_recall": func(req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			query, _ := req.GetArguments()["query"].(string)
			queries = append(queries, query)
			resp, _ := json.Marshal(map[string]any{
				"handles": []map[string]any{{"id": "m1", "score": 0.9}},
			})
			return &mcp.CallToolResult{
				Content: []mcp.Content{mcp.TextContent{Type: "text", Text: string(resp)}},
			}, nil
		},
		"memory_fetch": func(req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			resp, _ := json.Marshal(map[string]any{
				"memory": map[string]any{"content": "Session date: 2024-01-10\nNeutral context."},
			})
			return &mcp.CallToolResult{
				Content: []mcp.Content{mcp.TextContent{Type: "text", Text: string(resp)}},
			}, nil
		},
	})
	llmSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"choices":[{"message":{"content":"answer"}}]}`)
	}))
	defer llmSrv.Close()

	ctx := context.Background()
	c, err := longmemeval.Connect(ctx, url, "")
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer c.Close()

	item := longmemeval.Item{
		QuestionID:   "q-neutral-pref-type",
		QuestionType: "single-session-preference",
		Question:     "What happened last week?",
		QuestionDate: "2024-01-15",
	}
	ingest := longmemeval.IngestEntry{QuestionID: item.QuestionID, Project: "lme-r-q-neutral-pref-type", MemoryMap: map[string]string{"m1": "sid-1"}}
	cfg := &Config{LLMBaseURL: llmSrv.URL, LLMModel: "test", Retries: 0, DualPreferenceRecall: true}

	entry := runOne(ctx, cfg, c, item, ingest)
	if entry.Status != "done" {
		t.Fatalf("runOne status = %q error=%q, want done", entry.Status, entry.Error)
	}
	if len(queries) != 1 {
		t.Fatalf("memory_recall call count = %d, want 1 for non-inferred preference question; queries=%v", len(queries), queries)
	}
}

func TestRunOne_DualPreferenceRecall_RanksByMaxScoreAndUsesCleanAnchor(t *testing.T) {
	var queries []string
	url := newTestEngram(t, map[string]func(mcp.CallToolRequest) (*mcp.CallToolResult, error){
		"memory_recall": func(req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			query, _ := req.GetArguments()["query"].(string)
			queries = append(queries, query)

			var payload map[string]any
			switch query {
			case "user preference a conference about AI in healthcare? like dislike use avoid":
				payload = map[string]any{
					"handles": []map[string]any{
						{"id": "m1", "score": 0.93},
						{"id": "m2", "score": 0.61},
					},
				}
			case "conference AI healthcare":
				payload = map[string]any{
					"handles": []map[string]any{
						{"id": "m2", "score": 0.97},
						{"id": "m3", "score": 0.88},
					},
				}
			default:
				t.Fatalf("unexpected recall query: %q", query)
			}

			resp, _ := json.Marshal(payload)
			return &mcp.CallToolResult{
				Content: []mcp.Content{mcp.TextContent{Type: "text", Text: string(resp)}},
			}, nil
		},
		"memory_fetch": func(req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			id, _ := req.GetArguments()["id"].(string)
			resp, _ := json.Marshal(map[string]any{
				"memory": map[string]any{"content": "Session date: 2024-01-10\nContext for " + id},
			})
			return &mcp.CallToolResult{
				Content: []mcp.Content{mcp.TextContent{Type: "text", Text: string(resp)}},
			}, nil
		},
	})
	llmSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"choices":[{"message":{"content":"answer"}}]}`)
	}))
	defer llmSrv.Close()

	ctx := context.Background()
	c, err := longmemeval.Connect(ctx, url, "")
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer c.Close()

	item := longmemeval.Item{
		QuestionID:   "q-pref",
		QuestionType: "single-session-preference",
		Question:     "Can you recommend a conference about AI in healthcare?",
		QuestionDate: "2024-01-15",
	}
	ingest := longmemeval.IngestEntry{
		QuestionID: item.QuestionID,
		Project:    "lme-r-q-pref",
		MemoryMap:  map[string]string{"m1": "sid-1", "m2": "sid-2", "m3": "sid-3"},
	}
	cfg := &Config{LLMBaseURL: llmSrv.URL, LLMModel: "test", Retries: 0, DualPreferenceRecall: true}

	entry := runOne(ctx, cfg, c, item, ingest)
	if entry.Status != "done" {
		t.Fatalf("runOne status = %q error=%q, want done", entry.Status, entry.Error)
	}
	if len(queries) != 2 {
		t.Fatalf("memory_recall call count = %d, want 2; queries=%v", len(queries), queries)
	}
	wantQueries := []string{
		"user preference a conference about AI in healthcare? like dislike use avoid",
		"conference AI healthcare",
	}
	if !reflect.DeepEqual(queries, wantQueries) {
		t.Fatalf("memory_recall queries = %v, want %v", queries, wantQueries)
	}
	wantIDs := []string{"m2", "m1", "m3"}
	if !reflect.DeepEqual(entry.RetrievedIDs, wantIDs) {
		t.Fatalf("retrieved IDs = %v, want %v", entry.RetrievedIDs, wantIDs)
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
