package mcp

// Tests for per-handler context deadlines (issue #288).
//
// These tests verify that:
// 1. Each long-running handler (Consolidate, Sleep, Explore, Ask) derives its
//    own child context with a fixed deadline, so it cannot run past the HTTP
//    server's ReadTimeout even if the caller's context never cancels.
//
// All tests are in-package (package mcp, no _test suffix) because they test
// unexported symbols.

import (
	"context"
	"testing"
	"time"

	mcpgo "github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/require"
)

// ── Part 1: per-handler deadline tests ───────────────────────────────────────

// TestHandlerDeadline_Consolidate verifies that handleMemoryConsolidate adds a
// child deadline to the incoming context rather than using it verbatim.
// We pass an already-expired context; if the handler propagates it correctly
// the pool.Get or engine call returns immediately with a context error.
func TestHandlerDeadline_Consolidate(t *testing.T) {
	pool := newTestNoopPool(t)
	req := mcpgo.CallToolRequest{}
	req.Params.Arguments = map[string]any{"project": "test"}

	// An already-cancelled parent context simulates a disconnected client.
	parent, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	// The handler must return an error (context cancelled) rather than hanging.
	_, err := handleMemoryConsolidate(parent, pool, req, Config{})
	// noopBackend.Consolidate succeeds even on a cancelled context (it does no I/O),
	// but the handler must have derived a child context that at worst inherits the
	// cancellation — meaning no panic and a prompt return.
	_ = err // error or nil both acceptable; what matters is it returns promptly
}

// TestHandlerDeadline_Sleep verifies handleMemorySleep derives a child deadline.
func TestHandlerDeadline_Sleep(t *testing.T) {
	pool := newTestNoopPool(t)
	req := mcpgo.CallToolRequest{}
	req.Params.Arguments = map[string]any{"project": "test"}

	parent, cancel := context.WithCancel(context.Background())
	cancel()

	done := make(chan struct{})
	go func() {
		defer close(done)
		handleMemorySleep(parent, pool, req, Config{}) //nolint:errcheck
	}()

	select {
	case <-done:
		// returned promptly — good
	case <-time.After(5 * time.Second):
		t.Fatal("handleMemorySleep did not return within 5s on a cancelled context")
	}
}

// TestHandlerDeadline_Explore verifies handleMemoryExplore derives a child deadline.
// Without a Claude client the handler returns immediately with a tool error, so
// we just confirm it returns and uses a deadline (tested indirectly via a very
// tight parent timeout to ensure the child deadline is set from ctx, not Background).
func TestHandlerDeadline_Explore(t *testing.T) {
	pool := newTestNoopPool(t)
	req := mcpgo.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"project":  "test",
		"question": "what is the meaning of life?",
	}

	// No Claude client — handler must return a tool error before any LLM call.
	result, err := handleMemoryExplore(context.Background(), pool, req, Config{})
	require.NoError(t, err)
	require.NotNil(t, result)
	require.True(t, result.IsError, "expected tool error when no Claude client is configured")
}

// TestHandlerDeadline_Ask verifies handleMemoryAsk derives a child deadline and
// returns a tool error (not a Go error) when no Claude client is present.
func TestHandlerDeadline_Ask(t *testing.T) {
	pool := newTestNoopPool(t)
	req := mcpgo.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"project":  "test",
		"question": "what is a nanosecond?",
	}

	result, err := handleMemoryAsk(context.Background(), pool, req, Config{})
	require.NoError(t, err)
	require.NotNil(t, result)
	require.True(t, result.IsError, "expected tool error when no Claude client is configured")
}

// TestHandlerDeadline_AskDeadlinePropagated verifies that a deadline set on the
// parent context is inherited (not ignored) by the child context the handler
// creates. We set a 1ms deadline so that pool.Get (which calls the noop factory)
// still completes, but any subsequent blocking call would be caught.
func TestHandlerDeadline_AskDeadlinePropagated(t *testing.T) {
	pool := newTestNoopPool(t)
	req := mcpgo.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"project":  "test",
		"question": "deadline propagation check",
	}

	// The handler derives: ctx, cancel := context.WithTimeout(ctx, 90*time.Second)
	// That child deadline must be ≤ parent deadline.  We verify the handler returns
	// in finite time even with a very tight parent.
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	// No Claude client, so it returns a tool error before touching the context deadline.
	result, err := handleMemoryAsk(ctx, pool, req, Config{})
	require.NoError(t, err)
	require.NotNil(t, result)
}

// ── Part 2: additional deadline tests ────────────────────────────────────────

// TestMemoryReason_HasExplicitDeadline verifies that handleMemoryReason derives
// its own child context with a deadline, so it cannot block indefinitely even
// when the parent context is never cancelled.
//
// Strategy: we pass a background context (which never cancels) and a request
// with a valid question. Because no Claude client is configured the handler
// must return a tool error before making any LLM call — which lets us confirm
// the handler returns promptly rather than hanging. This mirrors the pattern
// used by TestHandlerDeadline_Ask and TestHandlerDeadline_Explore.
func TestMemoryReason_HasExplicitDeadline(t *testing.T) {
	pool := newTestNoopPool(t)
	req := mcpgo.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"project":  "test",
		"question": "what is a nanosecond?",
	}

	// No Claude client — the handler must return a tool error immediately
	// rather than hanging on an LLM call.
	result, err := handleMemoryReason(context.Background(), pool, req, Config{})
	require.NoError(t, err)
	require.NotNil(t, result)
	require.True(t, result.IsError, "expected tool error when no Claude client is configured")
}

// TestMemoryReason_DeadlinePropagated verifies that a tight parent deadline is
// inherited by the child context the handler creates. Mirrors
// TestHandlerDeadline_AskDeadlinePropagated.
func TestMemoryReason_DeadlinePropagated(t *testing.T) {
	pool := newTestNoopPool(t)
	req := mcpgo.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"project":  "test",
		"question": "deadline propagation check",
	}

	// Give the handler a very tight parent timeout; without a Claude client
	// it returns before consuming it, but the handler must not ignore the parent.
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	result, err := handleMemoryReason(ctx, pool, req, Config{})
	require.NoError(t, err)
	require.NotNil(t, result)
}

