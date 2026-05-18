package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
)

type scriptedClient struct {
	mu           sync.Mutex
	startErrs    []error
	initErrs     []error
	callErrs     []error
	starts       int
	inits        int
	calls        int
	closes       int
	lastToolName string
}

func (c *scriptedClient) Start(context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.starts++
	if len(c.startErrs) > 0 {
		err := c.startErrs[0]
		c.startErrs = c.startErrs[1:]
		return err
	}
	return nil
}

func (c *scriptedClient) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.closes++
	return nil
}

func (c *scriptedClient) Initialize(context.Context, mcp.InitializeRequest) (*mcp.InitializeResult, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.inits++
	if len(c.initErrs) > 0 {
		err := c.initErrs[0]
		c.initErrs = c.initErrs[1:]
		return nil, err
	}
	return &mcp.InitializeResult{}, nil
}

func (c *scriptedClient) CallTool(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.calls++
	c.lastToolName = req.Params.Name
	if len(c.callErrs) > 0 {
		err := c.callErrs[0]
		c.callErrs = c.callErrs[1:]
		return nil, err
	}
	return &mcp.CallToolResult{Content: []mcp.Content{mcp.TextContent{Type: "text", Text: `{"id":"ok"}`}}}, nil
}

func TestSSEEngramCallToolRetriesAfterReconnect(t *testing.T) {
	first := &scriptedClient{
		callErrs: []error{errors.New("transport EOF")},
	}
	second := &scriptedClient{}
	e := &sseEngram{
		c: first,
		factory: func() (mcpClient, error) {
			return second, nil
		},
	}

	out, err := e.callToolWithRetry(context.Background(), "memory_store", map[string]any{"content": "x"})
	if err != nil {
		t.Fatalf("callToolWithRetry: %v", err)
	}
	if out["id"] != "ok" {
		t.Fatalf("unexpected output: %#v", out)
	}
	if first.calls != 1 {
		t.Fatalf("first client calls = %d, want 1", first.calls)
	}
	if first.closes == 0 {
		t.Fatalf("expected first client to be closed on reconnect")
	}
	if second.starts != 1 || second.inits != 1 {
		t.Fatalf("second client not fully reconnected: starts=%d inits=%d", second.starts, second.inits)
	}
	if second.calls != 1 {
		t.Fatalf("second client calls = %d, want 1", second.calls)
	}
}

func TestSSEEngramCallToolFailsWhenReconnectCannotBuildClient(t *testing.T) {
	first := &scriptedClient{
		callErrs: []error{errors.New("transport EOF")},
	}
	e := &sseEngram{
		c: first,
		factory: func() (mcpClient, error) {
			return nil, errors.New("reconnect unavailable")
		},
	}

	_, err := e.callToolWithRetry(context.Background(), "memory_store", map[string]any{"content": "x"})
	if err == nil {
		t.Fatal("expected callToolWithRetry to fail when reconnect cannot build a client")
	}
	if first.calls != 1 {
		t.Fatalf("first client calls = %d, want 1", first.calls)
	}
}

func TestSSEEngramCallToolFailsAfterContextDeadlineOnPermanentOutage(t *testing.T) {
	first := &scriptedClient{
		callErrs: []error{
			errors.New("transport EOF"),
			errors.New("transport EOF"),
			errors.New("transport EOF"),
		},
	}
	second := &scriptedClient{
		callErrs: []error{
			errors.New("transport EOF"),
			errors.New("transport EOF"),
			errors.New("transport EOF"),
		},
	}
	e := &sseEngram{
		c: first,
		factory: func() (mcpClient, error) {
			return second, nil
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	_, err := e.callToolWithRetry(ctx, "memory_store", map[string]any{"content": "x"})
	if err == nil {
		t.Fatal("expected callToolWithRetry to fail on permanent outage")
	}
	if first.calls == 0 {
		t.Fatal("expected first client to be attempted at least once")
	}
}

// TestWriteEpisodeRetriesAfterTransientFailure verifies fix #735:
//  1. The outer per-event context (now mcpOperationTimeout=40s) is wide enough
//     for retry to reach a second attempt after a transient failure.
//  2. The episode completes successfully despite the first ingest call failing.
//  3. Total elapsed time > mcpConnectTimeout (10s) would be ideal but makes the
//     test slow, so we assert instead that a second call was made to the ingest
//     tool — proving retry actually ran rather than being cancelled by a short
//     outer deadline.
func TestWriteEpisodeRetriesAfterTransientFailure(t *testing.T) {
	// first client: fails the first CallTool (simulates transient SSE timeout)
	first := &scriptedClient{
		callErrs: []error{errors.New("context deadline exceeded")},
	}
	// second client (post-reconnect): succeeds
	second := &scriptedClient{}

	callCount := 0
	e := &sseEngram{
		c: first,
		factory: func() (mcpClient, error) {
			callCount++
			return second, nil
		},
	}

	ev := Event{
		Timestamp:     "2026-01-01T00:00:00Z",
		SessionID:     "sess-retry-test",
		ProjectID:     "test-proj",
		ToolName:      "Bash",
		ToolInputHash: "abc123",
		ExitStatus:    0,
		SchemaVersion: 1,
	}

	start := time.Now()
	err := writeEpisode(context.Background(), e, ev.SessionID, ev.ProjectID, []Event{ev})
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("writeEpisode: %v", err)
	}

	// Reconnect factory must have been called at least once (retry happened).
	if callCount == 0 {
		t.Error("factory never called — retry did not happen; outer timeout likely still too short")
	}

	// Second client must have received at least one call (the retried ingest).
	second.mu.Lock()
	secondCalls := second.calls
	second.mu.Unlock()
	if secondCalls == 0 {
		t.Error("second client never called — retry did not reach the new connection")
	}

	// Sanity: the test shouldn't take unreasonably long (< 5s expected in happy path).
	if elapsed > 10*time.Second {
		t.Logf("warning: writeEpisode took %v — longer than expected", elapsed)
	}
}

func TestRequeueRestoresProcessedBuffer(t *testing.T) {
	dir := t.TempDir()
	bufferPath := filepath.Join(dir, "buffer.jsonl")
	processedPath := filepath.Join(dir, "buffer.jsonl.20260502T000000Z.processed")

	if err := os.WriteFile(processedPath, []byte("payload"), 0o600); err != nil {
		t.Fatalf("write processed: %v", err)
	}

	requeue(bufferPath, processedPath)

	if _, err := os.Stat(bufferPath); err != nil {
		t.Fatalf("buffer should be restored: %v", err)
	}
	if _, err := os.Stat(processedPath); !os.IsNotExist(err) {
		t.Fatalf("processed file should be moved back, stat err=%v", err)
	}
}
