package main

import (
	"encoding/json"
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

	writeScoreReportWithCompleteness(cfg, scores, scoreCompleteness{
		ExpectedTotal:       4,
		IngestDoneTotal:     4,
		CompletedRunTotal:   4,
		CompletedScoreTotal: 3,
		ScoreErrorTotal:     1,
		Complete:            false,
	})

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

func containsAll(s string, parts ...string) bool {
	for _, part := range parts {
		if !strings.Contains(s, part) {
			return false
		}
	}
	return true
}
