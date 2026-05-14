package search

import (
	"log/slog"
	"math"
	"sync"

	"github.com/petersimmons1972/engram/internal/envconf"
)

const (
	weightVector    = 0.40 // was 0.45; reduced to strengthen recency signal
	weightBM25      = 0.30
	weightRecency   = 0.15 // was 0.10; raised to help knowledge-update and temporal recall
	weightPrecision = 0.15
	decayRate       = 0.01 // per hour — canonical default, used when env var is absent or invalid

	// temporalWeightRecency and temporalWeightVector define the recency-boosted
	// weight profile applied when a recall query is classified as time-anchored.
	temporalWeightVector    = 0.35
	temporalWeightBM25      = 0.25
	temporalWeightRecency   = 0.30
	temporalWeightPrecision = 0.10

	// knowledgeUpdateWeights are a mid-point profile for queries asking about
	// current/changed state. Recency is raised above the default but below the
	// full temporal profile — enough to prefer the more recent fact without
	// completely discarding semantic relevance.
	kuWeightVector    = 0.38
	kuWeightBM25      = 0.27
	kuWeightRecency   = 0.25 // moderate value: better than 0.22 baseline, avoids over-weighting recency without RRF
	kuWeightPrecision = 0.13
)

// decayConfig holds the resolved env-var knobs for recency decay.
// It is populated exactly once via decayConfigOnce.
type decayConfig struct {
	rate  float64 // ENGRAM_DECAY_RATE_PER_HOUR; default decayRate
	floor float64 // ENGRAM_DECAY_FLOOR; default 0.0 (no floor)
}

var (
	decayConfigOnce sync.Once
	resolvedDecay   decayConfig
)

// ResetDecayConfigForTesting resets the decay config sync.Once so tests can inject different env var values.
func ResetDecayConfigForTesting() {
	decayConfigOnce = sync.Once{}
}

// loadDecayConfig reads ENGRAM_DECAY_RATE_PER_HOUR and ENGRAM_DECAY_FLOOR,
// applies sanity bounds, and populates resolvedDecay. Called exactly once.
func loadDecayConfig() {
	cfg := decayConfig{
		rate:  decayRate,
		floor: 0.0,
	}

	if v := envconf.Float("ENGRAM_DECAY_RATE_PER_HOUR", decayRate); v != decayRate {
		switch {
		case v <= 0:
			slog.Warn("ENGRAM_DECAY_RATE_PER_HOUR: must be positive, using default",
				"value", v, "default", decayRate)
		case v > 10:
			slog.Warn("ENGRAM_DECAY_RATE_PER_HOUR: value >10 is unusable, using default",
				"value", v, "default", decayRate)
		default:
			cfg.rate = v
		}
	}

	cfg.floor = envconf.FloatBounded("ENGRAM_DECAY_FLOOR", 0.0, 0.0, 1.0)

	resolvedDecay = cfg
}

// decayCfg returns the resolved decay configuration, loading it on first call.
func decayCfg() decayConfig {
	decayConfigOnce.Do(loadDecayConfig)
	return resolvedDecay
}

// Weights holds one complete set of composite scoring weights.
// The compile-time constants above are the canonical defaults.
// Per-project overrides loaded from the database replace these in the engine.
type Weights struct {
	Vector    float64
	BM25      float64
	Recency   float64
	Precision float64
}

// DefaultWeights returns the compile-time weight constants.
func DefaultWeights() Weights {
	return Weights{
		Vector:    weightVector,
		BM25:      weightBM25,
		Recency:   weightRecency,
		Precision: weightPrecision,
	}
}

// KnowledgeUpdateWeights returns a mid-point recency-boosted profile for queries
// asking about current or changed state (e.g., "where does X live currently?").
// Recency is higher than the default (0.22 vs 0.15) but below the full temporal
// profile (0.30), preserving semantic relevance for partially-updated facts.
func KnowledgeUpdateWeights() Weights {
	return Weights{
		Vector:    kuWeightVector,
		BM25:      kuWeightBM25,
		Recency:   kuWeightRecency,
		Precision: kuWeightPrecision,
	}
}

// TemporalWeights returns the recency-boosted weight profile used when a recall
// query is classified as time-anchored (via IsTemporalQuery). Recency is raised
// to 0.30 so that chronologically recent sessions rank above semantically similar
// but older sessions — directly targeting temporal-reasoning benchmark failures.
func TemporalWeights() Weights {
	return Weights{
		Vector:    temporalWeightVector,
		BM25:      temporalWeightBM25,
		Recency:   temporalWeightRecency,
		Precision: temporalWeightPrecision,
	}
}

// ScoreInput holds the raw signals for composite scoring.
type ScoreInput struct {
	Cosine             float64  // cosine similarity [0,1]
	BM25               float64  // normalized BM25 score [0,1]
	HoursSince         float64  // hours since last access
	Importance         int      // [0,4]; 0=critical (never pruned), 4=trivial (auto-pruned)
	DynamicImportance  *float64 // learned importance from spaced repetition; overrides Importance when non-nil
	RetrievalPrecision *float64 // times_useful/times_retrieved; nil during cold start (<5 retrievals) → treated as 0.5
	EpisodeMatch       bool     // true when memory.episode_id == current session episode
	MemoryType         string   // memory_type field; used to apply type-specific boosts
	IsPreferenceQuery  bool     // true when the recall query is preference-shaped (#364)
}

// RecencyDecay returns exp(-rate * hours), optionally clamped by a floor.
// The rate defaults to 0.01/hr; override via ENGRAM_DECAY_RATE_PER_HOUR.
// The floor defaults to 0 (no floor); override via ENGRAM_DECAY_FLOOR.
// When neither env var is set the result is exactly math.Exp(-0.01 * hours).
func RecencyDecay(hoursSince float64) float64 {
	cfg := decayCfg()
	v := math.Exp(-cfg.rate * hoursSince)
	if cfg.floor > 0 {
		v = math.Max(cfg.floor, v)
	}
	return v
}

// ImportanceBoost returns a multiplier reflecting memory importance.
// The importance scale is inverted: 0=critical (never pruned), 4=trivial (auto-pruned).
// We invert so critical memories receive the highest boost:
//
//	importance=0 → 5/3 ≈ 1.67 (critical)
//	importance=2 → 3/3 = 1.00 (neutral)
//	importance=4 → 1/3 ≈ 0.33 (trivial)
func ImportanceBoost(importance int) float64 {
	// Clamp to 0.1 floor so hand-inserted rows with importance>=5 still rank
	// above zero — without this, importance=5 returns exactly 0.0 and the
	// memory is invisible in composite scoring (#134).
	return math.Max(0.1, float64(5-importance)) / 3.0
}

// CompositeScore combines vector, BM25, recency, precision, and importance signals
// into a single rank score using the compile-time default weights.
//
//   - DynamicImportance: when non-nil, used as the boost multiplier (clamped to
//     [0.1, ∞]) instead of the static Importance field.
//   - RetrievalPrecision: when nil (cold-start, <5 retrievals), treated as 0.5
//     (neutral), so it neither helps nor hurts new memories.
func CompositeScore(in ScoreInput) float64 {
	return CompositeScoreWithWeights(in, DefaultWeights())
}

// RRFScore computes reciprocal rank fusion score for a memory ID across two rank lists.
// vectorRank and bm25Rank are 1-based positions in their respective sorted result lists;
// pass 0 to indicate the memory was absent from that leg.
// k=60 is the standard constant recommended by Cormack et al. 2009.
func RRFScore(vectorRank, bm25Rank, k int) float64 {
	score := 0.0
	if vectorRank > 0 {
		score += 1.0 / float64(k+vectorRank)
	}
	if bm25Rank > 0 {
		score += 1.0 / float64(k+bm25Rank)
	}
	return score
}

// CompositeScoreRRF is identical to CompositeScoreWithWeights but uses rrfBase
// (a pre-computed RRF score from RRFScore) in place of the raw cosine and BM25 signals.
// The RRF score is scaled into the same budget as the combined vector+BM25 weight
// terms so that the recency and precision weights retain their additive meaning.
// Post-fusion boosts (episode, preference, importance) are applied unchanged.
func CompositeScoreRRF(in ScoreInput, w Weights, rrfBase float64) float64 {
	const rrfK = 60
	// Scale RRF into the combined vector+BM25 budget.
	// Max RRF with both legs at rank 1: 2/(k+1). Multiply by (k+1)/2 to normalize to [0,1],
	// then by (w.Vector+w.BM25) to map into the weight budget.
	rrfScaled := rrfBase * float64(rrfK+1) / 2.0 * (w.Vector + w.BM25)

	recency := RecencyDecay(in.HoursSince)
	var boost float64
	if in.DynamicImportance != nil {
		boost = math.Max(0.1, *in.DynamicImportance)
	} else {
		boost = ImportanceBoost(in.Importance)
	}
	precision := 0.5
	if in.RetrievalPrecision != nil {
		precision = *in.RetrievalPrecision
	}
	raw := rrfScaled + w.Recency*recency + w.Precision*precision
	if in.EpisodeMatch {
		raw *= 1.15
	}
	if in.IsPreferenceQuery && in.MemoryType == "preference" {
		raw *= 1.8
	}
	return raw * boost
}

// CompositeScoreWithWeights is identical to CompositeScore but uses the
// caller-supplied weight set rather than the compile-time defaults.
// The engine uses this when per-project weights have been loaded from the DB.
func CompositeScoreWithWeights(in ScoreInput, w Weights) float64 {
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
	raw := w.Vector*in.Cosine + w.BM25*in.BM25 + w.Recency*recency + w.Precision*precision
	if in.EpisodeMatch {
		raw *= 1.15
	}
	// Boost preference-typed memories when the query is preference-shaped (#364).
	// Preference queries ("what does the user prefer?") don't always vector-match
	// well against preference-expressing memories ("I really like X"), so a
	// type-aware boost compensates for the embedding space gap.
	if in.IsPreferenceQuery && in.MemoryType == "preference" {
		raw *= 1.35
	}
	return raw * boost
}
