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
		score: func(*Config) {
			calls = append(calls, "score")
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
		score: func(*Config) {
			calls = append(calls, "score")
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
