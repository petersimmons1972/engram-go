package embed

// Tests for EmbedTokenBucket — Pillar 1B defense-in-depth rate limiting (issue #611).
//
// EmbedTokenBucket, NewEmbedTokenBucket, and ErrEmbedRateLimited do NOT exist yet.
// These tests will FAIL TO COMPILE until the implementation is added to
// internal/embed/token_bucket.go. Compilation failure == red phase for Go.

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestTokenBucket_AllowsCallsUnderRate verifies that calls within the bucket's
// capacity all succeed immediately.
func TestTokenBucket_AllowsCallsUnderRate(t *testing.T) {
	b := NewEmbedTokenBucket(10.0, 10) // rate=10/s, cap=10
	for i := 0; i < 5; i++ {
		require.NoError(t, b.TryConsume(), "call %d should be allowed under capacity", i)
	}
}

// TestTokenBucket_BlocksCallsOverRate_ReturnsErrRateLimited verifies that once
// the bucket is drained, TryConsume returns ErrEmbedRateLimited.
func TestTokenBucket_BlocksCallsOverRate_ReturnsErrRateLimited(t *testing.T) {
	b := NewEmbedTokenBucket(1.0, 2) // rate=1/s, cap=2
	// Drain the bucket.
	_ = b.TryConsume()
	_ = b.TryConsume()
	// Next call should fail.
	err := b.TryConsume()
	require.Error(t, err)
	require.ErrorIs(t, err, ErrEmbedRateLimited)
}

// TestTokenBucket_RefillsOverTime verifies that the bucket replenishes tokens
// at the configured rate so callers can retry after a short wait.
func TestTokenBucket_RefillsOverTime(t *testing.T) {
	b := NewEmbedTokenBucket(100.0, 1) // rate=100/s, cap=1
	_ = b.TryConsume()                 // drain
	time.Sleep(20 * time.Millisecond)  // wait for at least one refill at 100/s
	require.NoError(t, b.TryConsume(), "bucket should have refilled after 20ms at 100/s")
}

// TestTokenBucket_NilBucket_AlwaysAllows verifies that a nil *EmbedTokenBucket
// is a safe no-op — TryConsume always returns nil so callers don't need to
// nil-check before calling it.
func TestTokenBucket_NilBucket_AlwaysAllows(t *testing.T) {
	var b *EmbedTokenBucket
	require.NoError(t, b.TryConsume(), "nil bucket must always allow (no-op)")
}

// TestTokenBucket_ConcurrentConsume_NoRace verifies that concurrent calls to
// TryConsume do not cause data races. Run with -race to catch violations.
func TestTokenBucket_ConcurrentConsume_NoRace(t *testing.T) {
	b := NewEmbedTokenBucket(1000.0, 1000)
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = b.TryConsume()
		}()
	}
	wg.Wait()
}

// TestTokenBucket_ZeroCapacity_AlwaysRateLimits verifies the edge case where
// capacity is zero: every call should be rate-limited immediately.
func TestTokenBucket_ZeroCapacity_AlwaysRateLimits(t *testing.T) {
	b := NewEmbedTokenBucket(10.0, 0) // cap=0
	err := b.TryConsume()
	require.Error(t, err)
	require.ErrorIs(t, err, ErrEmbedRateLimited,
		"zero-capacity bucket must always rate-limit")
}

// TestTokenBucket_RateOnePerSecond_DrainRefillCycle exercises a full drain-and-refill
// cycle at a slow rate to confirm token accumulation.
func TestTokenBucket_RateOnePerSecond_DrainRefillCycle(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping refill cycle test in short mode")
	}
	b := NewEmbedTokenBucket(10.0, 1) // rate=10/s, cap=1
	_ = b.TryConsume()                // drain
	// At 10 tokens/s each token takes 100ms to replenish.
	time.Sleep(120 * time.Millisecond)
	require.NoError(t, b.TryConsume(), "bucket should have one token after 120ms at 10/s")
	// Immediately draining again should fail.
	err := b.TryConsume()
	require.ErrorIs(t, err, ErrEmbedRateLimited, "second immediate call must be rate limited")
}
