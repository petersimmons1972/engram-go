package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/petersimmons1972/engram/internal/longmemeval"
)

const (
	honestPlateauPct      = 70.0
	lenientArtifactPct    = 81.4
	baselineNearThreshold = 4.0
)

type scorerLockManifest struct {
	Version         string `json:"version"`
	ScorerURL       string `json:"scorer_url"`
	ScorerModel     string `json:"scorer_model"`
	ScorerThinking  bool   `json:"scorer_thinking"`
	ScorerMaxTokens int    `json:"scorer_max_tokens"`
}

func applyScorerLock(cfg *Config) error {
	if cfg == nil || strings.TrimSpace(cfg.ScorerLockPath) == "" {
		return nil
	}
	data, err := os.ReadFile(cfg.ScorerLockPath)
	if err != nil {
		return fmt.Errorf("read --scorer-lock %q: %w", cfg.ScorerLockPath, err)
	}
	var manifest scorerLockManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return fmt.Errorf("parse --scorer-lock %q: %w", cfg.ScorerLockPath, err)
	}
	if strings.TrimSpace(manifest.Version) == "" {
		return fmt.Errorf("--scorer-lock %q: missing version", cfg.ScorerLockPath)
	}
	if strings.TrimSpace(manifest.ScorerURL) == "" {
		return fmt.Errorf("--scorer-lock %q: missing scorer_url", cfg.ScorerLockPath)
	}
	if strings.TrimSpace(manifest.ScorerModel) == "" {
		return fmt.Errorf("--scorer-lock %q: missing scorer_model", cfg.ScorerLockPath)
	}
	if manifest.ScorerMaxTokens <= 0 {
		manifest.ScorerMaxTokens = longmemeval.DefaultScorerMaxTokens
	}

	if err := requireLockedString("--scorer-url", cfg.ScorerURL, manifest.ScorerURL); err != nil {
		return err
	}
	if err := requireLockedString("--scorer-model", cfg.ScorerModel, manifest.ScorerModel); err != nil {
		return err
	}
	if err := requireLockedBool("--scorer-thinking", cfg.ScorerThinking, manifest.ScorerThinking, cfg.scorerThinkingSet); err != nil {
		return err
	}
	if err := requireLockedInt("--scorer-max-tokens", cfg.ScorerMaxTokens, manifest.ScorerMaxTokens, cfg.scorerMaxTokensSet); err != nil {
		return err
	}

	cfg.ScorerVersion = manifest.Version
	cfg.ScorerURL = manifest.ScorerURL
	cfg.ScorerModel = manifest.ScorerModel
	cfg.ScorerThinking = manifest.ScorerThinking
	cfg.ScorerMaxTokens = manifest.ScorerMaxTokens
	return nil
}

func requireLockedString(flagName, got, want string) error {
	got = strings.TrimSpace(got)
	if got == "" || got == want {
		return nil
	}
	return fmt.Errorf("%s=%q conflicts with locked scorer value %q", flagName, got, want)
}

func requireLockedBool(flagName string, got, want, explicit bool) error {
	if !explicit || got == want {
		return nil
	}
	return fmt.Errorf("%s=%t conflicts with locked scorer value %t", flagName, got, want)
}

func requireLockedInt(flagName string, got, want int, explicit bool) error {
	if !explicit || got == want {
		return nil
	}
	return fmt.Errorf("%s=%d conflicts with locked scorer value %d", flagName, got, want)
}

func validateScoreProvenance(cfg *Config) error {
	if cfg == nil || strings.TrimSpace(cfg.ScorerLockPath) == "" {
		return nil
	}
	var missing []string
	if strings.TrimSpace(cfg.GoldVersion) == "" {
		missing = append(missing, "--gold-version")
	}
	if strings.TrimSpace(cfg.ItemSet) == "" {
		missing = append(missing, "--item-set")
	}
	if strings.TrimSpace(cfg.ScorerVersion) == "" {
		missing = append(missing, "--scorer-lock version")
	}
	if len(missing) == 0 {
		return nil
	}
	return fmt.Errorf("locked scorer provenance requires %s", strings.Join(missing, " and "))
}

func scoreProvenanceForConfig(cfg *Config) longmemeval.ScoreProvenance {
	if cfg == nil {
		return longmemeval.ScoreProvenance{}
	}
	harnessSHA := strings.TrimSpace(cfg.HarnessSHA)
	if harnessSHA == "" {
		harnessSHA = bestEffortGit("rev-parse", "HEAD")
	}
	system := strings.TrimSpace(cfg.System)
	if system == "" {
		system = inferSystemName(cfg)
	}
	itemSet := strings.TrimSpace(cfg.ItemSet)
	if itemSet == "" {
		itemSet = inferItemSet(cfg.DataFile)
	}
	return longmemeval.ScoreProvenance{
		GoldVersion:       strings.TrimSpace(cfg.GoldVersion),
		ScorerVersion:     strings.TrimSpace(cfg.ScorerVersion),
		FeatureFlags:      buildFeatureFlags(cfg),
		System:            system,
		ItemSet:           itemSet,
		RunID:             strings.TrimSpace(cfg.RunID),
		HarnessSHA:        harnessSHA,
		GenerationContext: generationContextForArtifacts(cfg, "score"),
	}
}

func inferSystemName(cfg *Config) string {
	if cfg == nil {
		return "engram-go"
	}
	if strings.TrimSpace(cfg.System) != "" {
		return cfg.System
	}
	if strings.TrimSpace(cfg.LLMModel) != "" {
		return cfg.LLMModel
	}
	if strings.TrimSpace(cfg.GenerationModel) != "" {
		return cfg.GenerationModel
	}
	return "engram-go"
}

func inferItemSet(dataFile string) string {
	if strings.TrimSpace(dataFile) == "" {
		return ""
	}
	base := filepath.Base(dataFile)
	ext := filepath.Ext(base)
	return strings.TrimSuffix(base, ext)
}

func buildFeatureFlags(cfg *Config) map[string]any {
	if cfg == nil {
		return nil
	}
	flags := map[string]any{
		"preserve_correct":  cfg.PreserveCorrect,
		"force_rescore":     cfg.ForceRescore,
		"scorer_thinking":   cfg.ScorerThinking,
		"scorer_max_tokens": effectiveScorerMaxTokens(cfg),
	}
	if strings.TrimSpace(cfg.ScorerLockPath) != "" {
		flags["scorer_lock"] = cfg.ScorerLockPath
	}
	if cfg.RecallTopK != 0 {
		flags["recall_topk"] = cfg.RecallTopK
	}
	if cfg.ContextTopKOverride != 0 {
		flags["context_topk"] = cfg.ContextTopKOverride
	}
	if cfg.FullTimelineContext {
		flags["full_timeline_context"] = true
	}
	if cfg.QueryParaphrasePasses != 0 {
		flags["query_paraphrase_passes"] = cfg.QueryParaphrasePasses
	}
	if cfg.MaxBlockChars != 0 {
		flags["max_block_chars"] = cfg.MaxBlockChars
	}
	if cfg.ContextTopKBump {
		flags["context_topk_bump"] = true
	}
	if cfg.ChronoSort {
		flags["chrono_sort"] = true
	}
	if cfg.DisableQueryRewrite {
		flags["disable_query_rewrite"] = true
	}
	if cfg.InjectQuestionDate {
		flags["inject_question_date"] = true
	}
	if cfg.TemporalWindowRecall {
		flags["temporal_window_recall"] = true
	}
	if cfg.TemporalPromptAug {
		flags["temporal_prompt_aug"] = true
	}
	if cfg.DualPreferenceRecall {
		flags["dual_preference_recall"] = true
	}
	if cfg.TopicAnchorBoost {
		flags["topic_anchor_boost"] = true
	}
	if cfg.ExhaustiveAggregation {
		flags["exhaustive_aggregation"] = true
	}
	if cfg.EnumerateFirst {
		flags["enumerate_first"] = true
	}
	if cfg.RetrievalFusion {
		flags["retrieval_fusion"] = true
	}
	if cfg.ExactSignalBoost {
		flags["exact_signal_boost"] = true
	}
	if cfg.EvidenceFirstPacked {
		flags["evidence_first_pack"] = true
	}
	if cfg.SessionNDCGAgg {
		flags["session_ndcg_agg"] = true
	}
	if cfg.AtomMode {
		flags["atom_mode"] = true
	}
	if cfg.AtomOracle {
		flags["atom_oracle"] = true
	}
	if cfg.AntiHedgePrompts {
		flags["anti_hedge_prompts"] = true
	}
	return flags
}

func effectiveScorerMaxTokens(cfg *Config) int {
	if cfg == nil || cfg.ScorerMaxTokens <= 0 {
		return longmemeval.DefaultScorerMaxTokens
	}
	return cfg.ScorerMaxTokens
}

func weakTypesInOrder() []string {
	return []string{
		"single-session-preference",
		"multi-session",
		"temporal-reasoning",
		"single-session-user",
		"knowledge-update",
		"single-session-assistant",
	}
}

func buildBaselineComparison(overall *scoreReportCounts) map[string]any {
	if overall == nil || overall.Total == 0 {
		return map[string]any{
			"status":              "insufficient-data",
			"nearest_baseline":    "",
			"observed_strict_pct": 0.0,
		}
	}
	observed := float64(overall.Correct) / float64(overall.Total) * 100
	honestDelta := absFloat(observed - honestPlateauPct)
	artifactDelta := absFloat(observed - lenientArtifactPct)

	nearest := "honest_plateau"
	nearestTarget := honestPlateauPct
	nearestDelta := honestDelta
	if artifactDelta < honestDelta {
		nearest = "artifact_81_4"
		nearestTarget = lenientArtifactPct
		nearestDelta = artifactDelta
	}
	status := "near"
	if nearestDelta > baselineNearThreshold {
		status = "diverges"
	}
	return map[string]any{
		"status":                         status,
		"nearest_baseline":               nearest,
		"nearest_baseline_pct":           nearestTarget,
		"observed_strict_pct":            observed,
		"distance_to_honest_plateau_pct": honestDelta,
		"distance_to_artifact_pct":       artifactDelta,
		"honest_plateau_pct":             honestPlateauPct,
		"artifact_pct":                   lenientArtifactPct,
	}
}

func absFloat(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}
