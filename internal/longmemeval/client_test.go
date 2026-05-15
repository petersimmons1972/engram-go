package longmemeval_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/petersimmons1972/engram/internal/longmemeval"
)

// newTestMCPServer builds a minimal MCP server with stubs for the Engram tools
// used by the benchmark runner. It returns the test server URL and a cleanup fn.
func newTestMCPServer(t *testing.T, handlers map[string]func(req mcp.CallToolRequest) (*mcp.CallToolResult, error)) string {
	t.Helper()
	mcpServer := server.NewMCPServer("test-engram", "1.0.0", server.WithToolCapabilities(true))

	// Register a catch-all stub for each named tool.
	for name, h := range handlers {
		// Capture loop variable.
		toolName := name
		handler := h
		mcpServer.AddTool(mcp.NewTool(toolName), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return handler(req)
		})
	}

	ts := server.NewTestServer(mcpServer)
	t.Cleanup(ts.Close)
	return ts.URL
}

// TestConnect_HappyPath verifies that Connect succeeds against a live stub server.
func TestConnect_HappyPath(t *testing.T) {
	url := newTestMCPServer(t, map[string]func(mcp.CallToolRequest) (*mcp.CallToolResult, error){})
	ctx := context.Background()
	c, err := longmemeval.Connect(ctx, url, "")
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	if err := c.Close(); err != nil {
		t.Errorf("Close: %v", err)
	}
}

// TestDeleteProject_HappyPath verifies normal cleanup returns nil.
func TestDeleteProject_HappyPath(t *testing.T) {
	url := newTestMCPServer(t, map[string]func(mcp.CallToolRequest) (*mcp.CallToolResult, error){
		"memory_delete_project": func(req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return &mcp.CallToolResult{
				Content: []mcp.Content{mcp.TextContent{Type: "text", Text: `{"deleted":true}`}},
			}, nil
		},
	})
	ctx := context.Background()
	c, err := longmemeval.Connect(ctx, url, "")
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer c.Close()

	if err := c.DeleteProject(ctx, "test-project"); err != nil {
		t.Errorf("DeleteProject: %v", err)
	}
}

// TestDeleteProject_StaleSession verifies Bug #642 fix: stale-session errors
// from DeleteProject are silently consumed (returns nil, not the error).
//
// We simulate the stale-session condition by having the server return a
// transport-level error whose message contains "Invalid session ID".  In the
// real system this comes from the SSE session manager after an Engram restart;
// here we simulate it with a tool-level error that passes through the
// IsStaleSessionError detector.
func TestDeleteProject_StaleSession(t *testing.T) {
	url := newTestMCPServer(t, map[string]func(mcp.CallToolRequest) (*mcp.CallToolResult, error){
		"memory_delete_project": func(req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			// Simulate a stale-session condition by returning an error whose
			// message matches what the SSE transport emits.
			return nil, errors.New("invalid params: Invalid session ID")
		},
	})
	ctx := context.Background()
	c, err := longmemeval.Connect(ctx, url, "")
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer c.Close()

	// Bug #642: must return nil, not the stale-session error.
	err = c.DeleteProject(ctx, "stale-project")
	if err != nil {
		t.Errorf("DeleteProject stale session: expected nil, got %v", err)
	}
}

// TestDeleteProject_OtherError verifies that non-stale errors are propagated.
func TestDeleteProject_OtherError(t *testing.T) {
	url := newTestMCPServer(t, map[string]func(mcp.CallToolRequest) (*mcp.CallToolResult, error){
		"memory_delete_project": func(req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return nil, errors.New("connection reset by peer")
		},
	})
	ctx := context.Background()
	c, err := longmemeval.Connect(ctx, url, "")
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer c.Close()

	err = c.DeleteProject(ctx, "proj")
	if err == nil {
		t.Error("DeleteProject: expected error for non-stale failure, got nil")
	}
}

// TestRecall_HappyPath exercises the recall path through the MCP client.
func TestRecall_HappyPath(t *testing.T) {
	url := newTestMCPServer(t, map[string]func(mcp.CallToolRequest) (*mcp.CallToolResult, error){
		"memory_recall": func(req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			resp, _ := json.Marshal(map[string]any{
				"results": []map[string]any{
					{"memory": map[string]any{"id": "mem-111"}, "score": 0.9},
					{"memory": map[string]any{"id": "mem-222"}, "score": 0.7},
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

	ids, err := c.Recall(ctx, "proj", "what happened", 10)
	if err != nil {
		t.Fatalf("Recall: %v", err)
	}
	if len(ids) != 2 || ids[0] != "mem-111" || ids[1] != "mem-222" {
		t.Errorf("Recall ids = %v, want [mem-111, mem-222]", ids)
	}
}

// TestRecall_HandleMode verifies that the handle-mode recall response is parsed.
func TestRecall_HandleMode(t *testing.T) {
	url := newTestMCPServer(t, map[string]func(mcp.CallToolRequest) (*mcp.CallToolResult, error){
		"memory_recall": func(req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			resp, _ := json.Marshal(map[string]any{
				"handles": []map[string]any{
					{"id": "h-aaa", "score": 0.8},
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

	ids, err := c.Recall(ctx, "proj", "query", 5)
	if err != nil {
		t.Fatalf("Recall handle mode: %v", err)
	}
	if len(ids) != 1 || ids[0] != "h-aaa" {
		t.Errorf("Recall handle mode ids = %v, want [h-aaa]", ids)
	}
}

// TestFetchContent_HappyPath verifies content fetching.
func TestFetchContent_HappyPath(t *testing.T) {
	url := newTestMCPServer(t, map[string]func(mcp.CallToolRequest) (*mcp.CallToolResult, error){
		"memory_fetch": func(req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			resp, _ := json.Marshal(map[string]any{
				"memory": map[string]any{"content": "the memory content"},
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

	content, err := c.FetchContent(ctx, "proj", "mem-abc")
	if err != nil {
		t.Fatalf("FetchContent: %v", err)
	}
	if content != "the memory content" {
		t.Errorf("content = %q, want %q", content, "the memory content")
	}
}

// TestFetchContent_FlatContentField verifies the fallback to top-level "content" field.
func TestFetchContent_FlatContentField(t *testing.T) {
	url := newTestMCPServer(t, map[string]func(mcp.CallToolRequest) (*mcp.CallToolResult, error){
		"memory_fetch": func(req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			resp, _ := json.Marshal(map[string]any{
				"content": "flat content field",
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

	content, err := c.FetchContent(ctx, "proj", "mem-abc")
	if err != nil {
		t.Fatalf("FetchContent flat: %v", err)
	}
	if content != "flat content field" {
		t.Errorf("content = %q, want %q", content, "flat content field")
	}
}

// TestStore_HappyPath verifies the single-memory store path.
func TestStore_HappyPath(t *testing.T) {
	url := newTestMCPServer(t, map[string]func(mcp.CallToolRequest) (*mcp.CallToolResult, error){
		"memory_store": func(req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			resp, _ := json.Marshal(map[string]any{"id": "stored-id-1"})
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

	id, err := c.Store(ctx, "proj", "content", []string{"tag1"})
	if err != nil {
		t.Fatalf("Store: %v", err)
	}
	if id != "stored-id-1" {
		t.Errorf("Store id = %q, want stored-id-1", id)
	}
}

// TestStoreBatch_HappyPath verifies the batch store path.
func TestStoreBatch_HappyPath(t *testing.T) {
	url := newTestMCPServer(t, map[string]func(mcp.CallToolRequest) (*mcp.CallToolResult, error){
		"memory_store_batch": func(req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			resp, _ := json.Marshal(map[string]any{
				"ids":   []string{"id-a", "id-b"},
				"count": 2,
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

	items := []longmemeval.BatchItem{
		{Content: "content one", Tags: []string{"t1"}},
		{Content: "content two", Tags: []string{"t2"}},
	}
	ids, err := c.StoreBatch(ctx, "proj", items)
	if err != nil {
		t.Fatalf("StoreBatch: %v", err)
	}
	if len(ids) != 2 || ids[0] != "id-a" || ids[1] != "id-b" {
		t.Errorf("StoreBatch ids = %v, want [id-a, id-b]", ids)
	}
}

// TestStoreBatch_MismatchedCount verifies error when server returns wrong ID count.
func TestStoreBatch_MismatchedCount(t *testing.T) {
	url := newTestMCPServer(t, map[string]func(mcp.CallToolRequest) (*mcp.CallToolResult, error){
		"memory_store_batch": func(req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			// Return only 1 ID for 2 items.
			resp, _ := json.Marshal(map[string]any{
				"ids":   []string{"only-one"},
				"count": 1,
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

	items := []longmemeval.BatchItem{
		{Content: "c1"},
		{Content: "c2"},
	}
	_, err = c.StoreBatch(ctx, "proj", items)
	if err == nil {
		t.Error("StoreBatch: expected error for mismatched ID count, got nil")
	}
}

// TestStoreBatch_ServerRejectsItems verifies error surfacing from batch errors field.
func TestStoreBatch_ServerRejectsItems(t *testing.T) {
	url := newTestMCPServer(t, map[string]func(mcp.CallToolRequest) (*mcp.CallToolResult, error){
		"memory_store_batch": func(req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			resp, _ := json.Marshal(map[string]any{
				"ids":    []string{},
				"errors": []string{"item 0: content too short", "item 1: invalid tag"},
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

	_, err = c.StoreBatch(ctx, "proj", []longmemeval.BatchItem{{Content: "x"}, {Content: "y"}})
	if err == nil {
		t.Error("StoreBatch: expected error when server reports errors, got nil")
	}
	if !contains(fmt.Sprint(err), "rejected") {
		t.Errorf("error message should contain 'rejected': %v", err)
	}
}
