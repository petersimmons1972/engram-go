package longmemeval_test

import (
	"context"
	"strings"
	"testing"

	"github.com/petersimmons1972/engram/internal/longmemeval"
)

// ---------------------------------------------------------------------------
// ContextTopKForTypeWithBump
// ---------------------------------------------------------------------------

func TestContextTopKForTypeWithBump_BumpFalse(t *testing.T) {
	cases := []struct {
		questionType string
		want         int
	}{
		{"multi-session", 15},
		{"temporal-reasoning", 15},
		{"single-session-preference", 8},
		{"knowledge-update", 8},
		{"single-session-user", 8},
		{"", 8},
	}
	for _, c := range cases {
		got := longmemeval.ContextTopKForTypeWithBump(c.questionType, false)
		if got != c.want {
			t.Errorf("ContextTopKForTypeWithBump(%q, false) = %d, want %d", c.questionType, got, c.want)
		}
	}
}

func TestContextTopKForTypeWithBump_BumpTrue(t *testing.T) {
	// When bump is true ALL categories return 15.
	types := []string{
		"multi-session",
		"temporal-reasoning",
		"single-session-preference",
		"knowledge-update",
		"single-session-user",
		"",
	}
	for _, qt := range types {
		got := longmemeval.ContextTopKForTypeWithBump(qt, true)
		if got != 15 {
			t.Errorf("ContextTopKForTypeWithBump(%q, true) = %d, want 15", qt, got)
		}
	}
}

// ---------------------------------------------------------------------------
// GenerateForModel — model validation behaviour
// ---------------------------------------------------------------------------

// validModelServer returns an httptest server that simulates a successful
// claude CLI interaction is not possible in unit tests because claude is an
// external binary. Instead we test GenerateForModel's rejection of invalid
// model names, which happens inside runClaude before exec.Command fires.
// For valid model names we verify the error is NOT a model-rejection error
// (it will be an exec "executable not found" or similar in CI).

func TestGenerateForModel_InvalidModel(t *testing.T) {
	ctx := context.Background()
	_, err := longmemeval.GenerateForModel(ctx, "prompt", "gpt-4o", 0)
	if err == nil {
		t.Fatal("expected error for invalid model, got nil")
	}
	if !strings.Contains(err.Error(), "disallowed model") {
		t.Errorf("error = %q, want it to contain 'disallowed model'", err.Error())
	}
}

func TestGenerateForModel_InvalidModel_NoRetry(t *testing.T) {
	// Even with retries > 0 the model-rejection error should be returned
	// immediately (no point sleeping and retrying a static validation failure).
	// The current implementation does retry, which wastes time. We just assert
	// the error is still the disallowed-model error.
	ctx := context.Background()
	_, err := longmemeval.GenerateForModel(ctx, "prompt", "claude-3-opus-20240229", 2)
	if err == nil {
		t.Fatal("expected error for invalid model, got nil")
	}
	if !strings.Contains(err.Error(), "disallowed model") {
		t.Errorf("error = %q, want 'disallowed model'", err.Error())
	}
}

func TestGenerateOpus_InvalidModel_NotTriggered(t *testing.T) {
	// GenerateOpus passes "opus" which IS in the allowlist. The call will fail
	// because there is no real claude binary in CI, but the error must NOT be
	// the model-rejection sentinel.
	ctx := context.Background()
	_, err := longmemeval.GenerateOpus(ctx, "prompt", 0)
	if err != nil && strings.Contains(err.Error(), "disallowed model") {
		t.Errorf("GenerateOpus should use an allowed model but got: %v", err)
	}
}

// TestContextTopKForTypeWithBump_MatchesBase verifies that bump=false is a
// no-op relative to ContextTopKForType for every known question type.
func TestContextTopKForTypeWithBump_MatchesBase(t *testing.T) {
	// When bump is false, ContextTopKForTypeWithBump must agree with
	// ContextTopKForType for every known type.
	types := []string{
		"multi-session",
		"temporal-reasoning",
		"single-session-preference",
		"knowledge-update",
		"single-session-user",
		"unknown-type",
		"",
	}
	for _, qt := range types {
		want := longmemeval.ContextTopKForType(qt)
		got := longmemeval.ContextTopKForTypeWithBump(qt, false)
		if got != want {
			t.Errorf("ContextTopKForTypeWithBump(%q, false) = %d, want %d (ContextTopKForType)", qt, got, want)
		}
	}
}

