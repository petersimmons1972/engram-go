package mcp

import (
	"context"
	"sync"
	"time"
)

// EmbedderHealth probes the configured LiteLLM embedder and caches the result
// for a specified TTL. The cached result is returned until the TTL expires,
// at which point a fresh probe is made.
type EmbedderHealth struct {
	mu           sync.Mutex
	check        func(ctx context.Context) (ok bool, reason string)
	ttl          time.Duration
	lastCheck    time.Time
	cachedOK     bool
	cachedReason string
	now          func() time.Time // injectable for tests; defaults to time.Now
}

// NewEmbedderHealth constructs an EmbedderHealth probe with a default TTL of 5 seconds.
// The check function is called once per TTL interval to determine embedder health.
func NewEmbedderHealth(check func(ctx context.Context) (bool, string), ttl time.Duration) *EmbedderHealth {
	return &EmbedderHealth{
		check:   check,
		ttl:     ttl,
		cachedOK: true, // optimistic: assume healthy until proven otherwise
		now:     time.Now,
	}
}

// Snapshot returns the cached health status if it is still fresh (within TTL);
// otherwise it runs the check function, updates the cache, and returns the result.
// Snapshot is safe to call concurrently.
func (h *EmbedderHealth) Snapshot(ctx context.Context) (ok bool, reason string) {
	h.mu.Lock()
	defer h.mu.Unlock()

	now := h.now()

	// If cache is fresh, return it immediately.
	if !h.lastCheck.IsZero() && now.Sub(h.lastCheck) < h.ttl {
		return h.cachedOK, h.cachedReason
	}

	// Cache expired or not yet populated; run the check.
	ok, reason = h.check(ctx)

	// Update cache.
	h.lastCheck = now
	h.cachedOK = ok
	h.cachedReason = reason

	return ok, reason
}
