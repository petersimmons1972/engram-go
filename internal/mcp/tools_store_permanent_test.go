package mcp

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	mcpgo "github.com/mark3labs/mcp-go/mcp"
	"github.com/petersimmons1972/engram/internal/db"
	"github.com/petersimmons1972/engram/internal/embed"
	"github.com/petersimmons1972/engram/internal/search"
	"github.com/stretchr/testify/require"
)

// permanentErrorEmbedder returns a PermanentError for any Embed call.
type permanentErrorEmbedder struct {
	code        string
	stored      string
	current     string
	remediation string
}

func (e permanentErrorEmbedder) Embed(_ context.Context, _ string) ([]float32, error) {
	return nil, &embed.PermanentError{
		Code:        e.code,
		Stored:      e.stored,
		Current:     e.current,
		Remediation: e.remediation,
	}
}

func (e permanentErrorEmbedder) Name() string    { return e.current }
func (e permanentErrorEmbedder) Dimensions() int { return 384 }

var _ embed.Client = permanentErrorEmbedder{}

// permanentErrorBackend is a noopBackend that returns a PermanentError from GetMeta
// (simulating the embedder mismatch detection in checkEmbedderMeta).
type permanentErrorBackend struct {
	noopBackend
	code        string
	stored      string
	current     string
	remediation string
}

func (b permanentErrorBackend) GetMeta(ctx context.Context, project, key string) (string, bool, error) {
	if key == "embedder_name" {
		// Simulate a stored embedder name that differs from current.
		return b.stored, true, nil
	}
	return "", false, nil
}

var _ db.Backend = permanentErrorBackend{}

// newPermanentErrorPool returns an EnginePool whose engine.Store calls will fail
// with a PermanentError on embedder mismatch.
func newPermanentErrorPool(t *testing.T, code, stored, current, remediation string) *EnginePool {
	t.Helper()
	factory := func(ctx context.Context, project string) (*EngineHandle, error) {
		backend := permanentErrorBackend{
			code:        code,
			stored:      stored,
			current:     current,
			remediation: remediation,
		}
		embedder := permanentErrorEmbedder{
			code:        code,
			stored:      stored,
			current:     current,
			remediation: remediation,
		}
		engine := search.New(ctx, backend, embedder, project,
			"http://ollama-test:11434", "", false, nil, 0)
		t.Cleanup(engine.Close)
		return &EngineHandle{Engine: engine}, nil
	}
	return NewEnginePool(factory)
}

// ── TestHandleMemoryStore_PermanentError_FastFail ──────────────────────────────

// TestHandleMemoryStore_PermanentError_FastFail verifies that handleMemoryStore
// detects an embed.PermanentError during checkEmbedderMeta and returns it as a
// structured JSON error response (IsError=true, body contains code/stored/current/remediation)
// without propagating it as a Go error.
func TestHandleMemoryStore_PermanentError_FastFail(t *testing.T) {
	pool := newPermanentErrorPool(t, "embedder_mismatch", "old-model", "new-model", "run memory_migrate_embedder")

	req := mcpgo.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"project":     "test",
		"content":     "nanosecond wire — one billionth of a second",
		"memory_type": "context",
	}

	start := time.Now()
	result, err := handleMemoryStore(context.Background(), pool, req, testConfig())
	elapsed := time.Since(start)

	// No Go error should propagate.
	require.NoError(t, err, "handleMemoryStore must not return a Go error for PermanentError")

	// Result must be present and marked as error.
	require.NotNil(t, result, "handleMemoryStore must return a CallToolResult even for PermanentError")
	require.True(t, result.IsError, "CallToolResult.IsError must be true for PermanentError")

	// The error content must be valid JSON with the four required fields.
	var payload map[string]string
	textContent, ok := result.Content[0].(mcpgo.TextContent)
	require.True(t, ok, "Content[0] must be TextContent")
	err = json.Unmarshal([]byte(textContent.Text), &payload)
	require.NoError(t, err, "error content must be valid JSON")

	require.Equal(t, "embedder_mismatch", payload["code"])
	require.Equal(t, "old-model", payload["stored"])
	require.Equal(t, "new-model", payload["current"])
	require.Equal(t, "run memory_migrate_embedder", payload["remediation"])

	// Fast-fail should complete in well under 200ms.
	require.Less(t, elapsed, 200*time.Millisecond,
		"PermanentError fast-fail must complete in < 200ms; got %s", elapsed)
}

// ── TestHandleMemoryQuickStore_PermanentError_FastFail ───────────────────────

// TestHandleMemoryQuickStore_PermanentError_FastFail verifies that
// handleMemoryQuickStore (which delegates to handleMemoryStore) also properly
// detects and fast-fails on embed.PermanentError.
func TestHandleMemoryQuickStore_PermanentError_FastFail(t *testing.T) {
	pool := newPermanentErrorPool(t, "embedder_mismatch", "old-model", "new-model", "run memory_migrate_embedder")

	req := mcpgo.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"project": "test",
		"content": "understanding the mechanism, not reassembling all seven clocks",
	}

	start := time.Now()
	result, err := handleMemoryQuickStore(context.Background(), pool, req, testConfig())
	elapsed := time.Since(start)

	require.NoError(t, err, "handleMemoryQuickStore must not return a Go error for PermanentError")
	require.NotNil(t, result)
	require.True(t, result.IsError)

	var payload map[string]string
	textContent, ok := result.Content[0].(mcpgo.TextContent)
	require.True(t, ok, "Content[0] must be TextContent")
	err = json.Unmarshal([]byte(textContent.Text), &payload)
	require.NoError(t, err)

	require.Equal(t, "embedder_mismatch", payload["code"])
	require.Equal(t, "old-model", payload["stored"])
	require.Equal(t, "new-model", payload["current"])
	require.Equal(t, "run memory_migrate_embedder", payload["remediation"])

	require.Less(t, elapsed, 200*time.Millisecond)
}

// ── TestHandleMemoryStoreBatch_PermanentError_FastFail ──────────────────────

// TestHandleMemoryStoreBatch_PermanentError_FastFail verifies that
// handleMemoryStoreBatch also detects PermanentError during checkEmbedderMeta
// and fast-fails with structured JSON.
func TestHandleMemoryStoreBatch_PermanentError_FastFail(t *testing.T) {
	pool := newPermanentErrorPool(t, "embedder_mismatch", "old-model", "new-model", "run memory_migrate_embedder")

	req := mcpgo.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"project": "test",
		"memories": []any{
			map[string]any{
				"content":     "memory 1",
				"memory_type": "context",
			},
			map[string]any{
				"content":     "memory 2",
				"memory_type": "error",
			},
		},
	}

	start := time.Now()
	result, err := handleMemoryStoreBatch(context.Background(), pool, req, testConfig())
	elapsed := time.Since(start)

	require.NoError(t, err, "handleMemoryStoreBatch must not return a Go error for PermanentError")
	require.NotNil(t, result)
	require.True(t, result.IsError)

	var payload map[string]string
	textContent, ok := result.Content[0].(mcpgo.TextContent)
	require.True(t, ok, "Content[0] must be TextContent")
	err = json.Unmarshal([]byte(textContent.Text), &payload)
	require.NoError(t, err)

	require.Equal(t, "embedder_mismatch", payload["code"])
	require.Equal(t, "old-model", payload["stored"])
	require.Equal(t, "new-model", payload["current"])

	require.Less(t, elapsed, 200*time.Millisecond)
}
