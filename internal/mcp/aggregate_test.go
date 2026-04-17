// aggregate_test.go — RED integration tests for memory_aggregate handler and
// the extended memory_feedback handler (failure_class param).
//
// These tests reference export stubs that do not yet exist in export_test.go
// (CallHandleMemoryAggregate, CallHandleMemoryAggregateExpectError,
// CallHandleMemoryFeedbackWithClass, CallHandleMemoryFeedbackWithClassExpectError).
// They will NOT compile until Step 12 adds those stubs. That is intentional:
// this file establishes the RED state for Step 4 of the aggregate-lane plan.
package mcp_test

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/petersimmons1972/engram/internal/mcp"
	"github.com/stretchr/testify/require"
)

// testDSN_agg returns the integration-test DSN, skipping if unset.
func testDSN_agg(t *testing.T) string {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	return dsn
}

// uniqueProject_agg generates a collision-free project name for each test run.
func uniqueProject_agg(base string) string {
	return fmt.Sprintf("%s-%d", base, time.Now().UnixNano())
}

// ── memory_aggregate tests ────────────────────────────────────────────────────

// TestHandleMemoryAggregate_ByTag calls memory_aggregate with by="tag" on a
// fresh project. The response must contain "by"=="tag" and a "rows" slice
// (may be empty, but must be present and be a []any).
func TestHandleMemoryAggregate_ByTag(t *testing.T) {
	ctx := context.Background()
	dsn := testDSN_agg(t)
	proj := uniqueProject_agg("agg-tag")
	pool := mcp.NewTestPoolWithDSN(t, ctx, dsn, proj)

	out := mcp.CallHandleMemoryAggregate(ctx, t, pool, map[string]any{
		"project": proj,
		"by":      "tag",
	})

	require.Equal(t, "tag", out["by"], "response 'by' must echo the requested dimension")

	rows, ok := out["rows"]
	require.True(t, ok, "response must include 'rows' key")
	_, isSlice := rows.([]any)
	require.True(t, isSlice, "'rows' must be a []any, got %T", rows)
}

// TestHandleMemoryAggregate_ByType calls memory_aggregate with by="type".
// Same structural requirements as the tag test.
func TestHandleMemoryAggregate_ByType(t *testing.T) {
	ctx := context.Background()
	dsn := testDSN_agg(t)
	proj := uniqueProject_agg("agg-type")
	pool := mcp.NewTestPoolWithDSN(t, ctx, dsn, proj)

	out := mcp.CallHandleMemoryAggregate(ctx, t, pool, map[string]any{
		"project": proj,
		"by":      "type",
	})

	require.Equal(t, "type", out["by"])

	rows, ok := out["rows"]
	require.True(t, ok, "response must include 'rows' key")
	_, isSlice := rows.([]any)
	require.True(t, isSlice, "'rows' must be a []any, got %T", rows)
}

// TestHandleMemoryAggregate_ByFailureClass calls memory_aggregate with
// by="failure_class". Response must contain "by"=="failure_class".
func TestHandleMemoryAggregate_ByFailureClass(t *testing.T) {
	ctx := context.Background()
	dsn := testDSN_agg(t)
	proj := uniqueProject_agg("agg-fc")
	pool := mcp.NewTestPoolWithDSN(t, ctx, dsn, proj)

	out := mcp.CallHandleMemoryAggregate(ctx, t, pool, map[string]any{
		"project": proj,
		"by":      "failure_class",
	})

	require.Equal(t, "failure_class", out["by"])
}

// TestHandleMemoryAggregate_EmptyProject calls memory_aggregate against a
// project that has no memories at all. The response must have "rows" as an
// empty (non-nil) slice — not a missing key and not null.
func TestHandleMemoryAggregate_EmptyProject(t *testing.T) {
	ctx := context.Background()
	dsn := testDSN_agg(t)
	proj := uniqueProject_agg("agg-empty")
	pool := mcp.NewTestPoolWithDSN(t, ctx, dsn, proj)

	out := mcp.CallHandleMemoryAggregate(ctx, t, pool, map[string]any{
		"project": proj,
		"by":      "tag",
	})

	rows, ok := out["rows"]
	require.True(t, ok, "response must include 'rows' key even for an empty project")
	require.NotNil(t, rows, "'rows' must not be nil for an empty project")

	slice, isSlice := rows.([]any)
	require.True(t, isSlice, "'rows' must be a []any, got %T", rows)
	require.Empty(t, slice, "'rows' must be an empty slice for a project with no memories")
}

// TestHandleMemoryAggregate_InvalidBy calls memory_aggregate with an
// unrecognised by= value. The handler must return an error.
func TestHandleMemoryAggregate_InvalidBy(t *testing.T) {
	ctx := context.Background()
	dsn := testDSN_agg(t)
	proj := uniqueProject_agg("agg-invalid")
	pool := mcp.NewTestPoolWithDSN(t, ctx, dsn, proj)

	mcp.CallHandleMemoryAggregateExpectError(ctx, t, pool, map[string]any{
		"project": proj,
		"by":      "bogus",
	})
}

// TestHandleMemoryAggregate_LimitClamping verifies that passing limit=-1 does not
// produce an error at the MCP handler boundary. The handler silently clamps any
// limit < 1 to 20 before forwarding to the engine, so the call must succeed and
// return a valid aggregate response with a "rows" key.
func TestHandleMemoryAggregate_LimitClamping(t *testing.T) {
	ctx := context.Background()
	// Use the noopBackend-backed pool so this test runs without a real database.
	pool := mcp.NewTestNoopPool(t)

	out := mcp.CallHandleMemoryAggregate(ctx, t, pool, map[string]any{
		"project": "test",
		"by":      "tag",
		"limit":   float64(-1),
	})

	require.NotNil(t, out, "response must not be nil")

	rows, ok := out["rows"]
	require.True(t, ok, "response must include 'rows' key")
	_, isSlice := rows.([]any)
	require.True(t, isSlice, "'rows' must be a []any, got %T", rows)
}

// ── memory_feedback with failure_class tests ──────────────────────────────────

// TestHandleMemoryFeedback_WithClass calls memory_feedback with a valid
// failure_class="vocabulary_mismatch". A fake event_id is used; the handler
// may return an error about the event not being found (acceptable), but it
// must NOT return an error about an invalid failure_class.
func TestHandleMemoryFeedback_WithClass(t *testing.T) {
	ctx := context.Background()
	dsn := testDSN_agg(t)
	proj := uniqueProject_agg("fb-class")
	pool := mcp.NewTestPoolWithDSN(t, ctx, dsn, proj)

	// We use a fake event_id. The valid-class path must not reject the class
	// itself; any error must be about the missing event, not about the class value.
	out := mcp.CallHandleMemoryFeedbackWithClass(ctx, t, pool, map[string]any{
		"project":       proj,
		"event_id":      "fake-event-123",
		"memory_ids":    []any{},
		"failure_class": "vocabulary_mismatch",
	})

	// If we reach here without a fatal, the handler accepted the class.
	// The status field must be present (success path) or we just confirmed
	// no class-validation error was raised.
	_ = out
}

// TestHandleMemoryFeedback_InvalidClass calls memory_feedback with an
// unrecognised failure_class value. The handler must reject it at the MCP
// boundary — before any DB access — and return an error.
func TestHandleMemoryFeedback_InvalidClass(t *testing.T) {
	ctx := context.Background()
	dsn := testDSN_agg(t)
	proj := uniqueProject_agg("fb-badclass")
	pool := mcp.NewTestPoolWithDSN(t, ctx, dsn, proj)

	mcp.CallHandleMemoryFeedbackWithClassExpectError(ctx, t, pool, map[string]any{
		"project":       proj,
		"event_id":      "fake-event-123",
		"memory_ids":    []any{},
		"failure_class": "not_a_valid_class",
	})
}
