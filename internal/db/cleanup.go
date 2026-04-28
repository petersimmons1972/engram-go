package db

import (
	"context"
	"log/slog"
	"time"
)

// CleanupRetentionEvents deletes retrieval_events rows older than 90 days and
// returns the number of rows deleted.
func (b *PostgresBackend) CleanupRetentionEvents(ctx context.Context) (int64, error) {
	return b.deleteOldRetrievalEvents(ctx)
}

// StartRetentionWorker runs CleanupRetentionEvents once at startup and then
// every 24 hours. It is designed to be called as a goroutine:
//
//	go backend.StartRetentionWorker(ctx)
//
// The worker exits when ctx is cancelled (e.g. on SIGTERM).
func (b *PostgresBackend) StartRetentionWorker(ctx context.Context) {
	run := func() {
		n, err := b.deleteOldRetrievalEvents(ctx)
		if err != nil {
			slog.Error("retention worker: cleanup failed", "err", err)
		} else {
			slog.Info("retention worker: cleanup complete", "rows_deleted", n)
		}

		sn, serr := b.deleteOldSessions(ctx)
		if serr != nil {
			slog.Error("retention worker: session cleanup failed", "err", serr)
		} else {
			slog.Info("retention worker: session cleanup complete", "rows_deleted", sn)
		}
	}

	run() // catch-up pass at startup

	ticker := time.NewTicker(24 * time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			run()
		}
	}
}

// deleteOldRetrievalEvents issues the age-gated DELETE and returns the
// number of rows removed. Kept private; callers should use StartRetentionWorker
// or CleanupRetentionEvents.
func (b *PostgresBackend) deleteOldRetrievalEvents(ctx context.Context) (int64, error) {
	const q = `
DELETE FROM retrieval_events
WHERE created_at < NOW() - INTERVAL '90 days'`

	tag, err := b.pool.Exec(ctx, q)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

// deleteOldSessions removes mcp_sessions rows that have been inactive for more
// than 4 hours. The 4-hour window is 2× the 2-hour rehydration window in
// RehydrateSessions, ensuring sessions that could still be rehydrated are never
// prematurely deleted (#367).
func (b *PostgresBackend) deleteOldSessions(ctx context.Context) (int64, error) {
	if b.pool == nil {
		return 0, nil
	}
	const q = `
DELETE FROM mcp_sessions
WHERE last_seen_at < NOW() - INTERVAL '4 hours'`

	tag, err := b.pool.Exec(ctx, q)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}
