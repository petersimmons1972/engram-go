// Package reembed provides a background worker that re-embeds chunks with NULL embeddings.
package reembed

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/petersimmons1972/engram/internal/db"
	"github.com/petersimmons1972/engram/internal/embed"
	"github.com/petersimmons1972/engram/internal/metrics"
)

const (
	pollInterval = 30 * time.Second
	batchSize    = 20
)

// Worker re-embeds chunks with NULL embedding for a project.
type Worker struct {
	backend    db.Backend
	embedder   embed.Client
	project    string
	active     bool
	cancel     context.CancelFunc
	done       chan struct{}
	notify     chan struct{} // wakes the poll loop early; buffered size 1
	startOnce  sync.Once    // ensures the goroutine starts exactly once
	parentCtx  context.Context
}

// NewWorker creates a Worker. If active=false, the goroutine starts only when
// Notify() is first called (lazy activation for new projects).
func NewWorker(backend db.Backend, embedder embed.Client, project string, active bool) *Worker {
	return &Worker{
		backend:  backend,
		embedder: embedder,
		project:  project,
		active:   active,
		done:     make(chan struct{}),
		notify:   make(chan struct{}, 1),
	}
}

// IsActive reports whether the worker will process chunks when started.
func (w *Worker) IsActive() bool { return w.active }

// NewWorkerFromMeta creates a Worker and reads the migration flag from project_meta.
// It also activates if there are chunks with NULL embeddings — this handles Ollama
// outage recovery, where chunks were stored without embeddings and the migration flag
// was never set.
func NewWorkerFromMeta(ctx context.Context, backend db.Backend, embedder embed.Client, project string) *Worker {
	active := false
	if backend != nil {
		if v, ok, _ := backend.GetMeta(ctx, project, "embedding_migration_in_progress"); ok && v == "true" {
			active = true
		}
		if !active {
			if pending, err := backend.GetChunksPendingEmbedding(ctx, project, 1); err != nil {
				slog.Warn("reembed worker: could not query pending embeddings at startup",
					"project", project, "err", err)
			} else if len(pending) > 0 {
				slog.Info("reembed worker: found chunks with NULL embedding at startup, activating",
					"project", project, "pending_sample", pending[0].ID)
				active = true
			}
		}
	}
	return NewWorker(backend, embedder, project, active)
}

// Start launches the background goroutine if active.
func (w *Worker) Start() {
	w.StartWithContext(context.Background())
}

// StartWithContext launches the background goroutine using ctx as the parent
// lifecycle context. The worker stops when ctx is cancelled.
// If active=false at construction time, the goroutine starts lazily on the
// first Notify() call instead of immediately.
func (w *Worker) StartWithContext(ctx context.Context) {
	w.parentCtx = ctx
	if !w.active {
		return
	}
	w.startOnce.Do(func() {
		runCtx, cancel := context.WithCancel(ctx)
		w.cancel = cancel
		go w.run(runCtx)
	})
}

// Notify wakes the reembed worker to process newly available NULL-embedded
// chunks. If the worker was not yet started (inactive project at startup),
// it is lazily activated now. Safe to call from any goroutine.
func (w *Worker) Notify() {
	w.active = true
	if w.parentCtx != nil {
		w.startOnce.Do(func() {
			runCtx, cancel := context.WithCancel(w.parentCtx)
			w.cancel = cancel
			go w.run(runCtx)
		})
	}
	select {
	case w.notify <- struct{}{}:
	default: // already pending; skip
	}
}

// Stop signals the worker and waits up to 8s.
// If the goroutine was never started (inactive project, no Notify() called),
// Stop returns immediately.
func (w *Worker) Stop() {
	if w.cancel != nil {
		w.cancel()
	} else {
		// Goroutine never started; close done so callers don't block.
		w.startOnce.Do(func() { close(w.done) })
		return
	}
	select {
	case <-w.done:
	case <-time.After(8 * time.Second):
		slog.Warn("reembed worker did not stop within 8s", "project", w.project)
	}
}

const batchTimeout = 5 * time.Minute // max time for one runBatch iteration (#120)

func (w *Worker) run(ctx context.Context) {
	defer close(w.done)
	migrationCleared := false
	for {
		// Count pending chunks and update the Prometheus gauge at the start of
		// each tick so operators can observe backlog size without waiting for a pass.
		metrics.WorkerTicks.WithLabelValues("reembed").Inc()
		if w.backend != nil {
			countCtx, countCancel := context.WithTimeout(ctx, 5*time.Second)
			if count, err := w.backend.GetPendingEmbeddingCount(countCtx, w.project); err == nil {
				metrics.ChunksPendingReembed.Set(float64(count))
				if count > 0 {
					slog.Warn("reembed: chunks pending", "count", count, "project", w.project)
				}
			}
			countCancel()
		}

		// Per-iteration timeout prevents an Ollama hang from blocking the worker forever.
		iterCtx, cancel := context.WithTimeout(ctx, batchTimeout)
		batchDone := w.safeRunBatch(iterCtx)
		cancel()
		if ctx.Err() != nil {
			return
		}
		if batchDone && !migrationCleared {
			slog.Info("reembed: backfill pass complete", "project", w.project)
			flagCtx, flagCancel := context.WithTimeout(context.Background(), 10*time.Second)
			if err := w.backend.SetMeta(flagCtx, w.project, "embedding_migration_in_progress", "false"); err != nil {
				slog.Warn("reembed: failed to clear migration flag", "project", w.project, "err", err)
				metrics.WorkerErrors.WithLabelValues("reembed").Inc()
			}
			flagCancel()
			slog.Info("reembed complete", "project", w.project)
			migrationCleared = true
		}
		// Worker stays alive to service future Notify() calls from Store().
		select {
		case <-ctx.Done():
			return
		case <-w.notify:
		case <-time.After(pollInterval):
		}
	}
}

// safeRunBatch wraps runBatch with per-iteration panic recovery (#106).
// Returns false on panic (treat as not-done so the loop retries after backoff).
func (w *Worker) safeRunBatch(ctx context.Context) (done bool) {
	defer func() {
		if r := recover(); r != nil {
			slog.Error("reembed worker panic — will retry after poll interval",
				"project", w.project, "panic", r)
			done = false
		}
	}()
	return w.runBatch(ctx)
}

func (w *Worker) runBatch(ctx context.Context) bool {
	if w.backend == nil {
		return true
	}
	chunks, err := w.backend.GetChunksPendingEmbedding(ctx, w.project, batchSize)
	if err != nil {
		slog.Warn("reembed fetch failed", "err", err)
		metrics.WorkerErrors.WithLabelValues("reembed").Inc()
		return false
	}
	if len(chunks) == 0 {
		return true
	}
	// Process chunks concurrently with a limit of 8 goroutines so we can
	// saturate Ollama's available threads without overwhelming the embedder.
	// Embed errors are non-fatal: we log and skip rather than propagating so
	// one bad chunk does not abort the remaining work in the batch.
	eg, egCtx := errgroup.WithContext(ctx)
	eg.SetLimit(8)
	for _, c := range chunks {
		c := c // capture loop variable for the closure
		if ctx.Err() != nil {
			break
		}
		eg.Go(func() error {
			// Independent 15s deadline (E5): isolates each embed call from the
			// worker context so a slow Ollama cannot stall the entire batch.
			embedCtx, embedCancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer embedCancel()
			vec, err := w.embedder.Embed(embedCtx, c.ChunkText)
			if err != nil {
				slog.Warn("reembed embed failed", "chunk", c.ID, "err", err)
				return nil // non-fatal: let other goroutines continue
			}
			if n, err := w.backend.UpdateChunkEmbedding(egCtx, c.ID, vec); err != nil || n == 0 {
				slog.Warn("reembed update failed or chunk deleted", "chunk", c.ID)
			}
			return nil
		})
	}
	// eg.Wait() always returns nil because goroutines never return non-nil errors.
	_ = eg.Wait()
	if ctx.Err() != nil {
		return false
	}
	return len(chunks) < batchSize
}

