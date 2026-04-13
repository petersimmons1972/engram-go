package mcp_test

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

// TestMemoryIngest_SkipsDuplicates verifies that ingesting the same markdown
// directory twice stores each memory only once. The second call must return
// skipped=N, ingested=0 and the project must still contain exactly the same
// number of memories as after the first call.
//
// This is an integration test — it requires a real PostgreSQL instance.
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

// TestMemoryIngest_PartialOverlap verifies that when a second ingest contains a
// mix of already-stored and new memories, only the new ones are stored.
// Boundary condition: ingested>0 AND skipped>0 in a single call.
//
// This is an integration test — it requires a real PostgreSQL instance.
func TestMemoryIngest_PartialOverlap(t *testing.T) {
	dsn := testIngestDSN(t)
	ctx := context.Background()
	proj := fmt.Sprintf("test-ingest-partial-%d", time.Now().UnixNano())

	dataDir1 := t.TempDir()
	dataDir2 := t.TempDir()

	content1 := "# Memory Alpha\n\nThis is the first memory for partial overlap testing.\n"
	content2 := "# Memory Beta\n\nThis is the second memory for partial overlap testing.\n"
	content3 := "# Memory Gamma\n\nThis is a brand new memory only in the second ingest.\n"

	// First ingest: Alpha + Beta
	require.NoError(t, os.WriteFile(dataDir1+"/alpha.md", []byte(content1), 0o644))
	require.NoError(t, os.WriteFile(dataDir1+"/beta.md", []byte(content2), 0o644))

	pool := internalmcp.NewTestPoolWithDSN(t, ctx, dsn, proj)

	first := internalmcp.CallHandleMemoryIngest(ctx, t, pool, proj, dataDir1, dataDir1)
	assert.Equal(t, 2, first.Ingested, "first ingest: expected 2 new memories")
	assert.Equal(t, 0, first.Skipped, "first ingest: expected 0 skipped")

	// Second ingest: Beta (duplicate) + Gamma (new) — should store exactly 1, skip exactly 1.
	require.NoError(t, os.WriteFile(dataDir2+"/beta.md", []byte(content2), 0o644))
	require.NoError(t, os.WriteFile(dataDir2+"/gamma.md", []byte(content3), 0o644))

	second := internalmcp.CallHandleMemoryIngest(ctx, t, pool, proj, dataDir2, dataDir2)
	assert.Equal(t, 1, second.Ingested, "second ingest: expected 1 new memory (Gamma)")
	assert.Equal(t, 1, second.Skipped, "second ingest: expected 1 skipped (Beta duplicate)")
	assert.Len(t, second.IDs, 1, "second ingest: expected exactly 1 new ID")

	// Confirm exactly 3 memories in project: Alpha, Beta, Gamma
	h, err := pool.Get(ctx, proj)
	require.NoError(t, err)
	ids, err := h.Engine.Backend().GetAllMemoryIDs(ctx, proj)
	require.NoError(t, err)
	assert.Len(t, ids, 3, "project must contain exactly 3 memories after partial-overlap ingests")
}
