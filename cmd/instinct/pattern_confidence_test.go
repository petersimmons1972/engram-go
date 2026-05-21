package main

// Tests for E2 rewire: instinct uses pattern_confidence (float) instead of
// importance (integer-truncated) when storing and correcting pattern memories.
//
// Coverage targets:
//   - TestInstinctWritesPatternConfidence  — store() sends pattern_confidence as float
//   - TestInstinctCorrectWritesPatternConfidence — correct() sends pattern_confidence as float
//   - TestInstinctReadsPatternConfidence   — recall() reads pattern_confidence from response
//   - TestInstinctBackwardCompatNilPatternConfidence — old records (no pattern_confidence) handled

import (
	"context"
	"encoding/json"
	"testing"

	mcpmcp "github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// capturedArgs records the arguments passed to an MCP tool call.
type capturedArgs struct {
	args map[string]any
}

// newCapturingServer builds a minimal MCP test server that captures the args
// sent to the named tool and returns the given response JSON.
func newCapturingServer(t *testing.T, toolName, responseJSON string, cap *capturedArgs) (engramAPI, func()) {
	t.Helper()
	mcpServer := server.NewMCPServer("test-engram", "1.0.0", server.WithToolCapabilities(true))

	// Register all tools instinct may call so the client can initialise cleanly.
	for _, name := range []string{
		"memory_episode_start", "memory_episode_end",
		"memory_ingest", "memory_recall", "memory_store", "memory_correct",
	} {
		n, resp := name, `{}`
		if n == "memory_episode_start" {
			resp = `{"episode_id":"ep-1"}`
		}
		if n == toolName {
			resp = responseJSON
		}
		rCopy := resp
		mcpServer.AddTool(mcpmcp.NewTool(n), func(ctx context.Context, req mcpmcp.CallToolRequest) (*mcpmcp.CallToolResult, error) {
			if n == toolName {
				if b, err := json.Marshal(req.Params.Arguments); err == nil {
					var a map[string]any
					_ = json.Unmarshal(b, &a)
					cap.args = a
				}
			}
			return &mcpmcp.CallToolResult{Content: []mcpmcp.Content{
				mcpmcp.TextContent{Type: "text", Text: rCopy},
			}}, nil
		})
	}

	ts := server.NewTestServer(mcpServer)
	e, err := newSSEEngram(ts.URL, "")
	if err != nil {
		t.Fatalf("newSSEEngram: %v", err)
	}
	if err := e.connect(context.Background()); err != nil {
		t.Fatalf("connect: %v", err)
	}
	return e, func() {
		e.close()
		ts.Close()
	}
}

// TestInstinctWritesPatternConfidence verifies that store() sends
// "pattern_confidence" as a float in the MCP call — not "importance".
func TestInstinctWritesPatternConfidence(t *testing.T) {
	cap := &capturedArgs{}
	e, cleanup := newCapturingServer(t, "memory_store", `{"id":"mem-pc-1"}`, cap)
	defer cleanup()

	p := Pattern{
		Type:         "workflow",
		Description:  "writes pattern_confidence test",
		Domain:       "git",
		Evidence:     "seen 2x",
		TagSignature: "sig-pc-write",
	}
	id, err := e.store(context.Background(), p, 0.7, "proj1")
	if err != nil {
		t.Fatalf("store() error: %v", err)
	}
	if id != "mem-pc-1" {
		t.Errorf("store() returned id=%q, want mem-pc-1", id)
	}

	if cap.args == nil {
		t.Fatal("no args captured for memory_store call")
	}

	// Must use pattern_confidence, not importance.
	if _, hasImportance := cap.args["importance"]; hasImportance {
		t.Errorf("store() must not send 'importance'; got args: %v", cap.args)
	}
	pc, ok := cap.args["pattern_confidence"].(float64)
	if !ok {
		t.Fatalf("store() did not send 'pattern_confidence' as float64; got args: %v", cap.args)
	}
	if pc != 0.7 {
		t.Errorf("store() pattern_confidence = %v, want 0.7", pc)
	}
}

// TestInstinctCorrectWritesPatternConfidence verifies that correct() sends
// "pattern_confidence" as a float in the MCP call — not "importance".
func TestInstinctCorrectWritesPatternConfidence(t *testing.T) {
	cap := &capturedArgs{}
	e, cleanup := newCapturingServer(t, "memory_correct", `{}`, cap)
	defer cleanup()

	if err := e.correct(context.Background(), "mem-abc", 0.9); err != nil {
		t.Fatalf("correct() error: %v", err)
	}

	if cap.args == nil {
		t.Fatal("no args captured for memory_correct call")
	}

	// Must use pattern_confidence, not importance.
	if _, hasImportance := cap.args["importance"]; hasImportance {
		t.Errorf("correct() must not send 'importance'; got args: %v", cap.args)
	}
	pc, ok := cap.args["pattern_confidence"].(float64)
	if !ok {
		t.Fatalf("correct() did not send 'pattern_confidence' as float64; got args: %v", cap.args)
	}
	if pc != 0.9 {
		t.Errorf("correct() pattern_confidence = %v, want 0.9", pc)
	}
}

// TestInstinctReadsPatternConfidence verifies that recall() reads
// pattern_confidence from the response when present.
func TestInstinctReadsPatternConfidence(t *testing.T) {
	// Simulate a response where the memory has pattern_confidence set.
	resp := `{"memories":[{"id":"mem-pc-2","pattern_confidence":0.7,"importance":0,"tags":["instinct","sig-pc-read"]}]}`
	cap := &capturedArgs{}
	e, cleanup := newCapturingServer(t, "memory_recall", resp, cap)
	defer cleanup()

	r, err := e.recall(context.Background(), "sig-pc-read", "proj1")
	if err != nil {
		t.Fatalf("recall() error: %v", err)
	}
	if r == nil {
		t.Fatal("recall() returned nil, want match")
	}
	if r.id != "mem-pc-2" {
		t.Errorf("recall() id = %q, want mem-pc-2", r.id)
	}
	if r.confidence != 0.7 {
		t.Errorf("recall() confidence = %v, want 0.7 (from pattern_confidence)", r.confidence)
	}
}

// TestMemoryStorePatternConfidenceExplicitZero verifies that store() sends
// pattern_confidence=0.0 explicitly — distinct from an absent field — so
// that Engram can distinguish "no confidence supplied" from "confidence is
// zero".  Ref: #726 (R1.N3).
func TestMemoryStorePatternConfidenceExplicitZero(t *testing.T) {
	cap := &capturedArgs{}
	e, cleanup := newCapturingServer(t, "memory_store", `{"id":"mem-zero"}`, cap)
	defer cleanup()

	p := Pattern{
		Type:         "workflow",
		Description:  "zero confidence test",
		Domain:       "git",
		Evidence:     "seen 1x",
		TagSignature: "sig-zero",
	}
	if _, err := e.store(context.Background(), p, 0.0, "proj1"); err != nil {
		t.Fatalf("store() error: %v", err)
	}
	if cap.args == nil {
		t.Fatal("no args captured for memory_store call")
	}
	// pattern_confidence must be present and exactly 0.0, not absent.
	raw, exists := cap.args["pattern_confidence"]
	if !exists {
		t.Fatal("store() with confidence=0.0 must send pattern_confidence field (distinct from absent)")
	}
	pc, ok := raw.(float64)
	if !ok {
		t.Fatalf("pattern_confidence type = %T, want float64", raw)
	}
	if pc != 0.0 {
		t.Errorf("pattern_confidence = %v, want 0.0", pc)
	}
}

// TestInstinctBackwardCompatNilPatternConfidence verifies that recall()
// gracefully handles old records that have no pattern_confidence field
// by falling back to importance.
func TestInstinctBackwardCompatNilPatternConfidence(t *testing.T) {
	// Legacy response: no pattern_confidence field; importance is 2 (integer).
	resp := `{"memories":[{"id":"mem-legacy","importance":2,"tags":["instinct","sig-legacy"]}]}`
	cap := &capturedArgs{}
	e, cleanup := newCapturingServer(t, "memory_recall", resp, cap)
	defer cleanup()

	r, err := e.recall(context.Background(), "sig-legacy", "proj1")
	if err != nil {
		t.Fatalf("recall() error: %v", err)
	}
	if r == nil {
		t.Fatal("recall() returned nil for legacy record, want match")
	}
	if r.id != "mem-legacy" {
		t.Errorf("recall() id = %q, want mem-legacy", r.id)
	}
	// Fallback: importance=2 is read as float64 from JSON.
	if r.confidence != 2.0 {
		t.Errorf("recall() confidence = %v, want 2.0 (legacy fallback via importance)", r.confidence)
	}
}
