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
	// Must complete well within caller deadline (embed timeout is ~500ms).
	require.Less(t, elapsed, 1500*time.Millisecond,
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
