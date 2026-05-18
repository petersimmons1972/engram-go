package mcp

// Tests for pattern_confidence handling in handleMemoryStoreBatch.
// Track E1-FIX — Blocker 1: per-item pattern_confidence was silently dropped.
//
// These tests reuse capturingBackend / newCapturingPool from simple_tools_test.go
// and the mcpgo.TextContent extraction pattern from integration_test_helpers.go.

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	mcpgo "github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/require"
)

// parseBatchBody decodes the first TextContent item from a batch result into a
// map so tests can inspect "errors", "ids", etc.
func parseBatchBody(t *testing.T, res *mcpgo.CallToolResult) map[string]any {
	t.Helper()
	if len(res.Content) == 0 {
		return nil
	}
	tc, ok := res.Content[0].(mcpgo.TextContent)
	if !ok {
		return nil
	}
	var out map[string]any
	if err := json.Unmarshal([]byte(tc.Text), &out); err != nil {
		return nil
	}
	return out
}

// TestBatchStorePatternConfidenceHappyPath stores 3 items with different
// pattern_confidence values and asserts each memory gets its correct value.
func TestBatchStorePatternConfidenceHappyPath(t *testing.T) {
	pool, cap := newCapturingPool(t)

	req := mcpgo.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"project": "test",
		"memories": []any{
			map[string]any{
				"content":            "pattern with 0.0 confidence",
				"memory_type":        "pattern",
				"pattern_confidence": float64(0.0),
			},
			map[string]any{
				"content":            "pattern with 0.5 confidence",
				"memory_type":        "pattern",
				"pattern_confidence": float64(0.5),
			},
			map[string]any{
				"content":            "pattern with 1.0 confidence",
				"memory_type":        "pattern",
				"pattern_confidence": float64(1.0),
			},
		},
	}

	res, err := handleMemoryStoreBatch(context.Background(), pool, req, testConfig())
	require.NoError(t, err)
	require.NotNil(t, res)
	require.False(t, res.IsError, "expected non-error result, got: %+v", res.Content)

	require.Len(t, cap.stored, 3, "exactly 3 memories must be stored")

	require.NotNil(t, cap.stored[0].PatternConfidence, "item 0 PatternConfidence must be set")
	require.InDelta(t, 0.0, *cap.stored[0].PatternConfidence, 1e-9, "item 0 confidence must be 0.0")

	require.NotNil(t, cap.stored[1].PatternConfidence, "item 1 PatternConfidence must be set")
	require.InDelta(t, 0.5, *cap.stored[1].PatternConfidence, 1e-9, "item 1 confidence must be 0.5")

	require.NotNil(t, cap.stored[2].PatternConfidence, "item 2 PatternConfidence must be set")
	require.InDelta(t, 1.0, *cap.stored[2].PatternConfidence, 1e-9, "item 2 confidence must be 1.0")
}

// TestBatchStorePatternConfidenceMixedAbsentPresent stores 3 items — 1 with
// pattern_confidence, 2 without — and asserts no cross-contamination.
func TestBatchStorePatternConfidenceMixedAbsentPresent(t *testing.T) {
	pool, cap := newCapturingPool(t)

	req := mcpgo.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"project": "test",
		"memories": []any{
			map[string]any{
				"content":     "no confidence item A",
				"memory_type": "context",
			},
			map[string]any{
				"content":            "has confidence",
				"memory_type":        "pattern",
				"pattern_confidence": float64(0.77),
			},
			map[string]any{
				"content":     "no confidence item B",
				"memory_type": "context",
			},
		},
	}

	res, err := handleMemoryStoreBatch(context.Background(), pool, req, testConfig())
	require.NoError(t, err)
	require.NotNil(t, res)
	require.False(t, res.IsError, "expected non-error result, got: %+v", res.Content)

	require.Len(t, cap.stored, 3, "exactly 3 memories must be stored")

	require.Nil(t, cap.stored[0].PatternConfidence,
		"item 0 (no arg) must have nil PatternConfidence — must not default to 0.0")
	require.NotNil(t, cap.stored[1].PatternConfidence, "item 1 PatternConfidence must be set")
	require.InDelta(t, 0.77, *cap.stored[1].PatternConfidence, 1e-9)
	require.Nil(t, cap.stored[2].PatternConfidence,
		"item 2 (no arg) must have nil PatternConfidence — no cross-contamination from item 1")
}

// TestBatchStorePatternConfidenceValidationError stores 3 items where item at
// index 1 has an out-of-range pattern_confidence of 1.5. The batch must fail,
// error must identify item index 1, and no memories must be written.
func TestBatchStorePatternConfidenceValidationError(t *testing.T) {
	pool, cap := newCapturingPool(t)

	req := mcpgo.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"project": "test",
		"memories": []any{
			map[string]any{
				"content":            "valid item 0",
				"memory_type":        "pattern",
				"pattern_confidence": float64(0.5),
			},
			map[string]any{
				"content":            "invalid item 1",
				"memory_type":        "pattern",
				"pattern_confidence": float64(1.5), // out of range
			},
			map[string]any{
				"content":     "valid item 2",
				"memory_type": "context",
			},
		},
	}

	res, err := handleMemoryStoreBatch(context.Background(), pool, req, testConfig())
	require.NoError(t, err, "validation error must be returned as tool result, not Go error")
	require.NotNil(t, res)

	// Batch result with validation errors uses the existing convention:
	// toolResult with "errors" key (non-nil, non-empty slice).
	body := parseBatchBody(t, res)
	require.NotNil(t, body, "result must have parseable JSON body")
	errs, hasErrors := body["errors"]
	require.True(t, res.IsError || hasErrors,
		"batch with invalid pattern_confidence must fail; got body: %v", body)

	if hasErrors {
		errSlice, ok := errs.([]any)
		require.True(t, ok, "errors must be a slice")
		combined := make([]string, 0, len(errSlice))
		for _, e := range errSlice {
			combined = append(combined, e.(string))
		}
		joined := strings.Join(combined, "\n")
		require.Contains(t, joined, "1",
			"error message must identify item index 1; got: %s", joined)
		require.Contains(t, strings.ToLower(joined), "pattern_confidence",
			"error message must name the offending field; got: %s", joined)
	}

	require.Empty(t, cap.stored, "no memories must be stored when batch validation fails")
}

// TestBatchStorePatternConfidenceWrongType sends a non-numeric pattern_confidence
// on one item and asserts the batch fails with a validation error.
func TestBatchStorePatternConfidenceWrongType(t *testing.T) {
	pool, cap := newCapturingPool(t)

	req := mcpgo.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"project": "test",
		"memories": []any{
			map[string]any{
				"content":            "bad type item",
				"memory_type":        "pattern",
				"pattern_confidence": "not-a-number", // wrong type: string instead of float64
			},
		},
	}

	res, err := handleMemoryStoreBatch(context.Background(), pool, req, testConfig())
	require.NoError(t, err, "validation error must be returned as tool result, not Go error")
	require.NotNil(t, res)

	// Batch result with validation errors uses the existing convention:
	// toolResult with "errors" key (non-nil, non-empty slice).
	body := parseBatchBody(t, res)
	require.NotNil(t, body, "result must have parseable JSON body")
	errs, hasErrors := body["errors"]
	require.True(t, res.IsError || hasErrors,
		"batch with wrong-type pattern_confidence must fail; got body: %v", body)

	if hasErrors {
		errSlice, ok := errs.([]any)
		require.True(t, ok, "errors must be a slice")
		combined := make([]string, 0, len(errSlice))
		for _, e := range errSlice {
			combined = append(combined, e.(string))
		}
		joined := strings.Join(combined, "\n")
		require.Contains(t, joined, "0",
			"error message must identify item index 0; got: %s", joined)
		require.Contains(t, strings.ToLower(joined), "pattern_confidence",
			"error message must name the offending field; got: %s", joined)
		require.Contains(t, strings.ToLower(joined), "number",
			"error message must indicate type mismatch; got: %s", joined)
	}

	require.Empty(t, cap.stored, "no memories must be stored when batch validation fails")
}
