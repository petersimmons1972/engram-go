package mcp

// Tests for the pattern_confidence MCP arg surface on memory_store and memory_correct.
// See Track E1 of the instinct migration campaign for context.
//
// Test list (from campaign brief):
//   - TestMemoryStorePatternConfidence
//   - TestMemoryStorePatternConfidenceValidationError
//   - TestMemoryStorePatternConfidenceOmitted
//   - TestMemoryCorrectPatternConfidence
//   - TestMemoryCorrectPatternConfidenceOmitted
//   - TestImportanceFieldUnchangedBehavior
//
// This file is in the same package as simple_tools_test.go, so it reuses the
// capturingBackend and newCapturingPool helpers defined there.

import (
	"context"
	"testing"

	mcpgo "github.com/mark3labs/mcp-go/mcp"
	"github.com/petersimmons1972/engram/internal/db"
	"github.com/petersimmons1972/engram/internal/search"
	"github.com/petersimmons1972/engram/internal/types"
	"github.com/stretchr/testify/require"
)

// ── correctCapturingBackend ───────────────────────────────────────────────────

// pcCorrectBackend embeds noopBackend, overrides UpdateMemory to record the
// patternConfidence and importance arguments, and returns a valid memory so the
// handler doesn't short-circuit on nil.
type pcCorrectBackend struct {
	noopBackend
	capturedPC     *float64 // last patternConfidence passed to UpdateMemory
	capturedPCSet  bool     // true even when capturedPC is nil (nil is valid "don't touch")
	capturedImport *int     // last importance passed to UpdateMemory
}

func (b *pcCorrectBackend) UpdateMemory(_ context.Context, id string, _ *string, _ []string, imp *int, pc *float64) (*types.Memory, error) {
	b.capturedPCSet = true
	b.capturedPC = pc
	b.capturedImport = imp
	return &types.Memory{ID: id, Content: "updated", Project: "test"}, nil
}

func (b *pcCorrectBackend) Begin(_ context.Context) (db.Tx, error) { return noopTx{}, nil }

func newPCCorrectPool(t *testing.T, backend *pcCorrectBackend) *EnginePool {
	t.Helper()
	factory := func(ctx context.Context, project string) (*EngineHandle, error) {
		engine := search.New(ctx, backend, noopEmbedder{}, project,
			"http://ollama-test:11434", "", false, nil, 0)
		t.Cleanup(engine.Close)
		return &EngineHandle{Engine: engine}, nil
	}
	return NewEnginePool(factory)
}

// ── memory_store tests ────────────────────────────────────────────────────────

// TestMemoryStorePatternConfidence calls memory_store with a valid
// pattern_confidence arg and asserts the stored memory carries the value.
func TestMemoryStorePatternConfidence(t *testing.T) {
	pool, cap := newCapturingPool(t)

	req := mcpgo.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"content":            "a pattern with known confidence",
		"memory_type":        "pattern",
		"project":            "test",
		"pattern_confidence": float64(0.75),
	}

	res, err := handleMemoryStore(context.Background(), pool, req, testConfig())
	require.NoError(t, err)
	require.NotNil(t, res)
	require.False(t, res.IsError, "expected non-error result, got: %+v", res.Content)

	require.Len(t, cap.stored, 1, "exactly one memory must be stored")
	m := cap.stored[0]
	require.NotNil(t, m.PatternConfidence, "PatternConfidence must be set on the stored memory")
	require.InDelta(t, 0.75, *m.PatternConfidence, 1e-9, "PatternConfidence must match the arg")
}

// TestMemoryStorePatternConfidenceValidationError passes pattern_confidence=-0.5
// to memory_store and asserts a tool-error response (not a Go error).
func TestMemoryStorePatternConfidenceValidationError(t *testing.T) {
	pool := newStorePool(t)

	req := mcpgo.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"content":            "confidence out of range",
		"memory_type":        "pattern",
		"project":            "test",
		"pattern_confidence": float64(-0.5),
	}

	res, err := handleMemoryStore(context.Background(), pool, req, testConfig())
	require.NoError(t, err, "validation error must be returned as a tool error, not a Go error")
	require.NotNil(t, res)
	require.True(t, res.IsError, "expected tool-error result for out-of-range pattern_confidence")
}

// TestMemoryStorePatternConfidenceOmitted calls memory_store without the
// pattern_confidence arg and asserts no error, stored memory has nil confidence.
func TestMemoryStorePatternConfidenceOmitted(t *testing.T) {
	pool, cap := newCapturingPool(t)

	req := mcpgo.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"content":     "no confidence provided",
		"memory_type": "context",
		"project":     "test",
	}

	res, err := handleMemoryStore(context.Background(), pool, req, testConfig())
	require.NoError(t, err)
	require.NotNil(t, res)
	require.False(t, res.IsError)

	require.NotEmpty(t, cap.stored, "memory must be stored")
	// PatternConfidence must be nil when the arg is absent — stored as NULL in DB.
	require.Nil(t, cap.stored[0].PatternConfidence,
		"PatternConfidence must be nil when arg is omitted — must not default to 0.0")
}

// ── memory_correct tests ──────────────────────────────────────────────────────

// TestMemoryCorrectPatternConfidence calls memory_correct with a valid
// pattern_confidence arg and asserts UpdateMemory receives the value.
func TestMemoryCorrectPatternConfidence(t *testing.T) {
	backend := &pcCorrectBackend{}
	pool := newPCCorrectPool(t, backend)

	req := mcpgo.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"memory_id":          types.NewMemoryID(),
		"project":            "test",
		"pattern_confidence": float64(0.88),
	}

	_, err := handleMemoryCorrect(context.Background(), pool, req)
	require.NoError(t, err)
	require.True(t, backend.capturedPCSet, "UpdateMemory must be called")
	require.NotNil(t, backend.capturedPC, "patternConfidence must be non-nil for a present arg")
	require.InDelta(t, 0.88, *backend.capturedPC, 1e-9)
}

// TestMemoryCorrectPatternConfidenceOmitted calls memory_correct without
// pattern_confidence and asserts UpdateMemory receives nil for that field
// (meaning "do not touch this field").
func TestMemoryCorrectPatternConfidenceOmitted(t *testing.T) {
	backend := &pcCorrectBackend{}
	pool := newPCCorrectPool(t, backend)

	req := mcpgo.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"memory_id": types.NewMemoryID(),
		"project":   "test",
		"content":   "updated content only",
	}

	_, err := handleMemoryCorrect(context.Background(), pool, req)
	require.NoError(t, err)
	require.True(t, backend.capturedPCSet, "UpdateMemory must be called")
	require.Nil(t, backend.capturedPC,
		"patternConfidence must be nil when arg is absent — nil means 'do not touch'")
}

// TestImportanceFieldUnchangedBehavior is a regression test documenting that
// passing a float to importance still gets int-truncated as before (via int(v)).
// Track E1 did NOT fix the importance field — only added the new
// pattern_confidence field for the use case that needed float precision.
func TestImportanceFieldUnchangedBehavior(t *testing.T) {
	backend := &pcCorrectBackend{}
	pool := newPCCorrectPool(t, backend)

	req := mcpgo.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"memory_id":  types.NewMemoryID(),
		"project":    "test",
		"importance": float64(2.9), // float 2.9 → truncated to int 2 by int(v) cast
	}

	_, err := handleMemoryCorrect(context.Background(), pool, req)
	require.NoError(t, err)
	require.NotNil(t, backend.capturedImport, "importance must be set when arg is present")
	// int(2.9) = 2; this is the documented truncation behavior we are NOT fixing.
	require.Equal(t, 2, *backend.capturedImport,
		"importance float→int truncation must remain unchanged — E1 intentionally leaves this as-is")
}
