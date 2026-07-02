package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadScorerLockAppliesPinnedTier1Config(t *testing.T) {
	dir := t.TempDir()
	lockPath := filepath.Join(dir, "scorer-lock.json")
	lock := map[string]any{
		"schema_version": 1,
		"scorer_version": "tier1-qwen3-32b-nonthinking-v1",
		"tier1": map[string]any{
			"scorer_url":        "http://spark.local/olla/openai/v1",
			"scorer_model":      "inference",
			"scorer_thinking":   false,
			"scorer_max_tokens": 2048,
			"preserve_correct":  true,
			"force_rescore":     false,
		},
	}
	data, err := json.Marshal(lock)
	if err != nil {
		t.Fatalf("marshal lock: %v", err)
	}
	if err := os.WriteFile(lockPath, data, 0o644); err != nil {
		t.Fatalf("write lock: %v", err)
	}

	cfg := &Config{PreserveCorrect: true}
	if err := loadScorerLock(cfg, lockPath); err != nil {
		t.Fatalf("loadScorerLock: %v", err)
	}

	if cfg.ScorerVersion != "tier1-qwen3-32b-nonthinking-v1" {
		t.Fatalf("ScorerVersion = %q, want tier1-qwen3-32b-nonthinking-v1", cfg.ScorerVersion)
	}
	if cfg.ScorerURL != "http://spark.local/olla/openai/v1" {
		t.Fatalf("ScorerURL = %q, want locked URL", cfg.ScorerURL)
	}
	if cfg.ScorerModel != "inference" {
		t.Fatalf("ScorerModel = %q, want locked model", cfg.ScorerModel)
	}
	if cfg.ScorerThinking {
		t.Fatal("ScorerThinking = true, want false from lock")
	}
	if cfg.ScorerMaxTokens != 2048 {
		t.Fatalf("ScorerMaxTokens = %d, want 2048", cfg.ScorerMaxTokens)
	}
	if !cfg.PreserveCorrect {
		t.Fatal("PreserveCorrect = false, want true from lock")
	}
	if cfg.ForceRescore {
		t.Fatal("ForceRescore = true, want false from lock")
	}
}

func TestReadScorerLockRejectsInvalidManifest(t *testing.T) {
	dir := t.TempDir()
	lockPath := filepath.Join(dir, "scorer-lock.json")
	data := []byte(`{"schema_version":2,"scorer_version":"","tier1":{"scorer_url":"","scorer_model":"","scorer_max_tokens":0}}`)
	if err := os.WriteFile(lockPath, data, 0o644); err != nil {
		t.Fatalf("write lock: %v", err)
	}

	if _, err := readScorerLock(lockPath); err == nil {
		t.Fatal("readScorerLock returned nil error for invalid manifest")
	}
}
