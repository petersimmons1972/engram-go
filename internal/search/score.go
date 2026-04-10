package search

import "math"

const (
	weightVector  = 0.50
	weightBM25    = 0.35
	weightRecency = 0.15
	decayRate     = 0.01 // per hour
)

// ScoreInput holds the raw signals for composite scoring.
type ScoreInput struct {
	Cosine     float64 // cosine similarity [0,1]
	BM25       float64 // normalized BM25 score [0,1]
	HoursSince float64 // hours since last access
	Importance int     // [0,4]; 0=critical (never pruned), 4=trivial (auto-pruned)
}

// RecencyDecay returns exp(-decayRate * hours). Result is in (0,1].
func RecencyDecay(hoursSince float64) float64 {
	return math.Exp(-decayRate * hoursSince)
}

// ImportanceBoost returns a multiplier reflecting memory importance.
// The importance scale is inverted: 0=critical (never pruned), 4=trivial (auto-pruned).
// We invert so critical memories receive the highest boost:
//
//	importance=0 → 5/3 ≈ 1.67 (critical)
//	importance=2 → 3/3 = 1.00 (neutral)
//	importance=4 → 1/3 ≈ 0.33 (trivial)
func ImportanceBoost(importance int) float64 {
	return math.Max(0, float64(5-importance)) / 3.0
}

// CompositeScore combines vector, BM25, and recency signals into a single rank score.
func CompositeScore(in ScoreInput) float64 {
	recency := RecencyDecay(in.HoursSince)
	boost := ImportanceBoost(in.Importance)
	raw := weightVector*in.Cosine + weightBM25*in.BM25 + weightRecency*recency
	return raw * boost
}
