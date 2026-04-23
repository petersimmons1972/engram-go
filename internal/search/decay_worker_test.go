package search

// Tests for env-var-driven DecayWorker interval configuration.
// Package search (not search_test) so we can access w.interval directly.

import (
	"testing"
	"time"
)

// TestNewDecayWorker_DefaultInterval verifies that when interval==0 and no
// env var is set, the worker uses the 8-hour default.
func TestNewDecayWorker_DefaultInterval(t *testing.T) {
	resetDecayIntervalConfigForTesting()
	w := NewDecayWorker(nil, "test", 0)
	if w.interval != 8*time.Hour {
		t.Errorf("default interval = %v; want 8h", w.interval)
	}
}

// TestNewDecayWorker_ConfiguredInterval verifies ENGRAM_DECAY_INTERVAL_HOURS
// is respected when interval==0.
func TestNewDecayWorker_ConfiguredInterval(t *testing.T) {
	resetDecayIntervalConfigForTesting()
	t.Setenv("ENGRAM_DECAY_INTERVAL_HOURS", "4")
	w := NewDecayWorker(nil, "test", 0)
	if w.interval != 4*time.Hour {
		t.Errorf("configured interval = %v; want 4h", w.interval)
	}
}

// TestNewDecayWorker_ExplicitIntervalWins verifies that a non-zero caller
// value takes precedence over the env var.
func TestNewDecayWorker_ExplicitIntervalWins(t *testing.T) {
	resetDecayIntervalConfigForTesting()
	t.Setenv("ENGRAM_DECAY_INTERVAL_HOURS", "4")
	w := NewDecayWorker(nil, "test", 2*time.Hour)
	if w.interval != 2*time.Hour {
		t.Errorf("explicit interval = %v; want 2h", w.interval)
	}
}

// TestNewDecayWorker_InvalidEnvFallsBack verifies a non-numeric env value
// falls back to the 8-hour default.
func TestNewDecayWorker_InvalidEnvFallsBack(t *testing.T) {
	resetDecayIntervalConfigForTesting()
	t.Setenv("ENGRAM_DECAY_INTERVAL_HOURS", "abc")
	w := NewDecayWorker(nil, "test", 0)
	if w.interval != 8*time.Hour {
		t.Errorf("invalid env interval = %v; want 8h default", w.interval)
	}
}
