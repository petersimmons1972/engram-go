package db

import (
	"context"
	"crypto/sha256"
	"embed"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"regexp"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	pgvector "github.com/pgvector/pgvector-go"
	"github.com/petersimmons1972/engram/internal/types"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// projectSlugRE strips characters not safe for project names.
var projectSlugRE = regexp.MustCompile(`[^a-zA-Z0-9_-]`)

// PostgresBackend is the PostgreSQL implementation of Backend.
// It is safe for concurrent use from multiple goroutines.
type PostgresBackend struct {
	pool    *pgxpool.Pool
	project string // validated project slug
}

// NewPostgresBackend creates a new backend, validates the connection, and
// runs schema migrations. Returns an error if the database is unreachable.
func NewPostgresBackend(ctx context.Context, project, dsn string) (*PostgresBackend, error) {
	project = projectSlugRE.ReplaceAllString(project, "")
	if project == "" {
		project = "default"
	}

	warnDefaultPassword(dsn)

	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("invalid DSN: %w", err)
	}
	cfg.MinConns = 2
	cfg.MaxConns = 10

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("cannot create connection pool: %w", err)
	}

	// Verify connectivity before running migrations.
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("cannot connect to PostgreSQL — check DATABASE_URL: %w", err)
	}

	b := &PostgresBackend{pool: pool, project: project}
	if err := b.runMigrations(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("schema migration failed: %w", err)
	}

	return b, nil
}

func warnDefaultPassword(dsn string) {
	if strings.Contains(dsn, ":engram@") || strings.Contains(dsn, "%3Aengram%40") {
		slog.Warn("SECURITY: PostgreSQL using default password 'engram'; set a strong POSTGRES_PASSWORD before exposing this service")
	}
}

// Close releases the connection pool.
func (b *PostgresBackend) Close() {
	b.pool.Close()
}

func (b *PostgresBackend) runMigrations(ctx context.Context) error {
	// Ensure the migration tracking table exists. This is always idempotent.
	const createTracker = `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			filename   TEXT PRIMARY KEY,
			applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`
	if _, err := b.pool.Exec(ctx, createTracker); err != nil {
		return fmt.Errorf("create schema_migrations table: %w", err)
	}

	entries, err := migrationsFS.ReadDir("migrations")
	if err != nil {
		return fmt.Errorf("read migrations dir: %w", err)
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".sql") {
			continue
		}
		name := e.Name()

		// Skip already-applied migrations.
		var applied bool
		err := b.pool.QueryRow(ctx,
			`SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE filename = $1)`, name,
		).Scan(&applied)
		if err != nil {
			return fmt.Errorf("check migration %s: %w", name, err)
		}
		if applied {
			continue
		}

		// Gate 004: only apply if backfill is complete.
		if name == "004_pgvector_finalize.sql" {
			var pending string
			err := b.pool.QueryRow(ctx,
				`SELECT COALESCE(
					(SELECT value FROM project_meta WHERE project='_engram' AND key='pgvector_backfill_pending'),
					'false'
				)`).Scan(&pending)
			if err != nil {
				return fmt.Errorf("check backfill status: %w", err)
			}
			if pending == "true" {
				slog.Info("skipping 004_pgvector_finalize.sql — backfill still pending")
				continue
			}
		}

		sql, err := migrationsFS.ReadFile("migrations/" + name)
		if err != nil {
			return fmt.Errorf("read migration %s: %w", name, err)
		}
		if _, err := b.pool.Exec(ctx, string(sql)); err != nil {
			return fmt.Errorf("apply migration %s: %w", name, err)
		}
		if _, err := b.pool.Exec(ctx,
			`INSERT INTO schema_migrations (filename) VALUES ($1)`, name,
		); err != nil {
			return fmt.Errorf("record migration %s: %w", name, err)
		}
		slog.Info("applied migration", "file", name)

		// After 003: run the Go-side backfill.
		if name == "003_pgvector.sql" {
			if err := b.backfillVectors(ctx); err != nil {
				return fmt.Errorf("pgvector backfill failed: %w", err)
			}
		}
	}
	return nil
}

// backfillVectors converts existing BYTEA embeddings to the new embedding_vec
// vector(768) column. Called once after 003_pgvector.sql creates the column.
// Idempotent: skips rows where embedding_vec is already populated.
func (b *PostgresBackend) backfillVectors(ctx context.Context) error {
	rows, err := b.pool.Query(ctx, `
		SELECT id, embedding FROM chunks
		WHERE embedding IS NOT NULL AND embedding_vec IS NULL`)
	if err != nil {
		return fmt.Errorf("backfill query: %w", err)
	}
	defer rows.Close()

	var converted int
	for rows.Next() {
		var id string
		var blob []byte
		if err := rows.Scan(&id, &blob); err != nil {
			return fmt.Errorf("backfill scan: %w", err)
		}
		if len(blob)%4 != 0 || len(blob) == 0 {
			slog.Warn("backfill: skipping chunk with invalid BYTEA length", "id", id, "len", len(blob))
			continue
		}
		// Decode little-endian float32 blob.
		vec := make([]float32, len(blob)/4)
		for i := range vec {
			u := uint32(blob[4*i]) | uint32(blob[4*i+1])<<8 | uint32(blob[4*i+2])<<16 | uint32(blob[4*i+3])<<24
			vec[i] = math.Float32frombits(u)
		}
		// Write to the new vector column using pgvector encoding.
		if _, err := b.pool.Exec(ctx,
			"UPDATE chunks SET embedding_vec = $1 WHERE id = $2",
			pgvector.NewVector(vec), id,
		); err != nil {
			return fmt.Errorf("backfill update chunk %s: %w", id, err)
		}
		converted++
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("backfill iteration: %w", err)
	}

	slog.Info("pgvector backfill complete", "chunks_converted", converted)

	// Mark backfill done so 004 can run.
	_, err = b.pool.Exec(ctx,
		`UPDATE project_meta SET value='false' WHERE project='_engram' AND key='pgvector_backfill_pending'`)
	return err
}

// ── Transactions ─────────────────────────────────────────────────────────────

// pgxTx wraps pgx.Tx to implement the Tx interface.
type pgxTx struct{ tx pgx.Tx }

func (t *pgxTx) Commit(ctx context.Context) error   { return t.tx.Commit(ctx) }
func (t *pgxTx) Rollback(ctx context.Context) error { return t.tx.Rollback(ctx) }

// Begin starts a new transaction.
func (b *PostgresBackend) Begin(ctx context.Context) (Tx, error) {
	tx, err := b.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	return &pgxTx{tx: tx}, nil
}

// unwrapTx extracts the underlying pgx.Tx from a Tx interface value.
func unwrapTx(t Tx) pgx.Tx { return t.(*pgxTx).tx }

// ── Project metadata ──────────────────────────────────────────────────────────

func (b *PostgresBackend) GetMeta(ctx context.Context, project, key string) (string, bool, error) {
	var value string
	err := b.pool.QueryRow(ctx,
		"SELECT value FROM project_meta WHERE project=$1 AND key=$2",
		project, key,
	).Scan(&value)
	if err == pgx.ErrNoRows {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return value, true, nil
}

func (b *PostgresBackend) SetMeta(ctx context.Context, project, key, value string) error {
	_, err := b.pool.Exec(ctx,
		"INSERT INTO project_meta (project, key, value) VALUES ($1,$2,$3) "+
			"ON CONFLICT (project, key) DO UPDATE SET value = EXCLUDED.value",
		project, key, value,
	)
	return err
}

// ── Memory CRUD ───────────────────────────────────────────────────────────────

func contentHash(content string) string {
	h := sha256.Sum256([]byte(content))
	return fmt.Sprintf("%x", h)
}

func (b *PostgresBackend) StoreMemory(ctx context.Context, m *types.Memory) error {
	return b.storeMemoryExec(ctx, b.pool, m)
}

func (b *PostgresBackend) StoreMemoryTx(ctx context.Context, tx Tx, m *types.Memory) error {
	return b.storeMemoryExec(ctx, unwrapTx(tx), m)
}

// execer is satisfied by both *pgxpool.Pool and pgx.Tx.
type execer interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

func (b *PostgresBackend) storeMemoryExec(ctx context.Context, ex execer, m *types.Memory) error {
	now := time.Now().UTC()
	m.CreatedAt = now
	m.UpdatedAt = now
	m.LastAccessed = now
	m.Project = b.project
	hash := contentHash(m.Content)
	m.ContentHash = &hash

	tagsJSON, err := json.Marshal(m.Tags)
	if err != nil {
		return fmt.Errorf("marshal tags: %w", err)
	}

	_, err = ex.Exec(ctx, `
		INSERT INTO memories
		  (id, content, memory_type, project, tags,
		   importance, access_count, last_accessed, created_at, updated_at,
		   immutable, expires_at, content_hash, storage_mode)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)`,
		m.ID, m.Content, m.MemoryType, m.Project, tagsJSON,
		m.Importance, m.AccessCount, now, now, now,
		m.Immutable, m.ExpiresAt, hash, m.StorageMode,
	)
	return err
}

func (b *PostgresBackend) GetMemory(ctx context.Context, id string) (*types.Memory, error) {
	row, err := b.pool.Query(ctx,
		"SELECT * FROM memories WHERE id=$1 AND project=$2", id, b.project)
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
		expected := contentHash(m.Content)
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
		"SELECT * FROM memories WHERE project=$1 AND id=ANY($2)",
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
		"SELECT * FROM memories WHERE id=$1 AND project=$2 FOR UPDATE",
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
		hash := contentHash(m.Content)
		m.ContentHash = &hash
		_, err = tx.Exec(ctx,
			"UPDATE memories SET content=$1, tags=$2, importance=$3, updated_at=$4, content_hash=$5 WHERE id=$6 AND project=$7",
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
	tag, err := tx.Exec(ctx, "DELETE FROM memories WHERE id=$1", id)
	if err != nil {
		return false, err
	}

	if err := tx.Commit(ctx); err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

func (b *PostgresBackend) ListMemories(ctx context.Context, project string, opts ListOptions) ([]*types.Memory, error) {
	q := "SELECT * FROM memories WHERE project=$1"
	args := []any{project}
	n := 2

	if opts.MemoryType != nil {
		q += fmt.Sprintf(" AND memory_type=$%d", n)
		args = append(args, *opts.MemoryType)
		n++
	}
	if opts.ImportanceCeiling != nil {
		q += fmt.Sprintf(" AND importance<=$%d", n)
		args = append(args, *opts.ImportanceCeiling)
		n++
	}
	for _, tag := range opts.Tags {
		q += fmt.Sprintf(" AND tags @> $%d::jsonb", n)
		j, _ := json.Marshal([]string{tag})
		args = append(args, string(j))
		n++
	}

	limit := opts.Limit
	if limit <= 0 {
		limit = 20
	}
	q += fmt.Sprintf(" ORDER BY updated_at DESC LIMIT $%d OFFSET $%d", n, n+1)
	args = append(args, limit, opts.Offset)

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

// ── Chunk CRUD ────────────────────────────────────────────────────────────────

func (b *PostgresBackend) StoreChunks(ctx context.Context, chunks []*types.Chunk) error {
	return b.storeChunksExec(ctx, b.pool, chunks)
}

func (b *PostgresBackend) StoreChunksTx(ctx context.Context, tx Tx, chunks []*types.Chunk) error {
	return b.storeChunksExec(ctx, unwrapTx(tx), chunks)
}

func (b *PostgresBackend) storeChunksExec(ctx context.Context, ex execer, chunks []*types.Chunk) error {
	if len(chunks) == 0 {
		return nil
	}
	const chunkSQL = `
		INSERT INTO chunks (id, memory_id, project, chunk_text, chunk_index,
		                    chunk_hash, embedding, section_heading, chunk_type)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
		ON CONFLICT (id) DO NOTHING`
	for _, c := range chunks {
		var embParam any
		if len(c.Embedding) > 0 {
			embParam = pgvector.NewVector(c.Embedding)
		}
		_, err := ex.Exec(ctx, chunkSQL,
			c.ID, c.MemoryID, c.Project,
			c.ChunkText, c.ChunkIndex, c.ChunkHash,
			embParam, c.SectionHeading, c.ChunkType,
		)
		if err != nil {
			return err
		}
	}
	return nil
}

func (b *PostgresBackend) GetChunksForMemory(ctx context.Context, memoryID string) ([]*types.Chunk, error) {
	rows, err := b.pool.Query(ctx,
		"SELECT * FROM chunks WHERE memory_id=$1 ORDER BY chunk_index", memoryID,
	)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, rowToChunk)
}

func (b *PostgresBackend) GetAllChunksWithEmbeddings(ctx context.Context, project string, limit int) ([]*types.Chunk, error) {
	rows, err := b.pool.Query(ctx, `
		SELECT c.* FROM chunks c
		JOIN memories m ON m.id = c.memory_id
		WHERE c.embedding IS NOT NULL AND m.project=$1
		ORDER BY m.last_accessed DESC
		LIMIT $2`, project, limit,
	)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, rowToChunk)
}

func (b *PostgresBackend) GetAllChunkTexts(ctx context.Context, project string, limit int) ([]string, error) {
	rows, err := b.pool.Query(ctx, `
		SELECT c.chunk_text FROM chunks c
		JOIN memories m ON m.id = c.memory_id
		WHERE m.project=$1 LIMIT $2`, project, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var texts []string
	for rows.Next() {
		var t string
		if err := rows.Scan(&t); err != nil {
			return nil, err
		}
		texts = append(texts, t)
	}
	return texts, rows.Err()
}

func (b *PostgresBackend) GetChunksForMemories(ctx context.Context, memoryIDs []string) ([]*types.Chunk, error) {
	if len(memoryIDs) == 0 {
		return nil, nil
	}
	rows, err := b.pool.Query(ctx, `
		SELECT c.* FROM chunks c
		WHERE c.memory_id = ANY($1) AND c.embedding IS NOT NULL
		ORDER BY c.chunk_index`, memoryIDs,
	)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, rowToChunk)
}

func (b *PostgresBackend) ChunkHashExists(ctx context.Context, chunkHash, memoryID string) (bool, error) {
	var exists bool
	err := b.pool.QueryRow(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM chunks
			WHERE chunk_hash=$1 AND memory_id=$2
		)`, chunkHash, memoryID,
	).Scan(&exists)
	return exists, err
}

func (b *PostgresBackend) DeleteChunksForMemory(ctx context.Context, memoryID string) error {
	_, err := b.pool.Exec(ctx, "DELETE FROM chunks WHERE memory_id=$1", memoryID)
	return err
}

func (b *PostgresBackend) DeleteChunksByIDs(ctx context.Context, chunkIDs []string) (int, error) {
	if len(chunkIDs) == 0 {
		return 0, nil
	}
	tag, err := b.pool.Exec(ctx, "DELETE FROM chunks WHERE id=ANY($1)", chunkIDs)
	return int(tag.RowsAffected()), err
}

func (b *PostgresBackend) NullAllEmbeddings(ctx context.Context, project string) (int, error) {
	tag, err := b.pool.Exec(ctx,
		"UPDATE chunks SET embedding=NULL WHERE memory_id IN (SELECT id FROM memories WHERE project=$1)",
		project,
	)
	return int(tag.RowsAffected()), err
}

func (b *PostgresBackend) GetChunksPendingEmbedding(ctx context.Context, project string, limit int) ([]*types.Chunk, error) {
	rows, err := b.pool.Query(ctx, `
		SELECT c.* FROM chunks c
		JOIN memories m ON m.id = c.memory_id
		WHERE m.project=$1 AND c.embedding IS NULL
		ORDER BY m.last_accessed DESC
		LIMIT $2`, project, limit,
	)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, rowToChunk)
}

func (b *PostgresBackend) UpdateChunkEmbedding(ctx context.Context, chunkID string, embedding []float32) (int, error) {
	tag, err := b.pool.Exec(ctx,
		"UPDATE chunks SET embedding=$1 WHERE id=$2", pgvector.NewVector(embedding), chunkID,
	)
	return int(tag.RowsAffected()), err
}

func (b *PostgresBackend) VectorSearch(ctx context.Context, project string, queryVec []float32, limit int) ([]VectorHit, error) {
	rows, err := b.pool.Query(ctx, `
		SELECT c.id, c.memory_id,
		       c.embedding <=> $1::vector AS distance,
		       c.chunk_text, c.chunk_index, c.section_heading
		FROM chunks c
		WHERE c.project = $2 AND c.embedding IS NOT NULL
		ORDER BY c.embedding <=> $1::vector
		LIMIT $3`,
		pgvector.NewVector(queryVec), project, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var hits []VectorHit
	for rows.Next() {
		var h VectorHit
		if err := rows.Scan(&h.ChunkID, &h.MemoryID, &h.Distance,
			&h.ChunkText, &h.ChunkIndex, &h.SectionHeading); err != nil {
			return nil, err
		}
		hits = append(hits, h)
	}
	return hits, rows.Err()
}

func (b *PostgresBackend) ChunkEmbeddingDistance(ctx context.Context, memAID, memBID string) (float64, error) {
	var dist *float64
	err := b.pool.QueryRow(ctx, `
		SELECT MIN(ca.embedding <=> cb.embedding)
		FROM chunks ca, chunks cb
		WHERE ca.memory_id = $1 AND cb.memory_id = $2
		  AND ca.embedding IS NOT NULL AND cb.embedding IS NOT NULL`,
		memAID, memBID,
	).Scan(&dist)
	if err != nil {
		return 2.0, err
	}
	if dist == nil {
		return 2.0, nil // no embedded chunks
	}
	return *dist, nil
}

func (b *PostgresBackend) UpdateChunkLastMatched(ctx context.Context, chunkID string) error {
	_, err := b.pool.Exec(ctx,
		"UPDATE chunks SET last_matched=NOW() WHERE id=$1", chunkID,
	)
	return err
}

func (b *PostgresBackend) GetPendingEmbeddingCount(ctx context.Context, project string) (int, error) {
	var count int
	err := b.pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM chunks c
		JOIN memories m ON m.id = c.memory_id
		WHERE m.project=$1 AND c.embedding IS NULL`, project,
	).Scan(&count)
	return count, err
}

// ── Relationship CRUD ─────────────────────────────────────────────────────────

func (b *PostgresBackend) StoreRelationship(ctx context.Context, rel *types.Relationship) error {
	// Verify both memories exist.
	var dummy int
	if err := b.pool.QueryRow(ctx, "SELECT 1 FROM memories WHERE id=$1", rel.SourceID).Scan(&dummy); err != nil {
		if err == pgx.ErrNoRows {
			return fmt.Errorf("source memory %q does not exist", rel.SourceID)
		}
		return fmt.Errorf("check source memory: %w", err)
	}
	if err := b.pool.QueryRow(ctx, "SELECT 1 FROM memories WHERE id=$1", rel.TargetID).Scan(&dummy); err != nil {
		if err == pgx.ErrNoRows {
			return fmt.Errorf("target memory %q does not exist", rel.TargetID)
		}
		return fmt.Errorf("check target memory: %w", err)
	}

	rel.Project = b.project
	_, err := b.pool.Exec(ctx, `
		INSERT INTO relationships
		  (id, source_id, target_id, rel_type, strength, project, created_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7)
		ON CONFLICT (source_id, target_id, rel_type)
		DO UPDATE SET strength = EXCLUDED.strength`,
		rel.ID, rel.SourceID, rel.TargetID,
		rel.RelType, rel.Strength, rel.Project, rel.CreatedAt,
	)
	return err
}

func (b *PostgresBackend) GetConnected(ctx context.Context, memoryID string, maxHops int) ([]ConnectedResult, error) {
	visited := map[string]struct{}{memoryID: {}}
	var results []ConnectedResult
	frontier := []string{memoryID}

	for hop := 0; hop < maxHops && len(frontier) > 0; hop++ {
		type pending struct {
			nid       string
			relType   string
			direction string
			strength  float64
		}
		var batch []pending

		// Outgoing edges
		rows, err := b.pool.Query(ctx,
			"SELECT target_id, rel_type, strength FROM relationships WHERE source_id=ANY($1) AND project=$2",
			frontier, b.project,
		)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var nid, rt string
			var strength float64
			if err := rows.Scan(&nid, &rt, &strength); err != nil {
				rows.Close()
				return nil, err
			}
			if _, seen := visited[nid]; !seen {
				visited[nid] = struct{}{}
				batch = append(batch, pending{nid, rt, "outgoing", strength})
			}
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return nil, err
		}

		// Incoming edges
		rows, err = b.pool.Query(ctx,
			"SELECT source_id, rel_type, strength FROM relationships WHERE target_id=ANY($1) AND project=$2",
			frontier, b.project,
		)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var nid, rt string
			var strength float64
			if err := rows.Scan(&nid, &rt, &strength); err != nil {
				rows.Close()
				return nil, err
			}
			if _, seen := visited[nid]; !seen {
				visited[nid] = struct{}{}
				batch = append(batch, pending{nid, rt, "incoming", strength})
			}
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return nil, err
		}

		if len(batch) == 0 {
			break
		}

		// Resolve new nodes in one query.
		newIDs := make([]string, len(batch))
		for i, p := range batch {
			newIDs[i] = p.nid
		}
		memRows, err := b.pool.Query(ctx, "SELECT * FROM memories WHERE id=ANY($1)", newIDs)
		if err != nil {
			return nil, err
		}
		memByID := map[string]*types.Memory{}
		for memRows.Next() {
			m, err := rowToMemory(memRows)
			if err != nil {
				memRows.Close()
				return nil, err
			}
			memByID[m.ID] = m
		}
		memRows.Close()
		if err := memRows.Err(); err != nil {
			return nil, err
		}

		frontier = frontier[:0]
		for _, p := range batch {
			if m, ok := memByID[p.nid]; ok {
				results = append(results, ConnectedResult{
					Memory:    m,
					RelType:   p.relType,
					Direction: p.direction,
					Strength:  p.strength,
				})
				frontier = append(frontier, p.nid)
			}
		}
	}

	return results, nil
}

func (b *PostgresBackend) BoostEdgesForMemory(ctx context.Context, memoryID string, factor float64) (int, error) {
	tag, err := b.pool.Exec(ctx, `
		UPDATE relationships SET strength=LEAST(1.0, strength*$1)
		WHERE source_id=$2 OR target_id=$2`, factor, memoryID,
	)
	return int(tag.RowsAffected()), err
}

func (b *PostgresBackend) DecayEdgesForMemory(ctx context.Context, memoryID string, factor float64) (int, error) {
	tag, err := b.pool.Exec(ctx, `
		UPDATE relationships SET strength=GREATEST(0.0, strength-$1)
		WHERE source_id=$2 OR target_id=$2`, factor, memoryID,
	)
	return int(tag.RowsAffected()), err
}

func (b *PostgresBackend) GetConnectionCount(ctx context.Context, memoryID, project string) (int, error) {
	var count int
	err := b.pool.QueryRow(ctx,
		"SELECT COUNT(*) FROM relationships WHERE (source_id=$1 OR target_id=$1) AND project=$2",
		memoryID, project,
	).Scan(&count)
	return count, err
}

func (b *PostgresBackend) DecayAllEdges(ctx context.Context, project string, decayFactor, minStrength float64) (int, int, error) {
	var decayed, pruned int
	tag, err := b.pool.Exec(ctx, `
		UPDATE relationships SET strength=GREATEST(0.0, strength-$1)
		WHERE project=$2`, decayFactor, project,
	)
	if err != nil {
		return 0, 0, err
	}
	decayed = int(tag.RowsAffected())

	tag, err = b.pool.Exec(ctx,
		"DELETE FROM relationships WHERE strength<$1 AND project=$2",
		minStrength, project,
	)
	if err != nil {
		return decayed, 0, err
	}
	pruned = int(tag.RowsAffected())
	return decayed, pruned, nil
}

func (b *PostgresBackend) DeleteRelationshipsForMemory(ctx context.Context, memoryID string) error {
	_, err := b.pool.Exec(ctx,
		"DELETE FROM relationships WHERE source_id=$1 OR target_id=$1", memoryID,
	)
	return err
}

// ── Pruning ───────────────────────────────────────────────────────────────────

func (b *PostgresBackend) PruneStaleMemories(ctx context.Context, project string, maxAgeHours float64, maxImportance int) (int, error) {
	cutoff := time.Now().UTC().Add(-time.Duration(maxAgeHours * float64(time.Hour)))
	tag, err := b.pool.Exec(ctx, `
		DELETE FROM memories
		WHERE project=$1 AND NOT immutable AND (
			(importance>=$2 AND last_accessed<$3 AND access_count=0)
			OR (expires_at IS NOT NULL AND expires_at<NOW())
		)`, project, maxImportance, cutoff,
	)
	return int(tag.RowsAffected()), err
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

// ── Full-text search ──────────────────────────────────────────────────────────

func (b *PostgresBackend) FTSSearch(ctx context.Context, project, query string, limit int, since, before *time.Time) ([]FTSResult, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, nil
	}

	q := `SELECT m.*, ts_rank(m.search_vector, plainto_tsquery('english', $1)) AS rank
		  FROM memories m
		  WHERE m.search_vector @@ plainto_tsquery('english', $2)
		  AND m.project=$3`
	args := []any{query, query, project}
	n := 4

	if since != nil {
		q += fmt.Sprintf(" AND m.created_at>=$%d", n)
		args = append(args, since)
		n++
	}
	if before != nil {
		q += fmt.Sprintf(" AND m.created_at<=$%d", n)
		args = append(args, before)
		n++
	}
	q += fmt.Sprintf(" ORDER BY rank DESC LIMIT $%d", n)
	args = append(args, limit)

	rows, err := b.pool.Query(ctx, q, args...)
	if err != nil {
		slog.Debug("FTS query failed", "query_len", len(query), "err", err)
		return nil, nil
	}
	defer rows.Close()

	var results []FTSResult
	for rows.Next() {
		// SELECT m.*, rank — 17 columns: 16 memory fields + rank.
		// Scan all fields in one call to avoid consuming the cursor twice.
		var (
			id, content, memType, project string
			tags                          []byte
			importance, accessCount       int
			lastAccessed, createdAt, updatedAt time.Time
			immutable                     bool
			expiresAt                     *time.Time
			summary, contentHash          *string
			storageMode                   string
			searchVector                  []byte
			rank                          float64
		)
		if err := rows.Scan(
			&id, &content, &memType, &project, &tags,
			&importance, &accessCount, &lastAccessed, &createdAt, &updatedAt,
			&immutable, &expiresAt, &summary, &contentHash, &storageMode,
			&searchVector, &rank,
		); err != nil {
			return nil, err
		}
		var tagSlice []string
		if len(tags) > 0 {
			_ = json.Unmarshal(tags, &tagSlice)
		}
		if tagSlice == nil {
			tagSlice = []string{}
		}
		if storageMode == "" {
			storageMode = "focused"
		}
		m := &types.Memory{
			ID:           id,
			Content:      content,
			MemoryType:   memType,
			Project:      project,
			Tags:         tagSlice,
			Importance:   importance,
			AccessCount:  accessCount,
			LastAccessed: lastAccessed,
			CreatedAt:    createdAt,
			UpdatedAt:    updatedAt,
			Immutable:    immutable,
			ExpiresAt:    expiresAt,
			Summary:      summary,
			ContentHash:  contentHash,
			StorageMode:  storageMode,
		}
		results = append(results, FTSResult{Memory: m, Score: rank})
	}
	return results, rows.Err()
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

// ── Stats and integrity ───────────────────────────────────────────────────────

func (b *PostgresBackend) GetStats(ctx context.Context, project string) (*types.MemoryStats, error) {
	stats := &types.MemoryStats{
		ByType:        map[string]int{},
		ByImportance:  map[string]int{},
		Summarization: map[string]any{},
	}

	if err := b.pool.QueryRow(ctx, "SELECT COUNT(*) FROM memories WHERE project=$1", project).Scan(&stats.TotalMemories); err != nil {
		return nil, err
	}
	if err := b.pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM chunks c JOIN memories m ON m.id=c.memory_id WHERE m.project=$1`, project,
	).Scan(&stats.TotalChunks); err != nil {
		return nil, err
	}
	if err := b.pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM relationships WHERE project=$1`, project,
	).Scan(&stats.TotalRelationships); err != nil {
		return nil, err
	}

	typeRows, err := b.pool.Query(ctx,
		"SELECT memory_type, COUNT(*) FROM memories WHERE project=$1 GROUP BY memory_type", project)
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
		"SELECT importance, COUNT(*) FROM memories WHERE project=$1 GROUP BY importance", project)
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
	if err := b.pool.QueryRow(ctx, "SELECT MIN(created_at) FROM memories WHERE project=$1", project).Scan(&oldest); err != nil {
		return nil, err
	}
	if err := b.pool.QueryRow(ctx, "SELECT MAX(created_at) FROM memories WHERE project=$1", project).Scan(&newest); err != nil {
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
		"SELECT COUNT(*) FROM memories WHERE project=$1 AND summary IS NULL", project,
	).Scan(&stats.PendingSummarization); err != nil {
		return nil, err
	}

	if err := b.pool.QueryRow(ctx, "SELECT pg_database_size(current_database())").Scan(&stats.DBSizeBytes); err != nil {
		return nil, err
	}

	return stats, nil
}

func (b *PostgresBackend) ListAllProjects(ctx context.Context) ([]string, error) {
	rows, err := b.pool.Query(ctx, "SELECT DISTINCT project FROM memories ORDER BY project")
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

func (b *PostgresBackend) GetAllMemoryIDs(ctx context.Context, project string) (map[string]struct{}, error) {
	rows, err := b.pool.Query(ctx, "SELECT id FROM memories WHERE project=$1", project)
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
		"SELECT id, content FROM memories WHERE project=$1 AND summary IS NULL LIMIT $2",
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
		"SELECT COUNT(*) FROM memories WHERE project=$1 AND summary IS NULL", project,
	).Scan(&count)
	return count, err
}

func (b *PostgresBackend) GetMemoriesMissingHash(ctx context.Context, project string, limit int) ([]IDContent, error) {
	rows, err := b.pool.Query(ctx,
		"SELECT id, content FROM memories WHERE project=$1 AND content_hash IS NULL LIMIT $2",
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

func (b *PostgresBackend) UpdateMemoryHash(ctx context.Context, memoryID, contentHash string) error {
	_, err := b.pool.Exec(ctx,
		"UPDATE memories SET content_hash=$1 WHERE id=$2 AND project=$3",
		contentHash, memoryID, b.project)
	return err
}

func (b *PostgresBackend) GetIntegrityStats(ctx context.Context, project string) (IntegrityStats, error) {
	var stats IntegrityStats
	if err := b.pool.QueryRow(ctx, "SELECT COUNT(*) FROM memories WHERE project=$1", project).Scan(&stats.Total); err != nil {
		return stats, err
	}
	if err := b.pool.QueryRow(ctx,
		"SELECT COUNT(*) FROM memories WHERE project=$1 AND content_hash IS NOT NULL", project,
	).Scan(&stats.Hashed); err != nil {
		return stats, err
	}
	if err := b.pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM memories
		WHERE project=$1 AND content_hash IS NOT NULL
		AND content_hash != encode(sha256(content::bytea),'hex')`, project,
	).Scan(&stats.Corrupt); err != nil {
		return stats, err
	}
	return stats, nil
}

// ── Row mappers ───────────────────────────────────────────────────────────────

func rowToMemory(row pgx.CollectableRow) (*types.Memory, error) {
	type raw struct {
		ID           string
		Content      string
		MemoryType   string
		Project      string
		Tags         []byte
		Importance   int
		AccessCount  int
		LastAccessed time.Time
		CreatedAt    time.Time
		UpdatedAt    time.Time
		Immutable    bool
		ExpiresAt    *time.Time
		Summary      *string
		ContentHash  *string
		StorageMode  string
		SearchVector []byte // ignored — generated column
	}
	var r raw
	err := row.Scan(
		&r.ID, &r.Content, &r.MemoryType, &r.Project, &r.Tags,
		&r.Importance, &r.AccessCount, &r.LastAccessed, &r.CreatedAt, &r.UpdatedAt,
		&r.Immutable, &r.ExpiresAt, &r.Summary, &r.ContentHash, &r.StorageMode,
		&r.SearchVector,
	)
	if err != nil {
		return nil, err
	}

	var tags []string
	if len(r.Tags) > 0 {
		if err := json.Unmarshal(r.Tags, &tags); err != nil {
			tags = []string{}
		}
	}
	if tags == nil {
		tags = []string{}
	}

	storageMode := r.StorageMode
	if storageMode == "" {
		storageMode = "focused"
	}

	return &types.Memory{
		ID:           r.ID,
		Content:      r.Content,
		MemoryType:   r.MemoryType,
		Project:      r.Project,
		Tags:         tags,
		Importance:   r.Importance,
		AccessCount:  r.AccessCount,
		LastAccessed: r.LastAccessed,
		CreatedAt:    r.CreatedAt,
		UpdatedAt:    r.UpdatedAt,
		Immutable:    r.Immutable,
		ExpiresAt:    r.ExpiresAt,
		Summary:      r.Summary,
		ContentHash:  r.ContentHash,
		StorageMode:  storageMode,
	}, nil
}

func rowToChunk(row pgx.CollectableRow) (*types.Chunk, error) {
	type raw struct {
		ID             string
		MemoryID       string
		Project        string
		ChunkText      string
		ChunkIndex     int
		ChunkHash      string
		Embedding      pgvector.Vector
		SectionHeading *string
		ChunkType      string
		LastMatched    *time.Time
	}
	var r raw
	err := row.Scan(
		&r.ID, &r.MemoryID, &r.Project,
		&r.ChunkText, &r.ChunkIndex, &r.ChunkHash,
		&r.Embedding, &r.SectionHeading, &r.ChunkType, &r.LastMatched,
	)
	if err != nil {
		return nil, err
	}

	chunkType := r.ChunkType
	if chunkType == "" {
		chunkType = "sentence_window"
	}

	return &types.Chunk{
		ID:             r.ID,
		MemoryID:       r.MemoryID,
		Project:        r.Project,
		ChunkText:      r.ChunkText,
		ChunkIndex:     r.ChunkIndex,
		ChunkHash:      r.ChunkHash,
		Embedding:      r.Embedding.Slice(),
		SectionHeading: r.SectionHeading,
		ChunkType:      chunkType,
		LastMatched:    r.LastMatched,
	}, nil
}
