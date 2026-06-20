package main

import (
	"reflect"
	"testing"
)

func TestRunAllWithStagesStopsBeforeScoreWhenRunFails(t *testing.T) {
	var calls []string
	stages := allStages{
		ingest: func(*Config) {
			calls = append(calls, "ingest")
		},
		run: func(*Config) int {
			calls = append(calls, "run")
			return 7
		},
		score: func(*Config) int {
			calls = append(calls, "score")
			return 0
		},
	}

	exit := runAllWithStages(&Config{DataFile: "questions.json"}, stages)

	if exit != 7 {
		t.Fatalf("runAllWithStages exit = %d, want run stage exit 7", exit)
	}
	if want := []string{"ingest", "run"}; !reflect.DeepEqual(calls, want) {
		t.Fatalf("stage calls = %v, want %v", calls, want)
	}
}

func TestRunAllWithStagesRunsScoreAfterSuccessfulRun(t *testing.T) {
	var calls []string
	cfg := &Config{DataFile: "questions.json"}
	stages := allStages{
		ingest: func(*Config) {
			calls = append(calls, "ingest")
		},
		run: func(*Config) int {
			calls = append(calls, "run")
			return 0
		},
		score: func(*Config) int {
			calls = append(calls, "score")
			return 0
		},
	}

	exit := runAllWithStages(cfg, stages)

	if exit != 0 {
		t.Fatalf("runAllWithStages exit = %d, want 0", exit)
	}
	if cfg.RunID == "" {
		t.Fatal("runAllWithStages must assign a run ID when missing")
	}
	if want := []string{"ingest", "run", "score"}; !reflect.DeepEqual(calls, want) {
		t.Fatalf("stage calls = %v, want %v", calls, want)
	}
}

func TestApplyRepairPresetRecallRepair(t *testing.T) {
	cfg := &Config{RepairPreset: "recall-repair"}
	if err := applyRepairPreset(cfg); err != nil {
		t.Fatalf("applyRepairPreset: %v", err)
	}
	if !cfg.DualPreferenceRecall {
		t.Error("recall-repair preset should enable dual preference recall")
	}
	if !cfg.ExhaustiveAggregation {
		t.Error("recall-repair preset should enable exhaustive aggregation")
	}
	if !cfg.EnumerateFirst {
		t.Error("recall-repair preset should enable enumerate-first")
	}
	if !cfg.TemporalPromptAug {
		t.Error("recall-repair preset should enable temporal prompt augmentation")
	}
	if !cfg.ChronoSort {
		t.Error("recall-repair preset should enable chronological context ordering")
	}
	if !cfg.PreferenceSessionRerank {
		t.Error("recall-repair preset should enable preference session rerank")
	}
}

func TestApplyRepairPresetDefaultBaselineNoop(t *testing.T) {
	cfg := &Config{}
	if err := applyRepairPreset(cfg); err != nil {
		t.Fatalf("applyRepairPreset default: %v", err)
	}
	if cfg.DualPreferenceRecall || cfg.ExhaustiveAggregation || cfg.EnumerateFirst ||
		cfg.TemporalPromptAug || cfg.ChronoSort || cfg.PreferenceSessionRerank {
		t.Fatalf("default config should leave repair switches off: %+v", cfg)
	}
}
