package mcp_test

// TestMemoryIngest_SkipsDuplicates verifies that ingesting the same markdown
// directory twice stores each memory only once. The second call must return
// skipped=N, ingested=0 and the project must still contain exactly the same
// number of memories as after the first call.
//
// This is an integration test — it requires a real PostgreSQL instance.
// Skip with: (no TEST_DATABASE_URL set)

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	internalmcp "github.com/petersimmons1972/engram/internal/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testIngestDSN skips the test when TEST_DATABASE_URL is not set.
func testIngestDSN(t *testing.T) string {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set — skipping integration test")
	}
	return dsn
}

func TestMemoryIngest_SkipsDuplicates(t *testing.T) {
	dsn := testIngestDSN(t)
	ctx := context.Background()
	proj := fmt.Sprintf("test-ingest-dedup-%d", time.Now().UnixNano())

	// Create a temporary directory acting as both DataDir and the ingest path.
	dataDir := t.TempDir()

	// Write two markdown files with distinct content.
	content1 := "# Memory One\n\nThis is the first test memory for dedup.\n"
	content2 := "# Memory Two\n\nThis is the second test memory for dedup.\n"
	require.NoError(t, os.WriteFile(dataDir+"/mem1.md", []byte(content1), 0o644))
	require.NoError(t, os.WriteFile(dataDir+"/mem2.md", []byte(content2), 0o644))

	pool := internalmcp.NewTestPoolWithDSN(t, ctx, dsn, proj)

	// ── First ingest ──────────────────────────────────────────────────────────
	first := internalmcp.CallHandleMemoryIngest(ctx, t, pool, proj, dataDir, dataDir)
	assert.Equal(t, 2, first.Ingested, "first ingest: expected 2 new memories")
	assert.Equal(t, 0, first.Skipped, "first ingest: expected 0 skipped")
	assert.Len(t, first.IDs, 2, "first ingest: expected 2 IDs returned")

	// ── Second ingest (same directory, same files) ────────────────────────────
	second := internalmcp.CallHandleMemoryIngest(ctx, t, pool, proj, dataDir, dataDir)
	assert.Equal(t, 0, second.Ingested, "second ingest: expected 0 new memories (duplicates)")
	assert.Equal(t, 2, second.Skipped, "second ingest: expected 2 skipped (duplicates)")
	assert.Len(t, second.IDs, 0, "second ingest: expected no new IDs")

	// ── Confirm only 2 memories exist in the project ──────────────────────────
	h, err := pool.Get(ctx, proj)
	require.NoError(t, err)
	ids, err := h.Engine.Backend().GetAllMemoryIDs(ctx, proj)
	require.NoError(t, err)
	assert.Len(t, ids, 2, "project must contain exactly 2 memories after two ingests")
}
