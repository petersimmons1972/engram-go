package mcp

// Tests for EnginePool.WarmProjects — Pillar 2B pre-warming.
//
// WarmProjects does NOT exist yet on EnginePool. These tests will FAIL TO COMPILE
// until the method is added to internal/mcp/pool.go.
// That is the expected red-phase state.

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/petersimmons1972/engram/internal/search"
	"github.com/stretchr/testify/require"
)

// TestWarmProjects_HappyPath_AllProjectsInPool verifies that after WarmProjects
// completes, a subsequent Get for each project is a cache-hit (fast).
func TestWarmProjects_HappyPath_AllProjectsInPool(t *testing.T) {
	pool := newTestNoopPool(t)
	projects := []string{"alpha", "beta", "gamma"}

	pool.WarmProjects(context.Background(), projects, 2) // method doesn't exist yet

	// After warming, Get should be a fast cache-hit for every project.
	for _, p := range projects {
		_, err := pool.Get(context.Background(), p)
		require.NoError(t, err, "project %q should be in pool after WarmProjects", p)
	}
}

// TestWarmProjects_FactoryError_DoesNotPanic verifies that a factory that always
// errors does not cause WarmProjects to panic or block indefinitely.
func TestWarmProjects_FactoryError_DoesNotPanic(t *testing.T) {
	pool := NewEnginePool(func(ctx context.Context, project string) (*EngineHandle, error) {
		return nil, errors.New("factory error")
	})

	// Must not panic or hang.
	done := make(chan struct{})
	go func() {
		defer close(done)
		pool.WarmProjects(context.Background(), []string{"broken"}, 1)
	}()

	select {
	case <-done:
		// completed without panic
	case <-time.After(5 * time.Second):
		t.Fatal("WarmProjects hung for 5s when factory always errors")
	}
}

// TestWarmProjects_CancelledContext_ExitsCleanly verifies that cancelling the
// context causes WarmProjects to exit rather than block on slow factory calls.
func TestWarmProjects_CancelledContext_ExitsCleanly(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	pool := NewEnginePool(func(ctx context.Context, project string) (*EngineHandle, error) {
		// Simulate a slow factory: wait for ctx cancellation.
		<-ctx.Done()
		return nil, ctx.Err()
	})

	cancel() // cancel immediately before WarmProjects is called

	done := make(chan struct{})
	go func() {
		defer close(done)
		pool.WarmProjects(ctx, []string{"a", "b", "c"}, 1)
	}()

	select {
	case <-done:
		// exited cleanly after context cancellation
	case <-time.After(500 * time.Millisecond):
		t.Fatal("WarmProjects did not exit within 500ms after context cancellation")
	}
}

// TestWarmProjects_EmptyList_IsNoop verifies that passing nil or an empty slice
// is a safe no-op — no panic, no hang.
func TestWarmProjects_EmptyList_IsNoop(t *testing.T) {
	pool := newTestNoopPool(t)
	require.NotPanics(t, func() {
		pool.WarmProjects(context.Background(), nil, 1)
		pool.WarmProjects(context.Background(), []string{}, 1)
	})
}

// TestWarmProjects_Concurrency_Bounded verifies that WarmProjects respects the
// concurrency parameter and does not deadlock or hang.
func TestWarmProjects_Concurrency_Bounded(t *testing.T) {
	const concurrency = 2
	const numProjects = 6

	var mu sync.Mutex
	inFlight := 0
	maxSeen := 0

	pool := NewEnginePool(func(ctx context.Context, project string) (*EngineHandle, error) {
		mu.Lock()
		inFlight++
		if inFlight > maxSeen {
			maxSeen = inFlight
		}
		mu.Unlock()

		// Small delay to let concurrency accumulate.
		time.Sleep(10 * time.Millisecond)

		mu.Lock()
		inFlight--
		mu.Unlock()

		return &EngineHandle{Engine: &search.SearchEngine{}}, nil
	})

	projectList := make([]string, numProjects)
	for i := range projectList {
		projectList[i] = "proj" + string(rune('a'+i))
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		pool.WarmProjects(context.Background(), projectList, concurrency)
	}()

	select {
	case <-done:
		mu.Lock()
		seen := maxSeen
		mu.Unlock()
		require.LessOrEqual(t, seen, concurrency,
			"WarmProjects must not exceed concurrency=%d goroutines simultaneously", concurrency)
	case <-time.After(5 * time.Second):
		t.Fatal("WarmProjects did not complete within 5s")
	}
}
