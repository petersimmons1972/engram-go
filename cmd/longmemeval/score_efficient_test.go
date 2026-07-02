package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/petersimmons1972/engram/internal/longmemeval"
)

func TestOllaHealthCheck_unreachable(t *testing.T) {
	if ollaHealthCheck("http://127.0.0.1:19999/v1") {
		t.Error("unreachable endpoint must return false")
	}
}

func TestBuildPreserveSkipSet_preserveMode(t *testing.T) {
	labels := map[string]string{"q1": "CORRECT", "q2": "PARTIALLY_CORRECT", "q3": "INCORRECT"}
	skip, retry := buildPreserveSkipSet(labels, true)
	if !skip["q1"] {
		t.Error("CORRECT must be skipped")
	}
	if skip["q2"] {
		t.Error("PARTIALLY_CORRECT must not be skipped")
	}
	if !retry["q2"] {
		t.Error("PARTIALLY_CORRECT must be in retry set")
	}
	if !retry["q3"] {
		t.Error("INCORRECT must be in retry set")
	}
}

func TestBuildPreserveSkipSet_forceRescore(t *testing.T) {
	labels := map[string]string{"q1": "CORRECT"}
	skip, _ := buildPreserveSkipSet(labels, false)
	if skip["q1"] {
		t.Error("force-rescore must not skip CORRECT")
	}
}

func TestRunScoreEfficient_WritesRetrievalLogFromIngestCheckpoint(t *testing.T) {
	dir := t.TempDir()
	items := []longmemeval.Item{
		{
			QuestionID:       "q001",
			QuestionType:     "single-session-user",
			Question:         "Who won?",
			Answer:           "Alice",
			AnswerSessionIDs: []string{"sid-a"},
		},
	}
	data, _ := json.Marshal(items)
	dataPath := filepath.Join(dir, "data.json")
	if err := os.WriteFile(dataPath, data, 0o644); err != nil {
		t.Fatal(err)
	}
	writeCheckpointFile(t, filepath.Join(dir, "checkpoint-ingest.jsonl"), []any{
		longmemeval.IngestEntry{
			QuestionID: "q001",
			Status:     "done",
			MemoryMap:  map[string]string{"m1": "sid-a"},
		},
	})
	writeCheckpointFile(t, filepath.Join(dir, "checkpoint-run.jsonl"), []any{
		longmemeval.RunEntry{
			QuestionID:   "q001",
			Hypothesis:   "Alice",
			Status:       "done",
			RetrievedIDs: []string{"m1"},
		},
	})

	scorer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/models":
			fmt.Fprint(w, `{"data":[{"id":"test-model"}]}`)
		case "/chat/completions":
			fmt.Fprint(w, `{"choices":[{"message":{"content":"CORRECT\nExact match."}}]}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer scorer.Close()

	cfg := &Config{
		DataFile:        dataPath,
		OutDir:          dir,
		Workers:         1,
		Retries:         0,
		RunID:           "testrun",
		ScorerVersion:   "tier1-qwen3-32b-nonthinking-v1",
		ScorerURL:       scorer.URL,
		ScorerModel:     "test-model",
		ScorerMaxTokens: longmemeval.DefaultScorerMaxTokens,
		PreserveCorrect: true,
	}
	if exit := runScoreEfficient(cfg); exit != 0 {
		t.Fatalf("runScoreEfficient exit = %d, want 0", exit)
	}

	data, err := os.ReadFile(filepath.Join(dir, "retrieval_log.jsonl"))
	if err != nil {
		t.Fatalf("read retrieval_log.jsonl: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("retrieval_log.jsonl is empty")
	}
	if !json.Valid(data[:len(data)-1]) && !json.Valid(data) {
		t.Fatalf("retrieval_log.jsonl line is not valid JSON: %s", data)
	}

	scoreData, err := os.ReadFile(filepath.Join(dir, "checkpoint-score.jsonl"))
	if err != nil {
		t.Fatalf("read checkpoint-score.jsonl: %v", err)
	}
	if !bytes.Contains(scoreData, []byte(`"scorer_version":"tier1-qwen3-32b-nonthinking-v1"`)) {
		t.Fatalf("checkpoint-score.jsonl missing scorer_version:\n%s", scoreData)
	}
}

func TestRunScoreEfficient_FailsClosedWhenScorerUnavailable(t *testing.T) {
	dir := t.TempDir()
	dataPath := filepath.Join(dir, "data.json")
	items := []longmemeval.Item{
		{QuestionID: "q001", QuestionType: "single-session-user", Question: "Who won?", Answer: "Alice"},
	}
	data, _ := json.Marshal(items)
	if err := os.WriteFile(dataPath, data, 0o644); err != nil {
		t.Fatal(err)
	}
	writeCheckpointFile(t, filepath.Join(dir, "checkpoint-ingest.jsonl"), []any{})
	writeCheckpointFile(t, filepath.Join(dir, "checkpoint-run.jsonl"), []any{})

	cfg := &Config{
		DataFile:    dataPath,
		OutDir:      dir,
		Workers:     1,
		ScorerURL:   "http://127.0.0.1:19999/v1",
		ScorerModel: "test-model",
	}
	if exit := runScoreEfficient(cfg); exit == 0 {
		t.Fatal("runScoreEfficient exit = 0, want non-zero when scorer is unavailable")
	}
}

func TestScoreErrorRetriedThenCounted(t *testing.T) {
	dir := t.TempDir()
	items := []longmemeval.Item{
		{
			QuestionID:       "q001",
			QuestionType:     "single-session-user",
			Question:         "Who won?",
			Answer:           "Alice",
			AnswerSessionIDs: []string{"sid-a"},
		},
	}
	data, _ := json.Marshal(items)
	dataPath := filepath.Join(dir, "data.json")
	if err := os.WriteFile(dataPath, data, 0o644); err != nil {
		t.Fatal(err)
	}
	writeCheckpointFile(t, filepath.Join(dir, "checkpoint-ingest.jsonl"), []any{
		longmemeval.IngestEntry{
			QuestionID: "q001",
			Status:     "done",
			MemoryMap:  map[string]string{"m1": "sid-a"},
		},
	})
	writeCheckpointFile(t, filepath.Join(dir, "checkpoint-run.jsonl"), []any{
		longmemeval.RunEntry{
			QuestionID:   "q001",
			Hypothesis:   "Alice",
			Status:       "done",
			RetrievedIDs: []string{"m1"},
		},
	})

	var calls atomic.Int32
	scorer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/models" {
			fmt.Fprint(w, `{"data":[{"id":"test-model"}]}`)
			return
		}
		calls.Add(1)
		http.Error(w, "judge down", http.StatusBadGateway)
	}))
	defer scorer.Close()

	cfg := &Config{
		DataFile:        dataPath,
		OutDir:          dir,
		Workers:         1,
		Retries:         2,
		RunID:           "testrun",
		ScorerURL:       scorer.URL,
		ScorerModel:     "test-model",
		ScorerMaxTokens: longmemeval.DefaultScorerMaxTokens,
		PreserveCorrect: true,
		Now:             func() time.Time { return time.Date(2026, 6, 3, 10, 0, 0, 0, time.UTC) },
	}
	if exit := runScoreEfficient(cfg); exit != 0 {
		t.Fatalf("runScoreEfficient exit = %d, want 0", exit)
	}
	if calls.Load() != int32(cfg.Retries+1) {
		t.Fatalf("expected %d score requests, got %d", cfg.Retries+1, calls.Load())
	}

	reportPath := filepath.Join(dir, "score_report.json")
	data, err := os.ReadFile(reportPath)
	if err != nil {
		t.Fatalf("read score_report.json: %v", err)
	}
	var report map[string]any
	if err := json.Unmarshal(data, &report); err != nil {
		t.Fatalf("parse score_report.json: %v", err)
	}
	scoreErrorTotal, ok := report["score_error_total"].(float64)
	if !ok {
		t.Fatalf("score_error_total missing or not numeric: %v", report["score_error_total"])
	}
	if int(scoreErrorTotal) != 1 {
		t.Errorf("score_error_total = %v, want 1", scoreErrorTotal)
	}
}
