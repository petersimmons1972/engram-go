package search_test

// LME Experiment #5 — temporal validity windows (server-side recall).
//
// Tests in this file are the TDD anchor for ENGRAM_VALIDITY_WINDOW_FILTER.
// They must FAIL before implementation and PASS after.
//
// Failure class targeted: knowledge-update (LME-M baseline 46.2% on run c3d9f1;
// updated baseline 4bf268c to be measured after re-ingest with this flag ON).
//
// Re-ingest caveat (Engram 019e100c): existing benchmark data must be re-ingested
// with date: tags for ValidFrom to be set. Tests here use in-memory mocks and
// do not require prod data.

import (
	"context"
	"math"
	"testing"
	"time"

	"github.com/petersimmons1972/engram/internal/search"
	"github.com/petersimmons1972/engram/internal/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestValidFromBoost_FlagOff verifies that ValidFromBoost returns 1.0 (neutral)
// when ENGRAM_VALIDITY_WINDOW_FILTER is unset (default OFF), so the baseline
// composite score is unchanged. This is the ablation guard.
func TestValidFromBoost_FlagOff(t *testing.T) {
	t.Setenv("ENGRAM_VALIDITY_WINDOW_FILTER", "")
	search.ResetValidityWindowForTesting()

	old := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	recent := time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC)

	// With flag OFF, both old and recent dates must return boost = 1.0 (no-op).
	assert.InDelta(t, 1.0, search.ValidFromBoost(&old), 0.001,
		"flag OFF: old valid_from must return neutral boost 1.0")
	assert.InDelta(t, 1.0, search.ValidFromBoost(&recent), 0.001,
		"flag OFF: recent valid_from must return neutral boost 1.0")
	assert.InDelta(t, 1.0, search.ValidFromBoost(nil), 0.001,
		"flag OFF: nil valid_from must return neutral boost 1.0")
}

// TestValidFromBoost_FlagOn_RecentBeatsOld verifies that when
// ENGRAM_VALIDITY_WINDOW_FILTER=true, a memory with a more recent valid_from
// receives a higher boost than one with an older valid_from.
//
// This is the core invariant: "current-value-wins".
func TestValidFromBoost_FlagOn_RecentBeatsOld(t *testing.T) {
	t.Setenv("ENGRAM_VALIDITY_WINDOW_FILTER", "true")
	search.ResetValidityWindowForTesting()

	old := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	recent := time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC)

	boostOld := search.ValidFromBoost(&old)
	boostRecent := search.ValidFromBoost(&recent)

	require.Greater(t, boostRecent, boostOld,
		"flag ON: more recent valid_from must yield a higher boost than older valid_from")
}

// TestValidFromBoost_FlagOn_NilIsNeutral verifies that memories without a
// valid_from (most pre-existing memories that lack date: tags) receive a
// neutral boost (1.0) rather than a penalty. This prevents the flag from
// degrading recall for undated memories.
func TestValidFromBoost_FlagOn_NilIsNeutral(t *testing.T) {
	t.Setenv("ENGRAM_VALIDITY_WINDOW_FILTER", "true")
	search.ResetValidityWindowForTesting()

	boost := search.ValidFromBoost(nil)
	assert.InDelta(t, 1.0, boost, 0.001,
		"flag ON: nil valid_from must return neutral boost 1.0 (no penalty for undated memories)")
}

// TestValidFromBoost_FlagOn_PresentDayIsMax verifies that a memory timestamped
// very close to now (within minutes) receives a boost >= 1.0, confirming that
// recency is rewarded, not penalized.
func TestValidFromBoost_FlagOn_PresentDayIsMax(t *testing.T) {
	t.Setenv("ENGRAM_VALIDITY_WINDOW_FILTER", "true")
	search.ResetValidityWindowForTesting()

	now := time.Now().UTC().Add(-time.Minute) // 1 minute ago
	boost := search.ValidFromBoost(&now)
	// boost = T/(T+yearsAgo) at 1 minute ago ≈ T/(T+tiny) ≈ 0.9999990 — just below 1.0.
	// Assert > 0.999 rather than >= 1.0: the formula is strictly < 1.0 for any past time,
	// but a 1-minute-old memory is effectively not penalized (0.001% reduction).
	assert.Greater(t, boost, 0.999,
		"flag ON: very recent valid_from must not be meaningfully penalised (boost > 0.999)")
}

// TestValidityWindowFilter_SupersededFactDeranked verifies end-to-end via the
// search engine that when ENGRAM_VALIDITY_WINDOW_FILTER=true, a memory with a
// more recent valid_from outscores a memory with the same semantic content but
// an older valid_from. This simulates the knowledge-update failure class where
// the outdated fact and the current fact are both in the candidate set.
//
// NOTE: This test requires a running Postgres instance (uses newTestEngine).
// It is an integration test gated by the DSN environment variable.
func TestValidityWindowFilter_SupersededFactDeranked(t *testing.T) {
	t.Setenv("ENGRAM_VALIDITY_WINDOW_FILTER", "true")
	search.ResetValidityWindowForTesting()

	engine := newTestEngine(t, uniqueProject("test-validity-window"))
	t.Cleanup(func() { engine.Close() })
	ctx := context.Background()

	// Outdated fact: John lives in New York (2020).
	outdated := time.Date(2020, 3, 10, 0, 0, 0, 0, time.UTC)
	mOld := &types.Memory{
		ID:          types.NewMemoryID(),
		Content:     "John currently lives in New York",
		MemoryType:  types.MemoryTypeContext,
		Importance:  2,
		StorageMode: "focused",
		ValidFrom:   &outdated,
	}
	// Current fact: John lives in San Francisco (2024).
	current := time.Date(2024, 9, 1, 0, 0, 0, 0, time.UTC)
	mNew := &types.Memory{
		ID:          types.NewMemoryID(),
		Content:     "John currently lives in San Francisco",
		MemoryType:  types.MemoryTypeContext,
		Importance:  2,
		StorageMode: "focused",
		ValidFrom:   &current,
	}
	require.NoError(t, engine.Store(ctx, mOld))
	require.NoError(t, engine.Store(ctx, mNew))

	results, err := engine.Recall(ctx, "Where does John currently live?", 10, "full")
	require.NoError(t, err)
	require.NotEmpty(t, results, "recall must return at least one result")

	// Find both results by ID.
	var scoreOld, scoreNew float64
	var foundOld, foundNew bool
	for _, r := range results {
		switch r.Memory.ID {
		case mOld.ID:
			scoreOld = r.Score
			foundOld = true
		case mNew.ID:
			scoreNew = r.Score
			foundNew = true
		}
	}

	require.True(t, foundOld, "outdated memory must be in recall results")
	require.True(t, foundNew, "current memory must be in recall results")

	// Guard against non-finite composite scores (NaN/Inf) *before* handing them
	// to assert.Greater. testify's numeric comparator falls through to a bare
	// "Can not compare type \"float64\"" failure when every ordering check on a
	// NaN operand evaluates false — that error gives no indication a NaN was
	// involved, which is what made this test flaky/confusing in CI (#1353).
	// Failing loudly here with the actual values makes the underlying scoring
	// issue (if any) visible instead of masquerading as a testify type error.
	require.False(t, math.IsNaN(scoreOld) || math.IsNaN(scoreNew),
		"composite scores must be finite, not NaN: scoreOld=%v scoreNew=%v", scoreOld, scoreNew)
	require.False(t, math.IsInf(scoreOld, 0) || math.IsInf(scoreNew, 0),
		"composite scores must be finite, not +/-Inf: scoreOld=%v scoreNew=%v", scoreOld, scoreNew)

	assert.Greater(t, scoreNew, scoreOld,
		"current fact (valid_from=2024) must outscore outdated fact (valid_from=2020) when validity window filter is ON")
}

// TestValidityWindowFilter_FlagOff_BaselineUnchanged verifies that with the
// flag OFF (default), ValidFromBoost returns identical values for different
// valid_from dates. This is the ablation guard ensuring the baseline is
// reproducible without any validity-window scoring change.
func TestValidityWindowFilter_FlagOff_BaselineUnchanged(t *testing.T) {
	t.Setenv("ENGRAM_VALIDITY_WINDOW_FILTER", "")
	search.ResetValidityWindowForTesting()

	old := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	recent := time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC)

	// Both get boost 1.0 with flag OFF — no ordering change.
	boostOld := search.ValidFromBoost(&old)
	boostRecent := search.ValidFromBoost(&recent)
	assert.Equal(t, boostOld, boostRecent,
		"flag OFF: valid_from must not affect score ordering (baseline ablation guard)")
}
