package search

// LME Experiment #5 — Temporal Validity Window Filter.
//
// ENGRAM_VALIDITY_WINDOW_FILTER (default: false/off)
//
// When enabled, RecallWithOpts applies a ValidFromBoost multiplier to each
// memory's composite score based on the recency of its valid_from timestamp.
// Memories with more-recent valid_from values score higher; memories without
// a valid_from (nil) receive a neutral multiplier of 1.0 so they are not
// penalized.
//
// This targets the knowledge-update failure class in LME-M: when the current
// fact and an outdated fact are both in the candidate set, the current-value
// (most-recent valid_from) wins.
//
// Design decision (advisory-gate, 2026-06-02, Option C):
//   - DB layer already enforces valid_to IS NULL (active-only) everywhere.
//   - The remaining gap is valid_from recency signal at scoring time.
//   - Flag gate required to preserve golden baseline (run_id 7a87fd) for
//     clean ablation: flag OFF = baseline, flag ON = experiment.
//
// Re-ingest caveat (Engram 019e100c): existing benchmark data must be
// re-ingested with date: tags for ValidFrom to be populated. This flag has
// no effect on memories without valid_from set at ingest time (they receive
// boost = 1.0 regardless of flag state).
//
// Bench+re-ingest command (do NOT run now — document only):
//
//   # 1. Re-ingest LME-M dataset with date: tags.
//   longmemeval ingest \
//     --data ~/path/to/longmemeval_m_cleaned.json \
//     --url http://localhost:8788 \
//     --workers 32 \
//     --out ~/benchmarks/lme-exp5 \
//     --cleanup-policy=never \
//     --scratch-ttl 168h
//
//   # 2. Run recall+generate with validity window flag ON.
//   ENGRAM_VALIDITY_WINDOW_FILTER=true \
//   longmemeval run \
//     --data ~/path/to/longmemeval_m_cleaned.json \
//     --url http://localhost:8788 \
//     --llm-url "${LME_LLM_URL}" \
//     --llm-model "${LME_LLM_MODEL}" \
//     --workers 32 \
//     --out ~/benchmarks/lme-exp5 \
//     --recall-topk 100 \
//     --context-topk 8
//
//   # 3. Score.
//   longmemeval score-efficient \
//     --data ~/path/to/longmemeval_m_cleaned.json \
//     --scorer-url "${LME_SCORER_URL:-${LME_LLM_URL}}" \
//     --scorer-model "${LME_SCORER_MODEL:-${LME_LLM_MODEL}}" \
//     --workers 16 \
//     --out ~/benchmarks/lme-exp5
//
//   # 4. Analyze — compare knowledge-update accuracy vs golden baseline (run_id 7a87fd).
//   longmemeval analyze --results ~/benchmarks/lme-exp5

import (
	"os"
	"sync"
	"time"
)

// validityWindowEnabled is the cached state of ENGRAM_VALIDITY_WINDOW_FILTER.
// Initialized once via validityWindowOnce; reset in tests via ResetValidityWindowForTesting.
var (
	validityWindowOnce    sync.Once
	validityWindowEnabled bool
)

// loadValidityWindowFlag reads ENGRAM_VALIDITY_WINDOW_FILTER from the environment.
// Any non-empty value other than "false", "0", or "no" is treated as enabled.
func loadValidityWindowFlag() {
	val := os.Getenv("ENGRAM_VALIDITY_WINDOW_FILTER")
	switch val {
	case "", "false", "0", "no":
		validityWindowEnabled = false
	default:
		validityWindowEnabled = true
	}
}

// isValidityWindowEnabled returns true when ENGRAM_VALIDITY_WINDOW_FILTER is
// set to a truthy value ("true", "1", "yes", or any non-empty non-false value).
// The result is cached after the first call.
func isValidityWindowEnabled() bool {
	validityWindowOnce.Do(loadValidityWindowFlag)
	return validityWindowEnabled
}

// ResetValidityWindowForTesting resets the validity window sync.Once so tests
// can inject different ENGRAM_VALIDITY_WINDOW_FILTER values between sub-tests.
// Must only be called from test code.
func ResetValidityWindowForTesting() {
	validityWindowOnce = sync.Once{}
}

// validityWindowYearScale is the characteristic time scale for the ValidFromBoost
// decay formula, expressed in years. A memory valid_from exactly this many years
// ago receives boost = 0.5; a memory from now receives boost approaching 1.0.
//
// 2.0 years chosen to span the LME-M benchmark range (sessions from 2022–2024
// evaluated in 2026): a 2024 fact (~2 yr ago) gets boost ≈ 0.5, while a 2020
// fact (~6 yr ago) gets boost ≈ 0.25 — meaningful separation without clamping.
//
// The boost is a Lorentzian (inverse-square) decay for long time scales rather
// than exponential, which would collapse all multi-year memories to the same floor.
const validityWindowYearScale = 2.0

// ValidFromBoost computes a multiplicative score boost based on the memory's
// valid_from recency. It is applied in RecallWithOpts when
// ENGRAM_VALIDITY_WINDOW_FILTER=true.
//
// Behavior:
//   - nil valid_from → 1.0 (neutral; no penalty for undated memories)
//   - flag OFF       → 1.0 (neutral; baseline-safe)
//   - flag ON        → monotonically decreasing from 1.0 (present-day)
//     as valid_from recedes into the past
//
// Formula: inverse-linear decay with characteristic scale T (years):
//
//	boost(t) = T / (T + yearsAgo)
//
// At yearsAgo=0:  boost = T/T = 1.0 (present-day → no penalty)
// At yearsAgo=T:  boost = T/(2T) = 0.5 (at scale T years: half boost)
// At yearsAgo=2T: boost = T/(3T) ≈ 0.333
// At yearsAgo=6T: boost = T/(7T) ≈ 0.143
//
// This gives continuous, monotonically decreasing values across multi-year spans.
// No floor clamp needed: the minimum is positive and bounded away from zero.
//
// Example (T=2 years, evaluated 2026):
//   - valid_from=2026: boost=1.0 (present)
//   - valid_from=2024: boost≈0.5 (2yr ago)
//   - valid_from=2022: boost≈0.25 (4yr ago)
//   - valid_from=2020: boost≈0.167 (6yr ago)
func ValidFromBoost(validFrom *time.Time) float64 {
	if !isValidityWindowEnabled() {
		return 1.0
	}
	if validFrom == nil || validFrom.IsZero() {
		return 1.0
	}
	yearsAgo := time.Since(*validFrom).Hours() / (365.25 * 24.0)
	if yearsAgo < 0 {
		// Future-dated memory (e.g., planned event): treat as present-day.
		yearsAgo = 0
	}
	// boost = T / (T + yearsAgo), where T = validityWindowYearScale.
	return validityWindowYearScale / (validityWindowYearScale + yearsAgo)
}
