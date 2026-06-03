package longmemeval_test

// recall_retry_test.go verifies that Recall() reconnects and retries on
// connection/unmarshal errors (the SSE drop race described in issue #861).

import (
	"context"
	"encoding/json"
	"errors"
	"sync/atomic"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/petersimmons1972/engram/internal/longmemeval"
)

// newTestMCPServerWithInitCount builds a minimal MCP server and counts how
// many times the client calls Initialize (one per Connect/reconnect).
func newTestMCPServerWithInitCount(t *testing.T, initCalls *atomic.Int32, handlers map[string]func(req mcp.CallToolRequest) (*mcp.CallToolResult, error)) string {
	t.Helper()
	mcpServer := server.NewMCPServer(
		"test-engram-reconnect", "1.0.0",
		server.WithToolCapabilities(true),
		server.WithHooks(&server.Hooks{
			OnBeforeInitialize: []server.OnBeforeInitializeFunc{
				func(ctx context.Context, id any, req *mcp.InitializeRequest) {
					initCalls.Add(1)
				},
			},
		}),
	)

	for name, h := range handlers {
		toolName := name
		handler := h
		mcpServer.AddTool(mcp.NewTool(toolName), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return handler(req)
		})
	}

	ts := server.NewTestStreamableHTTPServer(mcpServer)
	t.Cleanup(ts.Close)
	return ts.URL
}

// TestRecall_RetryReconnectsOnError verifies the three contracts from the spec:
//
//  1. When recall() fails on the first attempt, Recall() reconnects and retries.
//  2. When the retry succeeds, the result is returned.
//  3. When all retries are exhausted, the last error is returned.
func TestRecall_RetryReconnectsOnError(t *testing.T) {
	t.Run("RetrySucceeds_ReturnsResult_AndReconnects", func(t *testing.T) {
		// The memory_recall tool fails on the first call (simulating SSE drop),
		// then succeeds on the second call. We also verify that a reconnect
		// (new Initialize) happened before the retry.
		var recallCalls atomic.Int32
		var initCalls atomic.Int32

		url := newTestMCPServerWithInitCount(t, &initCalls, map[string]func(mcp.CallToolRequest) (*mcp.CallToolResult, error){
			"memory_recall": func(req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
				n := recallCalls.Add(1)
				if n == 1 {
					// Simulate the SSE drop: return a connection error.
					return nil, errors.New("unexpected end of JSON input")
				}
				// Second call succeeds.
				resp, _ := json.Marshal(map[string]any{
					"handles": []map[string]any{
						{"id": "retry-mem-1", "score": 0.95},
					},
				})
				return &mcp.CallToolResult{
					Content: []mcp.Content{mcp.TextContent{Type: "text", Text: string(resp)}},
				}, nil
			},
		})

		ctx := context.Background()
		c, err := longmemeval.Connect(ctx, url, "")
		if err != nil {
			t.Fatalf("Connect: %v", err)
		}
		defer c.Close()

		// Reset counter after initial Connect so we measure only reconnects.
		initCalls.Store(0)

		ids, err := c.Recall(ctx, "proj", "what happened", 5)
		if err != nil {
			t.Fatalf("Recall after retry: unexpected error: %v", err)
		}
		if len(ids) != 1 || ids[0] != "retry-mem-1" {
			t.Errorf("Recall ids = %v, want [retry-mem-1]", ids)
		}
		if got := recallCalls.Load(); got < 2 {
			t.Errorf("expected at least 2 recall calls (1 fail + 1 success), got %d", got)
		}
		// Verify that a reconnect (new SSE session + Initialize) happened.
		if got := initCalls.Load(); got < 1 {
			t.Errorf("expected at least 1 reconnect (Initialize call) during retry, got %d", got)
		}
	})

	t.Run("AllRetriesExhausted_ReturnsLastError", func(t *testing.T) {
		// The memory_recall tool always fails — all attempts should be exhausted
		// and the last error should be returned.
		var recallCalls atomic.Int32
		sentinelErr := "recall: failed to unmarshal response: unexpected end of JSON input"

		url := newTestMCPServer(t, map[string]func(mcp.CallToolRequest) (*mcp.CallToolResult, error){
			"memory_recall": func(req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
				recallCalls.Add(1)
				return nil, errors.New(sentinelErr)
			},
		})

		ctx := context.Background()
		c, err := longmemeval.Connect(ctx, url, "")
		if err != nil {
			t.Fatalf("Connect: %v", err)
		}
		defer c.Close()

		_, err = c.Recall(ctx, "proj", "query", 5)
		if err == nil {
			t.Fatal("Recall: expected error when all retries exhausted, got nil")
		}
		// The last error should be propagated.
		if !contains(err.Error(), sentinelErr) && !contains(err.Error(), "unexpected end") {
			t.Errorf("error = %q; want it to contain sentinel text", err.Error())
		}
		// With retries=1 the client should have attempted 2 total calls.
		if got := recallCalls.Load(); got < 2 {
			t.Errorf("expected at least 2 recall calls (initial + 1 retry), got %d", got)
		}
	})

	t.Run("FirstAttemptSucceeds_NoReconnect", func(t *testing.T) {
		// Happy-path guard: no retry or reconnect when the first call succeeds.
		var recallCalls atomic.Int32
		var initCalls atomic.Int32

		url := newTestMCPServerWithInitCount(t, &initCalls, map[string]func(mcp.CallToolRequest) (*mcp.CallToolResult, error){
			"memory_recall": func(req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
				recallCalls.Add(1)
				resp, _ := json.Marshal(map[string]any{
					"handles": []map[string]any{
						{"id": "first-call-mem", "score": 0.8},
					},
				})
				return &mcp.CallToolResult{
					Content: []mcp.Content{mcp.TextContent{Type: "text", Text: string(resp)}},
				}, nil
			},
		})

		ctx := context.Background()
		c, err := longmemeval.Connect(ctx, url, "")
		if err != nil {
			t.Fatalf("Connect: %v", err)
		}
		defer c.Close()

		// Reset counter after initial Connect.
		initCalls.Store(0)

		ids, err := c.Recall(ctx, "proj", "query", 5)
		if err != nil {
			t.Fatalf("Recall happy path: %v", err)
		}
		if len(ids) != 1 || ids[0] != "first-call-mem" {
			t.Errorf("ids = %v, want [first-call-mem]", ids)
		}
		if got := recallCalls.Load(); got != 1 {
			t.Errorf("expected exactly 1 recall call on success, got %d", got)
		}
		// No reconnect should happen when the first attempt succeeds.
		if got := initCalls.Load(); got != 0 {
			t.Errorf("expected 0 reconnects on happy path, got %d", got)
		}
	})
}
