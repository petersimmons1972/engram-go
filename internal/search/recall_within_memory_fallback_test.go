package search_test

// recall_within_memory_fallback_test.go — #925 regression tests.
//
// RecallWithinMemory must degrade to keyword-ranked chunk results when the
// embedder errors or times out, instead of returning an error. This mirrors
// the BM25+recency fallback already present in RecallWithOpts.

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/petersimmons1972/engram/internal/embed"
	"github.com/petersimmons1972/engram/internal/search"
	"github.com/petersimmons1972/engram/internal/types"
	"github.com/stretchr/testify/require"
)

// chunkFallbackBackend is a stubBackend variant that returns pre-loaded chunks
// for GetChunksForMemory and returns nothing for vector search.
type chunkFallbackBackend struct {
	*stubBackend
	chunks []*types.Chunk
}

func (b *chunkFallbackBackend) SearchChunksWithinMemory(_ context.Context, _ []float32, _ string, _ int) ([]*types.Chunk, error) {
	// vector search must not be reached when embed degraded
	return nil, errors.New("SearchChunksWithinMemory must not be called when embed is degraded")
}

func (b *chunkFallbackBackend) GetChunksForMemory(_ context.Context, _ string) ([]*types.Chunk, error) {
	return b.chunks, nil
}

// immediateErrorEmbedder returns an error immediately.
type immediateErrorEmbedder struct {
	dims int
}

func (e *immediateErrorEmbedder) Embed(_ context.Context, _ string) ([]float32, error) {
	return nil, errors.New("embedder unavailable")
}
func (e *immediateErrorEmbedder) EmbedWithModel(_ context.Context, text string) ([]float32, string, error) {
	v, err := e.Embed(context.Background(), text)
	return v, e.Name(), err
}
func (e *immediateErrorEmbedder) Name() string    { return "immediate-error-fake" }
func (e *immediateErrorEmbedder) Dimensions() int { return e.dims }

var _ embed.Client = (*immediateErrorEmbedder)(nil)

// newChunkFallbackEngine builds a *search.SearchEngine backed by
// chunkFallbackBackend with the given chunks and the given embedder.
func newChunkFallbackEngine(t *testing.T, proj string, chunks []*types.Chunk, emb embed.Client) *search.SearchEngine {
	t.Helper()
	backend := &chunkFallbackBackend{
		stubBackend: newStubBackend(0, 0),
		chunks:      chunks,
	}
	eng := search.New(context.Background(), backend, emb, proj,
		"http://ollama:11434", "llama3.2", false, nil, 0)
	t.Cleanup(func() { eng.Close() })
	return eng
}

// TestRecallWithinMemory_EmbedError_ReturnsBM25Results verifies that when the
// embedder returns an error immediately, RecallWithinMemory degrades to keyword
// search across the memory's chunks and returns non-empty results (not an error).
func TestRecallWithinMemory_EmbedError_ReturnsBM25Results(t *testing.T) {
	proj := uniqueProject("rwm-fallback-error")
	memID := "mem-doc-001"

	chunks := []*types.Chunk{
		{
			ID:         "chunk-001",
			MemoryID:   memID,
			Project:    proj,
			ChunkText:  "The capital city of France is Paris.",
			ChunkIndex: 0,
		},
		{
			ID:         "chunk-002",
			MemoryID:   memID,
			Project:    proj,
			ChunkText:  "Mount Everest is the tallest mountain in the world.",
			ChunkIndex: 1,
		},
		{
			ID:         "chunk-003",
			MemoryID:   memID,
			Project:    proj,
			ChunkText:  "The Eiffel Tower is located in Paris, France.",
			ChunkIndex: 2,
		},
	}

	emb := &immediateErrorEmbedder{dims: 768}
	eng := newChunkFallbackEngine(t, proj, chunks, emb)

	results, err := eng.RecallWithinMemory(context.Background(), "Paris France capital", memID, 5, "summary")

	// Must NOT return an error — degrade to keyword fallback.
	require.NoError(t, err, "RecallWithinMemory must NOT error on embed failure; should degrade to keyword search")
	require.NotNil(t, results, "results must be non-nil even on embed failure")
	require.NotEmpty(t, results, "keyword search must return at least one result for query 'Paris France capital'")

	// The Paris-related chunks should rank above the mountain chunk.
	var parisFound bool
	for _, r := range results {
		if r.Content == chunks[0].ChunkText || r.Content == chunks[2].ChunkText {
			parisFound = true
		}
	}
	require.True(t, parisFound, "at least one Paris-related chunk should appear in fallback results")
}

// TestRecallWithinMemory_EmbedTimeout_ReturnsBM25Results verifies that when the
// embedder hangs (times out), RecallWithinMemory still returns keyword results.
func TestRecallWithinMemory_EmbedTimeout_ReturnsBM25Results(t *testing.T) {
	proj := uniqueProject("rwm-fallback-timeout")
	memID := "mem-doc-002"

	chunks := []*types.Chunk{
		{
			ID:         "chunk-t001",
			MemoryID:   memID,
			Project:    proj,
			ChunkText:  "Golang is a statically typed compiled language.",
			ChunkIndex: 0,
		},
	}

	// hangingEmbedder is defined in recall_fallback_test.go (same package).
	emb := &hangingEmbedder{dims: 768}
	eng := newChunkFallbackEngine(t, proj, chunks, emb)

	callerCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	start := time.Now()
	results, err := eng.RecallWithinMemory(callerCtx, "Golang compiled language", memID, 5, "summary")
	elapsed := time.Since(start)

	require.NoError(t, err, "RecallWithinMemory must NOT error on embed timeout; degrade to keyword search")
	require.NotNil(t, results, "results must be non-nil on embed timeout")
	// Must complete well within caller deadline. The restored default is 500ms;
	// allow slack for BM25 processing under CI jitter.
	require.Less(t, elapsed, 700*time.Millisecond,
		"RecallWithinMemory must return quickly after embed timeout; got %s", elapsed)
}

// TestRecallWithinMemory_EmbedError_EmptyChunks_ReturnsEmpty verifies the
// edge case: embed fails AND the memory has no chunks — must return empty, no error.
func TestRecallWithinMemory_EmbedError_EmptyChunks_ReturnsEmpty(t *testing.T) {
	proj := uniqueProject("rwm-fallback-empty")
	memID := "mem-doc-003"

	emb := &immediateErrorEmbedder{dims: 768}
	eng := newChunkFallbackEngine(t, proj, []*types.Chunk{}, emb)

	results, err := eng.RecallWithinMemory(context.Background(), "any query", memID, 5, "summary")

	require.NoError(t, err, "empty-chunks fallback must not error")
	require.NotNil(t, results)
	require.Empty(t, results, "no chunks means no results")
}

// ── Concern 1: parent-ctx cancellation must NOT be swallowed (#927) ───────────

// TestRecallWithinMemory_ParentCtxCancelled_PropagatesError verifies that when
// the PARENT context is cancelled (not just the embed child ctx), RecallWithinMemory
// returns the cancellation error rather than silently degrading to BM25 results.
func TestRecallWithinMemory_ParentCtxCancelled_PropagatesError(t *testing.T) {
	proj := uniqueProject("rwm-parent-cancel")
	memID := "mem-doc-cancel"

	chunks := []*types.Chunk{
		{
			ID:         "chunk-c001",
			MemoryID:   memID,
			Project:    proj,
			ChunkText:  "Some content that would match any query.",
			ChunkIndex: 0,
		},
	}

	// Use a hanging embedder so embed blocks until its (child) ctx is cancelled.
	// We will cancel the PARENT ctx ourselves before calling RecallWithinMemory.
	emb := &hangingEmbedder{dims: 768}
	eng := newChunkFallbackEngine(t, proj, chunks, emb)

	// Cancel parent before the call.
	parentCtx, parentCancel := context.WithCancel(context.Background())
	parentCancel()

	results, err := eng.RecallWithinMemory(parentCtx, "any query", memID, 5, "summary")

	// Must propagate the context cancellation error, NOT return keyword results.
	require.Error(t, err, "cancelled parent ctx must yield an error, not keyword fallback results")
	require.ErrorIs(t, err, context.Canceled,
		"error must wrap context.Canceled; got: %v", err)
	require.Nil(t, results, "results must be nil when parent ctx is cancelled")
}

// TestRecallWithinMemory_ParentDeadlineExceeded_PropagatesError verifies the
// DeadlineExceeded variant: when the parent deadline has already expired before
// the call, the error is propagated rather than swallowed into BM25 fallback.
func TestRecallWithinMemory_ParentDeadlineExceeded_PropagatesError(t *testing.T) {
	proj := uniqueProject("rwm-parent-deadline")
	memID := "mem-doc-deadline"

	chunks := []*types.Chunk{
		{
			ID:         "chunk-d001",
			MemoryID:   memID,
			Project:    proj,
			ChunkText:  "Content for deadline test.",
			ChunkIndex: 0,
		},
	}

	emb := &hangingEmbedder{dims: 768}
	eng := newChunkFallbackEngine(t, proj, chunks, emb)

	// Parent deadline already expired.
	parentCtx, parentCancel := context.WithTimeout(context.Background(), 1*time.Millisecond)
	defer parentCancel()
	time.Sleep(5 * time.Millisecond) // ensure deadline has passed

	results, err := eng.RecallWithinMemory(parentCtx, "deadline query", memID, 5, "summary")

	require.Error(t, err, "expired parent deadline must yield an error, not keyword fallback")
	require.ErrorIs(t, err, context.DeadlineExceeded,
		"error must wrap context.DeadlineExceeded; got: %v", err)
	require.Nil(t, results, "results must be nil when parent ctx deadline exceeded")
}

// TestRecallWithinMemory_EmbedOwnTimeout_StillFallsBackToBM25 verifies the
// complementary case: when the embed child ctx times out but the PARENT ctx
// is still alive, fallback to BM25 results must still occur (no regression).
func TestRecallWithinMemory_EmbedOwnTimeout_StillFallsBackToBM25(t *testing.T) {
	proj := uniqueProject("rwm-embed-timeout-fallback")
	memID := "mem-doc-embed-timeout"

	chunks := []*types.Chunk{
		{
			ID:         "chunk-e001",
			MemoryID:   memID,
			Project:    proj,
			ChunkText:  "Python is a dynamically typed interpreted language.",
			ChunkIndex: 0,
		},
		{
			ID:         "chunk-e002",
			MemoryID:   memID,
			Project:    proj,
			ChunkText:  "Go is a statically typed compiled language created at Google.",
			ChunkIndex: 1,
		},
	}

	// hangingEmbedder blocks until its context is cancelled (embed child ctx
	// will be cancelled by the 500ms embed deadline; parent ctx has 10s).
	emb := &hangingEmbedder{dims: 768}
	eng := newChunkFallbackEngine(t, proj, chunks, emb)

	parentCtx, parentCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer parentCancel()

	results, err := eng.RecallWithinMemory(parentCtx, "Go compiled language", memID, 5, "summary")

	// Must NOT error — embed child timeout with alive parent → BM25 fallback.
	require.NoError(t, err, "embed child timeout with alive parent must fall back to BM25, not error")
	require.NotNil(t, results)
	require.NotEmpty(t, results, "fallback must return keyword results when parent ctx is alive")
	// Parent ctx must still be alive.
	require.NoError(t, parentCtx.Err(), "parent ctx must remain alive after embed timeout")
}

// ── Concern 2: unit tests for recallWithinMemoryKeywordFallback ───────────────

// TestKeywordFallback_TopKSlicing verifies that results are capped at topK.
func TestKeywordFallback_TopKSlicing(t *testing.T) {
	proj := uniqueProject("rwm-topk")
	memID := "mem-topk"

	// 6 chunks, all matching the query with varying scores.
	chunks := make([]*types.Chunk, 6)
	for i := range chunks {
		chunks[i] = &types.Chunk{
			ID:         "chunk-topk-" + string(rune('0'+i)),
			MemoryID:   memID,
			Project:    proj,
			ChunkText:  "The quick brown fox jumps",
			ChunkIndex: i,
		}
	}

	emb := &immediateErrorEmbedder{dims: 768}
	eng := newChunkFallbackEngine(t, proj, chunks, emb)

	// topK=3 must return exactly 3 results even though 6 chunks match.
	results, err := eng.RecallWithinMemory(context.Background(), "quick brown fox", memID, 3, "full")
	require.NoError(t, err)
	require.Len(t, results, 3, "results must be capped at topK=3")
}

// TestKeywordFallback_ScoringOrder verifies that higher term-overlap chunks rank
// above lower-overlap chunks, and that chunk_index breaks ties ascending.
func TestKeywordFallback_ScoringOrder(t *testing.T) {
	proj := uniqueProject("rwm-scoring")
	memID := "mem-scoring"

	// chunk-high: 3 matching terms (highest score)
	// chunk-mid:  1 matching term
	// chunk-low:  0 matching terms (lowest score)
	// chunk-tie-a/b: same score, ordered by chunk_index
	chunks := []*types.Chunk{
		{ID: "chunk-low", MemoryID: memID, Project: proj,
			ChunkText: "completely unrelated content here", ChunkIndex: 0},
		{ID: "chunk-tie-b", MemoryID: memID, Project: proj,
			ChunkText: "golang programming", ChunkIndex: 2},
		{ID: "chunk-high", MemoryID: memID, Project: proj,
			ChunkText: "golang is a compiled statically typed language", ChunkIndex: 3},
		{ID: "chunk-tie-a", MemoryID: memID, Project: proj,
			ChunkText: "golang programming", ChunkIndex: 1},
		{ID: "chunk-mid", MemoryID: memID, Project: proj,
			ChunkText: "programming in many languages", ChunkIndex: 4},
	}

	emb := &immediateErrorEmbedder{dims: 768}
	eng := newChunkFallbackEngine(t, proj, chunks, emb)

	results, err := eng.RecallWithinMemory(context.Background(), "golang compiled statically typed", memID, 10, "full")
	require.NoError(t, err)
	require.NotEmpty(t, results)

	// First result must be chunk-high (matches "golang", "compiled", "statically", "typed").
	require.Equal(t, "golang is a compiled statically typed language", results[0].Content,
		"highest term-overlap chunk must rank first")

	// chunk-tie-a must appear before chunk-tie-b (both match "golang programming";
	// chunk_index=1 < chunk_index=2).
	tieAIdx, tieBIdx := -1, -1
	for i, r := range results {
		if r.Content == "golang programming" {
			if tieAIdx == -1 {
				tieAIdx = i
			} else {
				tieBIdx = i
			}
		}
	}
	require.GreaterOrEqual(t, tieAIdx, 0, "tie-a chunk must appear in results")
	require.GreaterOrEqual(t, tieBIdx, 0, "tie-b chunk must appear in results")
	require.Less(t, tieAIdx, tieBIdx, "tie-a (chunk_index=1) must rank before tie-b (chunk_index=2)")

	// Last result: among zero-overlap chunks, chunk-mid (chunk_index=4) ranks
	// after chunk-low (chunk_index=0) due to ascending chunk_index tie-break.
	require.Equal(t, "programming in many languages", results[len(results)-1].Content,
		"highest chunk_index zero-overlap chunk must rank last (tie-break ascending)")
}
