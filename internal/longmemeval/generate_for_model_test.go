package longmemeval_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/petersimmons1972/engram/internal/longmemeval"
)

// ---------------------------------------------------------------------------
// GenerateForModel — model validation and no-retry behaviour
// ---------------------------------------------------------------------------

// TestGenerateForModel_InvalidModel verifies that an unknown model alias
// returns an error containing "disallowed model" (zero retries).
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

// TestGenerateForModel_InvalidModel_NoRetry verifies that with retries=2 and
// an invalid model name, GenerateForModel returns immediately (< 5s) without
// sleeping through the retry backoffs (30s + 60s = 90s wasted).
// Regression guard for the ErrDisallowedModel short-circuit added in #750.
func TestGenerateForModel_InvalidModel_NoRetry(t *testing.T) {
	// Even with retries > 0 the model-rejection error should be returned
	// immediately (no point sleeping and retrying a static validation failure).
	ctx := context.Background()
	start := time.Now()
	_, err := longmemeval.GenerateForModel(ctx, "prompt", "gpt-4o", 2)
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected error for invalid model, got nil")
	}
	if !strings.Contains(err.Error(), "disallowed model") {
		t.Errorf("error = %q, want 'disallowed model'", err.Error())
	}
	if elapsed > 5*time.Second {
		t.Errorf("GenerateForModel with invalid model took %v — retry not short-circuited (want < 5s)", elapsed)
	}
}

// TestGenerateForModel_InvalidModel_ErrIs verifies that errors.Is(err, ErrDisallowedModel)
// returns true, so callers can distinguish permanent model errors from transient ones.
func TestGenerateForModel_InvalidModel_ErrIs(t *testing.T) {
	ctx := context.Background()
	_, err := longmemeval.GenerateForModel(ctx, "prompt", "claude-3-opus-20240229", 0)
	if err == nil {
		t.Fatal("expected error for invalid model, got nil")
	}
	if !errors.Is(err, longmemeval.ErrDisallowedModel) {
		t.Errorf("errors.Is(err, ErrDisallowedModel) = false; err = %q", err.Error())
	}
	// The error string must still contain the model name for diagnostics.
	if !strings.Contains(err.Error(), "claude-3-opus-20240229") {
		t.Errorf("error = %q, want it to contain the model name for diagnostics", err.Error())
	}
}

// TestGenerateForModel_ValidModel_NoModelRejection verifies that GenerateForModel
// with a valid alias ("opus") does NOT return ErrDisallowedModel — it will fail
// because the claude binary is absent in CI, but not for model-validation reasons.
func TestGenerateForModel_ValidModel_NoModelRejection(t *testing.T) {
	ctx := context.Background()
	_, err := longmemeval.GenerateForModel(ctx, "prompt", "opus", 0)
	if err != nil && errors.Is(err, longmemeval.ErrDisallowedModel) {
		t.Errorf("GenerateForModel('opus') must not return ErrDisallowedModel, got: %v", err)
	}
}
