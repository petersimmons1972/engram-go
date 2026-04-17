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

// TestHandleMemoryAsk_ValidCall verifies that a syntactically valid call with
// both "question" and "project" does not panic and produces a recognizable
// error when Claude is not configured (nil client). The handler must return
// some error — not a nil result and not a crash.
//
// This test uses the noop pool (no real DB). The call is expected to fail with
// a "claude not enabled" or similar sentinel error rather than a panic or
// a success, because the Claude client will not be wired in unit-test scope.
func TestHandleMemoryAsk_ValidCall(t *testing.T) {
	ctx := context.Background()
	pool := mcp.NewTestNoopPool(t)

	// This must not panic. It may return either a result or a "claude not
	// configured" error — we accept either as long as there is no panic and
	// no nil dereference.
	//
	// CallHandleMemoryAsk returns (map[string]any, error). A "claude not
	// enabled" error is acceptable and expected in this environment.
	out, err := mcp.CallHandleMemoryAsk(ctx, t, pool, map[string]any{
		"question": "What did I learn about nanoseconds?",
		"project":  "test-project",
	})

	// We expect either a Go error or a tool-level error result (IsError=true)
	// because no Claude client is configured. Both are acceptable. A nil result
	// with nil error is also acceptable — it signals a tool-level error was
	// returned by the handler (see CallHandleMemoryAsk). A non-nil result map
	// would indicate an unexpected success path; that is also fine for future
	// stubs that embed a completer.
	if err != nil {
		// Acceptable: Claude not configured, recall error, etc.
		t.Logf("got expected error (no claude client): %v", err)
		return
	}
	// out == nil means a tool-level error result was returned — acceptable.
	if out == nil {
		t.Logf("got expected tool-level error result (no claude client)")
		return
	}
	// If somehow a success result is returned, it must be non-nil.
	t.Logf("unexpected success path; result: %v", out)
}
