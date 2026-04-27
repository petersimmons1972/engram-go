package search_test

import (
	"context"
	"testing"

	"github.com/petersimmons1972/engram/internal/db"
	"github.com/petersimmons1972/engram/internal/embed"
	"github.com/petersimmons1972/engram/internal/search"
	"github.com/petersimmons1972/engram/internal/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// callCountingClient counts Embed invocations without actually embedding.
type callCountingClient struct {
	calls int
}

func (c *callCountingClient) Embed(_ context.Context, _ string) ([]float32, error) {
	c.calls++
	return make([]float32, 4), nil
}
func (c *callCountingClient) Name() string    { return "counting" }
func (c *callCountingClient) Dimensions() int { return 4 }

var _ embed.Client = (*callCountingClient)(nil)

// TestStore_DoesNotCallEmbedder asserts that Store() completes without calling
// the embedder inline. Embedding must be fully async (handled by the reembed
// worker), so MCP store calls are never blocked by Ollama availability.
func TestStore_DoesNotCallEmbedder(t *testing.T) {
	proj := uniqueProject("test-async-embed")
	ctx := context.Background()

	backend, err := db.NewPostgresBackend(ctx, proj, testDSN(t))
	require.NoError(t, err)
	t.Cleanup(func() { backend.Close() })

	counter := &callCountingClient{}
	engine := search.New(ctx, backend, counter, proj,
		"http://ollama:11434", "llama3.2", false, nil, 0)
	t.Cleanup(func() { engine.Close() })

	m := &types.Memory{
		ID:          types.NewMemoryID(),
		Content:     "Embedding must be async so store calls are never blocked by Ollama.",
		MemoryType:  types.MemoryTypePattern,
		Importance:  2,
		StorageMode: "focused",
	}
	require.NoError(t, engine.Store(ctx, m))

	assert.Equal(t, 0, counter.calls,
		"Store must not call the embedder inline; embedding must be deferred to the reembed worker")
}

// TestStore_ChunksHaveNilEmbeddingAfterStore asserts that chunks written by
// Store() have nil (NULL) embeddings, ready for the reembed worker to fill in.
func TestStore_ChunksHaveNilEmbeddingAfterStore(t *testing.T) {
	proj := uniqueProject("test-async-embed-nil")
	ctx := context.Background()

	backend, err := db.NewPostgresBackend(ctx, proj, testDSN(t))
	require.NoError(t, err)
	t.Cleanup(func() { backend.Close() })

	engine := search.New(ctx, backend, &callCountingClient{}, proj,
		"http://ollama:11434", "llama3.2", false, nil, 0)
	t.Cleanup(func() { engine.Close() })

	m := &types.Memory{
		ID:          types.NewMemoryID(),
		Content:     "Chunks must be stored with nil embeddings for async backfill.",
		MemoryType:  types.MemoryTypeContext,
		Importance:  2,
		StorageMode: "focused",
	}
	require.NoError(t, engine.Store(ctx, m))

	// GetChunksPendingEmbedding returns only chunks with NULL embeddings — exactly
	// what we expect after an async-decoupled Store.
	pending, err := backend.GetChunksPendingEmbedding(ctx, proj, 100)
	require.NoError(t, err)
	require.NotEmpty(t, pending, "Store must have written at least one chunk with nil embedding")

	for _, ch := range pending {
		assert.Nil(t, ch.Embedding,
			"chunk %s must have nil embedding after Store; reembed worker handles it", ch.ID)
	}
}
