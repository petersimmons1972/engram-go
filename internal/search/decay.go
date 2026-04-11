package search

import (
	"context"
	"log/slog"
	"time"

	"github.com/petersimmons1972/engram/internal/db"
)

const (
	decayFactor          = 0.95
	defaultDecayInterval = 8 * time.Hour
)

// DecayWorker is a background goroutine that applies spaced-repetition decay to
// memories whose next_review_at timestamp has passed. It multiplies
// dynamic_importance by decayFactor (0.95) on each qualifying row and advances
// next_review_at by retrieval_interval_hrs. This implements the "background decay"
// half of Feature 2 (Adaptive Importance via Spaced Repetition).
type DecayWorker struct {
	backend  db.Backend
	project  string
	interval time.Duration
	cancel   context.CancelFunc
	done     chan struct{}
}

// NewDecayWorker creates a DecayWorker. interval is how often the decay pass runs;
// pass 0 to use the default (8 hours). The worker does not start until Start() is called.
func NewDecayWorker(backend db.Backend, project string, interval time.Duration) *DecayWorker {
	if interval <= 0 {
		interval = defaultDecayInterval
	}
	return &DecayWorker{
		backend:  backend,
		project:  project,
		interval: interval,
		done:     make(chan struct{}),
	}
}

// Start launches the background decay goroutine.
func (w *DecayWorker) Start() {
	ctx, cancel := context.WithCancel(context.Background())
	w.cancel = cancel
	go w.run(ctx)
}

// Stop signals the worker to stop and waits for it to exit (max 10s).
func (w *DecayWorker) Stop() {
	if w.cancel != nil {
		w.cancel()
	}
	select {
	case <-w.done:
	case <-time.After(10 * time.Second):
		slog.Warn("decay worker did not stop within 10s", "project", w.project)
	}
}

func (w *DecayWorker) run(ctx context.Context) {
	defer close(w.done)
	defer func() {
		if r := recover(); r != nil {
			slog.Error("decay worker panic", "project", w.project, "panic", r)
		}
	}()

	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.runOnce(ctx)
		}
	}
}

func (w *DecayWorker) runOnce(ctx context.Context) {
	n, err := w.backend.DecayStaleImportance(ctx, w.project, decayFactor)
	if err != nil {
		slog.Warn("decay worker: DecayStaleImportance failed",
			"project", w.project, "err", err)
		return
	}
	if n > 0 {
		slog.Info("decay worker: applied importance decay",
			"project", w.project, "rows", n)
	}
}
