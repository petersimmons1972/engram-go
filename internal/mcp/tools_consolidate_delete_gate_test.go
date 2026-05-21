package mcp

// Tests for A-4 (#689): memory_delete_project authorization gate.
// Two layers:
//   1. ENGRAM_ALLOW_PROJECT_DELETE env-var gate — server must be started with
//      it set, otherwise the tool returns a tool-level error.
//   2. confirm argument must match the project argument — typo guard.

import (
	"context"
	"strings"
	"testing"

	mcpgo "github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/require"
)

// TestMemoryDeleteProject_Gate_Closed_BlocksDelete verifies that when the env
// var is not set, the tool returns a tool-level error (not a Go error) and
// does NOT call the backend.
func TestMemoryDeleteProject_Gate_Closed_BlocksDelete(t *testing.T) {
	// Explicitly ensure the gate is closed for this test (don't depend on
	// test-environment defaults).
	t.Setenv("ENGRAM_ALLOW_PROJECT_DELETE", "")

	stub := &deleteProjectStubBackend{}
	pool := newDeleteProjectPool(t, stub)

	req := mcpgo.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"project": "test-eval-01",
		"confirm": "test-eval-01",
	}

	result, err := handleMemoryDeleteProject(context.Background(), pool, req)
	require.NoError(t, err, "gate-closed must return a tool-level error, not a Go error")
	require.NotNil(t, result)
	require.True(t, result.IsError, "expected tool-level error")

	require.NotEmpty(t, result.Content)
	text, ok := result.Content[0].(mcpgo.TextContent)
	require.True(t, ok)
	require.Contains(t, text.Text, "disabled",
		"error message must explain the gate is closed")
	require.Contains(t, text.Text, "ENGRAM_ALLOW_PROJECT_DELETE",
		"error message must name the env var")

	// Backend must NOT have been called.
	require.Empty(t, stub.deleteCalls,
		"backend must not be called when gate is closed")
}

// TestMemoryDeleteProject_Gate_Open_ConfirmMatches_Deletes verifies that with
// the gate open AND a matching confirm argument, the delete proceeds.
func TestMemoryDeleteProject_Gate_Open_ConfirmMatches_Deletes(t *testing.T) {
	t.Setenv("ENGRAM_ALLOW_PROJECT_DELETE", "1")

	stub := &deleteProjectStubBackend{}
	pool := newDeleteProjectPool(t, stub)

	req := mcpgo.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"project": "testproj",
		"confirm": "testproj",
	}

	result, err := handleMemoryDeleteProject(context.Background(), pool, req)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.False(t, result.IsError, "expected success: %+v", result.Content)

	require.Len(t, stub.deleteCalls, 1)
	require.Equal(t, "testproj", stub.deleteCalls[0])
}

// TestMemoryDeleteProject_Gate_Open_ConfirmMismatch_Blocks verifies that with
// the gate open but a mismatched confirm argument (typo guard), the tool
// errors and does NOT call the backend.
func TestMemoryDeleteProject_Gate_Open_ConfirmMismatch_Blocks(t *testing.T) {
	t.Setenv("ENGRAM_ALLOW_PROJECT_DELETE", "1")

	stub := &deleteProjectStubBackend{}
	pool := newDeleteProjectPool(t, stub)

	req := mcpgo.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"project": "clearwatch",
		"confirm": "clearwacth", // typo
	}

	result, err := handleMemoryDeleteProject(context.Background(), pool, req)
	require.NoError(t, err, "mismatch must return a tool-level error, not a Go error")
	require.NotNil(t, result)
	require.True(t, result.IsError, "expected tool-level error")

	text := result.Content[0].(mcpgo.TextContent).Text
	require.True(t,
		strings.Contains(text, "confirm") && strings.Contains(text, "match"),
		"error must mention confirm/match: %q", text)

	require.Empty(t, stub.deleteCalls,
		"backend must not be called on confirm mismatch")
}

// TestMemoryDeleteProject_Gate_Open_ConfirmMissing_Blocks verifies that
// omitting the confirm argument when the gate is open also blocks.
func TestMemoryDeleteProject_Gate_Open_ConfirmMissing_Blocks(t *testing.T) {
	t.Setenv("ENGRAM_ALLOW_PROJECT_DELETE", "1")

	stub := &deleteProjectStubBackend{}
	pool := newDeleteProjectPool(t, stub)

	req := mcpgo.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"project": "testproj",
		// no confirm
	}

	result, err := handleMemoryDeleteProject(context.Background(), pool, req)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.True(t, result.IsError, "missing confirm must error")

	require.Empty(t, stub.deleteCalls)
}
