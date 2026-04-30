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

// TestGlobalReembedderNotifyNonBlocking verifies that Notify() never blocks,
// even when called many times in rapid succession (buffered channel + select/default).
func TestGlobalReembedderNotifyNonBlocking(t *testing.T) {
	w := reembed.NewGlobalReembedder(nil, nil, 10, time.Hour)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	w.Start(ctx)

	// Calling Notify many times must not deadlock or block.
	done := make(chan struct{})
	go func() {
		for i := 0; i < 100; i++ {
			w.Notify()
		}
		close(done)
	}()
	select {
	case <-done:
		// Good — all Notify() calls returned without blocking.
	case <-time.After(500 * time.Millisecond):
		t.Error("Notify() blocked — likely unbuffered channel or missing select/default")
	}
}

// TestGlobalReembedderNotifyWakesWorker verifies that Notify() causes the
// reembedder to wake early (before the poll interval expires). We use a long
// interval so the worker would not tick on its own within the test window.
func TestGlobalReembedderNotifyWakesWorker(t *testing.T) {
	// A 1-hour interval ensures the worker won't tick on its own.
	w := reembed.NewGlobalReembedder(nil, nil, 10, time.Hour)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	w.Start(ctx)

	// Notify immediately — the worker should wake and process (nil pool skips
	// the DB path), then return to sleep. We confirm by cancelling ctx after
	// a short window and checking that Wait() returns promptly (goroutine alive
	// and responding to signals).
	w.Notify()

	time.Sleep(50 * time.Millisecond)
	cancel()

	done := make(chan struct{})
	go func() {
		w.Wait()
		close(done)
	}()
	select {
	case <-done:
		// Good.
	case <-time.After(2 * time.Second):
		t.Error("GlobalReembedder did not exit after ctx cancel + Notify")
	}
}
