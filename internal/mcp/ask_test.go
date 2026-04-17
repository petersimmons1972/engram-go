package mcp_test

import (
	"context"
	"testing"

	"github.com/petersimmons1972/engram/internal/mcp"
)

// TestHandleMemoryAsk_MissingQuestion verifies that the handler rejects a
// request that contains no "question" key. Input validation must fire before
// any engine or Claude access.
func TestHandleMemoryAsk_MissingQuestion(t *testing.T) {
	ctx := context.Background()
	pool := mcp.NewTestNoopPool(t)

	// No "question" key — must produce an error.
	mcp.CallHandleMemoryAskExpectError(ctx, t, pool, map[string]any{
		"project": "test-project",
	})
}

// TestHandleMemoryAsk_EmptyProject verifies that the handler rejects a request
// where "project" is absent or empty. A question without a project scope
// cannot be routed to the right engine.
func TestHandleMemoryAsk_EmptyProject(t *testing.T) {
	ctx := context.Background()
	pool := mcp.NewTestNoopPool(t)

	// "question" present but no "project" — must produce an error.
	mcp.CallHandleMemoryAskExpectError(ctx, t, pool, map[string]any{
		"question": "what is X",
	})

	// "project" key present but empty string — also must produce an error.
	mcp.CallHandleMemoryAskExpectError(ctx, t, pool, map[string]any{
		"question": "what is X",
		"project":  "",
	})
}

// TestHandleMemoryAsk_NoClaudeClient verifies that when no Claude client is
// configured (the default in unit-test scope), the handler returns a tool-level
// error rather than a success result or a panic.
func TestHandleMemoryAsk_NoClaudeClient(t *testing.T) {
	ctx := context.Background()
	pool := mcp.NewTestNoopPool(t)

	// Config passed by CallHandleMemoryAsk has no claudeClient, so the handler
	// must return a tool-level error (IsError=true). CallHandleMemoryAsk maps
	// that to (nil, nil) — out==nil is the expected outcome.
	out, err := mcp.CallHandleMemoryAsk(ctx, t, pool, map[string]any{
		"question": "What did I learn about nanoseconds?",
		"project":  "test-project",
	})

	if err != nil {
		t.Fatalf("expected tool-level error, got Go error: %v", err)
	}
	if out != nil {
		t.Fatalf("expected tool-level error result (out==nil), got success result: %v", out)
	}
	// out==nil + err==nil confirms the handler returned IsError=true, which is
	// the correct response when no Claude client is configured.
}
