package mcp_test

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"

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
