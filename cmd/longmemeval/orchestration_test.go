package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/petersimmons1972/engram/internal/longmemeval"
)

// TestRunScore_EndToEnd exercises the full runScore pipeline with stub data.
// This test verifies that the orchestration code reads checkpoints, dispatches
// workers, and writes output files correctly.
func TestRunScore_EndToEnd(t *testing.T) {
	dir := t.TempDir()

	// Write a minimal LongMemEval data file.
	items := []longmemeval.Item{
		{
			QuestionID:   "q001",
			QuestionType: "single-session-user",
			Question:     "Who won?",
			Answer:       "Alice",
		},
	}
	dataPath := filepath.Join(dir, "data.json")
	data, _ := json.Marshal(items)
	if err := os.WriteFile(dataPath, data, 0o644); err != nil {
		t.Fatal(err)
	}

	// Write ingest checkpoint (required by runScore but not used for scoring).
	writeCheckpointFile(t, filepath.Join(dir, "checkpoint-ingest.jsonl"), []any{
		longmemeval.IngestEntry{QuestionID: "q001", Project: "lme-x-q001", Status: "done", MemoryMap: map[string]string{"m1": "sid-a"}},
	})

	// Write run checkpoint with a hypothesis.
	writeCheckpointFile(t, filepath.Join(dir, "checkpoint-run.jsonl"), []any{
		longmemeval.RunEntry{QuestionID: "q001", Hypothesis: "Alice", Status: "done", RetrievedIDs: []string{"m1"}},
	})

	// Stub LLM that always returns CORRECT.
	llmSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"choices":[{"message":{"content":"CORRECT\nExact match."}}]}`)
	}))
	defer llmSrv.Close()

	cfg := &Config{
		DataFile:   dataPath,
		OutDir:     dir,
		Workers:    1,
		Retries:    0,
		RunID:      "testrun",
		LLMBaseURL: llmSrv.URL,
		LLMModel:   "test-model",
	}
	runScore(cfg)

	// Verify score_report.json was written.
	reportData, err := os.ReadFile(filepath.Join(dir, "score_report.json"))
	if err != nil {
		t.Fatalf("score_report.json not written: %v", err)
	}
	var report map[string]any
	if err := json.Unmarshal(reportData, &report); err != nil {
		t.Fatalf("parse score_report.json: %v", err)
	}
	overall, ok := report["overall"].(map[string]any)
	if !ok {
		t.Fatalf("overall not a map: %v", report["overall"])
	}
	if int(overall["correct"].(float64)) != 1 {
		t.Errorf("expected 1 correct, got overall=%v", overall)
	}

	// Verify hypotheses.jsonl was written.
	if _, err := os.ReadFile(filepath.Join(dir, "hypotheses.jsonl")); err != nil {
		t.Errorf("hypotheses.jsonl not written: %v", err)
	}

	// Verify retrieval_log.jsonl was written.
	if _, err := os.ReadFile(filepath.Join(dir, "retrieval_log.jsonl")); err != nil {
		t.Errorf("retrieval_log.jsonl not written: %v", err)
	}
}

// TestRunScore_SkipsAlreadyDone verifies checkpoint skipping works.
func TestRunScore_SkipsAlreadyDone(t *testing.T) {
	dir := t.TempDir()

	items := []longmemeval.Item{
		{QuestionID: "q001", QuestionType: "single-session-user", Question: "?", Answer: "A"},
	}
	data, _ := json.Marshal(items)
	if err := os.WriteFile(filepath.Join(dir, "data.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}
	writeCheckpointFile(t, filepath.Join(dir, "checkpoint-ingest.jsonl"), []any{
		longmemeval.IngestEntry{QuestionID: "q001", Status: "done"},
	})
	writeCheckpointFile(t, filepath.Join(dir, "checkpoint-run.jsonl"), []any{
		longmemeval.RunEntry{QuestionID: "q001", Hypothesis: "A", Status: "done"},
	})
	// Mark q001 as already scored.
	writeCheckpointFile(t, filepath.Join(dir, "checkpoint-score.jsonl"), []any{
		longmemeval.ScoreEntry{QuestionID: "q001", ScoreLabel: "CORRECT", Status: "done"},
	})

	calls := 0
	llmSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		fmt.Fprint(w, `{"choices":[{"message":{"content":"CORRECT\nOK"}}]}`)
	}))
	defer llmSrv.Close()

	cfg := &Config{
		DataFile:   filepath.Join(dir, "data.json"),
		OutDir:     dir,
		Workers:    1,
		Retries:    0,
		RunID:      "testrun",
		LLMBaseURL: llmSrv.URL,
		LLMModel:   "test-model",
	}
	runScore(cfg)

	// The LLM should NOT have been called since q001 is already done.
	if calls != 0 {
		t.Errorf("LLM called %d times, expected 0 (checkpoint should skip done items)", calls)
	}
}

func TestRunScore_ReportIncludesExpectedDenominatorForPartialRun(t *testing.T) {
	dir := t.TempDir()

	items := []longmemeval.Item{
		{QuestionID: "q001", QuestionType: "single-session-user", Question: "?", Answer: "A"},
		{QuestionID: "q002", QuestionType: "single-session-user", Question: "?", Answer: "B"},
	}
	data, _ := json.Marshal(items)
	dataPath := filepath.Join(dir, "data.json")
	if err := os.WriteFile(dataPath, data, 0o644); err != nil {
		t.Fatal(err)
	}
	writeCheckpointFile(t, filepath.Join(dir, "checkpoint-ingest.jsonl"), []any{
		longmemeval.IngestEntry{QuestionID: "q001", Status: "done"},
		longmemeval.IngestEntry{QuestionID: "q002", Status: "done"},
	})
	writeCheckpointFile(t, filepath.Join(dir, "checkpoint-run.jsonl"), []any{
		longmemeval.RunEntry{QuestionID: "q001", Hypothesis: "A", Status: "done"},
	})
	writeCheckpointFile(t, filepath.Join(dir, "checkpoint-score.jsonl"), []any{
		longmemeval.ScoreEntry{QuestionID: "q001", QuestionType: "single-session-user", ScoreLabel: "CORRECT", Status: "done"},
	})

	cfg := &Config{
		DataFile: dataPath,
		OutDir:   dir,
		Workers:  1,
		RunID:    "partial-run",
	}
	if exit := runScore(cfg); exit != 0 {
		t.Fatalf("runScore exit = %d, want 0 for resume-only partial report", exit)
	}

	data, err := os.ReadFile(filepath.Join(dir, "score_report.json"))
	if err != nil {
		t.Fatalf("read score_report.json: %v", err)
	}
	var report map[string]any
	if err := json.Unmarshal(data, &report); err != nil {
		t.Fatalf("parse score_report.json: %v", err)
	}
	if got := int(report["expected_total"].(float64)); got != 2 {
		t.Fatalf("expected_total = %d, want 2", got)
	}
	if got := int(report["completed_run_total"].(float64)); got != 1 {
		t.Fatalf("completed_run_total = %d, want 1", got)
	}
	if got := int(report["completed_score_total"].(float64)); got != 1 {
		t.Fatalf("completed_score_total = %d, want 1", got)
	}
	if complete, ok := report["complete"].(bool); !ok || complete {
		t.Fatalf("complete = %v (%T), want false", report["complete"], report["complete"])
	}
}

func TestRunScore_WritesRunManifest(t *testing.T) {
	dir := t.TempDir()

	items := []longmemeval.Item{
		{QuestionID: "q001", QuestionType: "single-session-user", Question: "?", Answer: "A"},
	}
	data, _ := json.Marshal(items)
	dataPath := filepath.Join(dir, "data.json")
	if err := os.WriteFile(dataPath, data, 0o644); err != nil {
		t.Fatal(err)
	}
	writeCheckpointFile(t, filepath.Join(dir, "checkpoint-ingest.jsonl"), []any{
		longmemeval.IngestEntry{QuestionID: "q001", Status: "done"},
	})
	writeCheckpointFile(t, filepath.Join(dir, "checkpoint-run.jsonl"), []any{
		longmemeval.RunEntry{QuestionID: "q001", Hypothesis: "A", Status: "done"},
	})
	writeCheckpointFile(t, filepath.Join(dir, "checkpoint-score.jsonl"), []any{
		longmemeval.ScoreEntry{QuestionID: "q001", QuestionType: "single-session-user", ScoreLabel: "CORRECT", Status: "done"},
	})

	cfg := &Config{
		DataFile: dataPath,
		OutDir:   dir,
		Workers:  1,
		RunID:    "manifest-run",
	}
	if exit := runScore(cfg); exit != 0 {
		t.Fatalf("runScore exit = %d, want 0", exit)
	}

	data, err := os.ReadFile(filepath.Join(dir, "run_manifest.json"))
	if err != nil {
		t.Fatalf("read run_manifest.json: %v", err)
	}
	var manifest map[string]any
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatalf("parse run_manifest.json: %v", err)
	}
	if manifest["run_id"] != "manifest-run" {
		t.Fatalf("run_id = %v, want manifest-run", manifest["run_id"])
	}
	if manifest["stage"] != "score" {
		t.Fatalf("stage = %v, want score", manifest["stage"])
	}
	if got := int(manifest["expected_total"].(float64)); got != 1 {
		t.Fatalf("expected_total = %d, want 1", got)
	}
	if complete, ok := manifest["complete"].(bool); !ok || !complete {
		t.Fatalf("complete = %v (%T), want true", manifest["complete"], manifest["complete"])
	}
}

func TestRunScore_AllAttemptedRowsFailReturnsNonZero(t *testing.T) {
	dir := t.TempDir()

	items := []longmemeval.Item{
		{QuestionID: "q001", QuestionType: "single-session-user", Question: "?", Answer: "A"},
		{QuestionID: "q002", QuestionType: "single-session-user", Question: "?", Answer: "B"},
	}
	data, _ := json.Marshal(items)
	dataPath := filepath.Join(dir, "data.json")
	if err := os.WriteFile(dataPath, data, 0o644); err != nil {
		t.Fatal(err)
	}
	writeCheckpointFile(t, filepath.Join(dir, "checkpoint-ingest.jsonl"), []any{
		longmemeval.IngestEntry{QuestionID: "q001", Status: "done"},
		longmemeval.IngestEntry{QuestionID: "q002", Status: "done"},
	})
	writeCheckpointFile(t, filepath.Join(dir, "checkpoint-run.jsonl"), []any{
		longmemeval.RunEntry{QuestionID: "q001", Hypothesis: "wrong", Status: "done"},
		longmemeval.RunEntry{QuestionID: "q002", Hypothesis: "wrong", Status: "done"},
	})

	llmSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "judge unavailable", http.StatusInternalServerError)
	}))
	defer llmSrv.Close()

	cfg := &Config{
		DataFile:   dataPath,
		OutDir:     dir,
		Workers:    1,
		Retries:    0,
		RunID:      "testrun",
		LLMBaseURL: llmSrv.URL,
		LLMModel:   "test-model",
	}
	if exit := runScore(cfg); exit == 0 {
		t.Fatal("runScore returned 0 after every attempted score row failed")
	}
}

// writeCheckpointFile writes a JSONL checkpoint file from a slice of values.
func writeCheckpointFile(t *testing.T, path string, items []any) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("create %s: %v", path, err)
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	for _, item := range items {
		if err := enc.Encode(item); err != nil {
			t.Fatalf("encode checkpoint: %v", err)
		}
	}
}
