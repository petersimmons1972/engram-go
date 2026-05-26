package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/petersimmons1972/engram/internal/longmemeval"
)

func TestPrepareSampleFiltersIngestCheckpoint(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "source")
	out := filepath.Join(dir, "out")
	if err := os.MkdirAll(source, 0o755); err != nil {
		t.Fatal(err)
	}

	dataPath := filepath.Join(dir, "cohort.json")
	writeItems(t, dataPath, []longmemeval.Item{
		{QuestionID: "q1", QuestionType: "temporal-reasoning", Question: "When?", Answer: "A"},
		{QuestionID: "q2", QuestionType: "single-session-preference", Question: "Prefer?", Answer: "B"},
	})
	writeCheckpointFile(t, filepath.Join(source, "checkpoint-ingest.jsonl"), []any{
		longmemeval.IngestEntry{QuestionID: "q1", Project: "lme-old-q1", Status: "done"},
		longmemeval.IngestEntry{QuestionID: "q2", Project: "lme-old-q2", Status: "done"},
		longmemeval.IngestEntry{QuestionID: "q3", Project: "lme-old-q3", Status: "done"},
	})

	result, err := prepareSample(samplePrepareConfig{
		DataFile: dataPath,
		Source:   source,
		OutDir:   out,
	})
	if err != nil {
		t.Fatalf("prepareSample: %v", err)
	}
	if result.Items != 2 || result.IngestEntries != 2 {
		t.Fatalf("result = %+v, want 2 items and 2 ingest entries", result)
	}

	got, err := longmemeval.ReadAllIngest(filepath.Join(out, "checkpoint-ingest.jsonl"))
	if err != nil {
		t.Fatalf("read filtered ingest: %v", err)
	}
	if len(got) != 2 || got[0].QuestionID != "q1" || got[1].QuestionID != "q2" {
		t.Fatalf("filtered ingest = %+v, want q1/q2 only", got)
	}
	if _, err := os.Stat(filepath.Join(out, "checkpoint-run.jsonl")); !os.IsNotExist(err) {
		t.Fatalf("checkpoint-run.jsonl should not be copied by default, stat err=%v", err)
	}
}

func TestPrepareSampleAppliesMaxPerType(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "source")
	out := filepath.Join(dir, "out")
	if err := os.MkdirAll(source, 0o755); err != nil {
		t.Fatal(err)
	}

	dataPath := filepath.Join(dir, "cohort.json")
	writeItems(t, dataPath, []longmemeval.Item{
		{QuestionID: "t1", QuestionType: "temporal-reasoning"},
		{QuestionID: "t2", QuestionType: "temporal-reasoning"},
		{QuestionID: "p1", QuestionType: "single-session-preference"},
		{QuestionID: "p2", QuestionType: "single-session-preference"},
	})
	writeCheckpointFile(t, filepath.Join(source, "checkpoint-ingest.jsonl"), []any{
		longmemeval.IngestEntry{QuestionID: "t1", Status: "done"},
		longmemeval.IngestEntry{QuestionID: "t2", Status: "done"},
		longmemeval.IngestEntry{QuestionID: "p1", Status: "done"},
		longmemeval.IngestEntry{QuestionID: "p2", Status: "done"},
	})

	result, err := prepareSample(samplePrepareConfig{
		DataFile:    dataPath,
		Source:      source,
		OutDir:      out,
		MaxPerType:  1,
		CopyRun:     true,
		CopyScore:   true,
		Description: "stratified smoke",
	})
	if err != nil {
		t.Fatalf("prepareSample: %v", err)
	}
	if result.Items != 2 {
		t.Fatalf("items = %d, want 2", result.Items)
	}

	data, err := os.ReadFile(filepath.Join(out, "data.json"))
	if err != nil {
		t.Fatalf("read sampled data: %v", err)
	}
	var items []longmemeval.Item
	if err := json.Unmarshal(data, &items); err != nil {
		t.Fatalf("parse sampled data: %v", err)
	}
	if len(items) != 2 || items[0].QuestionID != "t1" || items[1].QuestionID != "p1" {
		t.Fatalf("sampled item order = %+v, want first item per type", items)
	}
}

func TestAnalyzeSampleSummarizesScoresAndRetrievalCoverage(t *testing.T) {
	dir := t.TempDir()
	dataPath := filepath.Join(dir, "data.json")
	writeItems(t, dataPath, []longmemeval.Item{
		{QuestionID: "q1", QuestionType: "temporal-reasoning", AnswerSessionIDs: []string{"sid-a"}},
		{QuestionID: "q2", QuestionType: "single-session-user", AnswerSessionIDs: []string{"sid-b"}},
	})
	writeCheckpointFile(t, filepath.Join(dir, "checkpoint-ingest.jsonl"), []any{
		longmemeval.IngestEntry{QuestionID: "q1", Status: "done", MemoryMap: map[string]string{"m1": "sid-a"}},
		longmemeval.IngestEntry{QuestionID: "q2", Status: "done", MemoryMap: map[string]string{"m2": "sid-x"}},
	})
	writeCheckpointFile(t, filepath.Join(dir, "checkpoint-run.jsonl"), []any{
		longmemeval.RunEntry{QuestionID: "q1", Status: "done", RetrievedIDs: []string{"m1"}},
		longmemeval.RunEntry{QuestionID: "q2", Status: "done", RetrievedIDs: []string{"m2"}},
	})
	writeCheckpointFile(t, filepath.Join(dir, "checkpoint-score.jsonl"), []any{
		longmemeval.ScoreEntry{QuestionID: "q1", QuestionType: "temporal-reasoning", ScoreLabel: "INCORRECT", Status: "done"},
		longmemeval.ScoreEntry{QuestionID: "q1", QuestionType: "temporal-reasoning", ScoreLabel: "CORRECT", Status: "done"},
		longmemeval.ScoreEntry{QuestionID: "q2", QuestionType: "single-session-user", ScoreLabel: "INCORRECT", Status: "done"},
	})

	summary, err := analyzeSample(sampleAnalyzeConfig{DataFile: dataPath, ResultsDir: dir})
	if err != nil {
		t.Fatalf("analyzeSample: %v", err)
	}
	if summary.Items != 2 || summary.Scored != 2 || summary.Correct != 1 || summary.Incorrect != 1 {
		t.Fatalf("summary counts = %+v, want 2 items, 2 scored, 1 correct, 1 incorrect", summary)
	}
	if summary.RetrievedGoldSession != 1 || summary.RunDone != 2 {
		t.Fatalf("retrieval counts = %+v, want one gold-covered run out of two", summary)
	}
}

func writeItems(t *testing.T, path string, items []longmemeval.Item) {
	t.Helper()
	data, err := json.Marshal(items)
	if err != nil {
		t.Fatalf("marshal items: %v", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write items: %v", err)
	}
}
