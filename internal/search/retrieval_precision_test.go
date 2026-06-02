package search_test

// Feature 5: Retrieval Outcome Tracking
// All tests written BEFORE implementation (TDD).
// They must fail (compile or runtime) until Feature 5 is implemented.

import (
	"context"
	"errors"
	"testing"

	"github.com/petersimmons1972/engram/internal/db"
	"github.com/petersimmons1972/engram/internal/embed"
	"github.com/petersimmons1972/engram/internal/search"
	"github.com/petersimmons1972/engram/internal/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newEngineWithDims creates a test engine backed by a real Postgres backend
// using the given embedding dimension. The backend is registered for cleanup.
// Skips the test if TEST_DATABASE_URL is not set (via testDSN).
func newEngineWithDims(t *testing.T, proj string, dims int) *search.SearchEngine {
	t.Helper()
	ctx := context.Background()
	backend, err := db.NewPostgresBackend(ctx, proj, testDSN(t))
	require.NoError(t, err)
	engine := search.New(ctx, backend, &fakeClient{dims: dims}, proj,
		"http://ollama:11434", "llama3.2", false, nil, 0)
	// engine.Close() shuts down workers and closes the backend pool; register
	// it here so resources are always released even if the caller panics or
	// returns early (fix for #758: missing t.Cleanup registration).
	t.Cleanup(func() { engine.Close() })
	return engine
}

// ── Scoring unit tests ────────────────────────────────────────────────────────

// TestCompositeScore_PrecisionSignal verifies that a memory with high retrieval
// precision scores higher than one with low precision, all else equal.
func TestCompositeScore_PrecisionSignal(t *testing.T) {
	highPrec := 0.9
	lowPrec := 0.2

	sHigh := search.CompositeScore(search.ScoreInput{
		Cosine: 0.5, BM25: 0.5, HoursSince: 0, Importance: 2,
		RetrievalPrecision: &highPrec,
	})
	sLow := search.CompositeScore(search.ScoreInput{
		Cosine: 0.5, BM25: 0.5, HoursSince: 0, Importance: 2,
		RetrievalPrecision: &lowPrec,
	})
	assert.Greater(t, sHigh, sLow,
		"high retrieval precision must produce a higher composite score")
}

// TestCompositeScore_PrecisionColdStart verifies that a nil RetrievalPrecision
// (cold-start: < 5 retrievals) behaves as if precision = 0.5 (neutral).
func TestCompositeScore_PrecisionColdStart(t *testing.T) {
	neutral := 0.5
	sNil := search.CompositeScore(search.ScoreInput{
		Cosine: 0.5, BM25: 0.5, HoursSince: 0, Importance: 2,
		RetrievalPrecision: nil,
	})
	sNeutral := search.CompositeScore(search.ScoreInput{
		Cosine: 0.5, BM25: 0.5, HoursSince: 0, Importance: 2,
		RetrievalPrecision: &neutral,
	})
	assert.InDelta(t, sNeutral, sNil, 0.001,
		"nil precision (cold start) must behave identically to precision=0.5")
}

// ── Integration tests (skip without TEST_DATABASE_URL) ────────────────────────

// TestRetrievalTracking_RecallCreatesEvent verifies that Recall returns an
// event_id and that the event is stored in the retrieval_events table.
func TestRetrievalTracking_RecallCreatesEvent(t *testing.T) {
	engine := newTestEngine(t, uniqueProject("test-rt-event"))
	t.Cleanup(func() { engine.Close() })
	ctx := context.Background()

	m := &types.Memory{
		Content:    "Event tracking: recall must produce a retrieval event",
		MemoryType: types.MemoryTypeContext, Project: engine.Project(),
		Importance: 2, StorageMode: "focused",
	}
	require.NoError(t, engine.Store(ctx, m))

	results, eventID, err := engine.RecallWithEvent(ctx, "retrieval event tracking", 5, "normal")
	require.NoError(t, err)
	assert.NotEmpty(t, eventID, "Recall must return a non-empty event_id")
	_ = results

	// The event must be retrievable from the backend.
	event, err := engine.Backend().GetRetrievalEvent(ctx, eventID)
	require.NoError(t, err)
	require.NotNil(t, event)
	assert.Equal(t, eventID, event.ID)
	assert.Equal(t, engine.Project(), event.Project)
	assert.NotEmpty(t, event.ResultIDs)
}

// TestRetrievalTracking_FeedbackRecordsOutcome verifies that calling
// FeedbackWithEvent updates the retrieval event and increments times_retrieved
// and times_useful on memories that were marked useful.
func TestRetrievalTracking_FeedbackRecordsOutcome(t *testing.T) {
	engine := newTestEngine(t, uniqueProject("test-rt-feedback"))
	t.Cleanup(func() { engine.Close() })
	ctx := context.Background()

	m1 := &types.Memory{
		Content: "Useful memory for feedback test A", MemoryType: types.MemoryTypePattern,
		Project: engine.Project(), Importance: 2, StorageMode: "focused",
	}
	m2 := &types.Memory{
		Content: "Useful memory for feedback test B", MemoryType: types.MemoryTypePattern,
		Project: engine.Project(), Importance: 2, StorageMode: "focused",
	}
	require.NoError(t, engine.Store(ctx, m1))
	require.NoError(t, engine.Store(ctx, m2))

	_, eventID, err := engine.RecallWithEvent(ctx, "feedback test memory", 10, "normal")
	require.NoError(t, err)
	require.NotEmpty(t, eventID)

	// Record outcome: m1 was useful, m2 was not.
	require.NoError(t, engine.FeedbackWithEvent(ctx, eventID, []string{m1.ID}))

	// m1 should have times_useful incremented.
	after1, err := engine.Backend().GetMemory(ctx, m1.ID)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, after1.TimesUseful, 1,
		"times_useful must be incremented for memories marked useful")

	// Event should be marked with feedback.
	event, err := engine.Backend().GetRetrievalEvent(ctx, eventID)
	require.NoError(t, err)
	assert.NotNil(t, event.FeedbackAt, "feedback_at must be set after FeedbackWithEvent")
}

// TestRetrievalTracking_PrecisionUpdated verifies that after enough feedback
// cycles, retrieval_precision on a memory reflects the useful/retrieved ratio.
func TestRetrievalTracking_PrecisionUpdated(t *testing.T) {
	engine := newTestEngine(t, uniqueProject("test-rt-precision"))
	t.Cleanup(func() { engine.Close() })
	ctx := context.Background()

	m := &types.Memory{
		Content:    "Precision tracking: this memory is always useful",
		MemoryType: types.MemoryTypeDecision, Project: engine.Project(),
		Importance: 1, StorageMode: "focused",
	}
	require.NoError(t, engine.Store(ctx, m))

	// Run 5 recall + positive feedback cycles to move past cold-start threshold.
	for i := 0; i < 5; i++ {
		_, eventID, err := engine.RecallWithEvent(ctx, "precision tracking memory", 10, "normal")
		require.NoError(t, err)
		require.NoError(t, engine.FeedbackWithEvent(ctx, eventID, []string{m.ID}))
	}

	after, err := engine.Backend().GetMemory(ctx, m.ID)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, after.TimesRetrieved, 5)
	assert.GreaterOrEqual(t, after.TimesUseful, 5)
	assert.NotNil(t, after.RetrievalPrecision,
		"retrieval_precision must be computed once times_retrieved >= 5")
	assert.InDelta(t, 1.0, *after.RetrievalPrecision, 0.05,
		"always-useful memory must have precision near 1.0")
}

// TestRetrievalTracking_DimMismatch verifies that opening a project with a
// different embedding dimension than it was stored with returns a clear error
// from RecallWithEvent rather than silently returning zero results and
// recording phantom precision data.
//
// Regression guard for the embed-dim mismatch scenario described in #752.
// If this test fails with "RecallWithEvent with dim mismatch must return an
// error", the production code in engine.go needs a dim-mismatch guard in
// RecallWithOpts (analogous to checkEmbedderMeta in Store).
func TestRetrievalTracking_DimMismatch(t *testing.T) {
	ctx := context.Background()
	proj := uniqueProject("test-rt-dim-mismatch")

	// Store a memory using a 1024-dim engine.
	engine1024 := newEngineWithDims(t, proj, 1024)
	m := &types.Memory{
		Content:     "dim mismatch test memory",
		MemoryType:  types.MemoryTypeDecision,
		Project:     proj,
		Importance:  1,
		StorageMode: "focused",
	}
	require.NoError(t, engine1024.Store(ctx, m))

	// Attempt recall using a 768-dim engine — must fail clearly, not silently.
	// The fakeClient embeds query at 768 dims; pgvector will reject a cosine
	// similarity query where the stored vectors are 1024-dim.
	engine768 := newEngineWithDims(t, proj, 768)
	_, _, err := engine768.RecallWithEvent(ctx, "dim mismatch test memory", 5, "normal")
	require.Error(t, err, "RecallWithEvent with dim mismatch must return an error")

	// Assert on the structured embed.PermanentError type. The dim mismatch
	// surfaces via the Stored/Current fields, not as a "dim" substring in the
	// formatted Error() string (which only renders Code + Remediation).
	var permErr *embed.PermanentError
	require.True(t, errors.As(err, &permErr),
		"error must be an *embed.PermanentError (got: %v)", err)
	assert.NotEqual(t, permErr.Stored, permErr.Current,
		"PermanentError must record the dim mismatch in Stored vs Current (stored=%q current=%q)",
		permErr.Stored, permErr.Current)
}

// TestRecallWithinMemory_EmbedderMetaGuard is a regression test for issue #788.
// It verifies that RecallWithinMemory rejects calls when the current embedder
// name does not match what was stored — the same guard already wired into
// RecallWithOpts and StoreBatch via checkEmbedderMeta.
func TestRecallWithinMemory_EmbedderMetaGuard(t *testing.T) {
	ctx := context.Background()
	proj := uniqueProject("test-rwm-embedder-meta")

	// Store a memory using an engine named "fake" (fakeClient.Name() == "fake").
	engine1 := newTestEngine(t, proj)
	t.Cleanup(func() { engine1.Close() })
	m := &types.Memory{
		Content:     "embedder meta guard test memory",
		MemoryType:  types.MemoryTypeDecision,
		Project:     proj,
		Importance:  1,
		StorageMode: "focused",
	}
	require.NoError(t, engine1.Store(ctx, m))

	// Attempt RecallWithinMemory using a different embedder name — must return
	// an *embed.PermanentError, not silently return mismatched vectors.
	otherClient := &fakeClientWithName{dims: 768, name: "other-embedder"}
	backend2, err := db.NewPostgresBackend(ctx, proj, testDSN(t))
	require.NoError(t, err)
	t.Cleanup(func() { backend2.Close() })
	engine2 := search.New(ctx, backend2, otherClient, proj,
		"http://ollama:11434", "llama3.2", false, nil, 0)
	t.Cleanup(func() { engine2.Close() })

	_, err = engine2.RecallWithinMemory(ctx, "embedder meta guard test memory", m.ID, 5, "summary")
	require.Error(t, err, "RecallWithinMemory must return error when embedder name mismatches stored metadata")

	var permErr *embed.PermanentError
	require.True(t, errors.As(err, &permErr),
		"error must be an *embed.PermanentError (got: %v)", err)
	assert.NotEqual(t, permErr.Stored, permErr.Current,
		"PermanentError must record the embedder mismatch in Stored vs Current (stored=%q current=%q)",
		permErr.Stored, permErr.Current)
}
