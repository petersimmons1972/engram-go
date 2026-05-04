package mcp

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestEmbedderHealth_CachedWithinTTL(t *testing.T) {
	// Inject a check function that increments a counter on each call.
	checkCount := 0
	check := func(ctx context.Context) (bool, string) {
		checkCount++
		return true, ""
	}

	// Fake time for deterministic testing.
	fakeNow := time.Unix(1000, 0)
	h := NewEmbedderHealth(check, 5*time.Second)
	h.now = func() time.Time { return fakeNow }

	// Call Snapshot 3 times within TTL (all within 1 second of fake time).
	for i := 0; i < 3; i++ {
		ok, reason := h.Snapshot(context.Background())
		if !ok || reason != "" {
			t.Fatalf("call %d: expected ok=true, reason=\"\", got ok=%v, reason=%q", i, ok, reason)
		}
	}

	// Verify check was called exactly once.
	if checkCount != 1 {
		t.Errorf("expected check to be called 1 time, got %d", checkCount)
	}
}

func TestEmbedderHealth_RefreshesAfterTTL(t *testing.T) {
	checkCount := 0
	check := func(ctx context.Context) (bool, string) {
		checkCount++
		return true, ""
	}

	fakeNow := time.Unix(1000, 0)
	h := NewEmbedderHealth(check, 5*time.Second)
	h.now = func() time.Time { return fakeNow }

	// First call
	h.Snapshot(context.Background())
	if checkCount != 1 {
		t.Errorf("after first call: expected checkCount=1, got %d", checkCount)
	}

	// Advance time past TTL
	fakeNow = time.Unix(1010, 0)
	h.Snapshot(context.Background())
	if checkCount != 2 {
		t.Errorf("after advancing time past TTL: expected checkCount=2, got %d", checkCount)
	}
}

func TestEmbedderHealth_PropagatesUnhealthy(t *testing.T) {
	check := func(ctx context.Context) (bool, string) {
		return false, "litellm_unreachable"
	}

	h := NewEmbedderHealth(check, 5*time.Second)
	ok, reason := h.Snapshot(context.Background())
	if ok {
		t.Errorf("expected ok=false, got ok=%v", ok)
	}
	if reason != "litellm_unreachable" {
		t.Errorf("expected reason=\"litellm_unreachable\", got %q", reason)
	}
}

func TestEmbedderHealth_RaceSafe(t *testing.T) {
	// Use atomic counter for thread-safe counting.
	checkCount := atomic.Int32{}
	check := func(ctx context.Context) (bool, string) {
		checkCount.Add(1)
		return true, ""
	}

	h := NewEmbedderHealth(check, 5*time.Second)

	// Spawn 50 goroutines calling Snapshot concurrently.
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			h.Snapshot(context.Background())
		}()
	}
	wg.Wait()

	// Check should have been called exactly once.
	if checkCount.Load() != 1 {
		t.Errorf("expected check to be called 1 time, got %d", checkCount.Load())
	}
}
