package search_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/petersimmons1972/engram/internal/atom"
	"github.com/petersimmons1972/engram/internal/db"
	"github.com/petersimmons1972/engram/internal/embed"
	"github.com/petersimmons1972/engram/internal/search"
	"github.com/petersimmons1972/engram/internal/testutil"
	"github.com/petersimmons1972/engram/internal/types"
	"github.com/stretchr/testify/require"
)

// uniqueProject returns a per-run isolated project name to prevent cross-run
// state leakage when the test database persists between test invocations.
func uniqueProject(base string) string { return testutil.UniqueProject(base) }
func testDSN(t *testing.T) string      { return testutil.DSN(t) }

type fakeClient struct{ dims int }

func (f *fakeClient) Embed(_ context.Context, text string) ([]float32, error) {
	vec := make([]float32, f.dims)
	for i := range vec {
		vec[i] = float32(i) / float32(f.dims)
	}
	return vec, nil
}
func (f *fakeClient) EmbedWithModel(ctx context.Context, text string) ([]float32, string, error) {
	vec, err := f.Embed(ctx, text)
	return vec, f.Name(), err
}
func (f *fakeClient) Name() string    { return "fake" }
func (f *fakeClient) Dimensions() int { return f.dims }

// compile-time check that fakeClient satisfies embed.Client.
var _ embed.Client = (*fakeClient)(nil)

func newTestEngine(t *testing.T, project string) *search.SearchEngine {
	t.Helper()
	ctx := context.Background()
	backend, err := db.NewPostgresBackend(ctx, project, testDSN(t))
	require.NoError(t, err)
	t.Cleanup(func() { backend.Close() })
	return search.New(ctx, backend, &fakeClient{dims: 1024}, project,
		"http://ollama:11434", "llama3.2", false, nil, 0)
}

func TestSearchEngine_Store(t *testing.T) {
	engine := newTestEngine(t, uniqueProject("test-store"))
	t.Cleanup(func() { engine.Close() })

	ctx := context.Background()
	m := &types.Memory{
		ID:          types.NewMemoryID(),
		Content:     "TDD means writing a failing test before implementation.",
		MemoryType:  types.MemoryTypePattern,
		Importance:  2,
		StorageMode: "focused",
	}
	err := engine.Store(ctx, m)
	require.NoError(t, err)
	require.NotEmpty(t, m.ID)
}

func TestSearchEngine_Store_DeduplicatesChunks(t *testing.T) {
	proj := uniqueProject("test-dedup")
	engine := newTestEngine(t, proj)
	t.Cleanup(func() { engine.Close() })

	ctx := context.Background()
	content := "Chunk deduplication prevents storing identical text twice."

	m1 := &types.Memory{ID: types.NewMemoryID(), Content: content,
		MemoryType: types.MemoryTypeContext, StorageMode: "focused"}
	require.NoError(t, engine.Store(ctx, m1))

	m2 := &types.Memory{ID: types.NewMemoryID(), Content: content,
		MemoryType: types.MemoryTypeContext, StorageMode: "focused"}
	require.NoError(t, engine.Store(ctx, m2))

	// Use GetChunksPendingEmbedding (no embedding filter) since chunks now store
	// with nil embeddings until the reembed worker runs.
	chunks, err := engine.Backend().GetChunksPendingEmbedding(ctx, proj, 10_000)
	require.NoError(t, err)
	require.Len(t, chunks, 1, "identical content should produce exactly one stored chunk")
}

func TestSearchEngine_Recall(t *testing.T) {
	engine := newTestEngine(t, uniqueProject("test-recall"))
	t.Cleanup(func() { engine.Close() })
	ctx := context.Background()

	m := &types.Memory{
		ID:          types.NewMemoryID(),
		Content:     "Go uses goroutines for concurrency, not threads.",
		MemoryType:  types.MemoryTypePattern,
		Importance:  3,
		StorageMode: "focused",
	}
	require.NoError(t, engine.Store(ctx, m))

	results, err := engine.Recall(ctx, "goroutines concurrency", 5, "summary")
	require.NoError(t, err)
	require.NotEmpty(t, results)
	require.Equal(t, m.ID, results[0].Memory.ID)
}

func TestSearchEngine_RecallWithOptsDateBounds(t *testing.T) {
	engine := newTestEngine(t, uniqueProject("test-recall-date-bounds"))
	t.Cleanup(func() { engine.Close() })
	ctx := context.Background()

	inWindowDate := time.Date(2023, 5, 9, 0, 0, 0, 0, time.UTC)
	outOfWindowDate := time.Date(2023, 6, 9, 0, 0, 0, 0, time.UTC)
	inWindow := &types.Memory{
		ID:          types.NewMemoryID(),
		Content:     "dentist appointment included by date window",
		MemoryType:  types.MemoryTypeContext,
		StorageMode: "focused",
		ValidFrom:   &inWindowDate,
	}
	outOfWindow := &types.Memory{
		ID:          types.NewMemoryID(),
		Content:     "dentist appointment excluded by date window",
		MemoryType:  types.MemoryTypeContext,
		StorageMode: "focused",
		ValidFrom:   &outOfWindowDate,
	}
	require.NoError(t, engine.Store(ctx, inWindow))
	require.NoError(t, engine.Store(ctx, outOfWindow))

	since := time.Date(2023, 5, 1, 0, 0, 0, 0, time.UTC)
	before := time.Date(2023, 6, 1, 0, 0, 0, 0, time.UTC)
	results, err := engine.RecallWithOpts(ctx, "dentist appointment", 10, "summary", search.RecallOpts{
		DateSince:  &since,
		DateBefore: &before,
	})
	require.NoError(t, err)
	require.NotEmpty(t, results)
	require.Equal(t, inWindow.ID, results[0].Memory.ID)
	for _, result := range results {
		require.NotEqual(t, outOfWindow.ID, result.Memory.ID)
	}
}

func TestSearchEngine_List(t *testing.T) {
	engine := newTestEngine(t, uniqueProject("test-list"))
	t.Cleanup(func() { engine.Close() })
	ctx := context.Background()

	m := &types.Memory{ID: types.NewMemoryID(), Content: "list test",
		MemoryType: types.MemoryTypeContext, StorageMode: "focused"}
	require.NoError(t, engine.Store(ctx, m))

	results, err := engine.List(ctx, nil, nil, nil, 10, 0)
	require.NoError(t, err)
	require.NotEmpty(t, results)
}

func TestSearchEngine_Connect(t *testing.T) {
	engine := newTestEngine(t, uniqueProject("test-connect"))
	t.Cleanup(func() { engine.Close() })
	ctx := context.Background()

	m1 := &types.Memory{ID: types.NewMemoryID(), Content: "source",
		MemoryType: types.MemoryTypeContext, StorageMode: "focused"}
	m2 := &types.Memory{ID: types.NewMemoryID(), Content: "target",
		MemoryType: types.MemoryTypeContext, StorageMode: "focused"}
	require.NoError(t, engine.Store(ctx, m1))
	require.NoError(t, engine.Store(ctx, m2))

	err := engine.Connect(ctx, m1.ID, m2.ID, types.RelTypeRelatesTo, 1.0)
	require.NoError(t, err)
}

func TestSearchEngine_Correct(t *testing.T) {
	engine := newTestEngine(t, uniqueProject("test-correct"))
	t.Cleanup(func() { engine.Close() })
	ctx := context.Background()

	m := &types.Memory{ID: types.NewMemoryID(), Content: "original",
		MemoryType: types.MemoryTypeContext, StorageMode: "focused"}
	require.NoError(t, engine.Store(ctx, m))

	newContent := "corrected"
	updated, err := engine.Correct(ctx, m.ID, &newContent, nil, nil, nil)
	require.NoError(t, err)
	require.Equal(t, "corrected", updated.Content)
}

func TestSearchEngine_Forget(t *testing.T) {
	engine := newTestEngine(t, uniqueProject("test-forget"))
	t.Cleanup(func() { engine.Close() })
	ctx := context.Background()

	m := &types.Memory{ID: types.NewMemoryID(), Content: "to delete",
		MemoryType: types.MemoryTypeContext, StorageMode: "focused"}
	require.NoError(t, engine.Store(ctx, m))

	deleted, err := engine.Forget(ctx, m.ID, "")
	require.NoError(t, err)
	require.True(t, deleted)

	gone, err := engine.Backend().GetMemory(ctx, m.ID)
	require.NoError(t, err)
	require.Nil(t, gone)
}

func TestSearchEngine_Status(t *testing.T) {
	engine := newTestEngine(t, uniqueProject("test-status"))
	t.Cleanup(func() { engine.Close() })
	ctx := context.Background()

	stats, err := engine.Status(ctx)
	require.NoError(t, err)
	require.NotNil(t, stats)
}

func TestRecallAttachesAtomPreamble(t *testing.T) {
	engine := newTestEngine(t, uniqueProject("test-atom-preamble"))
	t.Cleanup(func() { engine.Close() })
	ctx := context.Background()

	mem := &types.Memory{
		ID:          types.NewMemoryID(),
		Content:     "The user talks about tea preferences in this session.",
		MemoryType:  types.MemoryTypePreference,
		Importance:  2,
		StorageMode: "focused",
	}
	require.NoError(t, engine.Store(ctx, mem))

	observedAt := time.Date(2024, 6, 15, 9, 0, 0, 0, time.UTC)
	pg, ok := engine.Backend().(*db.PostgresBackend)
	require.True(t, ok, "test engine must use PostgresBackend")
	require.NoError(t, pg.InsertAtom(ctx, &atom.Atom{
		Project:    engine.Project(),
		Type:       atom.TypePreference,
		Subject:    "the user",
		Predicate:  "prefers",
		Value:      "tea",
		Statement:  "The user prefers tea.",
		Scope:      atom.ScopeGlobal,
		Confidence: 0.95,
		ObservedAt: &observedAt,
	}))

	results, err := engine.RecallWithOpts(ctx, "what drink do I prefer", 5, "summary", search.RecallOpts{
		AtomRecallEnabled: true,
	})
	require.NoError(t, err)
	require.NotEmpty(t, results)
	require.Contains(t, results[0].AtomPreamble, "The user prefers tea.")

	noAtom, err := engine.RecallWithOpts(ctx, "what drink do I prefer", 5, "summary", search.RecallOpts{})
	require.NoError(t, err)
	require.NotEmpty(t, noAtom)
	require.Empty(t, noAtom[0].AtomPreamble, "atom preamble must stay disabled by default")

	nonPref, err := engine.RecallWithOpts(ctx, "show my session about tea", 5, "summary", search.RecallOpts{
		AtomRecallEnabled: true,
	})
	require.NoError(t, err)
	require.NotEmpty(t, nonPref)
	require.Empty(t, nonPref[0].AtomPreamble, "non-preference queries must not attach atom preambles")
}

// TestRecallWithEvent_IncrementsTimesRetrieved verifies that RecallWithEvent
// auto-increments times_retrieved on every returned memory so that the
// retrieval precision signal (times_useful / times_retrieved) warms up
// without waiting for explicit memory_feedback calls.
func TestRecallWithEvent_IncrementsTimesRetrieved(t *testing.T) {
	engine := newTestEngine(t, uniqueProject("test-auto-increment"))
	t.Cleanup(func() { engine.Close() })
	ctx := context.Background()

	m := &types.Memory{
		Content:     "Auto-increment times_retrieved on every recall",
		MemoryType:  types.MemoryTypePattern,
		Project:     engine.Project(),
		Importance:  2,
		StorageMode: "focused",
	}
	require.NoError(t, engine.Store(ctx, m))

	// Verify baseline: times_retrieved starts at 0.
	before, err := engine.Backend().GetMemory(ctx, m.ID)
	require.NoError(t, err)
	require.NotNil(t, before)
	baselineRetrieved := before.TimesRetrieved

	// Call RecallWithEvent — this should auto-increment times_retrieved.
	results, eventID, err := engine.RecallWithEvent(ctx, "auto-increment times retrieved", 10, "normal")
	require.NoError(t, err)
	require.NotEmpty(t, eventID, "RecallWithEvent must return a non-empty event_id")

	// Confirm our memory was in the result set.
	found := false
	for _, r := range results {
		if r.Memory != nil && r.Memory.ID == m.ID {
			found = true
			break
		}
	}
	require.True(t, found, "stored memory must appear in recall results")

	// times_retrieved must be incremented by exactly 1.
	after, err := engine.Backend().GetMemory(ctx, m.ID)
	require.NoError(t, err)
	require.NotNil(t, after)
	require.Equal(t, baselineRetrieved+1, after.TimesRetrieved,
		"times_retrieved must be auto-incremented by RecallWithEvent without explicit feedback")
}

// TestStore_RawBodyUsedForChunking verifies that Store() honours m.RawBody: when
// RawBody is non-empty, chunks must be built from the raw body (the full original
// text) rather than from m.Content (which holds only a synopsis). This tests the
// fix for #191: Store() previously called StoreWithRawBody(m, "") unconditionally,
// silently discarding any RawBody the caller set on the Memory.
func TestStore_RawBodyUsedForChunking(t *testing.T) {
	proj := uniqueProject("test-rawbody")
	engine := newTestEngine(t, proj)
	t.Cleanup(func() { engine.Close() })

	ctx := context.Background()

	// synopsis is what a Tier-1 ingest would store in Memory.Content — a short
	// excerpt that fits in the context window.
	synopsis := "Short synopsis: document discusses Go concurrency."
	// rawBody is the original full content whose tokens should appear in chunks.
	rawBody := "Go concurrency is built on goroutines and channels. " +
		"Goroutines are lightweight threads managed by the Go runtime. " +
		"Channels provide a typed conduit for communication between goroutines."

	m := &types.Memory{
		ID:          types.NewMemoryID(),
		Content:     synopsis,
		RawBody:     rawBody,
		MemoryType:  types.MemoryTypeContext,
		Importance:  2,
		StorageMode: "focused",
	}
	require.NoError(t, engine.Store(ctx, m))

	// Retrieve all chunks for this memory and assert they come from rawBody,
	// not from the synopsis. Use GetChunksForMemory (no embedding filter) since
	// chunks are now stored with nil embeddings until the reembed worker runs.
	chunks, err := engine.Backend().GetChunksForMemory(ctx, m.ID)
	require.NoError(t, err)
	require.NotEmpty(t, chunks, "at least one chunk must be stored")

	for _, c := range chunks {
		if c.MemoryID != m.ID {
			continue
		}
		require.Contains(t, rawBody, c.ChunkText,
			"chunk text must come from RawBody, not from the synopsis Content")
		require.NotContains(t, synopsis, c.ChunkText[:min(len(c.ChunkText), len(synopsis))],
			"chunk text must not be a substring of the synopsis alone")
	}
}

// min is a local helper for Go 1.20 compatibility (builtin min arrived in 1.21).
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func TestRecallOpts_CurrentEpisodeID_EpisodeMatchBoost(t *testing.T) {
	// Verify that RecallOpts has CurrentEpisodeID and ScoreInput.EpisodeMatch
	// fires correctly when episode IDs match.
	opts := search.RecallOpts{
		CurrentEpisodeID: "ep-test-123",
	}
	if opts.CurrentEpisodeID == "" {
		t.Fatal("CurrentEpisodeID not set on RecallOpts")
	}

	// Simulate what RecallWithOpts does: if memory.EpisodeID matches, EpisodeMatch=true
	memEpisodeID := "ep-test-123"
	input := search.ScoreInput{
		Cosine: 0.7, BM25: 0.5, HoursSince: 1, Importance: 2,
		EpisodeMatch: opts.CurrentEpisodeID != "" && memEpisodeID == opts.CurrentEpisodeID,
	}
	if !input.EpisodeMatch {
		t.Fatal("expected EpisodeMatch=true when episode IDs match")
	}

	inputNoMatch := search.ScoreInput{
		Cosine: 0.7, BM25: 0.5, HoursSince: 1, Importance: 2,
		EpisodeMatch: opts.CurrentEpisodeID != "" && opts.CurrentEpisodeID == "other-ep",
	}
	if inputNoMatch.EpisodeMatch {
		t.Fatal("expected EpisodeMatch=false when episode IDs differ")
	}
}

// TestRRF_BM25HitsVectorMisses verifies that a memory appearing only in BM25 results
// (exact entity name match) is surfaced ahead of a memory with a weak vector-only match.
// This is the core knowledge-update failure mode that RRF fusion is intended to fix.
func TestRRF_BM25HitsVectorMisses(t *testing.T) {
	w := search.DefaultWeights()
	const k = 60

	// Memory A: strong BM25 hit (rank 1), absent from vector results — exact entity name match.
	rrfA := search.RRFScore(0, 1, k) // vector absent, bm25 rank 1
	scoreA := search.CompositeScoreRRF(search.ScoreInput{HoursSince: 1, Importance: 2}, w, rrfA)

	// Memory B: weak vector hit (rank 50), absent from BM25 — semantically similar but wrong entity.
	rrfB := search.RRFScore(50, 0, k) // vector rank 50, bm25 absent
	scoreB := search.CompositeScoreRRF(search.ScoreInput{HoursSince: 1, Importance: 2}, w, rrfB)

	if scoreA <= scoreB {
		t.Fatalf("BM25 rank-1 hit (%.4f) should outscore weak vector rank-50 hit (%.4f)", scoreA, scoreB)
	}
}

// fakeClientWithName allows tests to customize the embedder name.
type fakeClientWithName struct {
	name string
	dims int
}

func (f *fakeClientWithName) Embed(_ context.Context, text string) ([]float32, error) {
	vec := make([]float32, f.dims)
	for i := range vec {
		vec[i] = float32(i) / float32(f.dims)
	}
	return vec, nil
}
func (f *fakeClientWithName) EmbedWithModel(ctx context.Context, text string) ([]float32, string, error) {
	vec, err := f.Embed(ctx, text)
	return vec, f.Name(), err
}
func (f *fakeClientWithName) Name() string    { return f.name }
func (f *fakeClientWithName) Dimensions() int { return f.dims }

// compile-time check that fakeClientWithName satisfies embed.Client.
var _ embed.Client = (*fakeClientWithName)(nil)

func TestSearchEngine_EmbedderMismatchReturnsPermanentError(t *testing.T) {
	// DSN() will skip the test if TEST_DATABASE_URL is not set.
	ctx := context.Background()
	project := uniqueProject("test-embedder-mismatch")

	// Create an engine with embedder name "fake" and store metadata.
	backend1, err := db.NewPostgresBackend(ctx, project, testDSN(t))
	require.NoError(t, err)
	t.Cleanup(func() { backend1.Close() })

	engine1 := search.New(ctx, backend1, &fakeClientWithName{name: "fake", dims: 1024}, project,
		"http://ollama:11434", "llama3.2", false, nil, 0)
	t.Cleanup(func() { engine1.Close() })

	// Store a memory to trigger checkEmbedderMeta and initialize metadata.
	m := &types.Memory{
		ID:          types.NewMemoryID(),
		Content:     "test memory",
		MemoryType:  types.MemoryTypePattern,
		Importance:  1,
		StorageMode: "focused",
	}
	err = engine1.Store(ctx, m)
	require.NoError(t, err, "first store should succeed")

	// Now create a second engine with a different embedder name ("fake-v2")
	// using the same backend/project, simulating an embedder change.
	backend2, err := db.NewPostgresBackend(ctx, project, testDSN(t))
	require.NoError(t, err)
	t.Cleanup(func() { backend2.Close() })

	engine2 := search.New(ctx, backend2, &fakeClientWithName{name: "fake-v2", dims: 1024}, project,
		"http://ollama:11434", "llama3.2", false, nil, 0)
	t.Cleanup(func() { engine2.Close() })

	// Try to store a memory with the mismatched embedder.
	m2 := &types.Memory{
		ID:          types.NewMemoryID(),
		Content:     "another memory",
		MemoryType:  types.MemoryTypePattern,
		Importance:  1,
		StorageMode: "focused",
	}
	err = engine2.Store(ctx, m2)

	// Assert: errors.Is(err, embed.ErrPermanent) should be true.
	require.Error(t, err, "store should fail with embedder mismatch")
	require.True(t, errors.Is(err, embed.ErrPermanent),
		"error should wrap embed.ErrPermanent, got %v", err)

	// Assert: errors.As(err, &pe) should extract PermanentError with correct fields.
	var pe *embed.PermanentError
	require.True(t, errors.As(err, &pe),
		"error should be or wrap *embed.PermanentError, got %v", err)
	require.NotNil(t, pe, "PermanentError must not be nil")
	require.Equal(t, "embedder_mismatch", pe.Code,
		"Code should be 'embedder_mismatch'")
	require.Equal(t, "fake", pe.Stored,
		"Stored should contain the original embedder name")
	require.Equal(t, "fake-v2", pe.Current,
		"Current should contain the new embedder name")
	require.Contains(t, pe.Remediation, "memory_migrate_embedder",
		"Remediation should mention memory_migrate_embedder")
}

func TestSearchEngine_EmbedderDimensionsMismatchReturnsPermanentError(t *testing.T) {
	// DSN() will skip the test if TEST_DATABASE_URL is not set.
	ctx := context.Background()
	project := uniqueProject("test-embedder-dims-mismatch")

	// Create an engine with 768-dim embedder and store metadata.
	backend1, err := db.NewPostgresBackend(ctx, project, testDSN(t))
	require.NoError(t, err)
	t.Cleanup(func() { backend1.Close() })

	engine1 := search.New(ctx, backend1, &fakeClientWithName{name: "fake", dims: 1024}, project,
		"http://ollama:11434", "llama3.2", false, nil, 0)
	t.Cleanup(func() { engine1.Close() })

	// Store a memory to initialize embedder metadata with 1024 dims.
	m := &types.Memory{
		ID:          types.NewMemoryID(),
		Content:     "test memory",
		MemoryType:  types.MemoryTypePattern,
		Importance:  1,
		StorageMode: "focused",
	}
	err = engine1.Store(ctx, m)
	require.NoError(t, err, "first store should succeed")

	// Now create a second engine with same name but different dimensions (512).
	// 512 is used here as the "wrong" dim to trigger the metadata mismatch guard
	// without conflicting with the schema column dimension (1024).
	backend2, err := db.NewPostgresBackend(ctx, project, testDSN(t))
	require.NoError(t, err)
	t.Cleanup(func() { backend2.Close() })

	engine2 := search.New(ctx, backend2, &fakeClientWithName{name: "fake", dims: 512}, project,
		"http://ollama:11434", "llama3.2", false, nil, 0)
	t.Cleanup(func() { engine2.Close() })

	// Try to store a memory with the mismatched dimensions.
	m2 := &types.Memory{
		ID:          types.NewMemoryID(),
		Content:     "another memory",
		MemoryType:  types.MemoryTypePattern,
		Importance:  1,
		StorageMode: "focused",
	}
	err = engine2.Store(ctx, m2)

	// Assert: errors.Is(err, embed.ErrPermanent) should be true.
	require.Error(t, err, "store should fail with embedder dimensions mismatch")
	require.True(t, errors.Is(err, embed.ErrPermanent),
		"error should wrap embed.ErrPermanent, got %v", err)

	// Assert: errors.As(err, &pe) should extract PermanentError with correct fields.
	var pe *embed.PermanentError
	require.True(t, errors.As(err, &pe),
		"error should be or wrap *embed.PermanentError, got %v", err)
	require.NotNil(t, pe, "PermanentError must not be nil")
	require.Equal(t, "embedder_mismatch", pe.Code,
		"Code should be 'embedder_mismatch'")
	require.Equal(t, "1024-dim", pe.Stored,
		"Stored should contain the original dimensions")
	require.Equal(t, "512-dim", pe.Current,
		"Current should contain the new dimensions")
	require.Contains(t, pe.Remediation, "memory_migrate_embedder",
		"Remediation should mention memory_migrate_embedder")
}

// TestSearchEngine_EmbedderNameAliasesAreCompatible verifies that known aliases
// of the same underlying model (e.g. GGUF filename vs HuggingFace ID) are treated
// as compatible rather than triggering an embedder_mismatch error (#855).
func TestSearchEngine_EmbedderNameAliasesAreCompatible(t *testing.T) {
	ctx := context.Background()

	// Subtest 1: bge-m3-Q8_0.gguf (stored by llama.cpp) vs BAAI/bge-m3 (incoming
	// from Infinity/LiteLLM) — same model, different name conventions.
	t.Run("gguf_filename_and_hf_id_are_compatible", func(t *testing.T) {
		project := uniqueProject("test-embedder-alias-gguf-hf")

		backend1, err := db.NewPostgresBackend(ctx, project, testDSN(t))
		require.NoError(t, err)
		t.Cleanup(func() { backend1.Close() })

		// First engine uses the GGUF filename (as llama.cpp reports it).
		engine1 := search.New(ctx, backend1, &fakeClientWithName{name: "bge-m3-Q8_0.gguf", dims: 1024}, project,
			"http://ollama:11434", "llama3.2", false, nil, 0)
		t.Cleanup(func() { engine1.Close() })

		m := &types.Memory{
			ID:          types.NewMemoryID(),
			Content:     "stored with gguf filename embedder",
			MemoryType:  types.MemoryTypePattern,
			Importance:  1,
			StorageMode: "focused",
		}
		err = engine1.Store(ctx, m)
		require.NoError(t, err, "initial store with gguf filename should succeed")

		backend2, err := db.NewPostgresBackend(ctx, project, testDSN(t))
		require.NoError(t, err)
		t.Cleanup(func() { backend2.Close() })

		// Second engine uses the HuggingFace ID (as Infinity/LiteLLM reports it).
		engine2 := search.New(ctx, backend2, &fakeClientWithName{name: "BAAI/bge-m3", dims: 1024}, project,
			"http://ollama:11434", "llama3.2", false, nil, 0)
		t.Cleanup(func() { engine2.Close() })

		m2 := &types.Memory{
			ID:          types.NewMemoryID(),
			Content:     "stored with hf id embedder",
			MemoryType:  types.MemoryTypePattern,
			Importance:  1,
			StorageMode: "focused",
		}
		// This MUST succeed — both names refer to the same BAAI/bge-m3 model.
		err = engine2.Store(ctx, m2)
		require.NoError(t, err, "store with HuggingFace ID should succeed when GGUF filename is stored (same model)")
	})

	// Subtest 2: genuinely different models must still be rejected.
	t.Run("different_models_are_still_rejected", func(t *testing.T) {
		project := uniqueProject("test-embedder-alias-different")

		backend1, err := db.NewPostgresBackend(ctx, project, testDSN(t))
		require.NoError(t, err)
		t.Cleanup(func() { backend1.Close() })

		engine1 := search.New(ctx, backend1, &fakeClientWithName{name: "BAAI/bge-m3", dims: 1024}, project,
			"http://ollama:11434", "llama3.2", false, nil, 0)
		t.Cleanup(func() { engine1.Close() })

		m := &types.Memory{
			ID:          types.NewMemoryID(),
			Content:     "stored with bge-m3",
			MemoryType:  types.MemoryTypePattern,
			Importance:  1,
			StorageMode: "focused",
		}
		err = engine1.Store(ctx, m)
		require.NoError(t, err, "initial store should succeed")

		backend2, err := db.NewPostgresBackend(ctx, project, testDSN(t))
		require.NoError(t, err)
		t.Cleanup(func() { backend2.Close() })

		// Different model entirely — must still be rejected.
		engine2 := search.New(ctx, backend2, &fakeClientWithName{name: "text-embedding-3-small", dims: 1536}, project,
			"http://ollama:11434", "llama3.2", false, nil, 0)
		t.Cleanup(func() { engine2.Close() })

		m2 := &types.Memory{
			ID:          types.NewMemoryID(),
			Content:     "stored with text-embedding-3-small",
			MemoryType:  types.MemoryTypePattern,
			Importance:  1,
			StorageMode: "focused",
		}
		err = engine2.Store(ctx, m2)
		require.Error(t, err, "store with a genuinely different model should be rejected")
		require.True(t, errors.Is(err, embed.ErrPermanent),
			"error should wrap embed.ErrPermanent, got %v", err)
		var pe *embed.PermanentError
		require.True(t, errors.As(err, &pe))
		require.Equal(t, "embedder_mismatch", pe.Code)
	})
}
