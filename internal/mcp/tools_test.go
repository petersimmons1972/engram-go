package mcp_test

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	internalmcp "github.com/petersimmons1972/engram/internal/mcp"
	"github.com/stretchr/testify/require"
)

// TestEnginePool_ConcurrentFastPath exercises the fast path (RLock) of
// EnginePool.Get under high concurrency. Twenty goroutines hit the same
// pre-warmed project key simultaneously. The race detector validates that
// the atomic lastAccess update inside the fast path is data-race-free.
func TestEnginePool_ConcurrentFastPath(t *testing.T) {
	var factoryCalls atomic.Int64

	pool := internalmcp.NewEnginePool(func(_ context.Context, project string) (*internalmcp.EngineHandle, error) {
		factoryCalls.Add(1)
		return &internalmcp.EngineHandle{}, nil
	})

	ctx := context.Background()

	// Pre-warm: populate the cache so all goroutines below take the fast path.
	primed, err := pool.Get(ctx, "test-project")
	require.NoError(t, err)
	require.NotNil(t, primed)

	const goroutines = 20
	results := make([]*internalmcp.EngineHandle, goroutines)
	errs := make([]error, goroutines)

	var wg sync.WaitGroup
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		i := i
		go func() {
			defer wg.Done()
			results[i], errs[i] = pool.Get(ctx, "test-project")
		}()
	}
	wg.Wait()

	// All 20 calls must succeed and return the same pointer as the primed handle.
	for i := 0; i < goroutines; i++ {
		require.NoError(t, errs[i], "goroutine %d returned an error", i)
		require.Same(t, primed, results[i], "goroutine %d returned a different handle", i)
	}

	// Factory must have been called exactly once — during the pre-warm step.
	require.Equal(t, int64(1), factoryCalls.Load())
}

func TestEnginePool_GetOrCreate_SameProject_SameInstance(t *testing.T) {
	calls := 0
	pool := internalmcp.NewEnginePool(func(_ context.Context, project string) (*internalmcp.EngineHandle, error) {
		calls++
		return &internalmcp.EngineHandle{}, nil
	})

	ctx := context.Background()
	h1, err := pool.Get(ctx, "proj-a")
	require.NoError(t, err)
	h2, err := pool.Get(ctx, "proj-a")
	require.NoError(t, err)

	require.Same(t, h1, h2)
	require.Equal(t, 1, calls)
}

// TestPoolGet_FactoryTimeout verifies that Pool.Get() does not block indefinitely
// when the factory hangs. The factory blocks until its context is cancelled. With
// the 10s internal timeout, Get() should return an error well within the 15s test
// deadline rather than blocking for the duration of the outer context.
func TestPoolGet_FactoryTimeout(t *testing.T) {
	// This channel lets us signal when the factory has started so we know
	// the singleflight is in progress before we start measuring.
	factoryStarted := make(chan struct{})

	pool := internalmcp.NewEnginePool(func(ctx context.Context, _ string) (*internalmcp.EngineHandle, error) {
		// Signal that the factory has been entered.
		close(factoryStarted)
		// Block until our context is cancelled — simulates a hung DB migration.
		<-ctx.Done()
		return nil, ctx.Err()
	})

	// The outer context has a generous deadline so it does not mask the fix.
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Run Get in a goroutine so we can measure how long it takes.
	type result struct {
		h   *internalmcp.EngineHandle
		err error
	}
	done := make(chan result, 1)
	go func() {
		h, err := pool.Get(ctx, "slow-project")
		done <- result{h, err}
	}()

	// Wait until the factory is executing so we know the slow path is active.
	select {
	case <-factoryStarted:
	case <-time.After(5 * time.Second):
		t.Fatal("factory never started within 5s")
	}

	// The factory is now blocked. Get() must return an error within the 10s
	// internal timeout, not after the 30s outer context expires.
	select {
	case r := <-done:
		require.Error(t, r.err, "expected an error when factory times out")
		require.Nil(t, r.h, "expected nil handle on timeout")
	case <-time.After(15 * time.Second):
		t.Fatal("Pool.Get() blocked for 15s — factory timeout is not working")
	}
}
