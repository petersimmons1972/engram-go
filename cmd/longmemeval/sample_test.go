package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/petersimmons1972/engram/internal/longmemeval"
	"github.com/petersimmons1972/engram/internal/types"
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

func TestPrepareSampleCreatesPrivateArtifacts(t *testing.T) {
	defer withPermissiveUmask(t)()
	dir := t.TempDir()
	source := filepath.Join(dir, "source")
	out := filepath.Join(dir, "out")
	if err := os.MkdirAll(source, 0o755); err != nil {
		t.Fatal(err)
	}

	dataPath := filepath.Join(dir, "cohort.json")
	writeItems(t, dataPath, []longmemeval.Item{
		{QuestionID: "q1", QuestionType: "temporal-reasoning", Question: "When?", Answer: "A"},
	})
	writeCheckpointFile(t, filepath.Join(source, "checkpoint-ingest.jsonl"), []any{
		longmemeval.IngestEntry{QuestionID: "q1", Project: "lme-old-q1", Status: "done"},
	})
	writeCheckpointFile(t, filepath.Join(source, "checkpoint-run.jsonl"), []any{
		longmemeval.RunEntry{QuestionID: "q1", Status: "done"},
	})
	writeCheckpointFile(t, filepath.Join(source, "checkpoint-score.jsonl"), []any{
		longmemeval.ScoreEntry{QuestionID: "q1", Status: "done", ScoreLabel: "CORRECT"},
	})

	if _, err := prepareSample(samplePrepareConfig{
		DataFile:  dataPath,
		Source:    source,
		OutDir:    out,
		CopyRun:   true,
		CopyScore: true,
	}); err != nil {
		t.Fatalf("prepareSample: %v", err)
	}

	assertMode(t, out, 0o700)
	assertMode(t, filepath.Join(out, "data.json"), 0o600)
	assertMode(t, filepath.Join(out, "checkpoint-ingest.jsonl"), 0o600)
	assertMode(t, filepath.Join(out, "checkpoint-run.jsonl"), 0o600)
	assertMode(t, filepath.Join(out, "checkpoint-score.jsonl"), 0o600)
	assertMode(t, filepath.Join(out, "sample_manifest.json"), 0o600)
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

func TestAnalyzeSampleClassifiesIncorrectFailureClusters(t *testing.T) {
	dir := t.TempDir()
	dataPath := filepath.Join(dir, "data.json")
	writeItems(t, dataPath, []longmemeval.Item{
		{QuestionID: "q1", QuestionType: "temporal-reasoning", AnswerSessionIDs: []string{"sid-a"}},
		{QuestionID: "q2", QuestionType: "multi-session", AnswerSessionIDs: []string{"sid-b"}},
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
		longmemeval.ScoreEntry{QuestionID: "q2", QuestionType: "multi-session", ScoreLabel: "INCORRECT", Status: "done"},
	})

	summary, err := analyzeSample(sampleAnalyzeConfig{DataFile: dataPath, ResultsDir: dir})
	if err != nil {
		t.Fatalf("analyzeSample: %v", err)
	}
	data, err := json.Marshal(summary)
	if err != nil {
		t.Fatalf("marshal summary: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal summary: %v", err)
	}
	if int(got["context_present_generation_miss"].(float64)) != 1 {
		t.Fatalf("context_present_generation_miss = %v, want 1", got["context_present_generation_miss"])
	}
	if int(got["retrieval_miss"].(float64)) != 1 {
		t.Fatalf("retrieval_miss = %v, want 1", got["retrieval_miss"])
	}
}

func TestDispatchAnalyzeAliasWritesSummaryJSON(t *testing.T) {
	dir := t.TempDir()
	dataPath := filepath.Join(dir, "data.json")
	writeItems(t, dataPath, []longmemeval.Item{
		{QuestionID: "q1", QuestionType: "single-session-user", AnswerSessionIDs: []string{"sid-a"}},
	})
	writeCheckpointFile(t, filepath.Join(dir, "checkpoint-ingest.jsonl"), []any{
		longmemeval.IngestEntry{QuestionID: "q1", Status: "done", MemoryMap: map[string]string{"m1": "sid-a"}},
	})
	writeCheckpointFile(t, filepath.Join(dir, "checkpoint-run.jsonl"), []any{
		longmemeval.RunEntry{QuestionID: "q1", Status: "done", RetrievedIDs: []string{"m1"}},
	})
	writeCheckpointFile(t, filepath.Join(dir, "checkpoint-score.jsonl"), []any{
		longmemeval.ScoreEntry{QuestionID: "q1", QuestionType: "single-session-user", ScoreLabel: "CORRECT", Status: "done"},
	})

	var stdout, stderr bytes.Buffer
	exit := dispatch([]string{"longmemeval", "analyze", "--data", dataPath, "--results", dir}, &stdout, &stderr)
	if exit != 0 {
		t.Fatalf("dispatch analyze exit = %d, stderr=%q", exit, stderr.String())
	}
	var summary map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &summary); err != nil {
		t.Fatalf("stdout is not JSON: %v\n%s", err, stdout.String())
	}
	if int(summary["items"].(float64)) != 1 {
		t.Fatalf("items = %v, want 1", summary["items"])
	}
}

func TestAnalyzeSampleWithoutDataSummarizesScoreCheckpoint(t *testing.T) {
	dir := t.TempDir()
	writeCheckpointFile(t, filepath.Join(dir, "checkpoint-ingest.jsonl"), []any{})
	writeCheckpointFile(t, filepath.Join(dir, "checkpoint-run.jsonl"), []any{
		longmemeval.RunEntry{QuestionID: "q1", Status: "done"},
		longmemeval.RunEntry{QuestionID: "q2", Status: "error"},
	})
	writeCheckpointFile(t, filepath.Join(dir, "checkpoint-score.jsonl"), []any{
		longmemeval.ScoreEntry{QuestionID: "q1", QuestionType: "temporal-reasoning", ScoreLabel: "CORRECT", Status: "done"},
		longmemeval.ScoreEntry{QuestionID: "q2", QuestionType: "multi-session", ScoreLabel: "INCORRECT", Status: "done"},
	})

	summary, err := analyzeSample(sampleAnalyzeConfig{ResultsDir: dir})
	if err != nil {
		t.Fatalf("analyzeSample without data: %v", err)
	}
	if summary.Items != 2 || summary.Scored != 2 || summary.Correct != 1 || summary.Incorrect != 1 {
		t.Fatalf("summary = %+v, want checkpoint-derived counts", summary)
	}
	if summary.ByType["temporal-reasoning"].Correct != 1 {
		t.Fatalf("temporal row = %+v, want one correct", summary.ByType["temporal-reasoning"])
	}
	if summary.RetrievalMiss != 0 || summary.ContextPresentGenerationMiss != 0 {
		t.Fatalf("gold-session failure clusters = retrieval_miss=%d generation_miss=%d, want zero without data",
			summary.RetrievalMiss, summary.ContextPresentGenerationMiss)
	}
}

func TestSessionDiagnostics_SingleDominantSession(t *testing.T) {
	results := []types.SearchResult{
		searchResultWithSession("m1", "s1"),
		searchResultWithSession("m2", "s1"),
		searchResultWithSession("m3", "s1"),
		searchResultWithSession("m4", "s1"),
		searchResultWithSession("m5", "s1"),
		searchResultWithSession("m6", "s2"),
	}

	dominance, count := computeSessionDiagnostics(results)

	if got, want := dominance, 5.0/6.0; got != want {
		t.Fatalf("dominance = %v, want %v", got, want)
	}
	if count != 2 {
		t.Fatalf("context session count = %d, want 2", count)
	}
}

func TestSessionDiagnostics_EvenSplit(t *testing.T) {
	results := []types.SearchResult{
		searchResultWithSession("m1", "s1"),
		searchResultWithSession("m2", "s1"),
		searchResultWithSession("m3", "s1"),
		searchResultWithSession("m4", "s2"),
		searchResultWithSession("m5", "s2"),
		searchResultWithSession("m6", "s2"),
	}

	dominance, count := computeSessionDiagnostics(results)

	if dominance != 0.5 {
		t.Fatalf("dominance = %v, want 0.5", dominance)
	}
	if count != 2 {
		t.Fatalf("context session count = %d, want 2", count)
	}
}

func TestSessionDiagnostics_EmptyResults(t *testing.T) {
	dominance, count := computeSessionDiagnostics(nil)

	if dominance != 0.0 {
		t.Fatalf("dominance = %v, want 0.0", dominance)
	}
	if count != 0 {
		t.Fatalf("context session count = %d, want 0", count)
	}
}

func TestSessionDiagnostics_AllSameSession(t *testing.T) {
	results := []types.SearchResult{
		searchResultWithSession("m1", "s1"),
		searchResultWithSession("m2", "s1"),
		searchResultWithSession("m3", "s1"),
		searchResultWithSession("m4", "s1"),
	}

	dominance, count := computeSessionDiagnostics(results)

	if dominance != 1.0 {
		t.Fatalf("dominance = %v, want 1.0", dominance)
	}
	if count != 1 {
		t.Fatalf("context session count = %d, want 1", count)
	}
}

func TestSessionDiagnostics_MissingSessionIDs(t *testing.T) {
	results := []types.SearchResult{
		searchResultWithSession("m1", "s1"),
		searchResultWithSession("m2", ""),
		searchResultWithSession("m3", ""),
		searchResultWithSession("m4", ""),
		searchResultWithSession("m5", "s2"),
	}

	dominance, count := computeSessionDiagnostics(results)

	if dominance != 0.2 {
		t.Fatalf("dominance = %v, want 0.2", dominance)
	}
	if count != 5 {
		t.Fatalf("context session count = %d, want 5", count)
	}
}

func searchResultWithSession(memoryID, sessionID string) types.SearchResult {
	tags := []string{"source:test"}
	if sessionID != "" {
		tags = append(tags, "sid:"+sessionID)
	} else {
		tags = append(tags, "sid:")
	}
	return types.SearchResult{
		Memory: &types.Memory{
			ID:   memoryID,
			Tags: tags,
		},
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
