package embed

import (
	"errors"
	"sync"
	"time"
)

// ErrEmbedRateLimited is returned by EmbedTokenBucket.TryConsume when the
// per-project embed rate budget is exhausted. Callers should fall back to
// BM25 text search instead of waiting.
var ErrEmbedRateLimited = errors.New("embed: rate limited — falling back to BM25")

// EmbedTokenBucket is a simple token bucket rate limiter for embed calls.
// One bucket per project prevents a single project's burst from saturating
// the GPU and timing out other projects' calls.
type EmbedTokenBucket struct {
	mu       sync.Mutex
	tokens   float64
	cap      float64
	ratePerS float64
	lastFill time.Time
}

// NewEmbedTokenBucket creates a bucket with the given sustained rate (tokens/second)
// and burst capacity. Both must be > 0; a zero/negative rate disables rate limiting.
func NewEmbedTokenBucket(ratePerSecond, capacity float64) *EmbedTokenBucket {
	return &EmbedTokenBucket{
		tokens:   capacity,
		cap:      capacity,
		ratePerS: ratePerSecond,
		lastFill: time.Now(),
	}
}

// TryConsume attempts to consume one token. Returns ErrEmbedRateLimited when
// the bucket is empty. Safe for concurrent use. A nil receiver always succeeds.
func (b *EmbedTokenBucket) TryConsume() error {
	if b == nil {
		return nil
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.refill()
	if b.tokens < 1 {
		return ErrEmbedRateLimited
	}
	b.tokens--
	return nil
}

func (b *EmbedTokenBucket) refill() {
	now := time.Now()
	elapsed := now.Sub(b.lastFill).Seconds()
	b.tokens += elapsed * b.ratePerS
	if b.tokens > b.cap {
		b.tokens = b.cap
	}
	b.lastFill = now
}
