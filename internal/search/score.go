package search

import (
	"log/slog"
	"math"
	"sync"
	"time"

	"github.com/petersimmons1972/engram/internal/envconf"
	"github.com/petersimmons1972/engram/internal/types"
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

// weightConfig holds the resolved env-var knobs for composite scoring weights.
// It is populated exactly once via weightConfigOnce.
type weightConfig struct {
	vector    float64 // ENGRAM_W_COSINE;     default weightVector
	bm25      float64 // ENGRAM_W_BM25;       default weightBM25
	recency   float64 // ENGRAM_W_RECENCY;    default weightRecency
	precision float64 // ENGRAM_W_PRECISION;  default weightPrecision
}

var (
	weightConfigOnce sync.Once
	resolvedWeights  weightConfig
)

// temporalWeightConfig holds the resolved env-var knobs for TemporalWeights.
// It is populated exactly once via temporalWeightConfigOnce.
type temporalWeightConfig struct {
	vector    float64 // ENGRAM_W_TEMPORAL_COSINE;    default temporalWeightVector
	bm25      float64 // ENGRAM_W_TEMPORAL_BM25;      default temporalWeightBM25
	recency   float64 // ENGRAM_W_TEMPORAL_RECENCY;   default temporalWeightRecency
	precision float64 // ENGRAM_W_TEMPORAL_PRECISION; default temporalWeightPrecision
}

var (
	temporalWeightConfigOnce sync.Once
	resolvedTemporalWeights  temporalWeightConfig
)

// kuWeightConfig holds the resolved env-var knobs for KnowledgeUpdateWeights.
// It is populated exactly once via kuWeightConfigOnce.
type kuWeightConfig struct {
	vector    float64 // ENGRAM_W_KU_COSINE;    default kuWeightVector
	bm25      float64 // ENGRAM_W_KU_BM25;      default kuWeightBM25
	recency   float64 // ENGRAM_W_KU_RECENCY;   default kuWeightRecency
	precision float64 // ENGRAM_W_KU_PRECISION; default kuWeightPrecision
}

var (
	kuWeightConfigOnce sync.Once
	resolvedKUWeights  kuWeightConfig
)

// loadTemporalWeightConfig reads ENGRAM_W_TEMPORAL_* env vars. Each is
// independently optional; missing or invalid values fall back to the
// compile-time temporal constants.
func loadTemporalWeightConfig() {
	resolvedTemporalWeights = temporalWeightConfig{
		vector:    envconf.FloatBounded("ENGRAM_W_TEMPORAL_COSINE", temporalWeightVector, 0.0, 1.0),
		bm25:      envconf.FloatBounded("ENGRAM_W_TEMPORAL_BM25", temporalWeightBM25, 0.0, 1.0),
		recency:   envconf.FloatBounded("ENGRAM_W_TEMPORAL_RECENCY", temporalWeightRecency, 0.0, 1.0),
		precision: envconf.FloatBounded("ENGRAM_W_TEMPORAL_PRECISION", temporalWeightPrecision, 0.0, 1.0),
	}
}

// temporalWeightCfg returns the resolved temporal weight configuration, loading it on first call.
func temporalWeightCfg() temporalWeightConfig {
	temporalWeightConfigOnce.Do(loadTemporalWeightConfig)
	return resolvedTemporalWeights
}

// loadKUWeightConfig reads ENGRAM_W_KU_* env vars. Each is independently
// optional; missing or invalid values fall back to the compile-time KU constants.
func loadKUWeightConfig() {
	resolvedKUWeights = kuWeightConfig{
		vector:    envconf.FloatBounded("ENGRAM_W_KU_COSINE", kuWeightVector, 0.0, 1.0),
		bm25:      envconf.FloatBounded("ENGRAM_W_KU_BM25", kuWeightBM25, 0.0, 1.0),
		recency:   envconf.FloatBounded("ENGRAM_W_KU_RECENCY", kuWeightRecency, 0.0, 1.0),
		precision: envconf.FloatBounded("ENGRAM_W_KU_PRECISION", kuWeightPrecision, 0.0, 1.0),
	}
}

// kuWeightCfg returns the resolved KU weight configuration, loading it on first call.
func kuWeightCfg() kuWeightConfig {
	kuWeightConfigOnce.Do(loadKUWeightConfig)
	return resolvedKUWeights
}

// loadWeightConfig reads ENGRAM_W_COSINE, ENGRAM_W_BM25, ENGRAM_W_RECENCY,
// and ENGRAM_W_PRECISION. Each is independently optional: unset variables fall
// back to the compile-time constants. Values outside [0.0, 1.0] are rejected
// with a warning and the constant default is used instead. The function never
// panics — a bad env var degrades gracefully.
func loadWeightConfig() {
	resolvedWeights = weightConfig{
		vector:    envconf.FloatBounded("ENGRAM_W_COSINE", weightVector, 0.0, 1.0),
		bm25:      envconf.FloatBounded("ENGRAM_W_BM25", weightBM25, 0.0, 1.0),
		recency:   envconf.FloatBounded("ENGRAM_W_RECENCY", weightRecency, 0.0, 1.0),
		precision: envconf.FloatBounded("ENGRAM_W_PRECISION", weightPrecision, 0.0, 1.0),
	}
}

// weightCfg returns the resolved weight configuration, loading it on first call.
func weightCfg() weightConfig {
	weightConfigOnce.Do(loadWeightConfig)
	return resolvedWeights
}

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

// DefaultWeights returns the effective composite scoring weights.
// Values are read once from ENGRAM_W_COSINE, ENGRAM_W_BM25, ENGRAM_W_RECENCY,
// and ENGRAM_W_PRECISION; each falls back to the compile-time constant when the
// variable is absent or invalid.  The resolved values are cached via sync.Once
// so callers pay no per-query overhead.
func DefaultWeights() Weights {
	cfg := weightCfg()
	return Weights{
		Vector:    cfg.vector,
		BM25:      cfg.bm25,
		Recency:   cfg.recency,
		Precision: cfg.precision,
	}
}

// KnowledgeUpdateWeights returns a mid-point recency-boosted profile for queries
// asking about current or changed state (e.g., "where does X live currently?").
// Recency is higher than the default (0.22 vs 0.15) but below the full temporal
// profile (0.30), preserving semantic relevance for partially-updated facts.
// Override individual weights via ENGRAM_W_KU_COSINE, ENGRAM_W_KU_BM25,
// ENGRAM_W_KU_RECENCY, ENGRAM_W_KU_PRECISION.
func KnowledgeUpdateWeights() Weights {
	cfg := kuWeightCfg()
	return Weights{
		Vector:    cfg.vector,
		BM25:      cfg.bm25,
		Recency:   cfg.recency,
		Precision: cfg.precision,
	}
}

// TemporalWeights returns the recency-boosted weight profile used when a recall
// query is classified as time-anchored (via IsTemporalQuery). Recency is raised
// to 0.30 so that chronologically recent sessions rank above semantically similar
// but older sessions — directly targeting temporal-reasoning benchmark failures.
// Override individual weights via ENGRAM_W_TEMPORAL_COSINE, ENGRAM_W_TEMPORAL_BM25,
// ENGRAM_W_TEMPORAL_RECENCY, ENGRAM_W_TEMPORAL_PRECISION.
func TemporalWeights() Weights {
	cfg := temporalWeightCfg()
	return Weights{
		Vector:    cfg.vector,
		BM25:      cfg.bm25,
		Recency:   cfg.recency,
		Precision: cfg.precision,
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
	TopicAnchorMatch   bool     // true when the memory content contains topic-anchor tokens from the query (H-TAB, LME exp #3)
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
	// H-TAB (LME exp #3): additional boost when the preference memory contains
	// domain-specific topic tokens from the query. Fires only on top of the
	// existing preference boost so it doesn't affect non-preference paths.
	if in.IsPreferenceQuery && in.TopicAnchorMatch && in.MemoryType == "preference" {
		raw *= 1.25
	}
	return raw * boost
}

// RankNormalizedRecency computes a [0,1] recency score by rank-normalizing
// validFrom within the candidate set's date range [minDate, maxDate].
//
// This replaces the absolute exponential RecencyDecay for temporal queries where
// content spans multiple years (e.g., LME 2022–2024 sessions evaluated in 2026).
// At those timescales exp(-0.01 * h) collapses to ~1e-77, making the recency
// signal numerically zero. Rank normalization preserves relative ordering:
//
//   - validFrom == maxDate → 1.0 (most recent in set)
//   - validFrom == minDate → 0.0 (oldest in set)
//   - maxDate == minDate   → 0.5 (all equal: neutral)
func RankNormalizedRecency(validFrom, minDate, maxDate time.Time) float64 {
	span := maxDate.Sub(minDate)
	if span <= 0 {
		// Single candidate or all same date: neutral score.
		return 0.5
	}
	score := float64(validFrom.Sub(minDate)) / float64(span)
	// Clamp to [0, 1] as a defensive measure against out-of-range inputs.
	if score < 0 {
		return 0.0
	}
	if score > 1 {
		return 1.0
	}
	return score
}

// RankNormalizedRecencyWithFallback is like RankNormalizedRecency but handles a
// zero validFrom by falling back to RecencyDecay computed from createdAt.
// A zero validFrom means valid_from was nil/unset at ingest time.
func RankNormalizedRecencyWithFallback(validFrom, createdAt, minDate, maxDate time.Time) float64 {
	if validFrom.IsZero() {
		hours := time.Since(createdAt).Hours()
		return RecencyDecay(hours)
	}
	return RankNormalizedRecency(validFrom, minDate, maxDate)
}

// candidateDateRange scans a slice of Memory pointers and returns the min and max
// valid_from timestamps across the set. Memories with a zero/nil ValidFrom are
// excluded from the range computation (they fall back to RecencyDecay).
// Returns zero times when no valid ValidFrom values are found.
func candidateDateRange(candidates []*types.Memory) (minDate, maxDate time.Time) {
	first := true
	for _, m := range candidates {
		if m == nil || m.ValidFrom == nil || m.ValidFrom.IsZero() {
			continue
		}
		vf := *m.ValidFrom
		if first {
			minDate = vf
			maxDate = vf
			first = false
			continue
		}
		if vf.Before(minDate) {
			minDate = vf
		}
		if vf.After(maxDate) {
			maxDate = vf
		}
	}
	return minDate, maxDate
}

// CompositeScoreWithRankNorm is like CompositeScoreWithWeights but uses
// RankNormalizedRecency for the recency component instead of RecencyDecay.
//
// validFrom is the memory's ValidFrom timestamp (may be zero for fallback).
// candidates is the full candidate set — used to compute the date range for
// rank normalization. The function pre-computes minDate/maxDate across the
// set on each call.
//
// Single-candidate rule: when len(candidates)==1 and it has a valid ValidFrom,
// recency is 1.0 (the single result is always fully recency-boosted; there is
// no other candidate to rank against).
//
// Used when TemporalWeights are in effect so that relative ordering across
// multi-year candidate sets is preserved (absolute decay collapses at LME timescales).
func CompositeScoreWithRankNorm(in ScoreInput, w Weights, validFrom time.Time, candidates []*types.Memory) float64 {
	var recency float64
	if validFrom.IsZero() {
		// No valid_from on this memory: fall back to absolute decay.
		recency = RecencyDecay(in.HoursSince)
	} else if len(candidates) == 1 {
		// Single candidate: fully recency-boosted — no relative ordering possible.
		recency = 1.0
	} else {
		minDate, maxDate := candidateDateRange(candidates)
		if minDate.IsZero() {
			// No candidates have valid_from: fall back.
			recency = RecencyDecay(in.HoursSince)
		} else {
			recency = RankNormalizedRecency(validFrom, minDate, maxDate)
		}
	}

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
	raw := w.Vector*in.Cosine + w.BM25*in.BM25 + w.Recency*recency + w.Precision*precision
	if in.EpisodeMatch {
		raw *= 1.15
	}
	if in.IsPreferenceQuery && in.MemoryType == "preference" {
		raw *= 1.35
	}
	// H-TAB (LME exp #3): topic-anchor boost for on-topic preference memories.
	if in.IsPreferenceQuery && in.TopicAnchorMatch && in.MemoryType == "preference" {
		raw *= 1.25
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
	// H-TAB (LME exp #3): topic-anchor boost for on-topic preference memories.
	// When TopicAnchorMatch is true the memory contains domain-specific tokens
	// from the query (e.g., "coffee" in a question about coffee preferences).
	// This directly targets multi-preference-session distraction: off-topic
	// preference sessions with high cosine (from generic "I like X" language)
	// no longer crowd out the on-topic gold session. Default OFF (flag-gated).
	if in.IsPreferenceQuery && in.TopicAnchorMatch && in.MemoryType == "preference" {
		raw *= 1.25
	}
	return raw * boost
}
