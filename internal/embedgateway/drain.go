package embedgateway

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/petersimmons1972/engram/internal/embedmodel"
	"github.com/petersimmons1972/engram/internal/metrics"
	pgvector "github.com/pgvector/pgvector-go"
	"golang.org/x/sync/errgroup"
)

const embedTimeout = 15 * time.Second

type pendingChunk struct {
	id        string
	chunkText string
}

func (g *EmbedGateway) drainBatch(ctx context.Context) (int, error) {
	if g.testDrain != nil {
		return g.testDrain(ctx)
	}
	if g.pool == nil || g.embedder == nil {
		return 0, nil
	}
	if remaining, held := g.inDegradedHold(time.Now()); held {
		slog.Warn("embed gateway degraded hold active; skipping drain", "remaining", remaining)
		return 0, nil
	}

	tx, err := g.pool.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("begin claim tx: %w", err)
	}
	rows, err := tx.Query(ctx, `
		SELECT c.id, c.chunk_text
		FROM chunks c
		WHERE c.embedding IS NULL
		ORDER BY c.id DESC
		LIMIT $1
		FOR UPDATE SKIP LOCKED`, g.batchSize)
	if err != nil {
		_ = tx.Rollback(ctx)
		return 0, fmt.Errorf("query pending chunks: %w", err)
	}

	var chunks []pendingChunk
	for rows.Next() {
		var c pendingChunk
		if err := rows.Scan(&c.id, &c.chunkText); err != nil {
			rows.Close()
			_ = tx.Rollback(ctx)
			return 0, fmt.Errorf("scan pending chunk: %w", err)
		}
		chunks = append(chunks, c)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		_ = tx.Rollback(ctx)
		return 0, fmt.Errorf("iterate pending chunks: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("commit claim tx: %w", err)
	}
	if len(chunks) == 0 {
		return 0, nil
	}

	eg, egCtx := errgroup.WithContext(ctx)
	eg.SetLimit(g.throttle.Concurrency())
	for _, chunk := range chunks {
		chunk := chunk
		eg.Go(func() error {
			vec, err := g.embedAndValidate(egCtx, chunk)
			if err != nil {
				return nil
			}
			if tag, err := g.pool.Exec(egCtx,
				"UPDATE chunks SET embedding=$1 WHERE id=$2 AND embedding IS NULL",
				pgvector.NewVector(vec), chunk.id,
			); err != nil {
				slog.Warn("embed gateway: update failed", "chunk", chunk.id, "err", err)
			} else if tag.RowsAffected() == 0 {
				slog.Warn("embed gateway: update matched zero rows — chunk may have been deleted or already embedded by a concurrent drainer", "chunk", chunk.id)
			}
			return nil
		})
	}
	_ = eg.Wait()
	return len(chunks), nil
}

func (g *EmbedGateway) embedAndValidate(ctx context.Context, chunk pendingChunk) ([]float32, error) {
	embedCtx, cancel := context.WithTimeout(ctx, embedTimeout)
	defer cancel()

	vec, modelID, err := g.embedder.EmbedWithModel(embedCtx, chunk.chunkText)
	if err != nil {
		metrics.WorkerErrors.WithLabelValues("embed_gateway").Inc()
		slog.Warn("embed gateway: embed failed", "chunk", chunk.id, "err", err)
		return nil, err
	}
	if err := validateEmbedResponse(vec, modelID); err != nil {
		metrics.EmbedValidationRejections.WithLabelValues(rejectionClass(vec, modelID)).Inc()
		slog.Error("embed response rejected", "chunk", chunk.id, "model_id", modelID, "dims", len(vec), "err", err)
		g.noteValidationRejected(modelID, len(vec))
		return nil, err
	}
	g.noteEmbedAccepted()
	return vec, nil
}

func rejectionClass(vec []float32, modelID string) string {
	if embedmodel.CanonicalName(modelID) != embedmodel.CanonicalBGEM3 {
		return "wrong_model"
	}
	if len(vec) != embedmodel.RequiredDims {
		return "wrong_dims"
	}
	return "none"
}
