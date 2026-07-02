package main

import (
	"encoding/json"
	"fmt"
	"os"
)

type scorerLockFile struct {
	SchemaVersion int             `json:"schema_version"`
	ScorerVersion string          `json:"scorer_version"`
	Tier1         scorerLockTier1 `json:"tier1"`
}

type scorerLockTier1 struct {
	ScorerURL       string `json:"scorer_url"`
	ScorerModel     string `json:"scorer_model"`
	ScorerThinking  bool   `json:"scorer_thinking"`
	ScorerMaxTokens int    `json:"scorer_max_tokens"`
	PreserveCorrect bool   `json:"preserve_correct"`
	ForceRescore    bool   `json:"force_rescore"`
}

func readScorerLock(path string) (scorerLockFile, error) {
	var lock scorerLockFile
	data, err := os.ReadFile(path)
	if err != nil {
		return lock, err
	}
	if err := json.Unmarshal(data, &lock); err != nil {
		return lock, err
	}
	if lock.SchemaVersion != 1 {
		return lock, fmt.Errorf("schema_version=%d, want 1", lock.SchemaVersion)
	}
	if lock.ScorerVersion == "" {
		return lock, fmt.Errorf("scorer_version is required")
	}
	if lock.Tier1.ScorerURL == "" {
		return lock, fmt.Errorf("tier1.scorer_url is required")
	}
	if lock.Tier1.ScorerModel == "" {
		return lock, fmt.Errorf("tier1.scorer_model is required")
	}
	if lock.Tier1.ScorerMaxTokens <= 0 {
		return lock, fmt.Errorf("tier1.scorer_max_tokens must be > 0")
	}
	return lock, nil
}

func loadScorerLock(cfg *Config, path string) error {
	lock, err := readScorerLock(path)
	if err != nil {
		return err
	}
	cfg.ScorerLockPath = path
	cfg.ScorerVersion = lock.ScorerVersion
	cfg.ScorerURL = lock.Tier1.ScorerURL
	cfg.ScorerModel = lock.Tier1.ScorerModel
	cfg.ScorerThinking = lock.Tier1.ScorerThinking
	cfg.ScorerMaxTokens = lock.Tier1.ScorerMaxTokens
	cfg.PreserveCorrect = lock.Tier1.PreserveCorrect
	cfg.ForceRescore = lock.Tier1.ForceRescore
	return nil
}
