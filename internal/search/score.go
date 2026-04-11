package search

import "math"

const (
	weightVector    = 0.45  // was 0.50; reduced to make room for precision signal
	weightBM25      = 0.30  // was 0.35
	weightRecency   = 0.10  // was 0.15
	weightPrecision = 0.15  // new: retrieval outcome precision
	decayRate       = 0.01  // per hour
)

// ScoreInput holds the raw signals for composite scoring.
type ScoreInput struct {
	Cosine             float64  // cosine similarity [0,1]
	BM25               float64  // normalized BM25 score [0,1]
	HoursSince         float64  // hours since last access
	Importance         int      // [0,4]; 0=critical (never pruned), 4=trivial (auto-pruned)
	DynamicImportance  *float64 // learned importance from spaced repetition; overrides Importance when non-nil
	RetrievalPrecision *float64 // times_useful/times_retrieved; nil during cold start (<5 retrievals) → treated as 0.5
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

// CompositeScore combines vector, BM25, recency, precision, and importance signals
// into a single rank score.
//
//   - DynamicImportance: when non-nil, used as the boost multiplier (clamped to
//     [0.1, ∞]) instead of the static Importance field.
//   - RetrievalPrecision: when nil (cold-start, <5 retrievals), treated as 0.5
//     (neutral), so it neither helps nor hurts new memories.
func CompositeScore(in ScoreInput) float64 {
	recency := RecencyDecay(in.HoursSince)
	var boost float64
	if in.DynamicImportance != nil {
		boost = math.Max(0.1, *in.DynamicImportance)
	} else {
		boost = ImportanceBoost(in.Importance)
	}
	precision := 0.5 // neutral cold-start
	if in.RetrievalPrecision != nil {
		precision = *in.RetrievalPrecision
	}
	raw := weightVector*in.Cosine + weightBM25*in.BM25 + weightRecency*recency + weightPrecision*precision
	return raw * boost
}
