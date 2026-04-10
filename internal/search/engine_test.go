package search_test

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/petersimmons1972/engram/internal/db"
	"github.com/petersimmons1972/engram/internal/embed"
	"github.com/petersimmons1972/engram/internal/search"
	"github.com/petersimmons1972/engram/internal/types"
	"github.com/stretchr/testify/require"
)

// uniqueProject returns a per-run isolated project name to prevent cross-run
// state leakage when the test database persists between test invocations.
func uniqueProject(base string) string {
	return fmt.Sprintf("%s-%d", base, time.Now().UnixNano())
}

func testDSN(t *testing.T) string {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set — skipping integration test")
	}
	return dsn
}

type fakeClient struct{ dims int }

func (f *fakeClient) Embed(_ context.Context, text string) ([]float32, error) {
	vec := make([]float32, f.dims)
	for i := range vec {
		vec[i] = float32(i) / float32(f.dims)
	}
	return vec, nil
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
	return search.New(ctx, backend, &fakeClient{dims: 768}, project)
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

	chunks, err := engine.Backend().GetAllChunksWithEmbeddings(ctx, proj, 10_000)
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
	updated, err := engine.Correct(ctx, m.ID, &newContent, nil, nil)
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

	deleted, err := engine.Forget(ctx, m.ID)
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
