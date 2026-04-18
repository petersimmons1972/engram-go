package reembed

import (
	"context"
	"log/slog"
	"time"

	"github.com/petersimmons1972/engram/internal/db"
	"github.com/petersimmons1972/engram/internal/embed"
)

const (
	pollInterval = 30 * time.Second
	batchSize    = 20
)

// Worker re-embeds chunks with NULL embedding for a project.
type Worker struct {
	backend  db.Backend
	embedder embed.Client
	project  string
	active   bool
	cancel   context.CancelFunc
	done     chan struct{}
}

// NewWorker creates a Worker. If active=false, Start is a no-op.
func NewWorker(backend db.Backend, embedder embed.Client, project string, active bool) *Worker {
	return &Worker{
		backend:  backend,
		embedder: embedder,
		project:  project,
		active:   active,
		done:     make(chan struct{}),
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
			if pending, err := backend.GetChunksPendingEmbedding(ctx, project, 1); err == nil && len(pending) > 0 {
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
func (w *Worker) StartWithContext(ctx context.Context) {
	if !w.active {
		close(w.done)
		return
	}
	ctx, cancel := context.WithCancel(ctx)
	w.cancel = cancel
	go w.run(ctx)
}

// Stop signals the worker and waits up to 8s.
func (w *Worker) Stop() {
	if w.cancel != nil {
		w.cancel()
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
	for {
		// Per-iteration timeout prevents an Ollama hang from blocking the worker forever.
		iterCtx, cancel := context.WithTimeout(ctx, batchTimeout)
		done := w.safeRunBatch(iterCtx)
		cancel()
		if ctx.Err() != nil {
			return
		}
		if done {
			// ctx is already cancelled here; use a fresh context for the flag write.
			flagCtx, flagCancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer flagCancel()
			if err := w.backend.SetMeta(flagCtx, w.project, "embedding_migration_in_progress", "false"); err != nil {
				slog.Warn("reembed: failed to clear migration flag", "project", w.project, "err", err)
			}
			slog.Info("reembed complete", "project", w.project)
			return
		}
		select {
		case <-ctx.Done():
			return
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
		return false
	}
	if len(chunks) == 0 {
		return true
	}
	for _, c := range chunks {
		if ctx.Err() != nil {
			return false
		}
		vec, err := w.embedder.Embed(ctx, c.ChunkText)
		if err != nil {
			slog.Warn("reembed embed failed", "chunk", c.ID, "err", err)
			continue
		}
		if n, err := w.backend.UpdateChunkEmbedding(ctx, c.ID, vec); err != nil || n == 0 {
			slog.Warn("reembed update failed or chunk deleted", "chunk", c.ID)
		}
	}
	return len(chunks) < batchSize
}

