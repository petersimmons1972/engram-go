package search_test

// Tests for SearchEngine.Correct pattern_confidence passthrough.
// Track E1-FIX — Blocker 3: SearchEngine.Correct had 0.0% function coverage.
//
// Uses the stubBackend defined in engine_bench_test.go (same package).
// A captureCorrectBackend embeds stubBackend and overrides UpdateMemory to
// capture the patternConfidence argument.

import (
	"context"
	"testing"

	"github.com/petersimmons1972/engram/internal/search"
	"github.com/petersimmons1972/engram/internal/types"
	"github.com/stretchr/testify/require"
)

// captureCorrectBackend embeds stubBackend and overrides UpdateMemory to record
// the arguments passed by SearchEngine.Correct.
type captureCorrectBackend struct {
	*stubBackend
	capturedID string
	capturedPC *float64
	pcSet      bool // true even when capturedPC is nil
}

func (b *captureCorrectBackend) UpdateMemory(
	_ context.Context,
	id string,
	_ *string,
	_ []string,
	_ *int,
	pc *float64,
) (*types.Memory, error) {
	b.capturedID = id
	b.pcSet = true
	b.capturedPC = pc
	return &types.Memory{ID: id, Content: "updated", Project: "test"}, nil
}

// newCaptureEngine builds a SearchEngine backed by captureCorrectBackend.
func newCaptureEngine(t *testing.T) (*search.SearchEngine, *captureCorrectBackend) {
	t.Helper()
	stub := newStubBackend(0, 0)
	cap := &captureCorrectBackend{stubBackend: stub}
	ctx := context.Background()
	engine := search.New(ctx, cap, &fakeClient{dims: 768}, "test",
		"http://ollama-test:11434", "", false, nil, 0)
	t.Cleanup(engine.Close)
	return engine, cap
}

// TestEngineCorrectPassesPatternConfidence verifies that a non-nil
// patternConfidence passed to SearchEngine.Correct reaches UpdateMemory.
func TestEngineCorrectPassesPatternConfidence(t *testing.T) {
	engine, cap := newCaptureEngine(t)

	id := types.NewMemoryID()
	val := 0.73
	_, err := engine.Correct(context.Background(), id, nil, nil, nil, &val)
	require.NoError(t, err)

	require.True(t, cap.pcSet, "UpdateMemory must be called")
	require.NotNil(t, cap.capturedPC, "patternConfidence must be non-nil when a value is passed")
	require.InDelta(t, 0.73, *cap.capturedPC, 1e-9,
		"patternConfidence must be passed through unchanged")
}

// TestEngineCorrectNilPatternConfidencePassthrough verifies that a nil
// patternConfidence ("do not touch this field") is preserved as nil through
// SearchEngine.Correct → UpdateMemory.
func TestEngineCorrectNilPatternConfidencePassthrough(t *testing.T) {
	engine, cap := newCaptureEngine(t)

	id := types.NewMemoryID()
	_, err := engine.Correct(context.Background(), id, nil, nil, nil, nil)
	require.NoError(t, err)

	require.True(t, cap.pcSet, "UpdateMemory must be called")
	require.Nil(t, cap.capturedPC,
		"nil patternConfidence must reach UpdateMemory as nil — 'do not touch' semantics")
}
