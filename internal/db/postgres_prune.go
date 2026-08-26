package db

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/petersimmons1972/engram/internal/types"
)

// minPruneAgeHours is the smallest maxAgeHours accepted by pruneGuard. It is
// chosen so that maxAgeHours*float64(time.Hour) converts to a time.Duration
// of at least one full second — comfortably above the point where float64
// rounding or truncation to an int64 nanosecond count could collapse the
// value to zero (round 1's finding: a value as small as 1/(2*time.Hour)
// converts to a sub-nanosecond, i.e. zero, time.Duration, silently turning
// the cutoff into time.Now()).
const minPruneAgeHours = 1.0 / 3600.0 // 1 second, expressed in hours

// maxPruneAgeHours caps maxAgeHours so that maxAgeHours*float64(time.Hour)
// stays well inside the range time.Duration (int64 nanoseconds) can
// represent without overflowing or losing precision at the top end.
const maxPruneAgeHours = float64(math.MaxInt64) / float64(time.Hour) / 2

// highestImportanceLevel and lowestImportanceLevel mirror the 0-4 importance
// scale enforced elsewhere (see internal/mcp/tools_store.go): 0 = Critical
// (highest retention priority), 4 = Low (lowest retention priority). Redefined
// here (rather than imported) because internal/db must not depend on
// internal/mcp; keep both definitions of the domain in sync.
const highestImportanceLevel = 0
const lowestImportanceLevel = 4

// pruneGuard is the FM-21 (hindsight #112) self-guard shared by the prune
// methods below. Historically neither method checked its own inputs at all —
// "no confirm gate at all" per the issue's scoping receipt — relying entirely
// on the one production caller (internal/search/engine.go's consolidation
// scheduler) to pass sane values. A future caller that forgot to scope by
// project, passed a non-positive/NaN/Inf maxAgeHours (cutoff collapses to
// now-or-later, matching nearly every non-immutable row immediately), or
// passed an out-of-range maxImportance (widening the destructive DELETE
// predicate to include critical, importance=0, memories) got no refusal.
// This does not duplicate a policy decision the caller owns (what age counts
// as "stale", or what importance floor to prune at, are scheduling choices);
// it refuses only the inputs no legitimate caller ever intends.
//
// Round-1 review findings this closes (PR #1442):
//   - A positive but sub-second maxAgeHours (e.g. 1/(2*time.Hour)) passed the
//     old `<= 0` check but converted to a zero (or near-zero) time.Duration,
//     collapsing the cutoff to time.Now() and pruning almost everything.
//   - math.NaN() and math.Inf(1) also passed `<= 0` (both compare false
//     against 0) and produced an undefined/degenerate cutoff.
//   - maxImportance was never validated at all: a caller passing -1 broadens
//     `importance >= maxImportance` to include importance=0 (critical) rows,
//     which this guard exists specifically to protect.
func pruneGuard(project string, maxAgeHours float64, maxImportance int) error {
	if project == "" {
		return fmt.Errorf("prune refused: project must not be empty")
	}
	if math.IsNaN(maxAgeHours) || math.IsInf(maxAgeHours, 0) {
		return fmt.Errorf("prune refused: maxAgeHours must be a finite number (got %v)", maxAgeHours)
	}
	if maxAgeHours < minPruneAgeHours {
		return fmt.Errorf("prune refused: maxAgeHours must be >= %v (got %v); a smaller value either "+
			"is non-positive (cutoff=now-or-later, pruning nearly every non-immutable row immediately) "+
			"or converts to a sub-second time.Duration after float64->int64 nanosecond conversion, which "+
			"can round down to zero and produce the same effect", minPruneAgeHours, maxAgeHours)
	}
	if maxAgeHours > maxPruneAgeHours {
		return fmt.Errorf("prune refused: maxAgeHours must be <= %v (got %v); larger values risk "+
			"overflowing the time.Duration used to compute the cutoff", maxPruneAgeHours, maxAgeHours)
	}
	if maxImportance < highestImportanceLevel || maxImportance > lowestImportanceLevel {
		return fmt.Errorf("prune refused: maxImportance must be within the valid importance range %d-%d "+
			"(got %d); an out-of-range value (e.g. negative) broadens the destructive DELETE predicate "+
			"'importance >= maxImportance' to include critical (importance=%d) memories",
			highestImportanceLevel, lowestImportanceLevel, maxImportance, highestImportanceLevel)
	}
	return nil
}

// PruneStaleMemories deletes old low-importance and expired memories.
// Uses RETURNING to emit an audit log line per deleted row (#107) so operators
// can reconstruct what was pruned from structured logs.
func (b *PostgresBackend) PruneStaleMemories(ctx context.Context, project string, maxAgeHours float64, maxImportance int) (int, error) {
	if err := pruneGuard(project, maxAgeHours, maxImportance); err != nil {
		return 0, err
	}
	cutoff := time.Now().UTC().Add(-time.Duration(maxAgeHours * float64(time.Hour)))
	tx, err := b.pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	rows, err := tx.Query(ctx, `
		DELETE FROM memories
		WHERE project=$1 AND NOT immutable AND (
			(importance>=$2 AND last_accessed<$3 AND access_count=0)
			OR (expires_at IS NOT NULL AND expires_at<NOW())
		)
		RETURNING id, content_hash, importance, document_id`, project, maxImportance, cutoff,
	)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	var count int
	docIDs := map[string]struct{}{}
	for rows.Next() {
		var id, contentHash string
		var importance int
		var documentID *string
		if err := rows.Scan(&id, &contentHash, &importance, &documentID); err != nil {
			return count, err
		}
		if documentID != nil && *documentID != "" {
			docIDs[*documentID] = struct{}{}
		}
		slog.Info("prune: deleted stale memory",
			"project", project, "id", id,
			"importance", importance, "content_hash", contentHash)
		count++
	}
	if err := rows.Err(); err != nil {
		return count, err
	}
	for docID := range docIDs {
		if _, err := b.DeleteOrphanedDocumentTx(ctx, tx, docID); err != nil {
			return count, err
		}
	}
	return count, tx.Commit(ctx)
}

func (b *PostgresBackend) PruneColdDocuments(ctx context.Context, project string, maxAgeHours float64, maxImportance int) (int, error) {
	if err := pruneGuard(project, maxAgeHours, maxImportance); err != nil {
		return 0, err
	}
	cutoff := time.Now().UTC().Add(-time.Duration(maxAgeHours * float64(time.Hour)))
	tx, err := b.pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	rows, err := tx.Query(ctx, `
		DELETE FROM memories WHERE id IN (
			SELECT m.id FROM memories m
			WHERE m.project=$1 AND m.storage_mode='document'
			  AND NOT m.immutable AND m.importance>=$2 AND m.created_at<$3
			  AND NOT EXISTS (
				SELECT 1 FROM chunks c
				WHERE c.memory_id=m.id AND c.last_matched IS NOT NULL
			  )
		)
		RETURNING document_id`, project, maxImportance, cutoff,
	)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	count := 0
	docIDs := map[string]struct{}{}
	for rows.Next() {
		var documentID *string
		if err := rows.Scan(&documentID); err != nil {
			return count, err
		}
		if documentID != nil && *documentID != "" {
			docIDs[*documentID] = struct{}{}
		}
		count++
	}
	if err := rows.Err(); err != nil {
		return count, err
	}
	for docID := range docIDs {
		if _, err := b.DeleteOrphanedDocumentTx(ctx, tx, docID); err != nil {
			return count, err
		}
	}
	return count, tx.Commit(ctx)
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
		SELECT COUNT(*) FROM relationships WHERE project=$1`, project,
	).Scan(&stats.TotalRelationships); err != nil {
		return nil, err
	}

	typeRows, err := b.pool.Query(ctx,
		"SELECT memory_type, COUNT(*) FROM memories WHERE project=$1 AND valid_to IS NULL GROUP BY memory_type", project)
	if err != nil {
		return nil, err
	}
	defer typeRows.Close()
	for typeRows.Next() {
		var mt string
		var c int
		if err := typeRows.Scan(&mt, &c); err != nil {
			return nil, err
		}
		stats.ByType[mt] = c
	}
	if err := typeRows.Err(); err != nil {
		return nil, err
	}

	impRows, err := b.pool.Query(ctx,
		"SELECT importance, COUNT(*) FROM memories WHERE project=$1 AND valid_to IS NULL GROUP BY importance", project)
	if err != nil {
		return nil, err
	}
	defer impRows.Close()
	for impRows.Next() {
		var imp, c int
		if err := impRows.Scan(&imp, &c); err != nil {
			return nil, err
		}
		stats.ByImportance[fmt.Sprintf("%d", imp)] = c
	}
	if err := impRows.Err(); err != nil {
		return nil, err
	}

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

	// Single-snapshot query: total and pending come from the same COUNT so
	// ChunksEmbedded = total - pending is guaranteed non-negative even under
	// concurrent inserts (avoids the TOCTOU race between two separate COUNTs).
	// TotalChunks is also set from this snapshot so all three chunk-count fields
	// are consistent with each other.
	var chunksTotal, chunksPending int
	if err := b.pool.QueryRow(ctx, `
		SELECT COUNT(*) AS total,
		       COUNT(*) FILTER (WHERE c.embedding IS NULL) AS pending
		FROM chunks c JOIN memories m ON m.id = c.memory_id
		WHERE m.project = $1 AND m.valid_to IS NULL`, project,
	).Scan(&chunksTotal, &chunksPending); err != nil {
		return nil, err
	}
	stats.TotalChunks = chunksTotal
	stats.ChunksTotal = chunksTotal
	stats.ChunksPendingEmbedding = chunksPending
	stats.ChunksEmbedded = chunksTotal - chunksPending

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

	// OR-semantics FTS: extract lexemes from the query and join with | so a
	// session only needs to match *some* content terms, not all of them.
	// plainto_tsquery AND semantics excluded sessions that lacked conversational
	// framing words ("previous", "remind") even when they contained the key
	// content terms ("plesiosaur", "dinosaur"). The CTE is NULL-safe: if all
	// query words are stop words, q IS NULL and the WHERE clause matches nothing.
	baseQ := `WITH orq AS (
		  SELECT to_tsquery('english', string_agg(lexeme, ' | ')) AS q
		  FROM unnest(to_tsvector('english', $1))
		)
		SELECT m.*, ts_rank(m.search_vector, orq.q) AS rank
		FROM memories m, orq
		WHERE orq.q IS NOT NULL
		  AND m.search_vector @@ orq.q
		  AND m.project=$2 AND m.valid_to IS NULL`
	args := []any{query, project}

	// Build optional time-range clauses using the same parameterized approach as ListMemories.
	var timeWhere []string
	if since != nil {
		args = append(args, since)
		timeWhere = append(timeWhere, fmt.Sprintf("m.valid_from>=$%d", len(args)))
	}
	if before != nil {
		args = append(args, before)
		timeWhere = append(timeWhere, fmt.Sprintf("m.valid_from<$%d", len(args)))
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
