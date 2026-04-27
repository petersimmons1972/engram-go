package mcp

// sweep_episodes_test.go — tests for sweepStaleEpisodes / runEpisodeSweep.
//
// These tests run entirely in-process: no real database, no HTTP server.
// They exercise the sweeper logic by injecting a fake backend that records
// CloseStaleEpisodes calls.

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/petersimmons1972/engram/internal/search"
)

// ---------------------------------------------------------------------------
// sweepTrackingBackend tracks CloseStaleEpisodes invocations.
// ---------------------------------------------------------------------------

type sweepTrackingBackend struct {
	noopBackend
	calls    atomic.Int32   // number of CloseStaleEpisodes calls
	lastTTL  atomic.Value   // stores time.Duration of last call
	returnN  int64          // rows to report as affected
	returnErr error         // error to return (nil by default)
}

func (b *sweepTrackingBackend) CloseStaleEpisodes(_ context.Context, olderThan time.Duration) (int64, error) {
	b.calls.Add(1)
	b.lastTTL.Store(olderThan)
	return b.returnN, b.returnErr
}

// newSweepServer builds a *Server with a Config.EpisodeTTL set to the given value.
func newSweepServer(t *testing.T, backend *sweepTrackingBackend, ttl time.Duration) *Server {
	t.Helper()
	factory := func(ctx context.Context, project string) (*EngineHandle, error) {
		engine := search.New(ctx, backend, noopEmbedder{}, project,
			"http://ollama-test:11434", "", false, nil, 0)
		t.Cleanup(engine.Close)
		return &EngineHandle{Engine: engine}, nil
	}
	pool := NewEnginePool(factory)
	return NewServer(pool, Config{EpisodeTTL: ttl})
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

// TestSweepStaleEpisodes_DisabledWhenTTLZero verifies that sweepStaleEpisodes
// returns immediately without starting a ticker when EpisodeTTL == 0.
func TestSweepStaleEpisodes_DisabledWhenTTLZero(t *testing.T) {
	backend := &sweepTrackingBackend{}
	s := newSweepServer(t, backend, 0)

	// sweepStaleEpisodes should exit immediately for TTL=0.
	done := make(chan struct{})
	go func() {
		defer close(done)
		s.sweepStaleEpisodes(context.Background())
	}()

	select {
	case <-done:
		// Exited immediately — correct behaviour.
	case <-time.After(500 * time.Millisecond):
		t.Fatal("sweepStaleEpisodes did not return immediately when EpisodeTTL==0")
	}

	if n := backend.calls.Load(); n != 0 {
		t.Fatalf("expected 0 CloseStaleEpisodes calls with TTL=0; got %d", n)
	}
}

// TestSweepStaleEpisodes_ExitsOnContextCancel verifies that sweepStaleEpisodes
// exits when its context is cancelled.
func TestSweepStaleEpisodes_ExitsOnContextCancel(t *testing.T) {
	backend := &sweepTrackingBackend{}
	// Use a very long ticker interval (1 hour) so it never fires in the test;
	// we just verify cancellation works.
	s := newSweepServer(t, backend, time.Hour)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		s.sweepStaleEpisodes(ctx)
	}()

	cancel()
	select {
	case <-done:
		// Exited after cancel — correct.
	case <-time.After(500 * time.Millisecond):
		t.Fatal("sweepStaleEpisodes did not exit after context cancellation")
	}
}

// TestRunEpisodeSweep_CallsCloseStaleEpisodes verifies that runEpisodeSweep
// calls CloseStaleEpisodes once with the configured TTL.
func TestRunEpisodeSweep_CallsCloseStaleEpisodes(t *testing.T) {
	backend := &sweepTrackingBackend{returnN: 3}
	s := newSweepServer(t, backend, 2*time.Hour)

	s.runEpisodeSweep(context.Background())

	if n := backend.calls.Load(); n != 1 {
		t.Fatalf("expected 1 CloseStaleEpisodes call; got %d", n)
	}

	got, ok := backend.lastTTL.Load().(time.Duration)
	if !ok {
		t.Fatal("lastTTL was not set")
	}
	if got != 2*time.Hour {
		t.Fatalf("CloseStaleEpisodes called with TTL %v; want 2h", got)
	}
}

// TestRunEpisodeSweep_ToleratesBackendError verifies that runEpisodeSweep does
// not panic when CloseStaleEpisodes returns an error.
func TestRunEpisodeSweep_ToleratesBackendError(t *testing.T) {
	backend := &sweepTrackingBackend{returnErr: fmt.Errorf("db offline")}
	s := newSweepServer(t, backend, time.Hour)

	// Must not panic.
	s.runEpisodeSweep(context.Background())

	if n := backend.calls.Load(); n != 1 {
		t.Fatalf("expected 1 CloseStaleEpisodes call despite error; got %d", n)
	}
}
