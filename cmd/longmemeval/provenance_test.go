package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/petersimmons1972/engram/internal/longmemeval"
)

func TestApplyScorerLockUsesManifestAndRejectsMismatchedOverrides(t *testing.T) {
	dir := t.TempDir()
	lockPath := filepath.Join(dir, "scorer-lock.json")
	lockBody := `{
  "version": "tier1-qwen3-2026-06-22",
  "scorer_url": "http://lock.example.test/v1",
  "scorer_model": "inference",
  "scorer_thinking": false,
  "scorer_max_tokens": 1536
}`
	if err := os.WriteFile(lockPath, []byte(lockBody), 0o600); err != nil {
		t.Fatalf("write scorer-lock.json: %v", err)
	}

	cfg := &Config{ScorerLockPath: lockPath}
	if err := applyScorerLock(cfg); err != nil {
		t.Fatalf("applyScorerLock: %v", err)
	}
	if cfg.ScorerVersion != "tier1-qwen3-2026-06-22" {
		t.Fatalf("ScorerVersion = %q, want tier1-qwen3-2026-06-22", cfg.ScorerVersion)
	}
	if cfg.ScorerURL != "http://lock.example.test/v1" {
		t.Fatalf("ScorerURL = %q, want lock URL", cfg.ScorerURL)
	}
	if cfg.ScorerModel != "inference" {
		t.Fatalf("ScorerModel = %q, want inference", cfg.ScorerModel)
	}
	if cfg.ScorerThinking {
		t.Fatal("ScorerThinking = true, want false from lock manifest")
	}
	if cfg.ScorerMaxTokens != 1536 {
		t.Fatalf("ScorerMaxTokens = %d, want 1536", cfg.ScorerMaxTokens)
	}

	cfg = &Config{
		ScorerLockPath: lockPath,
		ScorerModel:    "different-model",
	}
	err := applyScorerLock(cfg)
	if err == nil {
		t.Fatal("applyScorerLock succeeded with mismatched scorer-model override")
	}
	if got := err.Error(); got == "" || !containsAll(got, "scorer-model", "different-model", "inference") {
		t.Fatalf("mismatch error = %q, want scorer-model conflict details", got)
	}
}

func TestValidateScoreProvenanceRequiresGoldVersionAndItemSetWhenLocked(t *testing.T) {
	cfg := &Config{
		RunID:          "locked-run",
		ScorerLockPath: "docs/lme-campaign/scorer-lock.json",
		ScorerVersion:  "tier1-qwen3-2026-06-22",
	}
	if err := validateScoreProvenance(cfg); err == nil || !containsAll(err.Error(), "gold-version", "item-set") {
		t.Fatalf("validateScoreProvenance error = %v, want missing gold-version and item-set", err)
	}

	cfg.GoldVersion = "gold-zfs-2026-07-03"
	if err := validateScoreProvenance(cfg); err == nil || !containsAll(err.Error(), "item-set") {
		t.Fatalf("validateScoreProvenance error = %v, want missing item-set", err)
	}

	cfg.ItemSet = "lme-s-500q"
	if err := validateScoreProvenance(cfg); err != nil {
		t.Fatalf("validateScoreProvenance with locked metadata: %v", err)
	}
}

func TestWriteScoreReportIncludesWeakTypeCoverageErrorsAndBaselineComparison(t *testing.T) {
	dir := t.TempDir()
	cfg := &Config{
		OutDir:        dir,
		RunID:         "locked-run",
		GoldVersion:   "gold-zfs-2026-07-03",
		ScorerVersion: "tier1-qwen3-2026-06-22",
		System:        "engram-go-main",
		ItemSet:       "lme-s-500q",
		HarnessSHA:    "abc123def456",
		FeatureFlags: map[string]any{
			"scorer_lock":      "docs/lme-campaign/scorer-lock.json",
			"preserve_correct": true,
		},
	}
	provenance := longmemeval.ScoreProvenance{
		GoldVersion:   cfg.GoldVersion,
		ScorerVersion: cfg.ScorerVersion,
		System:        cfg.System,
		ItemSet:       cfg.ItemSet,
		RunID:         cfg.RunID,
		HarnessSHA:    cfg.HarnessSHA,
		FeatureFlags:  cfg.FeatureFlags,
	}

	scores := []longmemeval.ScoreEntry{
		{QuestionID: "q-pref", QuestionType: "single-session-preference", ScoreLabel: "CORRECT", Status: "done", Provenance: provenance},
		{QuestionID: "q-multi", QuestionType: "multi-session", ScoreLabel: "CORRECT", Status: "done", Provenance: provenance},
		{QuestionID: "q-temp", QuestionType: "temporal-reasoning", ScoreLabel: "INCORRECT", Status: "done", Provenance: provenance},
		{QuestionID: "q-err", QuestionType: "single-session-user", Status: "error", Error: "judge unavailable", Provenance: provenance},
	}

	writeScoreReportWithCompleteness(cfg, scores, nil, scoreCompleteness{
		ExpectedTotal:       4,
		IngestDoneTotal:     4,
		CompletedRunTotal:   4,
		CompletedScoreTotal: 3,
		ScoreErrorTotal:     1,
		Complete:            false,
	}, nil)

	data, err := os.ReadFile(filepath.Join(dir, "score_report.json"))
	if err != nil {
		t.Fatalf("read score_report.json: %v", err)
	}
	var report map[string]any
	if err := json.Unmarshal(data, &report); err != nil {
		t.Fatalf("parse score_report.json: %v", err)
	}

	byType, ok := report["by_type"].(map[string]any)
	if !ok {
		t.Fatalf("by_type = %T, want map", report["by_type"])
	}
	for _, qtype := range weakTypesInOrder() {
		if _, ok := byType[qtype]; !ok {
			t.Fatalf("by_type missing weak type %q", qtype)
		}
	}

	errorItems, ok := report["error_items"].([]any)
	if !ok || len(errorItems) != 1 {
		t.Fatalf("error_items = %v (%T), want one error row", report["error_items"], report["error_items"])
	}
	firstErr, ok := errorItems[0].(map[string]any)
	if !ok {
		t.Fatalf("error_items[0] = %T, want object", errorItems[0])
	}
	if firstErr["question_id"] != "q-err" {
		t.Fatalf("error_items[0].question_id = %v, want q-err", firstErr["question_id"])
	}

	comparison, ok := report["baseline_comparison"].(map[string]any)
	if !ok {
		t.Fatalf("baseline_comparison = %T, want map", report["baseline_comparison"])
	}
	if comparison["nearest_baseline"] != "honest_plateau" {
		t.Fatalf("nearest_baseline = %v, want honest_plateau", comparison["nearest_baseline"])
	}
	if comparison["status"] != "near" {
		t.Fatalf("baseline comparison status = %v, want near", comparison["status"])
	}

	reportProvenance, ok := report["provenance"].(map[string]any)
	if !ok {
		t.Fatalf("provenance = %T, want map", report["provenance"])
	}
	for key, want := range map[string]any{
		"gold_version":   cfg.GoldVersion,
		"scorer_version": cfg.ScorerVersion,
		"system":         cfg.System,
		"item_set":       cfg.ItemSet,
		"run_id":         cfg.RunID,
		"harness_sha":    cfg.HarnessSHA,
	} {
		if got := reportProvenance[key]; got != want {
			t.Fatalf("provenance[%q] = %v, want %v", key, got, want)
		}
	}
}

// TestScoreReportRecordsFullTimelineContextFromPersistedArtifact verifies that
// score_report.json's provenance.generation_context reflects full_timeline_context
// when the *run*-stage artifact (RUN_STATUS.json) recorded it, even though the
// scoring invocation itself passes no --full-timeline-context flag. This is how
// score/score-efficient/score-batch stay correctly labeled without requiring the
// scoring CLI to repeat the run-time flag.
func TestScoreReportRecordsFullTimelineContextFromPersistedArtifact(t *testing.T) {
	dir := t.TempDir()
	runStatus := map[string]any{
		"generation_context": "full_timeline_context",
	}
	data, err := json.Marshal(runStatus)
	if err != nil {
		t.Fatalf("marshal RUN_STATUS.json fixture: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "RUN_STATUS.json"), data, 0o600); err != nil {
		t.Fatalf("write RUN_STATUS.json fixture: %v", err)
	}

	cfg := &Config{
		OutDir: dir,
		RunID:  "full-timeline-score-run",
		// FullTimelineContext intentionally left unset: the scoring CLI does not
		// pass --full-timeline-context. Labeling must come from the persisted
		// run-stage artifact above.
	}

	scores := []longmemeval.ScoreEntry{
		{QuestionID: "q1", QuestionType: "single-session-user", ScoreLabel: "CORRECT", Status: "done"},
	}
	writeScoreReportWithCompleteness(cfg, scores, nil, scoreCompleteness{
		ExpectedTotal:       1,
		IngestDoneTotal:     1,
		CompletedRunTotal:   1,
		CompletedScoreTotal: 1,
		Complete:            true,
	}, nil)

	reportData, err := os.ReadFile(filepath.Join(dir, "score_report.json"))
	if err != nil {
		t.Fatalf("read score_report.json: %v", err)
	}
	var report map[string]any
	if err := json.Unmarshal(reportData, &report); err != nil {
		t.Fatalf("parse score_report.json: %v", err)
	}
	provenance, ok := report["provenance"].(map[string]any)
	if !ok {
		t.Fatalf("provenance = %T, want map", report["provenance"])
	}
	if got := provenance["generation_context"]; got != "full_timeline_context" {
		t.Fatalf("provenance.generation_context = %v, want full_timeline_context", got)
	}
}

// TestScoreReportRecordsRetrievalGenerationContextByDefault verifies the
// counterpart: with no persisted artifact and no --full-timeline-context flag,
// provenance.generation_context defaults to "retrieval".
func TestScoreReportRecordsRetrievalGenerationContextByDefault(t *testing.T) {
	dir := t.TempDir()
	cfg := &Config{OutDir: dir, RunID: "retrieval-score-run"}
	scores := []longmemeval.ScoreEntry{
		{QuestionID: "q1", QuestionType: "single-session-user", ScoreLabel: "CORRECT", Status: "done"},
	}
	writeScoreReportWithCompleteness(cfg, scores, nil, scoreCompleteness{
		ExpectedTotal:       1,
		IngestDoneTotal:     1,
		CompletedRunTotal:   1,
		CompletedScoreTotal: 1,
		Complete:            true,
	}, nil)

	reportData, err := os.ReadFile(filepath.Join(dir, "score_report.json"))
	if err != nil {
		t.Fatalf("read score_report.json: %v", err)
	}
	var report map[string]any
	if err := json.Unmarshal(reportData, &report); err != nil {
		t.Fatalf("parse score_report.json: %v", err)
	}
	provenance, ok := report["provenance"].(map[string]any)
	if !ok {
		t.Fatalf("provenance = %T, want map", report["provenance"])
	}
	if got := provenance["generation_context"]; got != "retrieval" {
		t.Fatalf("provenance.generation_context = %v, want retrieval", got)
	}
}

// TestRequireLockedBool covers requireLockedBool's mismatch/match/not-explicit
// paths (provenance.go:81). Only an explicit override that disagrees with the
// locked value is rejected; an unset flag or an explicit-but-matching flag
// passes silently.
func TestRequireLockedBool(t *testing.T) {
	tests := []struct {
		name     string
		got      bool
		want     bool
		explicit bool
		wantErr  bool
	}{
		{name: "not explicit, mismatched value, no error", got: true, want: false, explicit: false, wantErr: false},
		{name: "explicit and matching, no error", got: true, want: true, explicit: true, wantErr: false},
		{name: "explicit and mismatched, error", got: true, want: false, explicit: true, wantErr: true},
		{name: "explicit and mismatched other direction, error", got: false, want: true, explicit: true, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := requireLockedBool("--scorer-thinking", tt.got, tt.want, tt.explicit)
			if tt.wantErr {
				if err == nil {
					t.Fatal("requireLockedBool: want error, got nil")
				}
				if !containsAll(err.Error(), "--scorer-thinking", fmt.Sprintf("%t", tt.got), fmt.Sprintf("%t", tt.want)) {
					t.Fatalf("requireLockedBool error = %q, want flag name and both values", err.Error())
				}
			} else if err != nil {
				t.Fatalf("requireLockedBool: want no error, got %v", err)
			}
		})
	}
}

// TestRequireLockedInt covers requireLockedInt's mismatch/match/not-explicit
// paths (provenance.go:88), mirroring TestRequireLockedBool.
func TestRequireLockedInt(t *testing.T) {
	tests := []struct {
		name     string
		got      int
		want     int
		explicit bool
		wantErr  bool
	}{
		{name: "not explicit, mismatched value, no error", got: 512, want: 1536, explicit: false, wantErr: false},
		{name: "explicit and matching, no error", got: 1536, want: 1536, explicit: true, wantErr: false},
		{name: "explicit and mismatched, error", got: 512, want: 1536, explicit: true, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := requireLockedInt("--scorer-max-tokens", tt.got, tt.want, tt.explicit)
			if tt.wantErr {
				if err == nil {
					t.Fatal("requireLockedInt: want error, got nil")
				}
				if !containsAll(err.Error(), "--scorer-max-tokens", fmt.Sprintf("%d", tt.got), fmt.Sprintf("%d", tt.want)) {
					t.Fatalf("requireLockedInt error = %q, want flag name and both values", err.Error())
				}
			} else if err != nil {
				t.Fatalf("requireLockedInt: want no error, got %v", err)
			}
		})
	}
}

// TestApplyScorerLockRejectsExplicitScorerThinkingOverride is an
// applyScorerLock-level integration test exercising the requireLockedBool
// rejection path end to end: --scorer-thinking passed explicitly and
// disagreeing with the locked manifest value must fail the run rather than
// silently overriding the lock.
func TestApplyScorerLockRejectsExplicitScorerThinkingOverride(t *testing.T) {
	dir := t.TempDir()
	lockPath := filepath.Join(dir, "scorer-lock.json")
	lockBody := `{
  "version": "tier1-qwen3-2026-06-22",
  "scorer_url": "http://lock.example.test/v1",
  "scorer_model": "inference",
  "scorer_thinking": false,
  "scorer_max_tokens": 1536
}`
	if err := os.WriteFile(lockPath, []byte(lockBody), 0o600); err != nil {
		t.Fatalf("write scorer-lock.json: %v", err)
	}

	cfg := &Config{
		ScorerLockPath:    lockPath,
		ScorerThinking:    true, // conflicts with locked scorer_thinking=false
		scorerThinkingSet: true,
	}
	err := applyScorerLock(cfg)
	if err == nil {
		t.Fatal("applyScorerLock succeeded with mismatched --scorer-thinking override")
	}
	if !containsAll(err.Error(), "--scorer-thinking", "true", "false") {
		t.Fatalf("applyScorerLock error = %q, want scorer-thinking conflict details", err.Error())
	}
}

// TestApplyScorerLockRejectsExplicitScorerMaxTokensOverride mirrors the above
// for requireLockedInt via --scorer-max-tokens.
func TestApplyScorerLockRejectsExplicitScorerMaxTokensOverride(t *testing.T) {
	dir := t.TempDir()
	lockPath := filepath.Join(dir, "scorer-lock.json")
	lockBody := `{
  "version": "tier1-qwen3-2026-06-22",
  "scorer_url": "http://lock.example.test/v1",
  "scorer_model": "inference",
  "scorer_thinking": false,
  "scorer_max_tokens": 1536
}`
	if err := os.WriteFile(lockPath, []byte(lockBody), 0o600); err != nil {
		t.Fatalf("write scorer-lock.json: %v", err)
	}

	cfg := &Config{
		ScorerLockPath:     lockPath,
		ScorerMaxTokens:    4096, // conflicts with locked scorer_max_tokens=1536
		scorerMaxTokensSet: true,
	}
	err := applyScorerLock(cfg)
	if err == nil {
		t.Fatal("applyScorerLock succeeded with mismatched --scorer-max-tokens override")
	}
	if !containsAll(err.Error(), "--scorer-max-tokens", "4096", "1536") {
		t.Fatalf("applyScorerLock error = %q, want scorer-max-tokens conflict details", err.Error())
	}
}

func containsAll(s string, parts ...string) bool {
	for _, part := range parts {
		if !strings.Contains(s, part) {
			return false
		}
	}
	return true
}
