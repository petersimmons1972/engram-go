package db

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/petersimmons1972/engram/internal/types"
)

// PruneStaleMemories deletes old low-importance and expired memories.
// Uses RETURNING to emit an audit log line per deleted row (#107) so operators
// can reconstruct what was pruned from structured logs.
func (b *PostgresBackend) PruneStaleMemories(ctx context.Context, project string, maxAgeHours float64, maxImportance int) (int, error) {
	cutoff := time.Now().UTC().Add(-time.Duration(maxAgeHours * float64(time.Hour)))
	rows, err := b.pool.Query(ctx, `
		DELETE FROM memories
		WHERE project=$1 AND NOT immutable AND (
			(importance>=$2 AND last_accessed<$3 AND access_count=0)
			OR (expires_at IS NOT NULL AND expires_at<NOW())
		)
		RETURNING id, content_hash, importance`, project, maxImportance, cutoff,
	)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	var count int
	for rows.Next() {
		var id, contentHash string
		var importance int
		if err := rows.Scan(&id, &contentHash, &importance); err != nil {
			return count, err
		}
		slog.Info("prune: deleted stale memory",
			"project", project, "id", id,
			"importance", importance, "content_hash", contentHash)
		count++
	}
	return count, rows.Err()
}

func (b *PostgresBackend) PruneColdDocuments(ctx context.Context, project string, maxAgeHours float64, maxImportance int) (int, error) {
	cutoff := time.Now().UTC().Add(-time.Duration(maxAgeHours * float64(time.Hour)))
	tag, err := b.pool.Exec(ctx, `
		DELETE FROM memories WHERE id IN (
			SELECT m.id FROM memories m
			WHERE m.project=$1 AND m.storage_mode='document'
			  AND NOT m.immutable AND m.importance>=$2 AND m.created_at<$3
			  AND NOT EXISTS (
				SELECT 1 FROM chunks c
				WHERE c.memory_id=m.id AND c.last_matched IS NOT NULL
			  )
		)`, project, maxImportance, cutoff,
	)
	return int(tag.RowsAffected()), err
}

func (b *PostgresBackend) GetStats(ctx context.Context, project string) (*types.MemoryStats, error) {
	stats := &types.MemoryStats{
		ByType:        map[string]int{},
		ByImportance:  map[string]int{},
		Summarization: map[string]any{},
	}

	if err := b.pool.QueryRow(ctx, "SELECT COUNT(*) FROM memories WHERE project=$1 AND valid_to IS NULL", project).Scan(&stats.TotalMemories); err != nil {
		return nil, err
	}
	if err := b.pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM chunks c JOIN memories m ON m.id=c.memory_id WHERE m.project=$1 AND m.valid_to IS NULL`, project,
	).Scan(&stats.TotalChunks); err != nil {
		return nil, err
	}
	if err := b.pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM relationships WHERE project=$1`, project,
	).Scan(&stats.TotalRelationships); err != nil {
		return nil, err
	}

	typeRows, err := b.pool.Query(ctx,
		"SELECT memory_type, COUNT(*) FROM memories WHERE project=$1 AND valid_to IS NULL GROUP BY memory_type", project)
	if err != nil {
		return nil, err
	}
	for typeRows.Next() {
		var mt string
		var c int
		if err := typeRows.Scan(&mt, &c); err != nil {
			typeRows.Close()
			return nil, err
		}
		stats.ByType[mt] = c
	}
	typeRows.Close()

	impRows, err := b.pool.Query(ctx,
		"SELECT importance, COUNT(*) FROM memories WHERE project=$1 AND valid_to IS NULL GROUP BY importance", project)
	if err != nil {
		return nil, err
	}
	for impRows.Next() {
		var imp, c int
		if err := impRows.Scan(&imp, &c); err != nil {
			impRows.Close()
			return nil, err
		}
		stats.ByImportance[fmt.Sprintf("%d", imp)] = c
	}
	impRows.Close()

	var oldest, newest *time.Time
	if err := b.pool.QueryRow(ctx, "SELECT MIN(created_at) FROM memories WHERE project=$1 AND valid_to IS NULL", project).Scan(&oldest); err != nil {
		return nil, err
	}
	if err := b.pool.QueryRow(ctx, "SELECT MAX(created_at) FROM memories WHERE project=$1 AND valid_to IS NULL", project).Scan(&newest); err != nil {
		return nil, err
	}
	if oldest != nil {
		s := oldest.Format(time.RFC3339)
		stats.Oldest = &s
	}
	if newest != nil {
		s := newest.Format(time.RFC3339)
		stats.Newest = &s
	}

	if err := b.pool.QueryRow(ctx,
		"SELECT COUNT(*) FROM memories WHERE project=$1 AND valid_to IS NULL AND summary IS NULL", project,
	).Scan(&stats.PendingSummarization); err != nil {
		return nil, err
	}

	var summarized int
	err = b.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM memories WHERE project = $1 AND valid_to IS NULL AND summary IS NOT NULL`,
		project).Scan(&summarized)
	if err != nil {
		summarized = 0
	}
	stats.Summarization = map[string]any{
		"pending":   stats.PendingSummarization,
		"completed": summarized,
	}

	if err := b.pool.QueryRow(ctx, "SELECT pg_database_size(current_database())").Scan(&stats.DBSizeBytes); err != nil {
		return nil, err
	}

	return stats, nil
}

func (b *PostgresBackend) ListAllProjects(ctx context.Context) ([]string, error) {
	rows, err := b.pool.Query(ctx, "SELECT DISTINCT project FROM memories WHERE valid_to IS NULL ORDER BY project")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var projects []string
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err != nil {
			return nil, err
		}
		projects = append(projects, p)
	}
	return projects, rows.Err()
}

func (b *PostgresBackend) RebuildFTS(ctx context.Context) error {
	// REINDEX CONCURRENTLY must run outside a transaction.
	conn, err := b.pool.Acquire(ctx)
	if err != nil {
		return err
	}
	defer conn.Release()
	_, err = conn.Exec(ctx, "REINDEX INDEX CONCURRENTLY idx_memories_search")
	return err
}

func (b *PostgresBackend) FTSSearch(ctx context.Context, project, query string, limit int, since, before *time.Time) ([]FTSResult, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, nil
	}

	baseQ := `SELECT m.*, ts_rank(m.search_vector, plainto_tsquery('english', $1)) AS rank
		  FROM memories m
		  WHERE m.search_vector @@ plainto_tsquery('english', $2)
		  AND m.project=$3 AND m.valid_to IS NULL`
	args := []any{query, query, project}

	// Build optional time-range clauses using the same parameterized approach as ListMemories.
	var timeWhere []string
	if since != nil {
		args = append(args, since)
		timeWhere = append(timeWhere, fmt.Sprintf("m.created_at>=$%d", len(args)))
	}
	if before != nil {
		args = append(args, before)
		timeWhere = append(timeWhere, fmt.Sprintf("m.created_at<=$%d", len(args)))
	}
	for _, c := range timeWhere {
		baseQ += " AND " + c
	}
	args = append(args, limit)
	q := baseQ + fmt.Sprintf(" ORDER BY rank DESC LIMIT $%d", len(args))

	rows, err := b.pool.Query(ctx, q, args...)
	if err != nil {
		slog.Debug("FTS query failed", "query_len", len(query), "err", err)
		return nil, fmt.Errorf("FTS search failed: %w", err)
	}
	defer rows.Close()

	// Use the shared rowToFTSResult helper to avoid duplicating the 26-column
	// scan that rowToMemory already owns (#112 — schema drift prevention).
	return pgx.CollectRows(rows, rowToFTSResult)
}

// DecayStaleImportance multiplies dynamic_importance by factor on all active
// memories whose next_review_at is in the past. Returns the number of rows updated.
func (b *PostgresBackend) DecayStaleImportance(ctx context.Context, project string, factor float64) (int, error) {
	tag, err := b.pool.Exec(ctx, `
		UPDATE memories
		SET dynamic_importance = GREATEST(0.1, dynamic_importance * $1),
		    next_review_at     = next_review_at + (retrieval_interval_hrs * INTERVAL '1 hour')
		WHERE project = $2
		  AND valid_to IS NULL
		  AND next_review_at IS NOT NULL
		  AND next_review_at < NOW()`,
		factor, project,
	)
	if err != nil {
		return 0, err
	}
	return int(tag.RowsAffected()), nil
}

// SetNextReviewAt overrides the next_review_at timestamp for a memory.
func (b *PostgresBackend) SetNextReviewAt(ctx context.Context, id string, t time.Time) error {
	_, err := b.pool.Exec(ctx,
		"UPDATE memories SET next_review_at=$1 WHERE id=$2",
		t, id,
	)
	return err
}
