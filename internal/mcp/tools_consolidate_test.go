package mcp

// Tests for handleMemoryDeleteProject — hard-delete tool handler.
// Uses noopBackend stub + newTestNoopPool helper (defined in explore_handler_test.go).

import (
	"context"
	"encoding/json"
	"testing"

	mcpgo "github.com/mark3labs/mcp-go/mcp"
	"github.com/petersimmons1972/engram/internal/db"
	"github.com/petersimmons1972/engram/internal/search"
	"github.com/stretchr/testify/require"
)

// ── deleteProjectStubBackend ──────────────────────────────────────────────────

// deleteProjectStubBackend embeds noopBackend and tracks DeleteProject calls for assertion.
type deleteProjectStubBackend struct {
	noopBackend
	deleteCalls []string // list of projects passed to DeleteProject
}

func (d *deleteProjectStubBackend) DeleteProject(_ context.Context, project string) error {
	d.deleteCalls = append(d.deleteCalls, project)
	return nil
}

var _ db.Backend = (*deleteProjectStubBackend)(nil)

// newDeleteProjectPool builds an EnginePool backed by deleteProjectStubBackend
// so we can verify that DeleteProject was called with the correct project name.
func newDeleteProjectPool(t *testing.T, stub *deleteProjectStubBackend) *EnginePool {
	t.Helper()
	factory := func(ctx context.Context, project string) (*EngineHandle, error) {
		engine := search.New(ctx, stub, noopEmbedder{}, project,
			"http://ollama-test:11434", "", false, nil, 0)
		t.Cleanup(engine.Close)
		return &EngineHandle{Engine: engine}, nil
	}
	return NewEnginePool(factory)
}

// ── TestMemoryDeleteProject_HappyPath ─────────────────────────────────────────

// TestMemoryDeleteProject_HappyPath verifies that a valid project name results in:
// - IsError = false
// - "deleted": true in the result
// - "project": <name> in the result
// - backend.DeleteProject called with the correct project name
func TestMemoryDeleteProject_HappyPath(t *testing.T) {
	stub := &deleteProjectStubBackend{}
	pool := newDeleteProjectPool(t, stub)

	req := mcpgo.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"project": "test-eval-01",
	}

	result, err := handleMemoryDeleteProject(context.Background(), pool, req)
	require.NoError(t, err, "handler must not return a Go error")
	require.NotNil(t, result)
	require.False(t, result.IsError, "expected non-error result, got: %+v", result.Content)

	// Parse the result JSON.
	require.NotEmpty(t, result.Content)
	text, ok := result.Content[0].(mcpgo.TextContent)
	require.True(t, ok, "expected TextContent, got %T", result.Content[0])

	var out map[string]any
	require.NoError(t, json.Unmarshal([]byte(text.Text), &out))

	// Verify the response fields.
	require.True(t, out["deleted"].(bool), "deleted must be true")
	require.Equal(t, "test-eval-01", out["project"])
	require.NotEmpty(t, out["message"])

	// Verify the backend was called with the correct project.
	require.Len(t, stub.deleteCalls, 1)
	require.Equal(t, "test-eval-01", stub.deleteCalls[0])
}

// ── TestMemoryDeleteProject_EmptyProject ──────────────────────────────────────

// TestMemoryDeleteProject_EmptyProject verifies that an empty or missing project
// argument returns a Go error (not a tool-level error), which will log a WARN.
// This is consistent with the handler design: missing required args error early.
func TestMemoryDeleteProject_EmptyProject(t *testing.T) {
	stub := &deleteProjectStubBackend{}
	pool := newDeleteProjectPool(t, stub)

	req := mcpgo.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"project": "",
	}

	result, err := handleMemoryDeleteProject(context.Background(), pool, req)
	require.Error(t, err, "empty project must return a Go error")
	require.Nil(t, result)

	// Backend must not have been called.
	require.Empty(t, stub.deleteCalls)
}

// ── TestMemoryDeleteProject_IdempotentDelete ──────────────────────────────────

// TestMemoryDeleteProject_IdempotentDelete verifies that deleting a non-existent
// project succeeds without error — the noopBackend's DeleteProject returns nil
// regardless of whether the project existed. This is the idempotent semantic
// expected from a delete operation.
func TestMemoryDeleteProject_IdempotentDelete(t *testing.T) {
	stub := &deleteProjectStubBackend{}
	pool := newDeleteProjectPool(t, stub)

	// Call delete for a project that was never created.
	req := mcpgo.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"project": "never-existed",
	}

	result, err := handleMemoryDeleteProject(context.Background(), pool, req)
	require.NoError(t, err, "handler must not error on non-existent project")
	require.NotNil(t, result)
	require.False(t, result.IsError, "expected non-error result")

	// Parse and verify the result.
	text, ok := result.Content[0].(mcpgo.TextContent)
	require.True(t, ok)

	var out map[string]any
	require.NoError(t, json.Unmarshal([]byte(text.Text), &out))
	require.True(t, out["deleted"].(bool), "deletion of non-existent project must still return deleted=true")
	require.Equal(t, "never-existed", out["project"])

	// Backend must have been called with the project name.
	require.Len(t, stub.deleteCalls, 1)
	require.Equal(t, "never-existed", stub.deleteCalls[0])
}
