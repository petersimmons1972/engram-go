package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/petersimmons1972/engram/internal/longmemeval"
)

func TestWriteScoreReport_QuietSuppressesStdout(t *testing.T) {
	dir := t.TempDir()
	var out bytes.Buffer
	cfg := &Config{OutDir: dir, RunID: "quiet-test", ScoreOutput: "quiet", Output: &out}

	writeScoreReport(cfg, []longmemeval.ScoreEntry{
		{QuestionID: "q1", QuestionType: "single-session-user", ScoreLabel: "CORRECT", Status: "done"},
	})

	if out.Len() != 0 {
		t.Fatalf("quiet score output wrote stdout: %q", out.String())
	}
	if _, err := os.Stat(filepath.Join(dir, "score_report.json")); err != nil {
		t.Fatalf("score_report.json was not written: %v", err)
	}
}

func TestWriteScoreReport_JSONWritesMachineSummaryToConfiguredOutput(t *testing.T) {
	dir := t.TempDir()
	var out bytes.Buffer
	cfg := &Config{OutDir: dir, RunID: "json-test", ScoreOutput: "json", Output: &out}

	writeScoreReport(cfg, []longmemeval.ScoreEntry{
		{QuestionID: "q1", QuestionType: "single-session-user", ScoreLabel: "CORRECT", Status: "done"},
		{QuestionID: "q2", QuestionType: "single-session-user", ScoreLabel: "INCORRECT", Status: "done"},
	})

	var report map[string]any
	if err := json.Unmarshal(out.Bytes(), &report); err != nil {
		t.Fatalf("stdout is not JSON: %v\n%s", err, out.String())
	}
	if report["run_id"] != "json-test" {
		t.Fatalf("stdout run_id = %v, want json-test", report["run_id"])
	}
	overall := report["overall"].(map[string]any)
	if int(overall["total"].(float64)) != 2 {
		t.Fatalf("stdout overall.total = %v, want 2", overall["total"])
	}
}

// TestWriteScoreReport_ScoreErrorCountsAsIncorrect is the regression guard for
// issue #793: a ScoreEntry with ScoreLabel="SCORE_ERROR" and Status="done" must
// fall to the default/Incorrect bucket in writeScoreReport, never silently
// inflating CORRECT or PARTIALLY_CORRECT counts.
func TestWriteScoreReport_ScoreErrorCountsAsIncorrect(t *testing.T) {
	dir := t.TempDir()
	cfg := &Config{OutDir: dir, RunID: "score-error-test"}

	scores := []longmemeval.ScoreEntry{
		{QuestionID: "q1", QuestionType: "single-session-user", ScoreLabel: "CORRECT", Status: "done"},
		{QuestionID: "q2", QuestionType: "single-session-user", ScoreLabel: "SCORE_ERROR", Status: "done"},
		{QuestionID: "q3", QuestionType: "single-session-user", ScoreLabel: "SCORE_ERROR", Status: "done"},
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

	overall, ok := report["overall"].(map[string]any)
	if !ok {
		t.Fatalf("overall not a map: %v", report["overall"])
	}

	gotTotal := int(overall["total"].(float64))
	gotCorrect := int(overall["correct"].(float64))
	gotPartial := int(overall["partially_correct"].(float64))
	gotIncorrect := int(overall["incorrect"].(float64))

	if gotTotal != 3 {
		t.Errorf("overall.total = %d, want 3", gotTotal)
	}
	if gotCorrect != 1 {
		t.Errorf("overall.correct = %d, want 1 (SCORE_ERROR must not inflate correct count)", gotCorrect)
	}
	if gotPartial != 0 {
		t.Errorf("overall.partially_correct = %d, want 0 (SCORE_ERROR must not inflate partial count)", gotPartial)
	}
	if gotIncorrect != 2 {
		t.Errorf("overall.incorrect = %d, want 2 (both SCORE_ERROR entries must count as incorrect)", gotIncorrect)
	}
}

func TestWriteScoreReport_DeduplicatesByQuestionID(t *testing.T) {
	dir := t.TempDir()
	cfg := &Config{OutDir: dir, RunID: "dedup-test"}

	// q1 appears twice: old=INCORRECT, new=CORRECT — last-write-wins.
	// q2 appears once: CORRECT.
	// Total unique questions = 2, both CORRECT.
	scores := []longmemeval.ScoreEntry{
		{QuestionID: "q1", QuestionType: "single-session-user", ScoreLabel: "INCORRECT", Status: "done"},
		{QuestionID: "q2", QuestionType: "single-session-user", ScoreLabel: "CORRECT", Status: "done"},
		{QuestionID: "q1", QuestionType: "single-session-user", ScoreLabel: "CORRECT", Status: "done"},
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

	overall, ok := report["overall"].(map[string]any)
	if !ok {
		t.Fatalf("overall not a map: %v", report["overall"])
	}

	gotTotal := int(overall["total"].(float64))
	gotCorrect := int(overall["correct"].(float64))

	if gotTotal != 2 {
		t.Errorf("overall.total = %d, want 2 (deduplicated)", gotTotal)
	}
	if gotCorrect != 2 {
		t.Errorf("overall.correct = %d, want 2 (last-write-wins for q1)", gotCorrect)
	}
}

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

func TestWriteScoreReport_IncludesStrictAndLenientAccuracy(t *testing.T) {
	dir := t.TempDir()
	cfg := &Config{OutDir: dir, RunID: "accuracy-breakdown-test"}

	scores := []longmemeval.ScoreEntry{
		{QuestionID: "q1", QuestionType: "single-session-user", ScoreLabel: "CORRECT", Status: "done"},
		{QuestionID: "q2", QuestionType: "single-session-user", ScoreLabel: "PARTIALLY_CORRECT", Status: "done"},
		{QuestionID: "q3", QuestionType: "single-session-user", ScoreLabel: "INCORRECT", Status: "done"},
	}

	writeScoreReport(cfg, scores)

	report := readScoreReport(t, filepath.Join(dir, "score_report.json"))
	overall := reportMap(t, report["overall"], "overall")

	assertAccuracySummary(t, reportMap(t, overall["strict"], "overall.strict"), 1, 3, 1.0/3.0)
	assertAccuracySummary(t, reportMap(t, overall["lenient"], "overall.lenient"), 2, 3, 2.0/3.0)
}

func TestWriteScoreReport_SingleSessionPreferenceLenientCountsPartialCredit(t *testing.T) {
	dir := t.TempDir()
	cfg := &Config{OutDir: dir, RunID: "ss-preference-lenient-test"}

	scores := []longmemeval.ScoreEntry{
		{QuestionID: "q1", QuestionType: "single-session-preference", ScoreLabel: "PARTIALLY_CORRECT", Status: "done"},
		{QuestionID: "q2", QuestionType: "single-session-preference", ScoreLabel: "PARTIALLY_CORRECT", Status: "done"},
		{QuestionID: "q3", QuestionType: "single-session-preference", ScoreLabel: "INCORRECT", Status: "done"},
		{QuestionID: "q4", QuestionType: "single-session-preference", ScoreLabel: "CORRECT", Status: "done"},
	}

	writeScoreReport(cfg, scores)

	report := readScoreReport(t, filepath.Join(dir, "score_report.json"))
	byType := reportMap(t, report["by_type"], "by_type")
	row := reportMap(t, byType["single-session-preference"], "by_type.single-session-preference")

	assertAccuracySummary(t, reportMap(t, row["strict"], "by_type.single-session-preference.strict"), 1, 4, 0.25)
	assertAccuracySummary(t, reportMap(t, row["lenient"], "by_type.single-session-preference.lenient"), 3, 4, 0.75)
}

func TestWriteScoreReport_RecordsJudgeAttributionMetadata(t *testing.T) {
	const fixed = "2026-06-03T10:00:00Z"
	cfg := &Config{
		OutDir:          t.TempDir(),
		RunID:           "judge-metadata-test",
		ScorerURL:       "https://api.openai.com/v1",
		ScorerModel:     "gpt-4o-2024-11-20",
		ScorerThinking:  false,
		ScorerMaxTokens: 4096,
		Now:             func() time.Time { return time.Date(2026, 6, 3, 10, 0, 0, 0, time.UTC) },
	}

	writeScoreReport(cfg, []longmemeval.ScoreEntry{
		{QuestionID: "q1", QuestionType: "single-session-user", ScoreLabel: "CORRECT", Status: "done"},
	})

	data, err := os.ReadFile(filepath.Join(cfg.OutDir, "score_report.json"))
	if err != nil {
		t.Fatalf("read score_report.json: %v", err)
	}
	var report map[string]any
	if err := json.Unmarshal(data, &report); err != nil {
		t.Fatalf("parse report: %v", err)
	}
	for key, want := range map[string]any{
		"scorer_model":      cfg.ScorerModel,
		"scorer_thinking":   cfg.ScorerThinking,
		"scorer_max_tokens": cfg.ScorerMaxTokens,
		"judged_at":         fixed,
		"scorer_url":        "https://api.openai.com/v1",
	} {
		got := report[key]
		switch expect := want.(type) {
		case int:
			num, ok := got.(float64)
			if !ok {
				t.Fatalf("%s type=%T, want float64 (%v)", key, got, expect)
			}
			if int(num) != expect {
				t.Fatalf("%s = %v, want %v", key, got, expect)
			}
		default:
			if got != want {
				t.Fatalf("%s = %v, want %v", key, got, want)
			}
		}
	}
}

func TestWriteOutputArtifactsArePrivate(t *testing.T) {
	defer withPermissiveUmask(t)()
	dir := t.TempDir()
	cfg := &Config{OutDir: dir, RunID: "private-artifacts-test"}
	scores := []longmemeval.ScoreEntry{
		{QuestionID: "q1", QuestionType: "single-session-user", Hypothesis: "answer one", ScoreLabel: "CORRECT", Status: "done"},
	}
	itemMap := map[string]longmemeval.Item{
		"q1": {QuestionID: "q1", AnswerSessionIDs: []string{"sid-a"}},
	}
	ingestMap := map[string]longmemeval.IngestEntry{
		"q1": {QuestionID: "q1", MemoryMap: map[string]string{"mem-1": "sid-a"}},
	}
	runMap := map[string]longmemeval.RunEntry{
		"q1": {QuestionID: "q1", RetrievedIDs: []string{"mem-1"}},
	}

	writeOutputs(cfg, itemMap, ingestMap, runMap, scores)

	assertMode(t, filepath.Join(dir, "hypotheses.jsonl"), 0o600)
	assertMode(t, filepath.Join(dir, "retrieval_log.jsonl"), 0o600)
	assertMode(t, filepath.Join(dir, "score_report.json"), 0o600)
}

func TestWriteOutputArtifactsTightenExistingFiles(t *testing.T) {
	dir := t.TempDir()
	cfg := &Config{OutDir: dir, RunID: "private-artifacts-test"}
	for _, name := range []string{"hypotheses.jsonl", "retrieval_log.jsonl", "score_report.json"} {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte("stale"), 0o600); err != nil {
			t.Fatalf("seed %s: %v", name, err)
		}
		if err := os.Chmod(path, 0o644); err != nil {
			t.Fatalf("chmod seed %s: %v", name, err)
		}
	}

	writeOutputs(cfg, nil, nil, nil, nil)

	assertMode(t, filepath.Join(dir, "hypotheses.jsonl"), 0o600)
	assertMode(t, filepath.Join(dir, "retrieval_log.jsonl"), 0o600)
	assertMode(t, filepath.Join(dir, "score_report.json"), 0o600)
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

func readScoreReport(t *testing.T, path string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", filepath.Base(path), err)
	}
	var report map[string]any
	if err := json.Unmarshal(data, &report); err != nil {
		t.Fatalf("parse %s: %v", filepath.Base(path), err)
	}
	return report
}

func reportMap(t *testing.T, v any, name string) map[string]any {
	t.Helper()
	m, ok := v.(map[string]any)
	if !ok {
		t.Fatalf("%s not a map: %T (%v)", name, v, v)
	}
	return m
}

func assertAccuracySummary(t *testing.T, got map[string]any, wantCredited, wantTotal int, wantAccuracy float64) {
	t.Helper()
	if credited := int(got["credited_correct"].(float64)); credited != wantCredited {
		t.Fatalf("credited_correct = %d, want %d", credited, wantCredited)
	}
	if total := int(got["total"].(float64)); total != wantTotal {
		t.Fatalf("total = %d, want %d", total, wantTotal)
	}
	accuracy, ok := got["accuracy"].(float64)
	if !ok {
		t.Fatalf("accuracy not a number: %T (%v)", got["accuracy"], got["accuracy"])
	}
	if diff := accuracy - wantAccuracy; diff < -1e-9 || diff > 1e-9 {
		t.Fatalf("accuracy = %.12f, want %.12f", accuracy, wantAccuracy)
	}
}

func assertMode(t *testing.T, path string, want os.FileMode) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat %s: %v", path, err)
	}
	if got := info.Mode().Perm(); got != want {
		t.Fatalf("%s mode = %o, want %o", filepath.Base(path), got, want)
	}
}

func withPermissiveUmask(t *testing.T) func() {
	t.Helper()
	old := syscall.Umask(0)
	return func() {
		syscall.Umask(old)
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
