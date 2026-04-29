package mcp

// Context-propagation spike for #379.
// Proves that context.WithTimeout in a dispatch wrapper correctly cancels
// the ctx handed to a blocking tool handler — so Option C (per-tool timeout
// annotation with WithTimeout wrapper) is safe to implement without the
// goroutine+channel fallback pattern.

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	mcpclient "github.com/mark3labs/mcp-go/client"
	mcpgo "github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// withTimeoutSpike is the proposed dispatch wrapper (Option C from Opus advisor).
func withTimeoutSpike(d time.Duration, h server.ToolHandlerFunc) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
		ctx, cancel := context.WithTimeout(ctx, d)
		defer cancel()
		return h(ctx, req)
	}
}

// TestDispatchTimeout_ContextPropagation proves that context.WithTimeout in
// the wrapper correctly cancels the ctx passed to a blocking handler.
//
// Verdict: if the test passes, Option C is safe — use context.WithTimeout.
// If it hangs past 5s (test timeout), Option C fails — use goroutine+channel.
func TestDispatchTimeout_ContextPropagation(t *testing.T) {
	var ctxCancelledInHandler atomic.Bool

	blockingHandler := func(ctx context.Context, _ mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
		<-ctx.Done()
		ctxCancelledInHandler.Store(true)
		return &mcpgo.CallToolResult{
			IsError: true,
			Content: []mcpgo.Content{
				mcpgo.TextContent{Type: "text", Text: "tool timed out"},
			},
		}, nil
	}

	mcpServer := server.NewMCPServer("spike-test", "1.0.0",
		server.WithToolCapabilities(true),
	)
	mcpServer.AddTool(
		mcpgo.NewTool("block_forever"),
		withTimeoutSpike(300*time.Millisecond, blockingHandler),
	)

	c, err := mcpclient.NewInProcessClient(mcpServer)
	if err != nil {
		t.Fatalf("NewInProcessClient: %v", err)
	}
	if err := c.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer c.Close()

	initReq := mcpgo.InitializeRequest{}
	initReq.Params.ProtocolVersion = mcpgo.LATEST_PROTOCOL_VERSION
	initReq.Params.ClientInfo = mcpgo.Implementation{Name: "spike", Version: "1.0.0"}
	if _, err := c.Initialize(context.Background(), initReq); err != nil {
		t.Fatalf("Initialize: %v", err)
	}

	start := time.Now()
	var callReq mcpgo.CallToolRequest
	callReq.Params.Name = "block_forever"
	result, err := c.CallTool(context.Background(), callReq)
	elapsed := time.Since(start)

	isErr := result != nil && result.IsError
	t.Logf("elapsed: %v | err: %v | isError: %v", elapsed, err, isErr)

	if !ctxCancelledInHandler.Load() {
		t.Fatal("SPIKE FAILED: handler ctx was NOT cancelled — context.WithTimeout does not propagate; must use goroutine+channel instead")
	}
	if elapsed > 2*time.Second {
		t.Errorf("SPIKE FAILED: call took %v — timeout wrapper did not terminate handler in time", elapsed)
	}

	t.Log("SPIKE PASSED: context.WithTimeout propagates correctly — Option C is safe")
}
