package search_test

import (
	"context"
	"testing"

	"github.com/petersimmons1972/engram/internal/types"
	"github.com/stretchr/testify/require"
)

// TestSearchEngine_Aggregate_ByTag verifies that engine.Aggregate(ctx, "tag", "", limit)
// returns aggregated rows grouped by tag, with no error.
func TestSearchEngine_Aggregate_ByTag(t *testing.T) {
	engine := newTestEngine(t, uniqueProject("test-aggregate-tag"))
	t.Cleanup(func() { engine.Close() })

	ctx := context.Background()

	// Store a memory with tags so we have something to aggregate.
	m := &types.Memory{
		ID:          types.NewMemoryID(),
		Content:     "test content for aggregation",
		MemoryType:  types.MemoryTypeContext,
		Importance:  1,
		StorageMode: "focused",
		Tags:        []string{"testag"},
	}
	err := engine.Store(ctx, m)
	require.NoError(t, err)

	// Call Aggregate with by="tag".
	rows, err := engine.Aggregate(ctx, "tag", "", 20)
	require.NoError(t, err)
	require.NotNil(t, rows)
	require.GreaterOrEqual(t, len(rows), 1, "should return at least one aggregated row when tags exist")
}

// TestSearchEngine_Aggregate_ByFailureClass verifies that engine.Aggregate(ctx, "failure_class", "", limit)
// returns aggregated rows for failure classes, with no error. Even if no failures have been recorded yet,
// the call must not error; it may return an empty slice.
func TestSearchEngine_Aggregate_ByFailureClass(t *testing.T) {
	engine := newTestEngine(t, uniqueProject("test-aggregate-failure-class"))
	t.Cleanup(func() { engine.Close() })

	ctx := context.Background()

	// Call Aggregate with by="failure_class".
	// The engine may return an empty slice if no retrieval events with failure_class have been recorded,
	// but it must not return an error.
	rows, err := engine.Aggregate(ctx, "failure_class", "", 20)
	require.NoError(t, err, "Aggregate must not error even if no failure classes exist")
	require.NotNil(t, rows)
}

// TestSearchEngine_Aggregate_InvalidBy verifies that engine.Aggregate rejects invalid "by" values.
func TestSearchEngine_Aggregate_InvalidBy(t *testing.T) {
	engine := newTestEngine(t, uniqueProject("test-aggregate-invalid"))
	t.Cleanup(func() { engine.Close() })

	ctx := context.Background()

	// Call Aggregate with an invalid "by" value.
	rows, err := engine.Aggregate(ctx, "bogus", "", 20)
	require.Error(t, err, "Aggregate must reject invalid by values")
	require.Nil(t, rows)
}
