package mcp_test

// Step 4: Re-Summarization on Model Change
// Tests are written BEFORE implementation (TDD).
// They must fail until ClearSummaries, handleMemoryResummarize, and the
// UpdateMemory summary-clearing fix are in place.

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	internalmcp "github.com/petersimmons1972/engram/internal/mcp"
	"github.com/petersimmons1972/engram/internal/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testResummarizeDSN skips the test when TEST_DATABASE_URL is not set.
func testResummarizeDSN(t *testing.T) string {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set — skipping integration test")
	}
	return dsn
}

// TestHandleMemoryResummarize_ClearsSummaries stores three memories, manually
// sets summaries on them, then calls memory_resummarize and verifies that all
// summaries for the project are NULL so the background worker regenerates them.
func TestHandleMemoryResummarize_ClearsSummaries(t *testing.T) {
	dsn := testResummarizeDSN(t)
	ctx := context.Background()
	proj := fmt.Sprintf("test-resummarize-%d", time.Now().UnixNano())

	pool := internalmcp.NewTestPoolWithDSN(t, ctx, dsn, proj)

	h, err := pool.Get(ctx, proj)
	require.NoError(t, err)

	// Store three memories.
	mems := []*types.Memory{
		{
			ID:          types.NewMemoryID(),
			Content:     "Use TDD — write the failing test first.",
			MemoryType:  types.MemoryTypePattern,
			Importance:  1,
			StorageMode: "focused",
		},
		{
			ID:          types.NewMemoryID(),
			Content:     "Deploy to staging before production.",
			MemoryType:  types.MemoryTypeDecision,
			Importance:  2,
			StorageMode: "focused",
		},
		{
			ID:          types.NewMemoryID(),
			Content:     "Prefer explicit over implicit in Go.",
			MemoryType:  types.MemoryTypePattern,
			Importance:  1,
			StorageMode: "focused",
		},
	}
	for _, m := range mems {
		require.NoError(t, h.Engine.Store(ctx, m))
	}

	// Manually write a summary for each memory so they all have summary IS NOT NULL.
	be := h.Engine.Backend()
	for _, m := range mems {
		require.NoError(t, be.StoreSummary(ctx, m.ID, "stub summary for "+m.ID))
	}

	// Confirm summaries are set before the test action.
	pendingBefore, err := be.GetPendingSummaryCount(ctx, proj)
	require.NoError(t, err)
	assert.Equal(t, 0, pendingBefore, "expected 0 pending summaries before clear")

	// Call memory_resummarize via the exported test helper.
	cleared, message := internalmcp.CallHandleMemoryResummarize(ctx, t, pool, proj)

	assert.Equal(t, 3, cleared, "expected 3 summaries cleared")
	assert.Contains(t, message, proj, "message should mention the project name")
	assert.Contains(t, message, "3", "message should state how many were cleared")

	// All three summaries must now be NULL — pending count should be 3.
	pendingAfter, err := be.GetPendingSummaryCount(ctx, proj)
	require.NoError(t, err)
	assert.Equal(t, 3, pendingAfter, "expected 3 pending summaries after clear")
}

// TestHandleMemoryCorrect_ClearsSummaryOnContentChange stores a memory,
// sets a summary on it, then updates the content via memory_correct and verifies
// that the summary was cleared (so the worker regenerates it with the new content).
func TestHandleMemoryCorrect_ClearsSummaryOnContentChange(t *testing.T) {
	dsn := testResummarizeDSN(t)
	ctx := context.Background()
	proj := fmt.Sprintf("test-correct-summary-%d", time.Now().UnixNano())

	pool := internalmcp.NewTestPoolWithDSN(t, ctx, dsn, proj)

	h, err := pool.Get(ctx, proj)
	require.NoError(t, err)

	// Store a memory.
	m := &types.Memory{
		ID:          types.NewMemoryID(),
		Content:     "Always check logs before restarting a service.",
		MemoryType:  types.MemoryTypePattern,
		Importance:  2,
		StorageMode: "focused",
	}
	require.NoError(t, h.Engine.Store(ctx, m))

	// Manually set a summary so we can verify it gets cleared.
	be := h.Engine.Backend()
	require.NoError(t, be.StoreSummary(ctx, m.ID, "stub summary before edit"))

	// Confirm the summary is set.
	pending, err := be.GetPendingSummaryCount(ctx, proj)
	require.NoError(t, err)
	assert.Equal(t, 0, pending, "expected 0 pending before content update")

	// Update the memory content via the exported test helper.
	internalmcp.CallHandleMemoryCorrect(ctx, t, pool, proj, m.ID,
		"Always check logs AND resource usage before restarting a service.")

	// The summary must now be NULL because content changed.
	pending, err = be.GetPendingSummaryCount(ctx, proj)
	require.NoError(t, err)
	assert.Equal(t, 1, pending, "summary must be NULL (pending=1) after content change")
}

// TestHandleMemoryCorrect_PreservesSummaryOnTagOnlyChange verifies that updating
// only tags does NOT clear the summary — content did not change.
func TestHandleMemoryCorrect_PreservesSummaryOnTagOnlyChange(t *testing.T) {
	dsn := testResummarizeDSN(t)
	ctx := context.Background()
	proj := fmt.Sprintf("test-correct-tags-only-%d", time.Now().UnixNano())

	pool := internalmcp.NewTestPoolWithDSN(t, ctx, dsn, proj)

	h, err := pool.Get(ctx, proj)
	require.NoError(t, err)

	m := &types.Memory{
		ID:          types.NewMemoryID(),
		Content:     "Use short variable names only in short scopes.",
		MemoryType:  types.MemoryTypePattern,
		Importance:  1,
		StorageMode: "focused",
	}
	require.NoError(t, h.Engine.Store(ctx, m))

	// Set a summary.
	be := h.Engine.Backend()
	require.NoError(t, be.StoreSummary(ctx, m.ID, "tag-only-test stub summary"))

	// Update only the tags — no content change.
	internalmcp.CallHandleMemoryCorrectTagsOnly(ctx, t, pool, proj, m.ID,
		[]string{"go", "style"})

	// Summary must still be set — content did not change.
	pending, err := be.GetPendingSummaryCount(ctx, proj)
	require.NoError(t, err)
	assert.Equal(t, 0, pending, "summary must be preserved when only tags change")
}
