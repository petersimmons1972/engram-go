package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/petersimmons1972/engram/internal/longmemeval"
)

func TestWriteScoreReport_Basic(t *testing.T) {
	dir := t.TempDir()
	cfg := &Config{OutDir: dir, RunID: "test-run"}

	scores := []longmemeval.ScoreEntry{
		{QuestionID: "q1", QuestionType: "single-session-user", ScoreLabel: "CORRECT", Status: "done"},
		{QuestionID: "q2", QuestionType: "single-session-user", ScoreLabel: "INCORRECT", Status: "done"},
		{QuestionID: "q3", QuestionType: "multi-session", ScoreLabel: "PARTIALLY_CORRECT", Status: "done"},
		{QuestionID: "q4", QuestionType: "single-session-user", Status: "error"}, // not counted
	}

	writeScoreReport(cfg, scores)

	data, err := os.ReadFile(filepath.Join(dir, "score_report.json"))
	if err != nil {
		t.Fatalf("read score_report.json: %v", err)
	}
	var report map[string]any
	if err := json.Unmarshal(data, &report); err != nil {
		t.Fatalf("parse score_report.json: %v", err)
	}
	if report["run_id"] != "test-run" {
		t.Errorf("run_id = %v, want test-run", report["run_id"])
	}
	// total_scored counts all entries passed to the function, including errors.
	ts, ok := report["total_scored"].(float64)
	if !ok {
		t.Fatalf("total_scored not a number: %v", report["total_scored"])
	}
	if int(ts) != 4 {
		t.Errorf("total_scored = %v, want 4", ts)
	}
	// Overall correct = 1, partially_correct = 1, incorrect = 1 (error excluded).
	overall, ok := report["overall"].(map[string]any)
	if !ok {
		t.Fatalf("overall not a map: %v", report["overall"])
	}
	if int(overall["total"].(float64)) != 3 {
		t.Errorf("overall.total = %v, want 3 (errors excluded)", overall["total"])
	}
}

func TestWriteHypotheses_Basic(t *testing.T) {
	dir := t.TempDir()
	cfg := &Config{OutDir: dir}

	scores := []longmemeval.ScoreEntry{
		{QuestionID: "q1", Hypothesis: "answer one"},
		{QuestionID: "q2", Hypothesis: "answer two"},
	}
	writeHypotheses(cfg, scores)

	data, err := os.ReadFile(filepath.Join(dir, "hypotheses.jsonl"))
	if err != nil {
		t.Fatalf("read hypotheses.jsonl: %v", err)
	}
	lines := splitNonEmpty(string(data))
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines, got %d", len(lines))
	}
	var h longmemeval.HypothesisLine
	if err := json.Unmarshal([]byte(lines[0]), &h); err != nil {
		t.Fatalf("parse line 0: %v", err)
	}
	if h.QuestionID != "q1" || h.Hypothesis != "answer one" {
		t.Errorf("line 0 = %+v, want q1/answer one", h)
	}
}

func TestWriteRetrievalLog_Basic(t *testing.T) {
	dir := t.TempDir()
	cfg := &Config{OutDir: dir}

	itemMap := map[string]longmemeval.Item{
		"q1": {QuestionID: "q1", AnswerSessionIDs: []string{"sid-a"}},
	}
	ingestMap := map[string]longmemeval.IngestEntry{
		"q1": {QuestionID: "q1", MemoryMap: map[string]string{"mem-1": "sid-a"}},
	}
	runMap := map[string]longmemeval.RunEntry{
		"q1": {QuestionID: "q1", RetrievedIDs: []string{"mem-1"}},
	}
	scores := []longmemeval.ScoreEntry{
		{QuestionID: "q1", Status: "done"},
	}

	writeRetrievalLog(cfg, itemMap, ingestMap, runMap, scores)

	_, err := os.ReadFile(filepath.Join(dir, "retrieval_log.jsonl"))
	if err != nil {
		t.Fatalf("read retrieval_log.jsonl: %v", err)
	}
}

func TestWriteRetrievalLog_SkipsAbstentions(t *testing.T) {
	dir := t.TempDir()
	cfg := &Config{OutDir: dir}

	// _abs suffix questions should be silently skipped.
	itemMap := map[string]longmemeval.Item{
		"q1_abs": {QuestionID: "q1_abs"},
	}
	ingestMap := map[string]longmemeval.IngestEntry{
		"q1_abs": {QuestionID: "q1_abs", MemoryMap: map[string]string{}},
	}
	runMap := map[string]longmemeval.RunEntry{
		"q1_abs": {QuestionID: "q1_abs"},
	}
	scores := []longmemeval.ScoreEntry{
		{QuestionID: "q1_abs", Status: "done"},
	}

	writeRetrievalLog(cfg, itemMap, ingestMap, runMap, scores)

	data, err := os.ReadFile(filepath.Join(dir, "retrieval_log.jsonl"))
	if err != nil {
		t.Fatalf("read retrieval_log.jsonl: %v", err)
	}
	if len(splitNonEmpty(string(data))) != 0 {
		t.Errorf("expected no entries for _abs questions, got: %s", data)
	}
}

func TestProjectName(t *testing.T) {
	got := projectName("abc123", "question-001")
	want := "lme-abc123-question-001"
	if got != want {
		t.Errorf("projectName = %q, want %q", got, want)
	}
}

func TestEnvOr(t *testing.T) {
	// envOr returns the fallback when env var is not set.
	got := envOr("__DEFINITELY_NOT_SET_VAR_XYZ__", "fallback")
	if got != "fallback" {
		t.Errorf("envOr = %q, want fallback", got)
	}
	// envOr returns the env var value when set.
	t.Setenv("__TEST_ENVOR_VAR__", "fromenv")
	got = envOr("__TEST_ENVOR_VAR__", "fallback")
	if got != "fromenv" {
		t.Errorf("envOr = %q, want fromenv", got)
	}
}

func TestNormalizeLabel(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"correct", "CORRECT"},
		{"  Partially_Correct  ", "PARTIALLY_CORRECT"},
		{"INCORRECT", "INCORRECT"},
		{"", ""},
	}
	for _, c := range cases {
		got := normalizeLabel(c.in)
		if got != c.want {
			t.Errorf("normalizeLabel(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// splitNonEmpty returns non-empty lines from s.
func splitNonEmpty(s string) []string {
	var out []string
	for _, line := range splitLines(s) {
		if line != "" {
			out = append(out, line)
		}
	}
	return out
}

func splitLines(s string) []string {
	var out []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			out = append(out, s[start:i])
			start = i + 1
		}
	}
	if start < len(s) {
		out = append(out, s[start:])
	}
	return out
}
