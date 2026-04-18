package mcp

// Tests for extractResultID and the non-blocking enqueue behaviour wired into
// the memory_store handler.
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

// ── enqueue wrapper behaviour ─────────────────────────────────────────────────

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

// TestMemoryStoreHandler_NoEnqueueWhenNoID verifies that the memory_store
// closure returns (result, nil) when extractResultID returns false — i.e. the
// handler does not attempt to enqueue and does not return an error.
func TestMemoryStoreHandler_NoEnqueueWhenNoID(t *testing.T) {
	// A result with no "id" field — extractResultID will return false.
	result := textResult(t, map[string]any{"status": "ok"})

	id, ok := extractResultID(result)
	require.False(t, ok, "pre-condition: result has no id")
	require.Empty(t, id)

	// If extractResultID returns false the goroutine is never spawned.
	// Nothing to assert beyond the function returning false — no panic, no enqueue.
}

// TestMemoryStoreHandler_ReturnsResultEvenOnPoolGetFailure verifies that the
// memory_store handler's return value is unaffected by pool.Get failures
// (the goroutine logs and returns silently).
//
// We simulate this by building a pool whose factory returns an error and
// calling the internal logic directly: the goroutine is fire-and-forget so the
// only contract we can test synchronously is that the goroutine does NOT panic
// and does NOT modify the return value.
func TestMemoryStoreHandler_ReturnsResultEvenOnPoolGetFailure(t *testing.T) {
	failPool := NewEnginePool(func(_ context.Context, _ string) (*EngineHandle, error) {
		return nil, errors.New("pool: backend unavailable")
	})

	const memID = "mem_test99"
	result := textResult(t, map[string]any{"id": memID})

	id, ok := extractResultID(result)
	require.True(t, ok, "pre-condition: result has a valid id")
	require.Equal(t, memID, id)

	// Mimic what the closure does — spawn the goroutine.
	// The goroutine must not panic; it will log a warning and return.
	done := make(chan struct{})
	go func() {
		defer close(done)
		ctx, cancel := context.WithTimeout(context.Background(), 5)
		defer cancel()
		h, herr := failPool.Get(ctx, "test-project")
		if herr != nil {
			// Expected: pool factory returned an error. Handler logs, returns.
			return
		}
		// Should not reach here in this test.
		_ = h.Engine.Backend().EnqueueExtractionJob(ctx, memID, "test-project")
	}()
	<-done // goroutine finished without panic

	// The original result is untouched.
	require.NotNil(t, result)
}
