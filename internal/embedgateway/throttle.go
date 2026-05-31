package embedgateway

import "sync"

type ThrottleStats struct {
	AcquiredConns int32
	MaxConns      int32
}

type AdaptiveThrottle struct {
	mu             sync.Mutex
	concurrency    int
	minConcurrency int
	maxConcurrency int
}

func NewAdaptiveThrottle(maxConcurrency int) *AdaptiveThrottle {
	if maxConcurrency < 1 {
		maxConcurrency = 1
	}
	return &AdaptiveThrottle{
		concurrency:    maxConcurrency,
		minConcurrency: 1,
		maxConcurrency: maxConcurrency,
	}
}

func (t *AdaptiveThrottle) Update(stat ThrottleStats) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if stat.MaxConns <= 0 {
		return
	}
	utilisation := float64(stat.AcquiredConns) / float64(stat.MaxConns)
	switch {
	case utilisation > 0.80:
		t.concurrency = t.minConcurrency
	case utilisation > 0.60:
		t.concurrency = max(t.concurrency-1, t.minConcurrency)
	case utilisation < 0.30:
		t.concurrency = min(t.concurrency+1, t.maxConcurrency)
	}
}

func (t *AdaptiveThrottle) Concurrency() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.concurrency
}
