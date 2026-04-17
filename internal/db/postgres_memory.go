package db

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/petersimmons1972/engram/internal/types"
)

// ContentHash returns the canonical SHA-256 hex digest for a memory's content.
// Used by both the storage layer and the ingest dedup path to guarantee
// identical hash values from the same input string.
func ContentHash(content string) string {
	h := sha256.Sum256([]byte(content))
	return fmt.Sprintf("%x", h)
}

func (b *PostgresBackend) StoreMemory(ctx context.Context, m *types.Memory) error {
	return b.storeMemoryExec(ctx, b.pool, m)
}

func (b *PostgresBackend) StoreMemoryTx(ctx context.Context, tx Tx, m *types.Memory) error {
	raw, err := unwrapTx(tx)
	if err != nil {
		return err
	}
	return b.storeMemoryExec(ctx, raw, m)
}

func (b *PostgresBackend) storeMemoryExec(ctx context.Context, ex execer, m *types.Memory) error {
	now := time.Now().UTC()
	m.CreatedAt = now
	m.UpdatedAt = now
	m.LastAccessed = now
	m.Project = b.project
	hash := ContentHash(m.Content)
	m.ContentHash = &hash

	tagsJSON, err := json.Marshal(m.Tags)
	if err != nil {
		return fmt.Errorf("marshal tags: %w", err)
	}

	episodeID := m.EpisodeID
	var episodeArg any
	if episodeID == "" {
		episodeArg = nil
	} else {
		episodeArg = episodeID
	}

	// document_id is optional — only set for Tier-2 (raw document) memories.
	// Pass NULL otherwise so the FK column stays unset.
	var documentArg any
	if m.DocumentID == "" {
		documentArg = nil
	} else {
		documentArg = m.DocumentID
	}

	// Seed dynamic_importance from static importance using Feature 2 formula.
	if m.DynamicImportance == nil {
		di := math.Max(0.1, (5.0-float64(m.Importance))/3.0)
		m.DynamicImportance = &di
	}

	_, err = ex.Exec(ctx, `
		INSERT INTO memories
		  (id, content, memory_type, project, tags,
		   importance, access_count, last_accessed, created_at, updated_at,
		   immutable, expires_at, content_hash, storage_mode, episode_id,
		   dynamic_importance, document_id)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17)`,
		m.ID, m.Content, m.MemoryType, m.Project, tagsJSON,
		m.Importance, m.AccessCount, now, now, now,
		m.Immutable, m.ExpiresAt, hash, m.StorageMode, episodeArg,
		m.DynamicImportance, documentArg,
	)
	return err
}

func (b *PostgresBackend) GetMemory(ctx context.Context, id string) (*types.Memory, error) {
	row, err := b.pool.Query(ctx,
		"SELECT * FROM memories WHERE id=$1 AND project=$2 AND valid_to IS NULL", id, b.project)
	if err != nil {
		return nil, err
	}
	m, err := pgx.CollectOneRow(row, rowToMemory)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	// Integrity check
	if m.ContentHash != nil {
		expected := ContentHash(m.Content)
		if *m.ContentHash != expected {
			slog.Warn("INTEGRITY: content_hash mismatch",
				"id", m.ID,
				"stored", (*m.ContentHash)[:8],
				"expected", expected[:8],
			)
		}
	}
	return m, nil
}

func (b *PostgresBackend) GetMemoriesByIDs(ctx context.Context, project string, ids []string) ([]*types.Memory, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	rows, err := b.pool.Query(ctx,
		"SELECT * FROM memories WHERE project=$1 AND id=ANY($2) AND valid_to IS NULL",
		project, ids,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var mems []*types.Memory
	for rows.Next() {
		m, err := rowToMemory(rows)
		if err != nil {
			return nil, err
		}
		mems = append(mems, m)
	}
	return mems, rows.Err()
}

func (b *PostgresBackend) UpdateMemory(
	ctx context.Context, id string,
	content *string, tags []string, importance *int,
) (*types.Memory, error) {
	tx, err := b.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	// Lock the row for the duration of the read-modify-write to prevent races.
	row, err := tx.Query(ctx,
		"SELECT * FROM memories WHERE id=$1 AND project=$2 AND valid_to IS NULL FOR UPDATE",
		id, b.project)
	if err != nil {
		return nil, err
	}
	m, err := pgx.CollectOneRow(row, rowToMemory)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if m.Immutable {
		return nil, fmt.Errorf("memory %q is immutable and cannot be updated", id)
	}

	// Snapshot current state into memory_versions before applying changes.
	if err := b.versionMemoryTx(ctx, tx, m, types.VersionChangeUpdate, ""); err != nil {
		return nil, fmt.Errorf("version snapshot: %w", err)
	}

	now := time.Now().UTC()
	if content != nil {
		m.Content = *content
	}
	if tags != nil {
		m.Tags = tags
	}
	if importance != nil {
		m.Importance = *importance
	}

	tagsJSON, err := json.Marshal(m.Tags)
	if err != nil {
		return nil, fmt.Errorf("marshal tags: %w", err)
	}

	if content != nil {
		hash := ContentHash(m.Content)
		m.ContentHash = &hash
		// Clear the summary so the background worker regenerates it with the new content.
		_, err = tx.Exec(ctx,
			"UPDATE memories SET content=$1, tags=$2, importance=$3, updated_at=$4, content_hash=$5, summary=NULL WHERE id=$6 AND project=$7",
			m.Content, tagsJSON, m.Importance, now, hash, id, b.project,
		)
	} else {
		_, err = tx.Exec(ctx,
			"UPDATE memories SET content=$1, tags=$2, importance=$3, updated_at=$4 WHERE id=$5 AND project=$6",
			m.Content, tagsJSON, m.Importance, now, id, b.project,
		)
	}
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	m.UpdatedAt = now
	return m, nil
}

// DeleteMemory removes a memory and all its dependent data (chunks, relationships)
// in a single atomic transaction. Routes through DeleteMemoryAtomic — keeping both
// for interface compatibility.
func (b *PostgresBackend) DeleteMemory(ctx context.Context, id string) (bool, error) {
	return b.DeleteMemoryAtomic(ctx, b.project, id, false)
}

func (b *PostgresBackend) DeleteMemoryAtomic(ctx context.Context, project, id string, force bool) (bool, error) {
	tx, err := b.pool.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	var immutable bool
	err = tx.QueryRow(ctx,
		"SELECT immutable FROM memories WHERE id=$1 AND project=$2 FOR UPDATE",
		id, project,
	).Scan(&immutable)
	if err == pgx.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}

	if !force && immutable {
		return false, fmt.Errorf("cannot delete immutable memory %s; use force=true only for rollback", id)
	}
	if force && immutable {
		slog.Warn("force-deleting immutable memory (rollback path)", "id", id)
	}

	if _, err := tx.Exec(ctx, "DELETE FROM chunks WHERE memory_id=$1", id); err != nil {
		return false, err
	}
	if _, err := tx.Exec(ctx, "DELETE FROM relationships WHERE source_id=$1 OR target_id=$1", id); err != nil {
		return false, err
	}
	tag, err := tx.Exec(ctx, "DELETE FROM memories WHERE id=$1 AND project=$2", id, project)
	if err != nil {
		return false, err
	}

	if err := tx.Commit(ctx); err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

// MergeMemoriesAtomic updates winnerID's content (if newContent is non-empty) and
// deletes loserID in a single transaction (#104). This prevents the state where
// winnerID has merged content but loserID was never deleted on a crash.
func (b *PostgresBackend) MergeMemoriesAtomic(ctx context.Context, project, winnerID, loserID, newContent string) error {
	tx, err := b.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	if newContent != "" {
		now := time.Now().UTC()
		hash := ContentHash(newContent)
		if _, err := tx.Exec(ctx,
			"UPDATE memories SET content=$1, content_hash=$2, updated_at=$3 WHERE id=$4 AND project=$5",
			newContent, hash, now, winnerID, project,
		); err != nil {
			return fmt.Errorf("MergeMemoriesAtomic update winner: %w", err)
		}
	}

	// Lock loser row to ensure it still exists before deleting.
	var immutable bool
	err = tx.QueryRow(ctx,
		"SELECT immutable FROM memories WHERE id=$1 AND project=$2 FOR UPDATE",
		loserID, project,
	).Scan(&immutable)
	if err == pgx.ErrNoRows {
		// Loser already gone — treat as already merged.
		return tx.Commit(ctx)
	}
	if err != nil {
		return fmt.Errorf("MergeMemoriesAtomic lock loser: %w", err)
	}
	if immutable {
		return fmt.Errorf("MergeMemoriesAtomic: loser %s is immutable", loserID)
	}

	if _, err := tx.Exec(ctx, "DELETE FROM chunks WHERE memory_id=$1", loserID); err != nil {
		return fmt.Errorf("MergeMemoriesAtomic delete chunks: %w", err)
	}
	if _, err := tx.Exec(ctx, "DELETE FROM relationships WHERE source_id=$1 OR target_id=$1", loserID); err != nil {
		return fmt.Errorf("MergeMemoriesAtomic delete relationships: %w", err)
	}
	if _, err := tx.Exec(ctx, "DELETE FROM memories WHERE id=$1 AND project=$2", loserID, project); err != nil {
		return fmt.Errorf("MergeMemoriesAtomic delete loser: %w", err)
	}

	return tx.Commit(ctx)
}

// ListMemories returns memories for project matching the given filters (#123).
// Uses a clause-slice builder instead of fmt.Sprintf string concat.
func (b *PostgresBackend) ListMemories(ctx context.Context, project string, opts ListOptions) ([]*types.Memory, error) {
	clauses := []string{"project=$1", "valid_to IS NULL"}
	args := []any{project}

	if opts.MemoryType != nil {
		args = append(args, *opts.MemoryType)
		clauses = append(clauses, fmt.Sprintf("memory_type=$%d", len(args)))
	}
	if opts.ImportanceCeiling != nil {
		args = append(args, *opts.ImportanceCeiling)
		clauses = append(clauses, fmt.Sprintf("importance<=$%d", len(args)))
	}
	for _, tag := range opts.Tags {
		j, _ := json.Marshal([]string{tag})
		args = append(args, string(j))
		clauses = append(clauses, fmt.Sprintf("tags @> $%d::jsonb", len(args)))
	}

	limit := opts.Limit
	if limit <= 0 {
		limit = 20
	}
	args = append(args, limit, opts.Offset)

	q := "SELECT * FROM memories WHERE " + strings.Join(clauses, " AND ") +
		fmt.Sprintf(" ORDER BY updated_at DESC LIMIT $%d OFFSET $%d", len(args)-1, len(args))

	rows, err := b.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, rowToMemory)
}

func (b *PostgresBackend) TouchMemory(ctx context.Context, id string) error {
	_, err := b.pool.Exec(ctx,
		"UPDATE memories SET access_count=access_count+1, last_accessed=$1 WHERE id=$2",
		time.Now().UTC(), id,
	)
	return err
}

// TouchMemories batch-updates access_count and last_accessed for multiple memories
// in a single query (#117), replacing the N+1 TouchMemory calls in RecallWithOpts.
func (b *PostgresBackend) TouchMemories(ctx context.Context, ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	_, err := b.pool.Exec(ctx,
		"UPDATE memories SET access_count=access_count+1, last_accessed=$1 WHERE id=ANY($2)",
		time.Now().UTC(), ids,
	)
	return err
}

// versionMemoryTx snapshots the current state of m into memory_versions.
// Must be called inside a pgx.Tx that already holds a FOR UPDATE lock on the row.
func (b *PostgresBackend) versionMemoryTx(ctx context.Context, tx pgx.Tx, m *types.Memory, changeType, changeReason string) error {
	tagsJSON, err := json.Marshal(m.Tags)
	if err != nil {
		return fmt.Errorf("versionMemoryTx: marshal tags: %w", err)
	}
	var reason *string
	if changeReason != "" {
		reason = &changeReason
	}
	now := time.Now().UTC()
	// Close the system_to window on all prior open versions for this memory.
	if _, err = tx.Exec(ctx,
		"UPDATE memory_versions SET system_to=$1 WHERE memory_id=$2 AND system_to IS NULL",
		now, m.ID,
	); err != nil {
		return fmt.Errorf("versionMemoryTx: close prior system_to: %w", err)
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO memory_versions
			(id, memory_id, content, memory_type, tags, importance,
			 system_from, valid_from, valid_to, change_type, change_reason, project)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`,
		types.NewMemoryID(), m.ID, m.Content, m.MemoryType, tagsJSON, m.Importance,
		now, m.ValidFrom, m.ValidTo, changeType, reason, m.Project,
	)
	return err
}

// GetMemoryHistory returns all version snapshots for memoryID in reverse
// chronological order (most recent change first).
func (b *PostgresBackend) GetMemoryHistory(ctx context.Context, project, memoryID string) ([]*types.MemoryVersion, error) {
	rows, err := b.pool.Query(ctx, `
		SELECT id, memory_id, content, memory_type, tags, importance,
		       system_from, system_to, valid_from, valid_to,
		       change_type, change_reason, project
		FROM memory_versions
		WHERE project=$1 AND memory_id=$2
		ORDER BY system_from DESC`,
		project, memoryID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []*types.MemoryVersion
	for rows.Next() {
		var v types.MemoryVersion
		var tagsJSON []byte
		if err := rows.Scan(
			&v.ID, &v.MemoryID, &v.Content, &v.MemoryType, &tagsJSON, &v.Importance,
			&v.SystemFrom, &v.SystemTo, &v.ValidFrom, &v.ValidTo,
			&v.ChangeType, &v.ChangeReason, &v.Project,
		); err != nil {
			return nil, err
		}
		if len(tagsJSON) > 0 {
			if err := json.Unmarshal(tagsJSON, &v.Tags); err != nil {
				return nil, fmt.Errorf("unmarshal tags: %w", err)
			}
		}
		if v.Tags == nil {
			v.Tags = []string{}
		}
		out = append(out, &v)
	}
	return out, rows.Err()
}

// SoftDeleteMemory marks a memory as invalid by setting valid_to=NOW() and
// storing the final state in memory_versions with change_type="invalidate".
// Returns false if not found or already invalidated. Returns error if immutable.
func (b *PostgresBackend) SoftDeleteMemory(ctx context.Context, project, id, reason string) (bool, error) {
	tx, err := b.pool.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	row, err := tx.Query(ctx,
		"SELECT * FROM memories WHERE id=$1 AND project=$2 AND valid_to IS NULL FOR UPDATE",
		id, project,
	)
	if err != nil {
		return false, err
	}
	m, err := pgx.CollectOneRow(row, rowToMemory)
	if err == pgx.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if m.Immutable {
		return false, fmt.Errorf("cannot soft-delete immutable memory %s", id)
	}

	if err := b.versionMemoryTx(ctx, tx, m, types.VersionChangeInvalidate, reason); err != nil {
		return false, fmt.Errorf("version snapshot: %w", err)
	}

	var reasonPtr *string
	if reason != "" {
		reasonPtr = &reason
	}
	now := time.Now().UTC()
	_, err = tx.Exec(ctx,
		"UPDATE memories SET valid_to=$1, invalidation_reason=$2 WHERE id=$3 AND project=$4",
		now, reasonPtr, id, project,
	)
	if err != nil {
		return false, err
	}

	if err := tx.Commit(ctx); err != nil {
		return false, err
	}
	return true, nil
}

// GetMemoriesAsOf returns memories that were active at the given point in time:
// created_at <= asOf AND (valid_to IS NULL OR valid_to > asOf).
func (b *PostgresBackend) GetMemoriesAsOf(ctx context.Context, project string, asOf time.Time, limit int) ([]*types.Memory, error) {
	rows, err := b.pool.Query(ctx, `
		SELECT * FROM memories
		WHERE project=$1 AND created_at <= $2
		  AND (valid_to IS NULL OR valid_to > $2)
		ORDER BY updated_at DESC
		LIMIT $3`,
		project, asOf, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return pgx.CollectRows(rows, rowToMemory)
}

func (b *PostgresBackend) GetMemoriesMissingHash(ctx context.Context, project string, limit int) ([]IDContent, error) {
	rows, err := b.pool.Query(ctx,
		"SELECT id, content FROM memories WHERE project=$1 AND valid_to IS NULL AND content_hash IS NULL LIMIT $2",
		project, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []IDContent
	for rows.Next() {
		var ic IDContent
		if err := rows.Scan(&ic.ID, &ic.Content); err != nil {
			return nil, err
		}
		out = append(out, ic)
	}
	return out, rows.Err()
}

func (b *PostgresBackend) UpdateMemoryHash(ctx context.Context, memoryID, hash string) error {
	_, err := b.pool.Exec(ctx,
		"UPDATE memories SET content_hash=$1 WHERE id=$2 AND project=$3",
		hash, memoryID, b.project)
	return err
}

// ExistsWithContentHash returns true if a non-invalidated memory with the
// given SHA-256 hex content hash already exists in the project. Used by
// handleMemoryIngest to skip duplicate content without storing it again.
func (b *PostgresBackend) ExistsWithContentHash(ctx context.Context, project, hash string) (bool, error) {
	var exists bool
	err := b.pool.QueryRow(ctx,
		"SELECT EXISTS(SELECT 1 FROM memories WHERE project=$1 AND valid_to IS NULL AND content_hash=$2)",
		project, hash).Scan(&exists)
	return exists, err
}

func (b *PostgresBackend) GetIntegrityStats(ctx context.Context, project string) (IntegrityStats, error) {
	var stats IntegrityStats
	if err := b.pool.QueryRow(ctx, "SELECT COUNT(*) FROM memories WHERE project=$1 AND valid_to IS NULL", project).Scan(&stats.Total); err != nil {
		return stats, err
	}
	if err := b.pool.QueryRow(ctx,
		"SELECT COUNT(*) FROM memories WHERE project=$1 AND valid_to IS NULL AND content_hash IS NOT NULL", project,
	).Scan(&stats.Hashed); err != nil {
		return stats, err
	}
	if err := b.pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM memories
		WHERE project=$1 AND valid_to IS NULL AND content_hash IS NOT NULL
		AND content_hash != encode(sha256(convert_to(content,'UTF8')),'hex')`, project,
	).Scan(&stats.Corrupt); err != nil {
		return stats, err
	}
	return stats, nil
}

func (b *PostgresBackend) GetAllMemoryIDs(ctx context.Context, project string) (map[string]struct{}, error) {
	rows, err := b.pool.Query(ctx, "SELECT id FROM memories WHERE project=$1 AND valid_to IS NULL", project)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	ids := map[string]struct{}{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids[id] = struct{}{}
	}
	return ids, rows.Err()
}

func (b *PostgresBackend) GetMemoriesPendingSummary(ctx context.Context, project string, limit int) ([]IDContent, error) {
	rows, err := b.pool.Query(ctx,
		"SELECT id, content FROM memories WHERE project=$1 AND valid_to IS NULL AND (summary IS NULL OR summary = content) LIMIT $2",
		project, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []IDContent
	for rows.Next() {
		var ic IDContent
		if err := rows.Scan(&ic.ID, &ic.Content); err != nil {
			return nil, err
		}
		out = append(out, ic)
	}
	return out, rows.Err()
}

func (b *PostgresBackend) StoreSummary(ctx context.Context, memoryID, summary string) error {
	_, err := b.pool.Exec(ctx,
		"UPDATE memories SET summary=$1 WHERE id=$2 AND project=$3",
		summary, memoryID, b.project)
	return err
}

func (b *PostgresBackend) GetPendingSummaryCount(ctx context.Context, project string) (int, error) {
	var count int
	err := b.pool.QueryRow(ctx,
		"SELECT COUNT(*) FROM memories WHERE project=$1 AND valid_to IS NULL AND (summary IS NULL OR summary = content)", project,
	).Scan(&count)
	return count, err
}

// ClearSummaries sets summary = NULL for all active memories in a project,
// causing the background summarize worker to regenerate them on its next tick.
// Returns the number of rows affected.
func (b *PostgresBackend) ClearSummaries(ctx context.Context, project string) (int, error) {
	result, err := b.pool.Exec(ctx,
		"UPDATE memories SET summary = NULL WHERE project = $1 AND valid_to IS NULL AND summary IS NOT NULL",
		project)
	if err != nil {
		return 0, err
	}
	return int(result.RowsAffected()), nil
}
