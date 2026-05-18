package search_test

// Tests for SearchEngine.Correct pattern_confidence passthrough.
// Track E1-FIX — Blocker 3: SearchEngine.Correct had 0.0% function coverage.
//
// Uses the stubBackend defined in engine_bench_test.go (same package).
// A captureCorrectBackend embeds stubBackend and overrides UpdateMemory to
// capture the patternConfidence argument.

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/petersimmons1972/engram/internal/db"
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
// SearchEngine.Correct -> UpdateMemory.
func TestEngineCorrectNilPatternConfidencePassthrough(t *testing.T) {
	engine, cap := newCaptureEngine(t)

	id := types.NewMemoryID()
	_, err := engine.Correct(context.Background(), id, nil, nil, nil, nil)
	require.NoError(t, err)

	require.True(t, cap.pcSet, "UpdateMemory must be called")
	require.Nil(t, cap.capturedPC,
		"nil patternConfidence must reach UpdateMemory as nil -- 'do not touch' semantics")
}

// errUpdateBackend wraps stubBackend and returns a configurable error from UpdateMemory.
type errUpdateBackend struct {
	*stubBackend
	updateErr error
}

func (b *errUpdateBackend) UpdateMemory(
	_ context.Context, _ string, _ *string, _ []string, _ *int, _ *float64,
) (*types.Memory, error) {
	return nil, b.updateErr
}

// newErrUpdateEngine builds a SearchEngine that returns an error from UpdateMemory.
func newErrUpdateEngine(t *testing.T, updateErr error) *search.SearchEngine {
	t.Helper()
	stub := newStubBackend(0, 0)
	back := &errUpdateBackend{stubBackend: stub, updateErr: updateErr}
	ctx := context.Background()
	engine := search.New(ctx, back, &fakeClient{dims: 768}, "test",
		"http://ollama-test:11434", "", false, nil, 0)
	t.Cleanup(engine.Close)
	return engine
}

// TestEngineCorrectUpdateMemoryError verifies that an error from UpdateMemory
// is propagated back to the caller (covers the err-check at lines 1104-1106).
func TestEngineCorrectUpdateMemoryError(t *testing.T) {
	sentinel := errors.New("simulated db error")
	engine := newErrUpdateEngine(t, sentinel)

	id := types.NewMemoryID()
	_, err := engine.Correct(context.Background(), id, nil, nil, nil, nil)
	require.ErrorIs(t, err, sentinel, "UpdateMemory error must be propagated by Correct")
}

// searchNoopTx is a do-nothing db.Tx for the search test package.
type searchNoopTx struct{}

func (searchNoopTx) Commit(_ context.Context) error   { return nil }
func (searchNoopTx) Rollback(_ context.Context) error { return nil }

var _ db.Tx = searchNoopTx{}

// errBeginBackend wraps captureCorrectBackend and returns an error from Begin,
// exercising the tx-open failure path in Correct.
type errBeginBackend struct {
	captureCorrectBackend
	beginErr error
}

func (b *errBeginBackend) Begin(_ context.Context) (db.Tx, error) {
	return nil, b.beginErr
}

// newErrBeginEngine builds a SearchEngine whose Begin returns an error.
func newErrBeginEngine(t *testing.T, beginErr error) *search.SearchEngine {
	t.Helper()
	stub := newStubBackend(0, 0)
	back := &errBeginBackend{
		captureCorrectBackend: captureCorrectBackend{stubBackend: stub},
		beginErr:              beginErr,
	}
	ctx := context.Background()
	engine := search.New(ctx, back, &fakeClient{dims: 768}, "test",
		"http://ollama-test:11434", "", false, nil, 0)
	t.Cleanup(engine.Close)
	return engine
}

// TestEngineCorrectBeginError verifies that a failure to open a transaction after
// re-chunking propagates back to the caller (covers the Begin err-check in Correct).
func TestEngineCorrectBeginError(t *testing.T) {
	sentinel := errors.New("simulated tx begin error")
	engine := newErrBeginEngine(t, sentinel)

	id := types.NewMemoryID()
	newContent := "content triggering rechunk then begin failure"
	_, err := engine.Correct(context.Background(), id, &newContent, nil, nil, nil)
	require.ErrorIs(t, err, sentinel, "Begin error must be propagated by Correct")
}

// rechunkCaptureBackend wraps captureCorrectBackend and overrides Begin to return
// a working no-op transaction, and captures StoreChunksTx calls.
type rechunkCaptureBackend struct {
	captureCorrectBackend
	mu              sync.Mutex
	chunksStored    []*types.Chunk
	deleteCallCount int
}

func (b *rechunkCaptureBackend) Begin(_ context.Context) (db.Tx, error) {
	return searchNoopTx{}, nil
}

func (b *rechunkCaptureBackend) DeleteChunksForMemoryTx(_ context.Context, _ db.Tx, _ string) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.deleteCallCount++
	return nil
}

func (b *rechunkCaptureBackend) StoreChunksTx(_ context.Context, _ db.Tx, chunks []*types.Chunk) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.chunksStored = append(b.chunksStored, chunks...)
	return nil
}

// newRechunkEngine builds a SearchEngine backed by rechunkCaptureBackend.
func newRechunkEngine(t *testing.T) (*search.SearchEngine, *rechunkCaptureBackend) {
	t.Helper()
	stub := newStubBackend(0, 0)
	back := &rechunkCaptureBackend{
		captureCorrectBackend: captureCorrectBackend{stubBackend: stub},
	}
	ctx := context.Background()
	engine := search.New(ctx, back, &fakeClient{dims: 768}, "test",
		"http://ollama-test:11434", "", false, nil, 0)
	t.Cleanup(engine.Close)
	return engine, back
}

// TestEngineCorrectWithNonNilContentRechunks verifies that passing a non-nil
// content to SearchEngine.Correct exercises the re-chunking branch: UpdateMemory
// is called, old chunks are deleted in a transaction, new chunks are written,
// and patternConfidence is still passed through correctly on the re-chunking path.
func TestEngineCorrectWithNonNilContentRechunks(t *testing.T) {
	engine, back := newRechunkEngine(t)

	id := types.NewMemoryID()
	newContent := "this is the corrected memory content for re-chunking test"
	conf := 0.5

	mem, err := engine.Correct(context.Background(), id, &newContent, nil, nil, &conf)
	require.NoError(t, err)
	require.NotNil(t, mem, "Correct must return the updated memory")

	require.True(t, back.captureCorrectBackend.pcSet, "UpdateMemory must be called")
	require.NotNil(t, back.captureCorrectBackend.capturedPC,
		"patternConfidence must be non-nil when a value is passed on the re-chunking path")
	require.InDelta(t, 0.5, *back.captureCorrectBackend.capturedPC, 1e-9,
		"patternConfidence must be passed through unchanged on the re-chunking path")

	back.mu.Lock()
	deleteCount := back.deleteCallCount
	back.mu.Unlock()
	require.Equal(t, 1, deleteCount,
		"DeleteChunksForMemoryTx must be called exactly once in the re-chunking branch")
}

// TestEngineCorrectPatternConfidenceJSONNull verifies that nil patternConfidence
// (representing JSON null from the MCP layer) is treated as "do not touch":
// UpdateMemory receives nil, no error returned.
func TestEngineCorrectPatternConfidenceJSONNull(t *testing.T) {
	engine, cap := newCaptureEngine(t)

	id := types.NewMemoryID()
	_, err := engine.Correct(context.Background(), id, nil, nil, nil, nil)
	require.NoError(t, err)

	require.True(t, cap.pcSet, "UpdateMemory must be called")
	require.Nil(t, cap.capturedPC,
		"nil (JSON null) patternConfidence must reach UpdateMemory as nil")
}
