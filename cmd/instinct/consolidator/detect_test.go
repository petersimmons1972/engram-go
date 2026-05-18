package consolidator_test

import (
	"context"
	"errors"
	"testing"

	"github.com/petersimmons1972/engram/cmd/instinct/consolidator"
	"github.com/petersimmons1972/engram/internal/instinctllm"
)

// mockLLMClient implements instinctllm.LLMClient for testing.
type mockLLMClient struct {
	response string
	err      error
	called   int
}

func (m *mockLLMClient) Complete(_ context.Context, _, _ string) (string, error) {
	m.called++
	return m.response, m.err
}

// failIfCalledClient causes the test to fail if Complete is invoked.
type failIfCalledClient struct {
	t *testing.T
}

func (f *failIfCalledClient) Complete(_ context.Context, _, _ string) (string, error) {
	f.t.Helper()
	f.t.Fatal("Complete() called but should not have been (zero events)")
	return "", nil
}

// Compile-time assertion that both mocks satisfy the interface.
var _ instinctllm.LLMClient = (*mockLLMClient)(nil)
var _ instinctllm.LLMClient = (*failIfCalledClient)(nil)

// TestDetectHappyPath: mock returns golden pattern JSON; Detect returns
// parsed []Pattern.
func TestDetectHappyPath(t *testing.T) {
	golden := `[{"type":"workflow","description":"User runs tests after edits","domain":"testing","evidence":"edit then test 3x","tag_signature":"sig-edit-test"}]`
	mock := &mockLLMClient{response: golden}
	events := []consolidator.Event{
		{Timestamp: "2026-01-01T00:00:00Z", ToolName: "Edit", ToolOutputSummary: "saved", ExitStatus: 0},
	}

	patterns, err := consolidator.Detect(context.Background(), mock, events)
	if err != nil {
		t.Fatalf("Detect() error: %v", err)
	}
	if len(patterns) != 1 {
		t.Fatalf("Detect() = %d patterns, want 1", len(patterns))
	}
	if patterns[0].TagSignature != "sig-edit-test" {
		t.Errorf("TagSignature = %q, want sig-edit-test", patterns[0].TagSignature)
	}
	if mock.called != 1 {
		t.Errorf("LLMClient.Complete called %d times, want 1", mock.called)
	}
}

// TestDetectEmptyEvents: zero events → empty patterns; LLM client NOT called.
func TestDetectEmptyEvents(t *testing.T) {
	c := &failIfCalledClient{t: t}
	patterns, err := consolidator.Detect(context.Background(), c, nil)
	if err != nil {
		t.Fatalf("Detect() error on empty events: %v", err)
	}
	if len(patterns) != 0 {
		t.Errorf("Detect() = %d patterns on zero events, want 0", len(patterns))
	}
}

// TestDetectInvalidJSON: mock returns garbage; Detect returns empty patterns
// + nil error (graceful degradation; consolidator must not crash).
func TestDetectInvalidJSON(t *testing.T) {
	mock := &mockLLMClient{response: "this is not json at all"}
	events := []consolidator.Event{{ToolName: "Bash"}}

	patterns, err := consolidator.Detect(context.Background(), mock, events)
	if err != nil {
		t.Errorf("Detect() returned error on invalid JSON, want nil: %v", err)
	}
	if len(patterns) != 0 {
		t.Errorf("Detect() = %d patterns on invalid JSON, want 0", len(patterns))
	}
}

// TestDetectFiltersInvalidPatterns: mock returns mix of valid + invalid
// (missing fields, wrong type); only valid ones returned.
func TestDetectFiltersInvalidPatterns(t *testing.T) {
	mixed := `[
		{"type":"workflow","description":"ok","domain":"git","evidence":"x","tag_signature":"sig-ok"},
		{"type":"correction","description":"bad","domain":"bash","evidence":"y"},
		{"type":"unknown_type","description":"also bad","domain":"git","evidence":"z","tag_signature":"sig-bad-type"}
	]`
	mock := &mockLLMClient{response: mixed}
	events := []consolidator.Event{{ToolName: "Bash"}}

	patterns, err := consolidator.Detect(context.Background(), mock, events)
	if err != nil {
		t.Fatalf("Detect() error: %v", err)
	}
	if len(patterns) != 1 {
		t.Errorf("Detect() = %d patterns, want 1 (invalid filtered)", len(patterns))
	}
	if len(patterns) > 0 && patterns[0].TagSignature != "sig-ok" {
		t.Errorf("TagSignature = %q, want sig-ok", patterns[0].TagSignature)
	}
}

// TestDetectStripsMarkdownFences: belt-and-suspenders stripping in domain
// layer (Olla strips at client; Anthropic might also wrap; this is defensive).
func TestDetectStripsMarkdownFences(t *testing.T) {
	fenced := "```json\n[{\"type\":\"workflow\",\"description\":\"test\",\"domain\":\"git\",\"evidence\":\"e\",\"tag_signature\":\"sig-t\"}]\n```"
	mock := &mockLLMClient{response: fenced}
	events := []consolidator.Event{{ToolName: "Edit"}}

	patterns, err := consolidator.Detect(context.Background(), mock, events)
	if err != nil {
		t.Fatalf("Detect() error: %v", err)
	}
	if len(patterns) != 1 {
		t.Errorf("Detect() = %d patterns after fence strip, want 1", len(patterns))
	}
}

// TestDetectLLMError: mock returns error; Detect returns empty patterns + the
// error (caller decides what to do).
func TestDetectLLMError(t *testing.T) {
	sentinel := errors.New("LLM exploded")
	mock := &mockLLMClient{err: sentinel}
	events := []consolidator.Event{{ToolName: "Bash"}}

	patterns, err := consolidator.Detect(context.Background(), mock, events)
	if err == nil {
		t.Error("Detect() should propagate LLM error")
	}
	if !errors.Is(err, sentinel) {
		t.Errorf("Detect() error = %v, want sentinel %v", err, sentinel)
	}
	if len(patterns) != 0 {
		t.Errorf("Detect() = %d patterns on LLM error, want 0", len(patterns))
	}
}
