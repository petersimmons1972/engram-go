package consolidate_test

// Feature 3: Sleep Consolidation Daemon
// All tests written BEFORE implementation (TDD).
// They must fail (compile or runtime) until Feature 3 is implemented.

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/petersimmons1972/engram/internal/consolidate"
	"github.com/petersimmons1972/engram/internal/db"
	"github.com/petersimmons1972/engram/internal/embed"
	"github.com/petersimmons1972/engram/internal/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testDSN(t *testing.T) string {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set — skipping integration test")
	}
	return dsn
}

func uniqueProject(base string) string {
	return fmt.Sprintf("%s-%d", base, time.Now().UnixNano())
}

// fakeEmbedder returns deterministic vectors that encode similarity via a simple hash.
type fakeEmbedder struct{ dims int }

func (f *fakeEmbedder) Embed(_ context.Context, text string) ([]float32, error) {
	vec := make([]float32, f.dims)
	// Make similar-content texts produce similar vectors by spreading hash bytes.
	h := 0
	for i, c := range text {
		h = h*31 + int(c) + i
	}
	for i := range vec {
		vec[i] = float32((h+i)%100) / 100.0
	}
	return vec, nil
}
func (f *fakeEmbedder) Name() string    { return "fake" }
func (f *fakeEmbedder) Dimensions() int { return f.dims }

var _ embed.Client = (*fakeEmbedder)(nil)

func newTestRunner(t *testing.T, project string) *consolidate.Runner {
	t.Helper()
	ctx := context.Background()
	backend, err := db.NewPostgresBackend(ctx, project, testDSN(t))
	require.NoError(t, err)
	t.Cleanup(func() { backend.Close() })
	return consolidate.NewRunner(backend, project, &fakeEmbedder{dims: 768})
}

// ── InferRelationships ────────────────────────────────────────────────────────

// TestInferRelationships_CreatesEdges verifies that InferRelationships creates
// relates_to edges between memories whose chunks are nearest neighbors.
func TestInferRelationships_CreatesEdges(t *testing.T) {
	project := uniqueProject("consolidate-infer")
	ctx := context.Background()

	backend, err := db.NewPostgresBackend(ctx, project, testDSN(t))
	require.NoError(t, err)
	t.Cleanup(func() { backend.Close() })

	runner := consolidate.NewRunner(backend, project, &fakeEmbedder{dims: 768})

	// Store two closely related memories and manually set their chunks with known embeddings.
	m1 := &types.Memory{
		ID: types.NewMemoryID(), Content: "PostgreSQL uses MVCC for transaction isolation",
		MemoryType: types.MemoryTypeArchitecture, Project: project, Importance: 2, StorageMode: "focused",
	}
	m2 := &types.Memory{
		ID: types.NewMemoryID(), Content: "PostgreSQL MVCC creates row versions for each transaction",
		MemoryType: types.MemoryTypeArchitecture, Project: project, Importance: 2, StorageMode: "focused",
	}
	require.NoError(t, backend.StoreMemory(ctx, m1))
	require.NoError(t, backend.StoreMemory(ctx, m2))

	// Store chunks with very similar embeddings (nearly identical vectors → should be detected as related).
	vec1 := make([]float32, 768)
	vec2 := make([]float32, 768)
	for i := range vec1 {
		vec1[i] = 0.5
		vec2[i] = 0.5 + float32(i)/float32(768*1000) // nearly identical
	}
	require.NoError(t, backend.StoreChunks(ctx, []*types.Chunk{
		{ID: types.NewMemoryID(), MemoryID: m1.ID, ChunkText: m1.Content, ChunkIndex: 0,
			ChunkHash: "hash1a", ChunkType: "sentence_window", Project: project, Embedding: vec1},
	}))
	require.NoError(t, backend.StoreChunks(ctx, []*types.Chunk{
		{ID: types.NewMemoryID(), MemoryID: m2.ID, ChunkText: m2.Content, ChunkIndex: 0,
			ChunkHash: "hash2a", ChunkType: "sentence_window", Project: project, Embedding: vec2},
	}))

	created, err := runner.InferRelationships(ctx, 0.3, 100) // low threshold to guarantee creation
	require.NoError(t, err)
	assert.Greater(t, created, 0, "InferRelationships must create at least one relates_to edge")

	// Verify the relationship exists.
	count, err := backend.GetConnectionCount(ctx, m1.ID, project)
	require.NoError(t, err)
	assert.Greater(t, count, 0, "memory m1 must have at least one connection after InferRelationships")
}

// TestInferRelationships_SkipsExistingEdges verifies that InferRelationships does
// not create duplicate edges when a relationship already exists between two memories.
func TestInferRelationships_SkipsExistingEdges(t *testing.T) {
	project := uniqueProject("consolidate-skip")
	ctx := context.Background()

	backend, err := db.NewPostgresBackend(ctx, project, testDSN(t))
	require.NoError(t, err)
	t.Cleanup(func() { backend.Close() })

	runner := consolidate.NewRunner(backend, project, &fakeEmbedder{dims: 768})

	m1 := &types.Memory{
		ID: types.NewMemoryID(), Content: "Go channels enable safe goroutine communication",
		MemoryType: types.MemoryTypePattern, Project: project, Importance: 2, StorageMode: "focused",
	}
	m2 := &types.Memory{
		ID: types.NewMemoryID(), Content: "Go goroutines communicate via channels",
		MemoryType: types.MemoryTypePattern, Project: project, Importance: 2, StorageMode: "focused",
	}
	require.NoError(t, backend.StoreMemory(ctx, m1))
	require.NoError(t, backend.StoreMemory(ctx, m2))

	// Pre-create the relationship.
	require.NoError(t, backend.StoreRelationship(ctx, &types.Relationship{
		ID: types.NewMemoryID(), SourceID: m1.ID, TargetID: m2.ID,
		RelType: types.RelTypeRelatesTo, Strength: 0.9, Project: project,
	}))

	vec := make([]float32, 768)
	for i := range vec {
		vec[i] = 0.5
	}
	require.NoError(t, backend.StoreChunks(ctx, []*types.Chunk{
		{ID: types.NewMemoryID(), MemoryID: m1.ID, ChunkText: m1.Content, ChunkIndex: 0,
			ChunkHash: "hash1b", ChunkType: "sentence_window", Project: project, Embedding: vec},
		{ID: types.NewMemoryID(), MemoryID: m2.ID, ChunkText: m2.Content, ChunkIndex: 0,
			ChunkHash: "hash2b", ChunkType: "sentence_window", Project: project, Embedding: vec},
	}))

	beforeCount, err := backend.GetConnectionCount(ctx, m1.ID, project)
	require.NoError(t, err)

	_, err = runner.InferRelationships(ctx, 0.0, 100) // threshold=0 → all pairs
	require.NoError(t, err)

	afterCount, err := backend.GetConnectionCount(ctx, m1.ID, project)
	require.NoError(t, err)
	assert.Equal(t, beforeCount, afterCount,
		"InferRelationships must not duplicate existing edges")
}

// ── RunAll ────────────────────────────────────────────────────────────────────

// TestRunAll_ReturnsStats verifies that RunAll returns a non-zero stats map.
func TestRunAll_ReturnsStats(t *testing.T) {
	project := uniqueProject("consolidate-runall")
	ctx := context.Background()

	backend, err := db.NewPostgresBackend(ctx, project, testDSN(t))
	require.NoError(t, err)
	t.Cleanup(func() { backend.Close() })

	runner := consolidate.NewRunner(backend, project, &fakeEmbedder{dims: 768})

	m := &types.Memory{
		Content: "Sleep consolidation: infer relationships between related memories",
		MemoryType: types.MemoryTypePattern, Project: project, Importance: 2, StorageMode: "focused",
	}
	m.ID = types.NewMemoryID()
	require.NoError(t, backend.StoreMemory(ctx, m))

	stats, err := runner.RunAll(ctx, consolidate.RunOptions{
		InferRelationshipsMinSimilarity: 0.5,
		InferRelationshipsLimit:         50,
	})
	require.NoError(t, err)
	assert.NotNil(t, stats, "RunAll must return stats")
	// InferRelationships must always run (the others are optional/LLM).
	_, ok := stats["inferred_relationships"]
	assert.True(t, ok, "stats must include inferred_relationships count")
}
