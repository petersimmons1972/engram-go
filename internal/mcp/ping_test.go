package mcp

// Tests for handleMemoryStatusPing — Pillar 3A lightweight health check.
//
// handleMemoryStatusPing does NOT exist yet. These tests will FAIL TO COMPILE
// until the implementation is added to internal/mcp/tools_admin.go.
// That is the expected red-phase state.

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	mcpgo "github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/require"
)

// TestMemoryStatusPing_PoolUp_ReturnsOK verifies that when the pool is healthy,
// handleMemoryStatusPing returns IsError=false with status:"ok" and a timestamp.
func TestMemoryStatusPing_PoolUp_ReturnsOK(t *testing.T) {
	pool := newTestNoopPool(t)
	req := mcpgo.CallToolRequest{}

	res, err := handleMemoryStatusPing(context.Background(), pool, req) // function doesn't exist yet
	require.NoError(t, err, "handleMemoryStatusPing must not return a Go error on success")
	require.NotNil(t, res)
	require.False(t, res.IsError, "healthy pool must return IsError=false")

	require.NotEmpty(t, res.Content)
	text, ok := res.Content[0].(mcpgo.TextContent)
	require.True(t, ok, "content[0] must be TextContent")

	var body map[string]any
	require.NoError(t, json.Unmarshal([]byte(text.Text), &body))
	require.Equal(t, "ok", body["status"], "status must be 'ok' when pool is healthy")
	require.NotEmpty(t, body["ts"], "ts field must be present and non-empty")
}

// TestMemoryStatusPing_PoolDown_ReturnsIsError verifies that when the pool
// factory fails (DB unreachable), the result is IsError=true with no Go error.
func TestMemoryStatusPing_PoolDown_ReturnsIsError(t *testing.T) {
	pool := NewEnginePool(func(ctx context.Context, project string) (*EngineHandle, error) {
		return nil, errors.New("pool unavailable")
	})
	req := mcpgo.CallToolRequest{}

	res, err := handleMemoryStatusPing(context.Background(), pool, req)
	require.NoError(t, err, "Go error must be nil — errors go in IsError result, not returned")
	require.NotNil(t, res)
	require.True(t, res.IsError, "pool failure must return IsError:true")
	require.NotEmpty(t, res.Content, "error result must have non-empty Content")
	text, ok := res.Content[0].(mcpgo.TextContent)
	require.True(t, ok, "error content must be TextContent")
	require.NotEmpty(t, text.Text, "error content text must describe the failure")
}

// TestMemoryStatusPing_RegisteredReadOnly verifies that memory_status_ping is
// in the readOnlyToolNames() set so Claude Code plan mode can call it freely.
func TestMemoryStatusPing_RegisteredReadOnly(t *testing.T) {
	ro := readOnlyToolNames()
	require.True(t, ro["memory_status_ping"],
		"memory_status_ping must be in readOnlyToolNames() (currently missing = FAIL)")
}

// TestMemoryStatusPing_Timeout_ExitsWithin3s verifies that the ping has an internal
// 2s timeout so it cannot block indefinitely on a slow or hung DB.
func TestMemoryStatusPing_Timeout_ExitsWithin3s(t *testing.T) {
	pool := NewEnginePool(func(ctx context.Context, project string) (*EngineHandle, error) {
		// Block until context is cancelled (simulates a hung DB connection).
		<-ctx.Done()
		return nil, ctx.Err()
	})
	req := mcpgo.CallToolRequest{}

	start := time.Now()
	res, err := handleMemoryStatusPing(context.Background(), pool, req)
	elapsed := time.Since(start)

	require.NoError(t, err, "Go error must be nil even on timeout")
	require.NotNil(t, res)
	require.True(t, res.IsError, "slow/hung pool must return IsError:true")
	require.Less(t, elapsed, 3*time.Second,
		"ping must exit within 3s (expects a 2s internal timeout)")
}

// TestMemoryStatusPing_ResponseContainsPingField verifies the response body
// includes a "ping" field so callers can distinguish this tool from memory_status.
func TestMemoryStatusPing_ResponseContainsPingField(t *testing.T) {
	pool := newTestNoopPool(t)
	req := mcpgo.CallToolRequest{}

	res, err := handleMemoryStatusPing(context.Background(), pool, req)
	require.NoError(t, err)
	require.NotNil(t, res)
	require.False(t, res.IsError)

	text, ok := res.Content[0].(mcpgo.TextContent)
	require.True(t, ok)
	var body map[string]any
	require.NoError(t, json.Unmarshal([]byte(text.Text), &body))
	_, hasPing := body["ping"]
	require.True(t, hasPing, "response must include a 'ping' field")
}
