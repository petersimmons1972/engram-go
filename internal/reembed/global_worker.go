package reembed

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	pgvector "github.com/pgvector/pgvector-go"
	"golang.org/x/sync/errgroup"

	"github.com/petersimmons1972/engram/internal/embed"
	"github.com/petersimmons1972/engram/internal/metrics"
)

const globalBatchTimeout = 5 * time.Minute
const globalEmbedTimeout = 15 * time.Second
const globalConcurrency = 8

// GlobalReembedder processes unembedded chunks across ALL projects from a
// single goroutine that is lifecycle-independent of any EnginePool entry.
// It uses FOR UPDATE SKIP LOCKED so multiple server instances can safely
// run concurrent GlobalReembedders against the same database (#359).
type GlobalReembedder struct {
	pool      *pgxpool.Pool
	embedder  embed.Client
	batchSize int
	interval  time.Duration
	startOnce sync.Once
	done      chan struct{}
}

// pendingChunk holds the minimal fields needed to embed and update a chunk.
type pendingChunk struct {
	id        string
	chunkText string
}

// NewGlobalReembedder creates a GlobalReembedder. pool and embedder may be nil
// in unit tests that only test lifecycle behaviour; nil values cause the worker
// to skip embedding iterations gracefully.
func NewGlobalReembedder(pool *pgxpool.Pool, embedder embed.Client, batchSize int, interval time.Duration) *GlobalReembedder {
	if batchSize <= 0 {
		batchSize = 100
	}
	return &GlobalReembedder{
		pool:      pool,
		embedder:  embedder,
		batchSize: batchSize,
		interval:  interval,
		done:      make(chan struct{}),
	}
}

// Start launches the background goroutine. Calling Start more than once is safe
// (subsequent calls are no-ops). The goroutine exits when ctx is cancelled.
func (g *GlobalReembedder) Start(ctx context.Context) {
	g.startOnce.Do(func() {
		go g.run(ctx)
	})
}

// Wait blocks until the background goroutine has exited. If Start was never
// called, Wait returns immediately.
func (g *GlobalReembedder) Wait() {
	// If Start was never called, startOnce fires here to close done immediately
	// so callers do not block.
	g.startOnce.Do(func() { close(g.done) })
	<-g.done
}

func (g *GlobalReembedder) run(ctx context.Context) {
	defer close(g.done)
	slog.Info("global reembedder started", "batch_size", g.batchSize, "interval", g.interval)
	for {
		metrics.WorkerTicks.WithLabelValues("global_reembed").Inc()
		if g.pool != nil && g.embedder != nil {
			// Update the pending-reembed gauge before each batch so dashboards and
			// alerts remain live. The per-project Worker used to do this; GlobalReembedder
			// now owns the reembed path and must carry the responsibility.
			countCtx, countCancel := context.WithTimeout(ctx, 5*time.Second)
			var pending int
			if err := g.pool.QueryRow(countCtx,
				"SELECT COUNT(*) FROM chunks WHERE embedding IS NULL",
			).Scan(&pending); err == nil {
				metrics.ChunksPendingReembed.Set(float64(pending))
			}
			countCancel()

			iterCtx, cancel := context.WithTimeout(ctx, globalBatchTimeout)
			if err := g.runBatch(iterCtx); err != nil && ctx.Err() == nil {
				slog.Warn("global reembedder batch error", "err", err)
				metrics.WorkerErrors.WithLabelValues("global_reembed").Inc()
			}
			cancel()
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(g.interval):
		}
	}
}

func (g *GlobalReembedder) runBatch(ctx context.Context) error {
	// Claim a batch inside a short transaction so concurrent replicas do not
	// pick the same chunks simultaneously. The transaction is committed as soon
	// as the chunk list is read; subsequent UPDATEs are idempotent if two replicas
	// race after the commit (re-embedding the same chunk produces the same vector).
	// FOR UPDATE SKIP LOCKED is a parameterized query — batchSize is bound as $1
	// to stay consistent with the rest of the codebase (no fmt.Sprintf SQL).
	tx, err := g.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin claim tx: %w", err)
	}

	rows, err := tx.Query(ctx, `
		SELECT c.id, c.chunk_text
		FROM chunks c
		WHERE c.embedding IS NULL
		ORDER BY c.id
		LIMIT $1
		FOR UPDATE SKIP LOCKED`, g.batchSize)
	if err != nil {
		_ = tx.Rollback(ctx)
		return fmt.Errorf("query pending chunks: %w", err)
	}

	var chunks []pendingChunk
	for rows.Next() {
		var pc pendingChunk
		if err := rows.Scan(&pc.id, &pc.chunkText); err != nil {
			rows.Close()
			_ = tx.Rollback(ctx)
			return fmt.Errorf("scan pending chunk: %w", err)
		}
		chunks = append(chunks, pc)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		_ = tx.Rollback(ctx)
		return fmt.Errorf("iterate pending chunks: %w", err)
	}

	// Commit the claim transaction immediately so the connection is returned to
	// the pool before the (potentially slow) Ollama embed calls begin.
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit claim tx: %w", err)
	}

	if len(chunks) == 0 {
		return nil
	}

	slog.Debug("global reembedder: processing batch", "count", len(chunks))

	eg, egCtx := errgroup.WithContext(ctx)
	eg.SetLimit(globalConcurrency)
	for _, c := range chunks {
		c := c
		if ctx.Err() != nil {
			break
		}
		eg.Go(func() error {
			// Derive embedCtx from egCtx so cancellation (e.g. shutdown or batch
			// timeout) propagates to in-flight Ollama calls rather than letting
			// them run for the full globalEmbedTimeout after the parent is done.
			embedCtx, embedCancel := context.WithTimeout(egCtx, globalEmbedTimeout)
			defer embedCancel()
			vec, err := g.embedder.Embed(embedCtx, c.chunkText)
			if err != nil {
				slog.Warn("global reembedder: embed failed", "chunk", c.id, "err", err)
				return nil // non-fatal: skip chunk, retry on next tick
			}
			if _, err := g.pool.Exec(egCtx,
				"UPDATE chunks SET embedding=$1 WHERE id=$2",
				pgvector.NewVector(vec), c.id,
			); err != nil {
				slog.Warn("global reembedder: update failed", "chunk", c.id, "err", err)
			}
			return nil
		})
	}
	_ = eg.Wait()
	return nil
}
