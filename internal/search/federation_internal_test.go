package search

import (
	"math"
	"testing"
)

func TestFederatedScoreGreaterTreatsNaNAsLowest(t *testing.T) {
	if federatedScoreGreater(math.NaN(), 0.5) {
		t.Fatal("NaN score must not outrank a finite score")
	}
	if !federatedScoreGreater(0.5, math.NaN()) {
		t.Fatal("finite score must outrank NaN")
	}
	if !federatedScoreGreater(0.8, 0.5) {
		t.Fatal("higher finite score should outrank lower finite score")
	}
}
