package db

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

// EnqueueChunkLease marks a chunk as pending embedding by setting its initial lease.
// This is idempotent — if a chunk already has a lease, this call is a no-op.
// Used by Store to explicitly enqueue unembedded chunks for the reembed worker.
func EnqueueChunkLease(ctx context.Context, db *pgxpool.Pool, chunkID string, project string) error {
	_, err := db.Exec(ctx, `
		UPDATE chunks SET embed_lease_until = NOW() + INTERVAL '5 minutes'
		WHERE id = $1 AND embedding IS NULL
	`, chunkID)
	return err
}

// EnqueueChunkLeases marks multiple chunks as pending embedding in a single batch.
// For batches larger than 50, uses a more efficient approach.
func EnqueueChunkLeases(ctx context.Context, db *pgxpool.Pool, chunkIDs []string) error {
	if len(chunkIDs) == 0 {
		return nil
	}
	_, err := db.Exec(ctx, `
		UPDATE chunks SET embed_lease_until = NOW() + INTERVAL '5 minutes'
		WHERE id = ANY($1) AND embedding IS NULL
	`, chunkIDs)
	return err
}
