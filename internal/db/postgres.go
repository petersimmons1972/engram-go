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
	"github.com/jackc/pgx/v5/pgtype"
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
	pool      *pgxpool.Pool
	project   string // validated project slug
	ownsPool  bool   // true only when NewPostgresBackend created the pool; false for shared-pool backends
}

// configurePool applies connection pool tuning for CLI tools that create a
// single project-scoped pool (cmd/reembed-worker, cmd/engram-setup). The server
// process uses configureSharedPool instead, which is tuned for a single pool
// shared across all project backends.
func configurePool(cfg *pgxpool.Config) {
	cfg.MinConns = 5
	cfg.MaxConns = 25
	// Evict connections after 30 minutes so the pool recovers cleanly after a
	// PostgreSQL restart or network flap.
	cfg.MaxConnLifetime = 30 * time.Minute
	// Reap idle connections after 5 minutes to avoid holding DB slots
	// unnecessarily during low-traffic periods.
	cfg.MaxConnIdleTime = 5 * time.Minute
	// Proactively ping idle connections every minute so dead ones are culled
	// from the pool before a caller receives them.
	cfg.HealthCheckPeriod = 1 * time.Minute
}

// configureSharedPool applies connection pool tuning for the single pool shared
// across all project backends in the server process. Higher MaxConns handles
// concurrent projects; lower MinConns avoids wasting slots when the server is
// idle. Tighter idle and health-check intervals keep the shared pool lean.
func configureSharedPool(cfg *pgxpool.Config) {
	cfg.MinConns = 2
	cfg.MaxConns = 50
	cfg.MaxConnLifetime = 30 * time.Minute
	cfg.MaxConnIdleTime = 3 * time.Minute
	cfg.HealthCheckPeriod = 30 * time.Second
}

// registerTypesAfterConnect registers custom type codecs that every connection
// in a pool needs. Extracted so both configurePool and configureSharedPool can
// share it via AfterConnect without duplicating the registration logic.
func registerTypesAfterConnect(ctx context.Context, conn *pgx.Conn) error {
	// tsvector (OID 3614) has no binary codec in pgx — register it as text so
	// SELECT * queries that include search_vector don't fail in binary mode.
	conn.TypeMap().RegisterType(&pgtype.Type{
		Name:  "tsvector",
		OID:   3614,
		Codec: pgtype.TextCodec{},
	})
	return nil
}

// NewSharedPool creates a single *pgxpool.Pool to be shared across all project
// backends in the server process. It validates the DSN, runs schema migrations
// (guarded by an advisory lock so concurrent startups are safe), and returns the
// pool. Callers are responsible for calling pool.Close() on shutdown.
//
// Use NewPostgresBackendWithPool to create per-project backends from this pool.
func NewSharedPool(ctx context.Context, dsn string) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("invalid DSN: %w", err)
	}
	if err := rejectDefaultPassword(cfg); err != nil {
		return nil, err
	}
	configureSharedPool(cfg)
	cfg.AfterConnect = registerTypesAfterConnect

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("cannot create connection pool: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("cannot connect to PostgreSQL — check DATABASE_URL: %w", err)
	}

	// Run migrations once on the shared pool. A temporary backend is constructed
	// with the reserved project slug "_shared" (never visible to callers) solely
	// to satisfy the runMigrations receiver. Migrations issue no project-scoped
	// DDL, so the slug does not appear in any DB row.
	b := &PostgresBackend{pool: pool, project: "_shared"}
	if err := b.runMigrations(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("schema migration failed: %w", err)
	}

	return pool, nil
}

// NewPostgresBackendWithPool creates a project-scoped Backend that uses an
// existing shared *pgxpool.Pool. It does not run migrations or ping the
// database — callers must ensure the pool was created via NewSharedPool.
func NewPostgresBackendWithPool(ctx context.Context, project string, pool *pgxpool.Pool) (*PostgresBackend, error) {
	return newPostgresBackendFromPool(pool, project)
}

// newPostgresBackendFromPool is the internal constructor shared by
// NewPostgresBackendWithPool and the test helpers. Accepts a nil pool so unit
// tests can verify slug sanitisation without a live database.
func newPostgresBackendFromPool(pool *pgxpool.Pool, project string) (*PostgresBackend, error) {
	project = projectSlugRE.ReplaceAllString(project, "")
	if project == "" {
		project = "default"
	}
	return &PostgresBackend{pool: pool, project: project}, nil
}

// NewPostgresBackend creates a new backend with its own connection pool,
// validates the connection, and runs schema migrations. Intended for CLI tools
// (cmd/reembed-worker, cmd/engram-setup) that own a single project pool.
//
// The server process should use NewSharedPool + NewPostgresBackendWithPool
// instead to avoid creating one pool per project.
func NewPostgresBackend(ctx context.Context, project, dsn string) (*PostgresBackend, error) {
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("invalid DSN: %w", err)
	}
	if err := rejectDefaultPassword(cfg); err != nil {
		return nil, err
	}
	configurePool(cfg)
	cfg.AfterConnect = registerTypesAfterConnect

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("cannot create connection pool: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("cannot connect to PostgreSQL — check DATABASE_URL: %w", err)
	}

	b, _ := newPostgresBackendFromPool(pool, project)
	b.ownsPool = true // CLI-created pool: this backend is responsible for closing it
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

// Close releases the connection pool if this backend owns it. Backends created
// via NewPostgresBackendWithPool share a pool they do not own — calling Close
// on them is a no-op so callers cannot accidentally tear down the shared pool.
func (b *PostgresBackend) Close() {
	if b.ownsPool {
		b.pool.Close()
	}
}

// Pool returns the underlying connection pool. Intended for integration test
// helpers that need to issue raw SQL (e.g. back-dating created_at for time-based
// test scenarios). Not part of the Backend interface.
func (b *PostgresBackend) Pool() *pgxpool.Pool {
	return b.pool
}

// PgxPool satisfies search.pgPooler — exposes the underlying pool so the
// search engine's weight cache can load per-project weights from weight_config.
func (b *PostgresBackend) PgxPool() *pgxpool.Pool {
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

	// Set lock_timeout so a hung migration on another node can't block startup forever.
	lockTimeoutMs := 30000
	if deadline, ok := ctx.Deadline(); ok {
		if rem := int(time.Until(deadline).Milliseconds()); rem < lockTimeoutMs {
			lockTimeoutMs = rem
		}
	}
	// integer-only — DO NOT change to %s; SET LOCAL cannot use parameter binding
	if _, err := conn.Exec(ctx,
		fmt.Sprintf("SET LOCAL lock_timeout = '%dms'", lockTimeoutMs),
	); err != nil {
		return fmt.Errorf("set lock_timeout: %w", err)
	}
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
			// Run backfill on the same connection that holds the advisory lock so
			// the lock covers the full backfill duration (issue #292).
			if err := b.backfillVectors(ctx, conn); err != nil {
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
//
// The q parameter must be the same connection that holds the migration advisory
// lock (issue #292): running the backfill on a pool connection would release the
// lock before the backfill completes, allowing a second replica to enter the same
// migration window and duplicate the work (or race into migration 004).
func (b *PostgresBackend) backfillVectors(ctx context.Context, q querier) error {
	rows, err := q.Query(ctx, `
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
		if _, err := q.Exec(ctx,
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
	if err := q.QueryRow(ctx,
		`SELECT COUNT(*) FROM chunks WHERE embedding IS NOT NULL AND embedding_vec IS NULL`,
	).Scan(&remaining); err != nil {
		return fmt.Errorf("backfill count check: %w", err)
	}
	if remaining > 0 {
		slog.Warn("pgvector backfill incomplete — will retry on next startup", "remaining", remaining)
		return nil
	}

	// Mark backfill done so 004 can run.
	_, err = q.Exec(ctx,
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

// querier is satisfied by *pgxpool.Pool and *pgxpool.Conn.
// Used to run backfillVectors on a specific connection so that an advisory
// lock held by that connection covers the full backfill (issue #292).
type querier interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
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
		documentID                 *string
		rank                       float64
	)
	// Column order matches the live DB schema (search_vector was added between
	// updated_at and immutable in an early migration, so it sits at position 10).
	// Live order: id, content, memory_type, project, tags, importance, access_count,
	//   last_accessed, created_at, updated_at, search_vector, immutable, expires_at,
	//   summary, content_hash, storage_mode, valid_from, valid_to, invalidation_reason,
	//   dynamic_importance, retrieval_interval_hrs, next_review_at, times_retrieved,
	//   times_useful, retrieval_precision, episode_id, document_id (+rank appended by FTSSearch query).
	if err := row.Scan(
		&id, &content, &memType, &proj, &tags,
		&importance, &accessCount, &lastAccessed, &createdAt, &updatedAt,
		&searchVector,
		&immutable, &expiresAt, &summary, &contentHash, &storageMode,
		&validFrom, &validTo, &invalidationReason,
		&dynamicImportance, &retrievalIntervalHrs, &nextReviewAt,
		&timesRetrieved, &timesUseful, &retrievalPrecision, &episodeID, &documentID, &rank,
	); err != nil {
		return FTSResult{}, err
	}
	var tagSlice []string
	if len(tags) > 0 {
		if err := json.Unmarshal(tags, &tagSlice); err != nil {
			slog.Warn("tags unmarshal failed — empty tags returned", "memory_id", id, "err", err)
			tagSlice = []string{}
		}
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
	var docID string
	if documentID != nil {
		docID = *documentID
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
		DocumentID:           docID,
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
		DocumentID           *string // nullable FK (010_documents)
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
	//   document_id                                          (010_documents)
	err := row.Scan(
		&r.ID, &r.Content, &r.MemoryType, &r.Project, &r.Tags,
		&r.Importance, &r.AccessCount, &r.LastAccessed, &r.CreatedAt, &r.UpdatedAt,
		&r.SearchVector,
		&r.Immutable, &r.ExpiresAt, &r.Summary, &r.ContentHash, &r.StorageMode,
		&r.ValidFrom, &r.ValidTo, &r.InvalidationReason,
		&r.DynamicImportance, &r.RetrievalIntervalHrs, &r.NextReviewAt,
		&r.TimesRetrieved, &r.TimesUseful, &r.RetrievalPrecision, &r.EpisodeID,
		&r.DocumentID,
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
	var documentID string
	if r.DocumentID != nil {
		documentID = *r.DocumentID
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
		DocumentID:           documentID,
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
		Embedding      *pgvector.Vector
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

	var embedding []float32
	if r.Embedding != nil {
		embedding = r.Embedding.Slice()
	}

	return &types.Chunk{
		ID:             r.ID,
		MemoryID:       r.MemoryID,
		Project:        r.Project,
		ChunkText:      r.ChunkText,
		ChunkIndex:     r.ChunkIndex,
		ChunkHash:      r.ChunkHash,
		Embedding:      embedding,
		SectionHeading: r.SectionHeading,
		ChunkType:      chunkType,
		LastMatched:    r.LastMatched,
	}, nil
}
