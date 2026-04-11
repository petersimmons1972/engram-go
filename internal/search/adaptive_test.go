package search_test

// Feature 2: Adaptive Importance via Spaced Repetition
// All tests in this file are written BEFORE implementation (TDD).
// They must fail (compile error or runtime failure) until Feature 2 is implemented.

import (
	"context"
	"testing"
	"time"

	"github.com/petersimmons1972/engram/internal/search"
	"github.com/petersimmons1972/engram/internal/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ── Scoring unit tests ────────────────────────────────────────────────────────

// TestCompositeScore_UsesDynamicImportance verifies that when DynamicImportance
// is set on ScoreInput, it is used as the boost multiplier instead of the
// static Importance value.
func TestCompositeScore_UsesDynamicImportance(t *testing.T) {
	// A memory marked trivial (importance=4) but with a high dynamic score
	// should rank the same as a critical (importance=0) static memory.
	criticalBoost := search.ImportanceBoost(0) // 5/3 ≈ 1.67
	di := criticalBoost

	withDynamic := search.CompositeScore(search.ScoreInput{
		Cosine:            0.5,
		BM25:              0.5,
		HoursSince:        0,
		Importance:        4,   // trivial static — must be overridden
		DynamicImportance: &di, // but dynamic says critical
	})
	withStatic := search.CompositeScore(search.ScoreInput{
		Cosine:     0.5,
		BM25:       0.5,
		HoursSince: 0,
		Importance: 0, // critical static
	})
	assert.InDelta(t, withStatic, withDynamic, 0.001,
		"dynamic importance must override the static importance value in scoring")
}

// TestCompositeScore_DynamicClamped verifies that very small or negative
// dynamic_importance values are clamped to a minimum of 0.1 (never zero boost).
func TestCompositeScore_DynamicClamped(t *testing.T) {
	negative := -1.0
	s := search.CompositeScore(search.ScoreInput{
		Cosine:            0.5,
		BM25:              0.5,
		HoursSince:        0,
		Importance:        2,
		DynamicImportance: &negative,
	})
	assert.Greater(t, s, 0.0, "composite score must remain positive even with negative dynamic_importance")
}

// TestCompositeScore_NilDynamicFallsBack verifies backward compatibility:
// when DynamicImportance is nil the static Importance field is used.
func TestCompositeScore_NilDynamicFallsBack(t *testing.T) {
	sStatic := search.CompositeScore(search.ScoreInput{
		Cosine: 0.5, BM25: 0.5, HoursSince: 0, Importance: 2,
	})
	sNilDynamic := search.CompositeScore(search.ScoreInput{
		Cosine: 0.5, BM25: 0.5, HoursSince: 0, Importance: 2,
		DynamicImportance: nil,
	})
	assert.InDelta(t, sStatic, sNilDynamic, 0.001,
		"nil DynamicImportance must behave identically to the old static-only path")
}

// ── Integration tests (skip without TEST_DATABASE_URL) ────────────────────────

// TestAdaptiveImportance_PositiveFeedback verifies that calling Feedback() on
// a memory increases its dynamic_importance and sets next_review_at.
func TestAdaptiveImportance_PositiveFeedback(t *testing.T) {
	engine := newTestEngine(t, uniqueProject("test-adapt-pos"))
	t.Cleanup(func() { engine.Close() })
	ctx := context.Background()

	m := &types.Memory{
		Content: "Use pgx v5 for all database access", MemoryType: types.MemoryTypePattern,
		Project: engine.Project(), Importance: 2, StorageMode: "focused",
	}
	require.NoError(t, engine.Store(ctx, m))

	before, err := engine.Backend().GetMemory(ctx, m.ID)
	require.NoError(t, err)
	require.NotNil(t, before)
	assert.NotNil(t, before.DynamicImportance, "DynamicImportance must be set at store time")
	initialDI := *before.DynamicImportance

	// Positive feedback.
	require.NoError(t, engine.Feedback(ctx, []string{m.ID}))

	after, err := engine.Backend().GetMemory(ctx, m.ID)
	require.NoError(t, err)
	require.NotNil(t, after.DynamicImportance)
	assert.Greater(t, *after.DynamicImportance, initialDI,
		"dynamic_importance must increase after positive feedback")
	assert.NotNil(t, after.NextReviewAt,
		"next_review_at must be set after positive feedback")
	assert.True(t, after.NextReviewAt.After(time.Now()),
		"next_review_at must be in the future")
}

// TestAdaptiveImportance_NegativeFeedback verifies that passing useful=false
// in Feedback decreases dynamic_importance.
func TestAdaptiveImportance_NegativeFeedback(t *testing.T) {
	engine := newTestEngine(t, uniqueProject("test-adapt-neg"))
	t.Cleanup(func() { engine.Close() })
	ctx := context.Background()

	m := &types.Memory{
		Content: "Always use mutex for shared state", MemoryType: types.MemoryTypePattern,
		Project: engine.Project(), Importance: 1, StorageMode: "focused",
	}
	require.NoError(t, engine.Store(ctx, m))

	before, err := engine.Backend().GetMemory(ctx, m.ID)
	require.NoError(t, err)
	initialDI := *before.DynamicImportance

	// Negative feedback.
	require.NoError(t, engine.FeedbackNegative(ctx, []string{m.ID}))

	after, err := engine.Backend().GetMemory(ctx, m.ID)
	require.NoError(t, err)
	require.NotNil(t, after.DynamicImportance)
	assert.Less(t, *after.DynamicImportance, initialDI,
		"dynamic_importance must decrease after negative feedback")
}

// TestAdaptiveImportance_DecayStale verifies that DecayStaleImportance reduces
// dynamic_importance on memories whose next_review_at is in the past.
func TestAdaptiveImportance_DecayStale(t *testing.T) {
	engine := newTestEngine(t, uniqueProject("test-adapt-decay"))
	t.Cleanup(func() { engine.Close() })
	ctx := context.Background()

	m := &types.Memory{
		Content: "Stale memory pending decay", MemoryType: types.MemoryTypeContext,
		Project: engine.Project(), Importance: 3, StorageMode: "focused",
	}
	require.NoError(t, engine.Store(ctx, m))

	// Force next_review_at into the past so this memory is stale.
	pastReview := time.Now().Add(-72 * time.Hour)
	require.NoError(t, engine.Backend().SetNextReviewAt(ctx, m.ID, pastReview))

	before, err := engine.Backend().GetMemory(ctx, m.ID)
	require.NoError(t, err)
	initialDI := *before.DynamicImportance

	decayed, err := engine.Backend().DecayStaleImportance(ctx, engine.Project(), 0.95)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, decayed, 1, "at least one stale memory must be decayed")

	after, err := engine.Backend().GetMemory(ctx, m.ID)
	require.NoError(t, err)
	assert.Less(t, *after.DynamicImportance, initialDI,
		"dynamic_importance must decrease after stale decay pass")
}

// TestAdaptiveImportance_UsedInRecall verifies that a memory with higher
// dynamic_importance ranks above one with lower dynamic_importance for the
// same query, even if static importance is equal.
func TestAdaptiveImportance_UsedInRecall(t *testing.T) {
	engine := newTestEngine(t, uniqueProject("test-adapt-recall"))
	t.Cleanup(func() { engine.Close() })
	ctx := context.Background()

	// Both memories have the same static importance.
	mHigh := &types.Memory{
		Content: "gRPC is the preferred RPC framework for microservices",
		MemoryType: types.MemoryTypeDecision, Project: engine.Project(),
		Importance: 2, StorageMode: "focused",
	}
	mLow := &types.Memory{
		Content: "gRPC may also be suitable for some use cases",
		MemoryType: types.MemoryTypeDecision, Project: engine.Project(),
		Importance: 2, StorageMode: "focused",
	}
	require.NoError(t, engine.Store(ctx, mHigh))
	require.NoError(t, engine.Store(ctx, mLow))

	// Boost mHigh with positive feedback several times.
	for i := 0; i < 5; i++ {
		require.NoError(t, engine.Feedback(ctx, []string{mHigh.ID}))
	}
	// Penalize mLow with negative feedback.
	for i := 0; i < 3; i++ {
		require.NoError(t, engine.FeedbackNegative(ctx, []string{mLow.ID}))
	}

	// Both memories should appear in recall for a relevant query.
	results, err := engine.Recall(ctx, "gRPC microservices", 10, "normal")
	require.NoError(t, err)

	var rankHigh, rankLow int = -1, -1
	for i, r := range results {
		if r.Memory != nil {
			if r.Memory.ID == mHigh.ID {
				rankHigh = i
			}
			if r.Memory.ID == mLow.ID {
				rankLow = i
			}
		}
	}
	if rankHigh != -1 && rankLow != -1 {
		assert.Less(t, rankHigh, rankLow,
			"memory with higher dynamic_importance must rank above one with lower")
	}
}
