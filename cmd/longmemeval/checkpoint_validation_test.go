package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/petersimmons1972/engram/internal/longmemeval"
)

func TestValidateCheckpointProjects_ProjectsAliveProceeds(t *testing.T) {
	entries := []longmemeval.IngestEntry{
		{QuestionID: "q001", Project: "lme-source-q001", Status: "done"},
		{QuestionID: "q002", Project: "lme-source-q002", Status: "done"},
	}
	queries := 0
	result, err := validateCheckpointProjects(
		context.Background(),
		"/results/checkpoint-ingest.jsonl",
		entries,
		func(context.Context, string) (bool, error) {
			queries++
			return true, nil
		},
	)
	if err != nil {
		t.Fatalf("validateCheckpointProjects() error = %v", err)
	}
	if result.Empty != 0 || result.Checked != 2 {
		t.Fatalf("result = %+v, want 0 empty of 2 checked", result)
	}
	if queries != 2 {
		t.Fatalf("project queries = %d, want 2", queries)
	}
}

func TestValidateCheckpointProjects_AllEmptyFailsBeforeGeneration(t *testing.T) {
	const checkpointPath = "/results/checkpoint-ingest.jsonl"
	entries := []longmemeval.IngestEntry{
		{QuestionID: "q001", Project: "lme-source-q001", Status: "done"},
		{QuestionID: "q002", Project: "lme-source-q002", Status: "done"},
	}
	generationCalls := 0

	_, err := validateCheckpointProjects(
		context.Background(),
		checkpointPath,
		entries,
		func(context.Context, string) (bool, error) { return false, nil },
	)
	if err == nil {
		generationCalls++
	}

	if err == nil {
		t.Fatal("validateCheckpointProjects() error = nil, want all-empty checkpoint failure")
	}
	if generationCalls != 0 {
		t.Fatalf("generation calls = %d, want 0", generationCalls)
	}
	got := err.Error()
	if !strings.Contains(got, checkpointPath) || !strings.Contains(got, "source run used cleanup-policy=auto?") {
		t.Fatalf("error = %q, want checkpoint path and likely cleanup-policy root cause", got)
	}
}

func TestRunRun_AllEmptyCheckpointDoesNotStartGenerationWorkers(t *testing.T) {
	dir := t.TempDir()
	dataPath := filepath.Join(dir, "data.json")
	data := []byte(`[{"question_id":"q001","question_type":"single-session-user","question":"Who?","answer":"Alice"}]`)
	if err := os.WriteFile(dataPath, data, 0o600); err != nil {
		t.Fatalf("write data: %v", err)
	}
	writeCheckpointFile(t, filepath.Join(dir, "checkpoint-ingest.jsonl"), []any{
		map[string]any{
			"question_id": "q001",
			"project":     "lme-source-q001",
			"status":      "done",
		},
	})

	originalQueryFactory := newCheckpointProjectQuery
	originalRunWorker := runRunWorker
	t.Cleanup(func() {
		newCheckpointProjectQuery = originalQueryFactory
		runRunWorker = originalRunWorker
	})
	newCheckpointProjectQuery = func(context.Context, *Config) (checkpointProjectQuery, func() error, error) {
		return func(context.Context, string) (bool, error) { return false, nil }, func() error { return nil }, nil
	}
	generationCalls := 0
	runRunWorker = func(
		_ *Config,
		_ map[string]longmemeval.Item,
		work <-chan longmemeval.IngestEntry,
		_ chan<- longmemeval.RunEntry,
		_ *preservedLog,
	) {
		for range work {
			generationCalls++
		}
	}

	code := runRun(&Config{
		DataFile:      dataPath,
		Workers:       1,
		OutDir:        dir,
		CleanupPolicy: CleanupPolicyNever,
	})
	if code == 0 {
		t.Fatal("runRun() = 0, want all-empty checkpoint failure")
	}
	if generationCalls != 0 {
		t.Fatalf("generation calls = %d, want 0", generationCalls)
	}
	if _, err := os.Stat(filepath.Join(dir, "checkpoint-run.jsonl")); !os.IsNotExist(err) {
		t.Fatalf("run checkpoint was created before validation completed: err=%v", err)
	}
}

func TestValidateCheckpointProjects_PartialEmptyWarnsAndProceeds(t *testing.T) {
	entries := []longmemeval.IngestEntry{
		{QuestionID: "q001", Project: "lme-source-q001", Status: "done"},
		{QuestionID: "q002", Project: "lme-source-q002", Status: "done"},
	}
	result, err := validateCheckpointProjects(
		context.Background(),
		"/results/checkpoint-ingest.jsonl",
		entries,
		func(_ context.Context, project string) (bool, error) {
			return project == "lme-source-q001", nil
		},
	)
	if err != nil {
		t.Fatalf("validateCheckpointProjects() error = %v, want partial emptiness to proceed", err)
	}
	if result.Empty != 1 || result.Checked != 2 {
		t.Fatalf("result = %+v, want 1 empty of 2 checked", result)
	}
	if warning := result.Warning(); !strings.Contains(warning, "WARN") || !strings.Contains(warning, "1/2") {
		t.Fatalf("warning = %q, want loud partial-empty count", warning)
	}
}

func TestValidateCheckpointProjects_BoundsProjectQueries(t *testing.T) {
	const wantMaxQueries = 8
	entries := make([]longmemeval.IngestEntry, 0, wantMaxQueries+4)
	for i := 0; i < wantMaxQueries+4; i++ {
		entries = append(entries, longmemeval.IngestEntry{
			QuestionID: "question-" + string(rune('a'+i)),
			Project:    "project-" + string(rune('a'+i)),
			Status:     "done",
		})
	}
	queries := 0
	result, err := validateCheckpointProjects(
		context.Background(),
		"/results/checkpoint-ingest.jsonl",
		entries,
		func(context.Context, string) (bool, error) {
			queries++
			return true, nil
		},
	)
	if err != nil {
		t.Fatalf("validateCheckpointProjects() error = %v", err)
	}
	if queries != wantMaxQueries || result.Checked != wantMaxQueries {
		t.Fatalf(
			"queries=%d checked=%d, want both bounded at %d",
			queries,
			result.Checked,
			wantMaxQueries,
		)
	}
}
