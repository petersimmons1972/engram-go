package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/petersimmons1972/engram/internal/longmemeval"
)

// TestRetrievalMetricsSubcommand_MissingFlags verifies required-flag
// validation is wired up in dispatch().
func TestRetrievalMetricsSubcommand_MissingData(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := dispatch([]string{"longmemeval", "retrieval-metrics", "--results", "/tmp"}, &stdout, &stderr)
	if code != 1 {
		t.Errorf("exit = %d, want 1 for missing --data", code)
	}
}

func TestRetrievalMetricsSubcommand_MissingResults(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := dispatch([]string{"longmemeval", "retrieval-metrics", "--data", "/tmp/data.json"}, &stdout, &stderr)
	if code != 1 {
		t.Errorf("exit = %d, want 1 for missing --results", code)
	}
}

// TestRunRetrievalMetrics_E2E writes minimal fixture files and verifies the
// subcommand produces a valid JSON report with expected metric values.
func TestRunRetrievalMetrics_E2E(t *testing.T) {
	// Build a tiny dataset: 2 items.
	items := []longmemeval.Item{
		{
			QuestionID:       "q1",
			QuestionType:     "single-session-preference",
			Question:         "Can you recommend a restaurant?",
			AnswerSessionIDs: []string{"sess-gold"},
		},
		{
			QuestionID:       "q2",
			QuestionType:     "temporal-reasoning",
			Question:         "How long ago did I visit Paris?",
			AnswerSessionIDs: []string{"sess-temporal-gold"},
		},
	}

	// Build ingest checkpoints: memory_id → session_id maps.
	ingestEntries := []longmemeval.IngestEntry{
		{
			QuestionID: "q1",
			Project:    "lme-test-q1",
			Status:     "done",
			MemoryMap: map[string]string{
				"mem-1": "sess-other",
				"mem-2": "sess-gold", // gold is mem-2
				"mem-3": "sess-noise",
			},
		},
		{
			QuestionID: "q2",
			Project:    "lme-test-q2",
			Status:     "done",
			MemoryMap: map[string]string{
				"mem-a": "sess-temporal-gold", // gold is first
				"mem-b": "sess-noise",
			},
		},
	}

	// Build run checkpoints: retrieved memory IDs in ranked order.
	// q1: gold at rank 2 → recall@5=1, recall@10=1
	// q2: gold at rank 1 → recall@5=1, recall@10=1
	runEntries := []longmemeval.RunEntry{
		{QuestionID: "q1", RetrievedIDs: []string{"mem-1", "mem-2", "mem-3"}, Status: "done"},
		{QuestionID: "q2", RetrievedIDs: []string{"mem-a", "mem-b"}, Status: "done"},
	}

	// Write fixtures to a temp dir.
	dir := t.TempDir()
	dataPath := filepath.Join(dir, "data.json")
	ingestPath := filepath.Join(dir, "checkpoint-ingest.jsonl")
	runPath := filepath.Join(dir, "checkpoint-run.jsonl")

	if err := writeJSONFile(dataPath, items); err != nil {
		t.Fatal(err)
	}
	if err := writeJSONLFile(ingestPath, ingestEntries); err != nil {
		t.Fatal(err)
	}
	if err := writeJSONLFile(runPath, runEntries); err != nil {
		t.Fatal(err)
	}

	outDir := filepath.Join(dir, "out")
	cfg := retrievalMetricsConfig{
		DataFile:   dataPath,
		ResultsDir: dir,
		OutDir:     outDir,
	}

	var stdout bytes.Buffer
	code := runRetrievalMetrics(cfg, &stdout)
	if code != 0 {
		t.Fatalf("runRetrievalMetrics exit = %d; stdout=%s", code, stdout.String())
	}

	// Verify JSON report was written.
	reportPath := filepath.Join(outDir, "retrieval_metrics_report.json")
	data, err := os.ReadFile(reportPath)
	if err != nil {
		t.Fatalf("report not written: %v", err)
	}

	var report longmemeval.RetrievalReport
	if err := json.Unmarshal(data, &report); err != nil {
		t.Fatalf("invalid JSON report: %v", err)
	}

	// Both questions have gold in context → overall rate should be 1.0.
	if report.Overall.GoldInContextRate != 1.0 {
		t.Errorf("Overall.GoldInContextRate = %.2f, want 1.0", report.Overall.GoldInContextRate)
	}
	if report.Overall.N != 2 {
		t.Errorf("Overall.N = %d, want 2", report.Overall.N)
	}

	// Both recall@5 should be 1.0 (gold within top 5).
	if report.Overall.AvgRecallAt5 != 1.0 {
		t.Errorf("Overall.AvgRecallAt5 = %.2f, want 1.0", report.Overall.AvgRecallAt5)
	}

	// Verify stdout has the table header.
	out := stdout.String()
	if len(out) == 0 {
		t.Error("stdout is empty; expected table output")
	}
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

func writeJSONFile(path string, v any) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

func writeJSONLFile[T any](path string, entries []T) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	enc := json.NewEncoder(f)
	for _, e := range entries {
		if err := enc.Encode(e); err != nil {
			return err
		}
	}
	return nil
}
