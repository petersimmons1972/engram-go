package reembed_test

import (
	"context"
	"testing"
	"time"

	"github.com/petersimmons1972/engram/internal/reembed"
)

// TestNewGlobalReembedder verifies the constructor does not panic and returns
// a non-nil worker when given nil pool and embedder (unit test, no real DB).
func TestNewGlobalReembedder(t *testing.T) {
	w := reembed.NewGlobalReembedder(nil, nil, 10, 5*time.Second)
	if w == nil {
		t.Fatal("NewGlobalReembedder returned nil")
	}
}

// TestGlobalReembedderWaitBeforeStart verifies that calling Wait() before
// Start() returns immediately rather than blocking forever.
func TestGlobalReembedderWaitBeforeStart(t *testing.T) {
	w := reembed.NewGlobalReembedder(nil, nil, 10, 5*time.Second)
	done := make(chan struct{})
	go func() {
		w.Wait()
		close(done)
	}()
	select {
	case <-done:
		// Good — Wait() returned without blocking.
	case <-time.After(500 * time.Millisecond):
		t.Error("Wait() blocked before Start() was called")
	}
}

// TestGlobalReembedderStartStop verifies that the worker goroutine stops cleanly
// when the context is cancelled, and Wait() returns promptly.
func TestGlobalReembedderStartStop(t *testing.T) {
	w := reembed.NewGlobalReembedder(nil, nil, 10, 50*time.Millisecond)
	ctx, cancel := context.WithCancel(context.Background())

	w.Start(ctx)

	// Cancel after a short delay.
	time.Sleep(20 * time.Millisecond)
	cancel()

	done := make(chan struct{})
	go func() {
		w.Wait()
		close(done)
	}()
	select {
	case <-done:
		// Good — goroutine exited.
	case <-time.After(2 * time.Second):
		t.Error("GlobalReembedder did not stop within 2s after context cancel")
	}
}

// TestGlobalReembedderStartIsIdempotent verifies calling Start() twice does not
// launch two goroutines (would cause double-processing and data races).
func TestGlobalReembedderStartIsIdempotent(t *testing.T) {
	w := reembed.NewGlobalReembedder(nil, nil, 10, 50*time.Millisecond)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	w.Start(ctx)
	w.Start(ctx) // second call must be a no-op

	cancel()
	w.Wait() // must return; if two goroutines were started, this may block or race
}
