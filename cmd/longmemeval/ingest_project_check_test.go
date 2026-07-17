package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/petersimmons1972/engram/internal/longmemeval"
)

// TestSampleIngestProjectsForCheck_BoundedAndDeterministic verifies sampling
// caps at maxSample and is stable across calls (even spacing over sorted unique).
func TestSampleIngestProjectsForCheck_BoundedAndDeterministic(t *testing.T) {
	entries := make([]longmemeval.IngestEntry, 0, 50)
	for i := 0; i < 50; i++ {
		entries = append(entries, longmemeval.IngestEntry{
			QuestionID: fmt.Sprintf("q%03d", i),
			Project:    fmt.Sprintf("lme-golden-q%03d", i),
			Status:     "done",
		})
	}
	// Duplicates and non-done/empty projects must be ignored.
	entries = append(entries,
		longmemeval.IngestEntry{QuestionID: "dup", Project: "lme-golden-q000", Status: "done"},
		longmemeval.IngestEntry{QuestionID: "err", Project: "lme-err", Status: "error"},
		longmemeval.IngestEntry{QuestionID: "empty-proj", Project: "", Status: "done"},
	)

	got := sampleIngestProjectsForCheck(entries, ingestProjectCheckMaxSample)
	if len(got) != ingestProjectCheckMaxSample {
		t.Fatalf("sample len = %d, want %d", len(got), ingestProjectCheckMaxSample)
	}
	again := sampleIngestProjectsForCheck(entries, ingestProjectCheckMaxSample)
	if strings.Join(got, ",") != strings.Join(again, ",") {
		t.Fatalf("sample not deterministic:\n first=%v\n second=%v", got, again)
	}
	// First/last should be extremes of sorted unique set.
	if got[0] != "lme-golden-q000" {
		t.Errorf("first sample = %q, want lme-golden-q000", got[0])
	}
	if got[len(got)-1] != "lme-golden-q049" {
		t.Errorf("last sample = %q, want lme-golden-q049", got[len(got)-1])
	}
}

// TestValidateIngestProjectsAlive_HappyPath proceeds when every sampled project
// still has memories (#1292).
func TestValidateIngestProjectsAlive_HappyPath(t *testing.T) {
	check := func(_ context.Context, project string) (bool, error) {
		return true, nil
	}
	err := validateIngestProjectsAlive(
		context.Background(),
		check,
		[]string{"lme-a", "lme-b", "lme-c"},
		false,
		"/tmp/checkpoint-ingest.jsonl",
	)
	if err != nil {
		t.Fatalf("validateIngestProjectsAlive: %v", err)
	}
}

// TestValidateIngestProjectsAlive_TransientCheckerErrorRetries verifies a
// transient probe failure does not reject an otherwise healthy checkpoint.
func TestValidateIngestProjectsAlive_TransientCheckerErrorRetries(t *testing.T) {
	var calls int
	check := func(ctx context.Context, _ string) (bool, error) {
		calls++
		if _, ok := ctx.Deadline(); !ok {
			return false, errors.New("probe context has no deadline")
		}
		if calls == 1 {
			return false, errors.New("temporary connection reset")
		}
		return true, nil
	}

	err := validateIngestProjectsAlive(
		context.Background(),
		check,
		[]string{"lme-a"},
		false,
		"/tmp/checkpoint-ingest.jsonl",
	)
	if err != nil {
		t.Fatalf("validateIngestProjectsAlive: %v", err)
	}
	if calls != 2 {
		t.Fatalf("checker calls = %d, want 2", calls)
	}
}

// TestValidateIngestProjectsAlive_AllEmpty_HardFail verifies the all-empty
// case hard-fails, names the cleanup-policy root cause, and includes the
// checkpoint path. No generation should be attempted by callers when this
// returns (#1292).
func TestValidateIngestProjectsAlive_AllEmpty_HardFail(t *testing.T) {
	var calls int
	check := func(_ context.Context, project string) (bool, error) {
		calls++
		return false, nil
	}
	ckpt := "/results/reused/checkpoint-ingest.jsonl"
	err := validateIngestProjectsAlive(
		context.Background(),
		check,
		[]string{"lme-a", "lme-b", "lme-c"},
		false,
		ckpt,
	)
	if err == nil {
		t.Fatal("expected hard-fail error for all-empty projects, got nil")
	}
	msg := err.Error()
	if !strings.Contains(msg, "cleanup-policy=auto") {
		t.Errorf("error should name likely root cause cleanup-policy=auto; got: %s", msg)
	}
	if !strings.Contains(msg, ckpt) {
		t.Errorf("error should name checkpoint path %q; got: %s", ckpt, msg)
	}
	if calls != 3 {
		t.Errorf("checker calls = %d, want 3 (full sample)", calls)
	}
}

// TestValidateIngestProjectsAlive_PartialEmpty_Boundary is the boundary case:
// some but not all sampled projects are empty. Default is hard-fail; the
// explicit --allow-empty-projects override continues after a WARN path
// (caller checks log separately; here we only assert error contract) (#1292).
func TestValidateIngestProjectsAlive_PartialEmpty_Boundary(t *testing.T) {
	check := func(_ context.Context, project string) (bool, error) {
		return project != "lme-empty", nil
	}
	ckpt := "/tmp/ckpt-partial/checkpoint-ingest.jsonl"

	// Without override: partial emptiness hard-fails (same class as all-empty —
	// partial zero-recall still burns LLM compute for the dead projects).
	err := validateIngestProjectsAlive(
		context.Background(),
		check,
		[]string{"lme-alive", "lme-empty"},
		false,
		ckpt,
	)
	if err == nil {
		t.Fatal("partial emptiness without override should hard-fail")
	}
	if !strings.Contains(err.Error(), ckpt) {
		t.Errorf("error missing checkpoint path: %v", err)
	}
	if !strings.Contains(err.Error(), "cleanup-policy=auto") {
		t.Errorf("error missing root-cause hint: %v", err)
	}

	// With override: proceed (nil error).
	err = validateIngestProjectsAlive(
		context.Background(),
		check,
		[]string{"lme-alive", "lme-empty"},
		true, // AllowEmptyProjects
		ckpt,
	)
	if err != nil {
		t.Fatalf("partial emptiness with allow-empty override should proceed, got: %v", err)
	}
}

// TestValidateIngestProjectsAlive_AllEmpty_OverrideStillFails documents that
// 100% empty is always a hard fail — override only covers partial emptiness
// (or we allow override for both?). Per brief: all-empty must hard-fail.
// Decision: --allow-empty-projects overrides BOTH partial and all-empty so
// intentional empty experiments are possible; all-empty without override hard-fails.
// This test locks the override-allows-all-empty behavior.
func TestValidateIngestProjectsAlive_AllEmpty_WithOverrideProceeds(t *testing.T) {
	check := func(_ context.Context, _ string) (bool, error) {
		return false, nil
	}
	err := validateIngestProjectsAlive(
		context.Background(),
		check,
		[]string{"lme-a", "lme-b"},
		true,
		"/tmp/checkpoint-ingest.jsonl",
	)
	if err != nil {
		t.Fatalf("allow-empty override should allow all-empty to proceed, got: %v", err)
	}
}

// TestValidateIngestProjectsAlive_CheckerError propagates a probe failure after
// exactly two attempts rather than treating it as empty or retrying forever.
func TestValidateIngestProjectsAlive_CheckerError(t *testing.T) {
	var calls int
	check := func(_ context.Context, _ string) (bool, error) {
		calls++
		return false, errors.New("connection refused")
	}
	err := validateIngestProjectsAliveWithTiming(
		context.Background(),
		check,
		[]string{"lme-a"},
		false,
		"/tmp/checkpoint-ingest.jsonl",
		time.Second,
		0,
	)
	if err == nil {
		t.Fatal("expected error when checker fails")
	}
	if !strings.Contains(err.Error(), "connection refused") {
		t.Errorf("error should wrap checker failure: %v", err)
	}
	if calls != 2 {
		t.Errorf("checker calls = %d, want exactly 2", calls)
	}
}

// TestValidateIngestProjectsAlive_PerProbeTimeout verifies every attempt gets
// a distinct deadline and a timed-out probe is retried once, then fails.
func TestValidateIngestProjectsAlive_PerProbeTimeout(t *testing.T) {
	var deadlines []time.Time
	check := func(ctx context.Context, _ string) (bool, error) {
		deadline, ok := ctx.Deadline()
		if !ok {
			return false, errors.New("probe context has no deadline")
		}
		deadlines = append(deadlines, deadline)
		<-ctx.Done()
		// A late nominal result must not override the attempt deadline.
		return true, nil
	}

	err := validateIngestProjectsAliveWithTiming(
		context.Background(),
		check,
		[]string{"lme-a"},
		false,
		"/tmp/checkpoint-ingest.jsonl",
		time.Millisecond,
		0,
	)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("error = %v, want context deadline exceeded", err)
	}
	if len(deadlines) != 2 {
		t.Fatalf("probe attempts with deadlines = %d, want 2", len(deadlines))
	}
	if !deadlines[1].After(deadlines[0]) {
		t.Fatalf("probe deadlines = %v, want a fresh deadline per attempt", deadlines)
	}
}

// TestRunRun_IngestProjectsEmpty_NoGenerationCalls is the integration guard:
// when checkpoint-ingest projects are all empty, runRun exits non-zero and
// never calls the generation LLM (#1292).
func TestRunRun_IngestProjectsEmpty_NoGenerationCalls(t *testing.T) {
	dir := t.TempDir()
	items := []longmemeval.Item{
		{QuestionID: "q001", QuestionType: "single-session-user", Question: "Who?", Answer: "A", QuestionDate: "2024-01-01"},
		{QuestionID: "q002", QuestionType: "single-session-user", Question: "What?", Answer: "B", QuestionDate: "2024-01-01"},
	}
	data, err := json.Marshal(items)
	if err != nil {
		t.Fatal(err)
	}
	dataPath := filepath.Join(dir, "data.json")
	if err := os.WriteFile(dataPath, data, 0o644); err != nil {
		t.Fatal(err)
	}
	writeCheckpointFile(t, filepath.Join(dir, "checkpoint-ingest.jsonl"), []any{
		longmemeval.IngestEntry{QuestionID: "q001", Project: "lme-dead-q001", Status: "done", MemoryMap: map[string]string{"m1": "s1"}},
		longmemeval.IngestEntry{QuestionID: "q002", Project: "lme-dead-q002", Status: "done", MemoryMap: map[string]string{"m2": "s2"}},
	})

	// Engram: memory_list always empty (cleaned-up projects).
	engramURL := newTestEngram(t, map[string]func(mcp.CallToolRequest) (*mcp.CallToolResult, error){
		"memory_list": func(req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			resp, _ := json.Marshal(map[string]any{"memories": []any{}, "count": 0})
			return &mcp.CallToolResult{
				Content: []mcp.Content{mcp.TextContent{Type: "text", Text: string(resp)}},
			}, nil
		},
		"memory_recall": func(req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			t.Error("memory_recall must not be called after empty-project fail-fast")
			resp, _ := json.Marshal(map[string]any{"results": []any{}})
			return &mcp.CallToolResult{
				Content: []mcp.Content{mcp.TextContent{Type: "text", Text: string(resp)}},
			}, nil
		},
	})

	var genCalls atomic.Int64
	llmSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		genCalls.Add(1)
		fmt.Fprint(w, `{"choices":[{"message":{"content":"should-not-run"}}]}`)
	}))
	defer llmSrv.Close()

	cfg := &Config{
		DataFile:         dataPath,
		OutDir:           dir,
		ServerURL:        engramURL,
		Workers:          1,
		Retries:          0,
		RunID:            "empty-check",
		LLMBaseURL:       llmSrv.URL,
		LLMModel:         "test",
		ExclusiveBackend: false,
		CleanupPolicy:    CleanupPolicyNever,
	}
	code := runRun(cfg)
	if code == 0 {
		t.Fatalf("runRun exit = 0, want non-zero on all-empty ingest projects")
	}
	if n := genCalls.Load(); n != 0 {
		t.Fatalf("generation LLM called %d times, want 0 (fail-fast before generate)", n)
	}
}

// TestRunRun_IngestProjectsAlive_Proceeds verifies healthy projects pass the
// check and generation runs (#1292 happy path at runRun boundary).
func TestRunRun_IngestProjectsAlive_Proceeds(t *testing.T) {
	dir := t.TempDir()
	items := []longmemeval.Item{
		{QuestionID: "q001", QuestionType: "single-session-user", Question: "Who?", Answer: "A", QuestionDate: "2024-01-01"},
	}
	data, err := json.Marshal(items)
	if err != nil {
		t.Fatal(err)
	}
	dataPath := filepath.Join(dir, "data.json")
	if err := os.WriteFile(dataPath, data, 0o644); err != nil {
		t.Fatal(err)
	}
	writeCheckpointFile(t, filepath.Join(dir, "checkpoint-ingest.jsonl"), []any{
		longmemeval.IngestEntry{QuestionID: "q001", Project: "lme-alive-q001", Status: "done", MemoryMap: map[string]string{"m1": "s1"}},
	})

	engramURL := newTestEngram(t, map[string]func(mcp.CallToolRequest) (*mcp.CallToolResult, error){
		"memory_list": func(req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			resp, _ := json.Marshal(map[string]any{
				"memories": []map[string]any{{"id": "m1", "content": "Alice was there.", "project": "lme-alive-q001"}},
				"count":    1,
			})
			return &mcp.CallToolResult{
				Content: []mcp.Content{mcp.TextContent{Type: "text", Text: string(resp)}},
			}, nil
		},
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
		"memory_delete_project": func(req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return &mcp.CallToolResult{
				Content: []mcp.Content{mcp.TextContent{Type: "text", Text: `{"deleted":true}`}},
			}, nil
		},
	})

	var genCalls atomic.Int64
	llmSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		genCalls.Add(1)
		fmt.Fprint(w, `{"choices":[{"message":{"content":"Alice"}}]}`)
	}))
	defer llmSrv.Close()

	cfg := &Config{
		DataFile:         dataPath,
		OutDir:           dir,
		ServerURL:        engramURL,
		Workers:          1,
		Retries:          0,
		RunID:            "alive-check",
		LLMBaseURL:       llmSrv.URL,
		LLMModel:         "test",
		ExclusiveBackend: false,
		CleanupPolicy:    CleanupPolicyNever,
		// Disable paraphrase passes so Haiku is not required.
		QueryParaphrasePasses: 0,
	}
	code := runRun(cfg)
	if code != 0 {
		t.Fatalf("runRun exit = %d, want 0 when projects have memories", code)
	}
	if n := genCalls.Load(); n == 0 {
		t.Fatal("generation LLM was not called; expected run to proceed")
	}
}
