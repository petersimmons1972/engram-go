package db

import (
	"context"
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

	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("invalid DSN: %w", err)
	}
	if err := rejectDefaultPassword(cfg); err != nil {
		return nil, err
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

// rejectDefaultPassword refuses to start if a well-known default password is detected (#124).
// A warning is insufficient: operators who miss the log line leave the database exposed indefinitely.
func rejectDefaultPassword(cfg *pgxpool.Config) error {
	if cfg.ConnConfig.Password == "engram" || cfg.ConnConfig.Password == "postgres" {
		return fmt.Errorf("SECURITY: PostgreSQL is using a well-known default password (%q) — "+
			"set a strong POSTGRES_PASSWORD in your environment before starting engram", cfg.ConnConfig.Password)
	}
	return nil
}

// Close releases the connection pool.
func (b *PostgresBackend) Close() {
	b.pool.Close()
}

// Pool returns the underlying connection pool. Intended for integration test
// helpers that need to issue raw SQL (e.g. back-dating created_at for time-based
// test scenarios). Not part of the Backend interface.
func (b *PostgresBackend) Pool() *pgxpool.Pool {
	return b.pool
}

func (b *PostgresBackend) runMigrations(ctx context.Context) error {
	// Serialize concurrent project initialization with a per-project advisory lock (#105).
	// Two backends for the same project initializing simultaneously would both pass the
	// "migration already applied?" check and then race to apply the same DDL.
	// Lock class 1986753120 is an arbitrary constant reserved for engram schema migrations.
	const lockClass = 1986753120
	conn, err := b.pool.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("acquire connection for advisory lock: %w", err)
	}
	defer conn.Release()

	if _, err := conn.Exec(ctx,
		`SELECT pg_advisory_lock($1, hashtext($2::text))`, lockClass, b.project,
	); err != nil {
		return fmt.Errorf("acquire advisory lock for project %q: %w", b.project, err)
	}
	defer func() {
		_, _ = conn.Exec(ctx,
			`SELECT pg_advisory_unlock($1, hashtext($2::text))`, lockClass, b.project,
		)
	}()

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

		// 003_pgvector.sql starts with CREATE EXTENSION which cannot run
		// inside a transaction in most PostgreSQL configurations. Run it
		// outside any transaction, complete the backfill, then record the
		// migration last — so a crash before recording causes a safe retry
		// on the next startup (CREATE EXTENSION IF NOT EXISTS is idempotent).
		// Fix for #100: previously the migration was recorded before the
		// backfill, leaving the schema permanently stuck if the backfill failed.
		if name == "003_pgvector.sql" {
			if _, err := b.pool.Exec(ctx, string(sql)); err != nil {
				return fmt.Errorf("apply migration %s: %w", name, err)
			}
			// Run backfill before recording so a crash here causes a retry.
			if err := b.backfillVectors(ctx); err != nil {
				return fmt.Errorf("pgvector backfill failed: %w", err)
			}
			// Record last — use ON CONFLICT DO NOTHING so a concurrent or retried
			// startup that already recorded doesn't fail here.
			if _, err := b.pool.Exec(ctx,
				`INSERT INTO schema_migrations (filename) VALUES ($1) ON CONFLICT DO NOTHING`, name,
			); err != nil {
				return fmt.Errorf("record migration %s: %w", name, err)
			}
			slog.Info("applied migration", "file", name)
			continue
		}

		// All other migrations: wrap apply + record in a single transaction.
		{
			tx, err := b.pool.Begin(ctx)
			if err != nil {
				return fmt.Errorf("begin migration tx for %s: %w", name, err)
			}
			if _, err := tx.Exec(ctx, string(sql)); err != nil {
				_ = tx.Rollback(ctx)
				return fmt.Errorf("apply migration %s: %w", name, err)
			}
			if _, err := tx.Exec(ctx,
				`INSERT INTO schema_migrations (filename) VALUES ($1) ON CONFLICT DO NOTHING`, name,
			); err != nil {
				_ = tx.Rollback(ctx)
				return fmt.Errorf("record migration %s: %w", name, err)
			}
			if err := tx.Commit(ctx); err != nil {
				return fmt.Errorf("commit migration %s: %w", name, err)
			}
			slog.Info("applied migration", "file", name)
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

	// Verify no rows were skipped (e.g. due to invalid BYTEA length) before
	// marking the backfill done. If remaining > 0, leave the flag set so the
	// next startup retries the conversion rather than silently proceeding.
	var remaining int
	if err := b.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM chunks WHERE embedding IS NOT NULL AND embedding_vec IS NULL`,
	).Scan(&remaining); err != nil {
		return fmt.Errorf("backfill count check: %w", err)
	}
	if remaining > 0 {
		slog.Warn("pgvector backfill incomplete — will retry on next startup", "remaining", remaining)
		return nil
	}

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

// unwrapTx extracts the underlying pgx.Tx from a Tx interface value.
func unwrapTx(t Tx) (pgx.Tx, error) {
	pt, ok := t.(*pgxTx)
	if !ok {
		return nil, fmt.Errorf("unwrapTx: expected *pgxTx, got %T", t)
	}
	return pt.tx, nil
}

// execer is satisfied by both *pgxpool.Pool and pgx.Tx.
type execer interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

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

func (b *PostgresBackend) SetMetaTx(ctx context.Context, tx Tx, project, key, value string) error {
	raw, err := unwrapTx(tx)
	if err != nil {
		return err
	}
	_, err = raw.Exec(ctx,
		"INSERT INTO project_meta (project, key, value) VALUES ($1,$2,$3) "+
			"ON CONFLICT (project, key) DO UPDATE SET value = EXCLUDED.value",
		project, key, value,
	)
	return err
}

// ── Row mappers ───────────────────────────────────────────────────────────────

// rowToFTSResult scans the output of FTSSearch queries: SELECT m.*, rank
// (26 memory columns + 1 rank float). Column order must stay in sync with the
// DDL and with rowToMemory below (#112 — single point of schema knowledge).
func rowToFTSResult(row pgx.CollectableRow) (FTSResult, error) {
	var (
		id, content, memType, proj string
		tags                       []byte
		importance, accessCount    int
		lastAccessed, createdAt, updatedAt time.Time
		immutable                  bool
		expiresAt                  *time.Time
		summary, contentHash       *string
		storageMode                string
		searchVector               []byte
		validFrom, validTo         *time.Time
		invalidationReason         *string
		dynamicImportance          *float64
		retrievalIntervalHrs       float64
		nextReviewAt               *time.Time
		timesRetrieved, timesUseful int
		retrievalPrecision         *float64
		episodeID                  *string
		rank                       float64
	)
	// Column order matches the live DB schema (search_vector was added between
	// updated_at and immutable in an early migration, so it sits at position 10).
	// Live order: id, content, memory_type, project, tags, importance, access_count,
	//   last_accessed, created_at, updated_at, search_vector, immutable, expires_at,
	//   summary, content_hash, storage_mode, valid_from, valid_to, invalidation_reason,
	//   dynamic_importance, retrieval_interval_hrs, next_review_at, times_retrieved,
	//   times_useful, retrieval_precision, episode_id (+rank appended by FTSSearch query).
	if err := row.Scan(
		&id, &content, &memType, &proj, &tags,
		&importance, &accessCount, &lastAccessed, &createdAt, &updatedAt,
		&searchVector,
		&immutable, &expiresAt, &summary, &contentHash, &storageMode,
		&validFrom, &validTo, &invalidationReason,
		&dynamicImportance, &retrievalIntervalHrs, &nextReviewAt,
		&timesRetrieved, &timesUseful, &retrievalPrecision, &episodeID, &rank,
	); err != nil {
		return FTSResult{}, err
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
	var epID string
	if episodeID != nil {
		epID = *episodeID
	}
	m := &types.Memory{
		ID:                   id,
		Content:              content,
		MemoryType:           memType,
		Project:              proj,
		Tags:                 tagSlice,
		Importance:           importance,
		AccessCount:          accessCount,
		LastAccessed:         lastAccessed,
		CreatedAt:            createdAt,
		UpdatedAt:            updatedAt,
		Immutable:            immutable,
		ExpiresAt:            expiresAt,
		Summary:              summary,
		ContentHash:          contentHash,
		StorageMode:          storageMode,
		ValidFrom:            validFrom,
		ValidTo:              validTo,
		InvalidationReason:   invalidationReason,
		DynamicImportance:    dynamicImportance,
		RetrievalIntervalHrs: retrievalIntervalHrs,
		NextReviewAt:         nextReviewAt,
		TimesRetrieved:       timesRetrieved,
		TimesUseful:          timesUseful,
		RetrievalPrecision:   retrievalPrecision,
		EpisodeID:            epID,
	}
	return FTSResult{Memory: m, Score: rank}, nil
}

func rowToMemory(row pgx.CollectableRow) (*types.Memory, error) {
	type raw struct {
		ID                   string
		Content              string
		MemoryType           string
		Project              string
		Tags                 []byte
		Importance           int
		AccessCount          int
		LastAccessed         time.Time
		CreatedAt            time.Time
		UpdatedAt            time.Time
		Immutable            bool
		ExpiresAt            *time.Time
		Summary              *string
		ContentHash          *string
		StorageMode          string
		SearchVector         []byte // ignored — generated column
		ValidFrom            *time.Time
		ValidTo              *time.Time
		InvalidationReason   *string
		DynamicImportance    *float64
		RetrievalIntervalHrs float64
		NextReviewAt         *time.Time
		TimesRetrieved       int
		TimesUseful          int
		RetrievalPrecision   *float64
		EpisodeID            *string // nullable FK
	}
	var r raw
	// Column order matches the live DB schema. search_vector was added between
	// updated_at and immutable in the original migration, so it sits at position 10
	// (0-indexed). Live order:
	//   id, content, memory_type, project, tags, importance, access_count,
	//   last_accessed, created_at, updated_at, search_vector, immutable, expires_at,
	//   summary, content_hash, storage_mode,
	//   valid_from, valid_to, invalidation_reason,          (005_temporal)
	//   dynamic_importance, retrieval_interval_hrs, next_review_at, (006_adaptive)
	//   times_retrieved, times_useful, retrieval_precision,  (007_retrieval)
	//   episode_id                                           (008_episodes)
	err := row.Scan(
		&r.ID, &r.Content, &r.MemoryType, &r.Project, &r.Tags,
		&r.Importance, &r.AccessCount, &r.LastAccessed, &r.CreatedAt, &r.UpdatedAt,
		&r.SearchVector,
		&r.Immutable, &r.ExpiresAt, &r.Summary, &r.ContentHash, &r.StorageMode,
		&r.ValidFrom, &r.ValidTo, &r.InvalidationReason,
		&r.DynamicImportance, &r.RetrievalIntervalHrs, &r.NextReviewAt,
		&r.TimesRetrieved, &r.TimesUseful, &r.RetrievalPrecision, &r.EpisodeID,
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

	var episodeID string
	if r.EpisodeID != nil {
		episodeID = *r.EpisodeID
	}
	return &types.Memory{
		ID:                   r.ID,
		Content:              r.Content,
		MemoryType:           r.MemoryType,
		Project:              r.Project,
		Tags:                 tags,
		Importance:           r.Importance,
		AccessCount:          r.AccessCount,
		LastAccessed:         r.LastAccessed,
		CreatedAt:            r.CreatedAt,
		UpdatedAt:            r.UpdatedAt,
		Immutable:            r.Immutable,
		ExpiresAt:            r.ExpiresAt,
		Summary:              r.Summary,
		ContentHash:          r.ContentHash,
		StorageMode:          storageMode,
		ValidFrom:            r.ValidFrom,
		ValidTo:              r.ValidTo,
		InvalidationReason:   r.InvalidationReason,
		DynamicImportance:    r.DynamicImportance,
		RetrievalIntervalHrs: r.RetrievalIntervalHrs,
		NextReviewAt:         r.NextReviewAt,
		TimesRetrieved:       r.TimesRetrieved,
		TimesUseful:          r.TimesUseful,
		RetrievalPrecision:   r.RetrievalPrecision,
		EpisodeID:            episodeID,
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
