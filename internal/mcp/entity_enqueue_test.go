package mcp

// Tests for extractResultID and enqueueExtractionAsync.
//
// extractResultID is unexported so these tests live in package mcp (same
// package, no _test suffix in the declaration). This follows the convention
// established by safety_test.go and fetch_recall_test.go in this directory.

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	mcpgo "github.com/mark3labs/mcp-go/mcp"
	"github.com/petersimmons1972/engram/internal/db"
	"github.com/petersimmons1972/engram/internal/entity"
	"github.com/stretchr/testify/require"
)

// ── helpers ───────────────────────────────────────────────────────────────────

// textResult builds a CallToolResult whose first Content element is
// TextContent with the given JSON payload.
func textResult(t *testing.T, payload map[string]any) *mcpgo.CallToolResult {
	t.Helper()
	b, err := json.Marshal(payload)
	require.NoError(t, err)
	return mcpgo.NewToolResultText(string(b))
}

// ── extractResultID unit tests ────────────────────────────────────────────────

func TestExtractResultID_NilResult(t *testing.T) {
	id, ok := extractResultID(nil)
	require.False(t, ok, "nil result must return false")
	require.Empty(t, id)
}

func TestExtractResultID_EmptyContent(t *testing.T) {
	result := &mcpgo.CallToolResult{}
	id, ok := extractResultID(result)
	require.False(t, ok, "empty Content must return false")
	require.Empty(t, id)
}

func TestExtractResultID_ContentNotTextContent(t *testing.T) {
	// ImageContent satisfies the Content interface but is not TextContent.
	result := &mcpgo.CallToolResult{
		Content: []mcpgo.Content{
			mcpgo.ImageContent{Type: "image", Data: "abc", MIMEType: "image/png"},
		},
	}
	id, ok := extractResultID(result)
	require.False(t, ok, "non-TextContent must return false")
	require.Empty(t, id)
}

func TestExtractResultID_InvalidJSON(t *testing.T) {
	result := &mcpgo.CallToolResult{
		Content: []mcpgo.Content{
			mcpgo.TextContent{Type: "text", Text: "not-json"},
		},
	}
	id, ok := extractResultID(result)
	require.False(t, ok, "invalid JSON must return false")
	require.Empty(t, id)
}

func TestExtractResultID_NoIDField(t *testing.T) {
	result := textResult(t, map[string]any{"status": "ok"})
	id, ok := extractResultID(result)
	require.False(t, ok, "JSON without id field must return false")
	require.Empty(t, id)
}

func TestExtractResultID_EmptyStringID(t *testing.T) {
	result := textResult(t, map[string]any{"id": ""})
	id, ok := extractResultID(result)
	require.False(t, ok, `id="" must return false`)
	require.Empty(t, id)
}

func TestExtractResultID_NonStringID(t *testing.T) {
	// id is a number — type assertion to string should fail.
	result := textResult(t, map[string]any{"id": 42})
	id, ok := extractResultID(result)
	require.False(t, ok, "non-string id must return false")
	require.Empty(t, id)
}

func TestExtractResultID_HappyPath(t *testing.T) {
	const want = "mem_abc123"
	result := textResult(t, map[string]any{"id": want, "status": "stored"})
	id, ok := extractResultID(result)
	require.True(t, ok, "valid id must return true")
	require.Equal(t, want, id)
}

// ── enqueueExtractionAsync tests ─────────────────────────────────────────────

// errBackend is a noopBackend override that makes EnqueueExtractionJob return
// an error, so we can verify the handler proceeds without surfacing it.
type errBackend struct{ noopBackend }

func (errBackend) EnqueueExtractionJob(_ context.Context, _, _ string) error {
	return errors.New("enqueue failed: simulated error")
}

var _ db.Backend = errBackend{}

// errEntityBackend also satisfies the entity UpsertEntity signature (unchanged
// from noopBackend — zero values).
func (errBackend) UpsertEntity(_ context.Context, _ *entity.Entity) (string, error) {
	return "", nil
}

// TestEnqueueExtractionAsync_PoolGetFailure verifies that a pool.Get error is
// swallowed: the function logs a warning and returns without panicking.
func TestEnqueueExtractionAsync_PoolGetFailure(t *testing.T) {
	failPool := NewEnginePool(func(_ context.Context, _ string) (*EngineHandle, error) {
		return nil, errors.New("pool: backend unavailable")
	})
	// Must not panic; failure is logged and swallowed.
	enqueueExtractionAsync(failPool, "mem_test99", "test-project")
}

// TestEnqueueExtractionAsync_EnqueueJobError verifies that an EnqueueExtractionJob
// error is also swallowed: the function logs a warning and returns without panicking.
func TestEnqueueExtractionAsync_EnqueueJobError(t *testing.T) {
	pool := newPoolWithBackend(t, errBackend{})
	// Must not panic; EnqueueExtractionJob error is logged and swallowed.
	enqueueExtractionAsync(pool, "mem_abc", "test-project")
}

// TestEnqueueExtractionAsync_HappyPath verifies that the function completes
// without error when the pool and backend both succeed.
func TestEnqueueExtractionAsync_HappyPath(t *testing.T) {
	pool := newTestNoopPool(t)
	// noopBackend.EnqueueExtractionJob returns nil — must not panic.
	enqueueExtractionAsync(pool, "mem_happy", "test-project")
}
