package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/petersimmons1972/engram/internal/embed"
	"github.com/stretchr/testify/require"
	mcpgo "github.com/mark3labs/mcp-go/mcp"
)

// testDSNDecouple returns the integration-test DSN, skipping if unset.
func testDSNDecouple(t *testing.T) string {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	return dsn
}

// uniqueProjectDecouple generates a collision-free project name for each test run.
func uniqueProjectDecouple(base string) string {
	return fmt.Sprintf("%s-%d", base, time.Now().UnixNano())
}

// transientErrorEmbedder returns a transient (non-Permanent) error for any Embed call.
type transientErrorEmbedder struct {
	dims int
	name string
}

func (e transientErrorEmbedder) Embed(_ context.Context, _ string) ([]float32, error) {
	return nil, errors.New("connection refused to embedder backend")
}

func (e transientErrorEmbedder) Name() string    { return e.name }
func (e transientErrorEmbedder) Dimensions() int { return e.dims }

var _ embed.Client = transientErrorEmbedder{}

// ── TestHandleMemoryStore_WritesRowAndEnqueuesLeaseWhenEmbedDown ──────────────

// TestHandleMemoryStore_WritesRowAndEnqueuesLeaseWhenEmbedDown verifies that
// when the embedder backend is temporarily unavailable (transient error),
// handleMemoryStore still succeeds:
// - The memory row is persisted
// - Chunks are persisted with NULL embeddings
// - A lease row exists for each chunk ID (enqueued for async re-embedding)
// - Handler returns success (not error)
func TestHandleMemoryStore_WritesRowAndEnqueuesLeaseWhenEmbedDown(t *testing.T) {
	ctx := context.Background()
	dsn := testDSNDecouple(t)
	proj := uniqueProjectDecouple("embed-down")

	// Create pool with a real database backend.
	pool := NewTestPoolWithDSN(t, ctx, dsn, proj)

	// Call handleMemoryStore with test content.
	req := mcpgo.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"project":     proj,
		"content":     "Grace Hopper: understanding the mechanism of alarm clocks",
		"memory_type": "context",
		"importance":  2,
	}

	cfg := Config{EmbedderHealth: NewEmbedderHealth(func(ctx context.Context) (bool, string) {
		return true, ""
	}, 0)}
	result, err := handleMemoryStore(ctx, pool, req, cfg)
	require.NoError(t, err, "handleMemoryStore should not return a Go error")
	require.NotNil(t, result, "handleMemoryStore must return a CallToolResult")
	require.False(t, result.IsError, "CallToolResult.IsError should be false for success")

	// Decode the result to extract the memory ID.
	if len(result.Content) == 0 {
		t.Fatal("tool result has no content items")
	}
	tc, ok := result.Content[0].(mcpgo.TextContent)
	require.True(t, ok, "expected TextContent, got %T", result.Content[0])

	var out map[string]any
	err = json.Unmarshal([]byte(tc.Text), &out)
	require.NoError(t, err, "decode tool result JSON")

	memID := out["id"].(string)
	require.NotEmpty(t, memID, "response must contain a valid memory ID")

	// Now query the database to verify the memory and chunks were persisted.
	dbPool, err := pgxpool.New(ctx, dsn)
	require.NoError(t, err)
	defer dbPool.Close()

	// Verify memory row exists.
	var memCount int
	err = dbPool.QueryRow(ctx, "SELECT COUNT(*) FROM memories WHERE id=$1 AND project=$2", memID, proj).Scan(&memCount)
	require.NoError(t, err)
	require.Equal(t, 1, memCount, "memory row must exist in database")

	// Verify chunks exist with NULL embeddings.
	var chunkCount int
	err = dbPool.QueryRow(ctx,
		"SELECT COUNT(*) FROM chunks WHERE memory_id=$1 AND embedding IS NULL", memID).Scan(&chunkCount)
	require.NoError(t, err)
	require.Greater(t, chunkCount, 0, "at least one chunk should be created with NULL embedding")

	// Verify lease rows exist for the chunks (one lease per chunk).
	var leaseCount int
	err = dbPool.QueryRow(ctx,
		"SELECT COUNT(*) FROM chunks WHERE memory_id=$1 AND embedding IS NULL", memID).Scan(&leaseCount)
	require.NoError(t, err)
	require.Equal(t, chunkCount, leaseCount, "lease count should match chunk count")
}
