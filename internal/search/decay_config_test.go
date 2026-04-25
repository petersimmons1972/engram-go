package search

// Tests for env-var-driven decay configuration.
// This file is package search (not search_test) to access unexported helpers.
// All tests call resetDecayConfigForTesting() before exercising the env knobs
// so that sync.Once state from a prior test does not contaminate later ones.

import (
	"math"
	"testing"
)

// TestRecencyDecay_DefaultBehavior is the no-regression contract: with no
// env var set, RecencyDecay(24) must return exactly math.Exp(-0.01 * 24).
func TestRecencyDecay_DefaultBehavior(t *testing.T) {
	resetDecayConfigForTesting()
	want := math.Exp(-0.01 * 24)
	got := RecencyDecay(24)
	if got != want {
		t.Errorf("RecencyDecay(24) default = %v; want %v", got, want)
	}
}

// TestRecencyDecay_ConfiguredRate verifies a custom rate is respected.
func TestRecencyDecay_ConfiguredRate(t *testing.T) {
	resetDecayConfigForTesting()
	t.Setenv("ENGRAM_DECAY_RATE_PER_HOUR", "0.001")
	want := math.Exp(-0.001 * 24)
	got := RecencyDecay(24)
	if math.Abs(got-want) > 1e-12 {
		t.Errorf("RecencyDecay(24) with rate=0.001 = %v; want %v", got, want)
	}
}

// TestRecencyDecay_FloorApplied verifies the floor clamps very-low results.
func TestRecencyDecay_FloorApplied(t *testing.T) {
	resetDecayConfigForTesting()
	t.Setenv("ENGRAM_DECAY_FLOOR", "0.1")
	got := RecencyDecay(1000)
	if got != 0.1 {
		t.Errorf("RecencyDecay(1000) with floor=0.1 = %v; want exactly 0.1", got)
	}
}

// TestRecencyDecay_InvalidRateFallsBack verifies malformed env var falls
// back silently to the default rate.
func TestRecencyDecay_InvalidRateFallsBack(t *testing.T) {
	resetDecayConfigForTesting()
	t.Setenv("ENGRAM_DECAY_RATE_PER_HOUR", "not-a-number")
	want := math.Exp(-0.01 * 24)
	got := RecencyDecay(24)
	if got != want {
		t.Errorf("RecencyDecay(24) invalid rate = %v; want default %v", got, want)
	}
}

// TestRecencyDecay_OutOfBoundsRateFallsBack verifies rate > 10 falls back.
func TestRecencyDecay_OutOfBoundsRateFallsBack(t *testing.T) {
	resetDecayConfigForTesting()
	t.Setenv("ENGRAM_DECAY_RATE_PER_HOUR", "100")
	want := math.Exp(-0.01 * 24)
	got := RecencyDecay(24)
	if got != want {
		t.Errorf("RecencyDecay(24) out-of-bounds rate = %v; want default %v", got, want)
	}
}

// TestRecencyDecay_NegativeRateFallsBack verifies a negative rate falls back.
func TestRecencyDecay_NegativeRateFallsBack(t *testing.T) {
	resetDecayConfigForTesting()
	t.Setenv("ENGRAM_DECAY_RATE_PER_HOUR", "-0.5")
	want := math.Exp(-0.01 * 24)
	got := RecencyDecay(24)
	if got != want {
		t.Errorf("RecencyDecay(24) negative rate = %v; want default %v", got, want)
	}
}

// TestRecencyDecay_InvalidFloorFallsBack verifies floor outside [0,1] is ignored.
func TestRecencyDecay_InvalidFloorFallsBack(t *testing.T) {
	t.Run("floor too high", func(t *testing.T) {
		resetDecayConfigForTesting()
		t.Setenv("ENGRAM_DECAY_FLOOR", "1.5")
		// With no floor and default rate, RecencyDecay(1000) is near zero but not 1.5
		got := RecencyDecay(1000)
		if got == 1.5 {
			t.Error("RecencyDecay should not return 1.5 for invalid floor")
		}
		// With no valid floor the result is just the raw exp (very small)
		want := math.Exp(-0.01 * 1000)
		if math.Abs(got-want) > 1e-12 {
			t.Errorf("RecencyDecay(1000) invalid floor high = %v; want default %v", got, want)
		}
	})
	t.Run("floor negative", func(t *testing.T) {
		resetDecayConfigForTesting()
		t.Setenv("ENGRAM_DECAY_FLOOR", "-0.5")
		want := math.Exp(-0.01 * 1000)
		got := RecencyDecay(1000)
		if math.Abs(got-want) > 1e-12 {
			t.Errorf("RecencyDecay(1000) invalid floor negative = %v; want default %v", got, want)
		}
	})
}
