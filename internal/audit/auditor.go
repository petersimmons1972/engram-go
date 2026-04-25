// Package audit implements the decay audit system.
// It runs canonical queries on a schedule and stores retrieval snapshots,
// enabling detection of ranking drift as embedders and weights change.
package audit

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/petersimmons1972/engram/internal/metrics"
)

const (
	rboP    = 0.9 // RBO persistence parameter; weights top ranks heavily
	topK    = 20  // number of results per canonical query audit run
	lockKey = 7331
)

// Recaller is the narrow interface the audit worker needs for recall.
// Satisfied by search.SearchEngine.
type Recaller interface {
	// Recall returns the top-k memory IDs for the given query in the given project.
	// Implementations should return IDs in ranked order (best first).
	Recall(ctx context.Context, project, query string, topK int) ([]string, error)
}

// CanonicalQuery is a registered query used for drift monitoring.
type CanonicalQuery struct {
	ID             string
	Project        string
	Query          string
	Description    string
	Active         bool
	AlertThreshold *float64
	CreatedAt      time.Time
}

// Snapshot is one retrieval run for one canonical query.
type Snapshot struct {
	ID                    string
	QueryID               string
	Project               string
	MemoryIDs             []string
	Scores                []float64
	EmbeddingModel        string
	EmbeddingModelVersion string
	RunAt                 time.Time
	RBOVsPrev             *float64
	JaccardAt5            *float64
	JaccardAt10           *float64
	JaccardFull           *float64
	Additions             []string
	Removals              []string
}

// SnapshotSummary is a lightweight view of one audit result — safe to return
// from the MCP tool without full memory-ID dumps.
type SnapshotSummary struct {
	QueryID    string   `json:"query_id"`
	QueryText  string   `json:"query"`
	Project    string   `json:"project"`
	SnapshotID string   `json:"snapshot_id"`
	RunAt      string   `json:"run_at"`
	RBOVsPrev  *float64 `json:"rbo_vs_prev,omitempty"`
	JaccardAt5 *float64 `json:"jaccard_at_5,omitempty"`
	IsBaseline bool     `json:"is_baseline"`
}

// AuditQuerier is the narrow DB interface used for audit data operations.
// Both *pgxpool.Pool and *pgxpool.Conn satisfy this interface.
type AuditQuerier interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// AuditWorker runs canonical queries on a schedule and stores snapshots.
type AuditWorker struct {
	pool       *pgxpool.Pool // used only for advisory lock Acquire
	db         AuditQuerier  // used for all data queries
	recaller   Recaller
	embedModel string
	interval   time.Duration
}

// NewAuditWorker creates an AuditWorker. The pool is used for advisory lock
// acquisition; all data queries go through pool (which satisfies AuditQuerier).
func NewAuditWorker(pool *pgxpool.Pool, recaller Recaller, embedModel string, interval time.Duration) *AuditWorker {
	return &AuditWorker{
		pool:       pool,
		db:         pool,
		recaller:   recaller,
		embedModel: embedModel,
		interval:   interval,
	}
}

// NewAuditWorkerWithDB creates an AuditWorker with a separate db querier, for testing.
func NewAuditWorkerWithDB(pool *pgxpool.Pool, db AuditQuerier, recaller Recaller, embedModel string, interval time.Duration) *AuditWorker {
	return &AuditWorker{
		pool:       pool,
		db:         db,
		recaller:   recaller,
		embedModel: embedModel,
		interval:   interval,
	}
}

// Run starts the background audit loop. Call as a goroutine.
// Fires once immediately then on each ticker tick.
func (w *AuditWorker) Run(ctx context.Context) {
	run := func() {
		metrics.WorkerTicks.WithLabelValues("audit").Inc()
		defer func() {
			if r := recover(); r != nil {
				slog.Error("audit worker: panic", "err", r)
				metrics.WorkerErrors.WithLabelValues("audit").Inc()
			}
		}()
		if _, err := w.RunPass(ctx); err != nil {
			slog.Error("audit worker: pass failed", "err", err)
			metrics.WorkerErrors.WithLabelValues("audit").Inc()
		}
	}

	run() // baseline pass at startup
	ticker := time.NewTicker(w.interval)
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

// RunPass executes one full audit pass — all active queries for all projects.
// Uses an advisory lock so concurrent invocations skip rather than double-run.
// The lock is acquired on a pinned connection to ensure lock and unlock happen
// on the same PostgreSQL session (advisory locks are session-scoped).
func (w *AuditWorker) RunPass(ctx context.Context) ([]SnapshotSummary, error) {
	conn, err := w.pool.Acquire(ctx)
	if err != nil {
		return nil, fmt.Errorf("acquire conn for advisory lock: %w", err)
	}
	defer conn.Release()

	var locked bool
	if err := conn.QueryRow(ctx, "SELECT pg_try_advisory_lock($1)", lockKey).Scan(&locked); err != nil {
		return nil, fmt.Errorf("advisory lock acquire: %w", err)
	}
	if !locked {
		slog.Info("audit worker: another pass is running, skipping")
		return nil, nil
	}
	defer func() {
		conn.QueryRow(context.Background(), "SELECT pg_advisory_unlock($1)", lockKey).Scan(new(bool)) //nolint:errcheck
	}()

	queries, err := w.loadActiveQueries(ctx, "")
	if err != nil {
		return nil, fmt.Errorf("load queries: %w", err)
	}

	var summaries []SnapshotSummary
	for _, q := range queries {
		snap, err := w.runQuerySnapshot(ctx, q)
		if err != nil {
			slog.Error("audit worker: snapshot failed", "query_id", q.ID, "err", err)
			continue
		}
		summaries = append(summaries, snapshotToSummary(q, snap))
	}
	return summaries, nil
}

// RegisterQuery creates a new canonical query and returns its ID.
func (w *AuditWorker) RegisterQuery(ctx context.Context, project, query, description string) (string, error) {
	id := uuid.New().String()
	_, err := w.db.Exec(ctx,
		`INSERT INTO audit_canonical_queries (id, project, query, description)
		 VALUES ($1, $2, $3, $4)`,
		id, project, query, description,
	)
	if err != nil {
		return "", fmt.Errorf("insert canonical query: %w", err)
	}
	return id, nil
}

// DeactivateQuery marks the given query as inactive.
func (w *AuditWorker) DeactivateQuery(ctx context.Context, queryID string) error {
	tag, err := w.db.Exec(ctx,
		`UPDATE audit_canonical_queries SET active = FALSE WHERE id = $1`, queryID)
	if err != nil {
		return fmt.Errorf("deactivate query: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("query %q not found", queryID)
	}
	return nil
}

// ListQueries returns all canonical queries for a project (both active and inactive).
// When project is empty, returns all queries across all projects regardless of active status.
func (w *AuditWorker) ListQueries(ctx context.Context, project string) ([]CanonicalQuery, error) {
	if project == "" {
		return w.loadAllQueries(ctx)
	}
	return w.loadQueriesByProject(ctx, project)
}

// RunProjectAudit executes an audit pass for one project only.
func (w *AuditWorker) RunProjectAudit(ctx context.Context, project string) ([]SnapshotSummary, error) {
	queries, err := w.loadQueriesByProject(ctx, project)
	if err != nil {
		return nil, fmt.Errorf("load queries: %w", err)
	}
	var summaries []SnapshotSummary
	for _, q := range queries {
		snap, err := w.runQuerySnapshot(ctx, q)
		if err != nil {
			slog.Error("audit worker: snapshot failed", "query_id", q.ID, "err", err)
			continue
		}
		summaries = append(summaries, snapshotToSummary(q, snap))
	}
	return summaries, nil
}

// GetSnapshots returns snapshots for a query, newest first, up to limit.
func (w *AuditWorker) GetSnapshots(ctx context.Context, queryID string, limit int) ([]Snapshot, error) {
	if limit <= 0 {
		limit = 10
	}
	rows, err := w.db.Query(ctx,
		`SELECT id, query_id, project, memory_ids, scores, embedding_model,
		        embedding_model_version, run_at, rbo_vs_prev,
		        jaccard_at_5, jaccard_at_10, jaccard_full, additions, removals
		 FROM audit_snapshots
		 WHERE query_id = $1
		 ORDER BY run_at DESC
		 LIMIT $2`,
		queryID, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("query snapshots: %w", err)
	}
	defer rows.Close()

	var snaps []Snapshot
	for rows.Next() {
		var s Snapshot
		var embModelVersion *string
		if err := rows.Scan(
			&s.ID, &s.QueryID, &s.Project, &s.MemoryIDs, &s.Scores,
			&s.EmbeddingModel, &embModelVersion, &s.RunAt,
			&s.RBOVsPrev, &s.JaccardAt5, &s.JaccardAt10, &s.JaccardFull,
			&s.Additions, &s.Removals,
		); err != nil {
			return nil, fmt.Errorf("scan snapshot: %w", err)
		}
		if embModelVersion != nil {
			s.EmbeddingModelVersion = *embModelVersion
		}
		snaps = append(snaps, s)
	}
	return snaps, rows.Err()
}

// loadActiveQueries loads all active canonical queries for all projects.
// Used internally by RunPass (the scheduler always processes all active queries).
func (w *AuditWorker) loadActiveQueries(ctx context.Context, project string) ([]CanonicalQuery, error) {
	if project == "" {
		// All active queries regardless of project.
		rows, err := w.db.Query(ctx,
			`SELECT id, project, query, description, active, alert_threshold, created_at
			 FROM audit_canonical_queries
			 WHERE active = TRUE
			 ORDER BY project, created_at`,
		)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		return scanQueries(rows)
	}
	return w.loadQueriesByProject(ctx, project)
}

// loadAllQueries loads all canonical queries across all projects regardless of
// active status. Used by ListQueries when no project filter is given.
func (w *AuditWorker) loadAllQueries(ctx context.Context) ([]CanonicalQuery, error) {
	rows, err := w.db.Query(ctx,
		`SELECT id, project, query, description, active, alert_threshold, created_at
		 FROM audit_canonical_queries
		 ORDER BY project, created_at`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanQueries(rows)
}

// loadQueriesByProject loads all canonical queries for a specific project
// (active and inactive alike — used for listing).
func (w *AuditWorker) loadQueriesByProject(ctx context.Context, project string) ([]CanonicalQuery, error) {
	rows, err := w.db.Query(ctx,
		`SELECT id, project, query, description, active, alert_threshold, created_at
		 FROM audit_canonical_queries
		 WHERE project = $1
		 ORDER BY created_at`,
		project,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanQueries(rows)
}

// scanQueries is a shared row scanner for canonical query result sets.
func scanQueries(rows interface {
	Next() bool
	Scan(dest ...any) error
	Err() error
}) ([]CanonicalQuery, error) {
	var queries []CanonicalQuery
	for rows.Next() {
		var q CanonicalQuery
		var desc *string
		if err := rows.Scan(&q.ID, &q.Project, &q.Query, &desc, &q.Active, &q.AlertThreshold, &q.CreatedAt); err != nil {
			return nil, err
		}
		if desc != nil {
			q.Description = *desc
		}
		queries = append(queries, q)
	}
	return queries, rows.Err()
}

// runQuerySnapshot executes one canonical query, computes metrics against the
// previous snapshot, and persists the new snapshot.
func (w *AuditWorker) runQuerySnapshot(ctx context.Context, q CanonicalQuery) (*Snapshot, error) {
	// Get the most recent snapshot for this query (if any).
	prev, err := w.latestSnapshot(ctx, q.ID)
	if err != nil {
		return nil, fmt.Errorf("load prev snapshot: %w", err)
	}

	// Run the recall.
	ids, err := w.recaller.Recall(ctx, q.Project, q.Query, topK)
	if err != nil {
		return nil, fmt.Errorf("recall: %w", err)
	}

	snap := &Snapshot{
		ID:             uuid.New().String(),
		QueryID:        q.ID,
		Project:        q.Project,
		MemoryIDs:      ids,
		Scores:         nil, // NULL: scores unavailable via Recaller interface; reserved for future use
		EmbeddingModel: w.embedModel,
		RunAt:          time.Now().UTC(),
	}

	// Compute drift metrics vs. previous snapshot.
	if prev != nil {
		rbo := RBO(ids, prev.MemoryIDs, rboP)
		snap.RBOVsPrev = &rbo

		j5 := JaccardTopK(ids, prev.MemoryIDs, 5)
		snap.JaccardAt5 = &j5

		j10 := JaccardTopK(ids, prev.MemoryIDs, 10)
		snap.JaccardAt10 = &j10

		jFull := JaccardTopK(ids, prev.MemoryIDs, len(ids))
		snap.JaccardFull = &jFull

		snap.Additions = setDiff(ids, prev.MemoryIDs)
		snap.Removals = setDiff(prev.MemoryIDs, ids)

		// Alert when RBO drops below the configured threshold (#275).
		if q.AlertThreshold != nil && snap.RBOVsPrev != nil && *snap.RBOVsPrev < *q.AlertThreshold {
			slog.Error("audit: retrieval drift alert — RBO below threshold",
				"query_id", q.ID,
				"project", q.Project,
				"query", q.Query,
				"rbo_vs_prev", *snap.RBOVsPrev,
				"alert_threshold", *q.AlertThreshold,
			)
		}
	}

	if err := w.insertSnapshot(ctx, snap); err != nil {
		return nil, fmt.Errorf("insert snapshot: %w", err)
	}
	return snap, nil
}

// latestSnapshot retrieves the most recent snapshot for a query, or nil if none.
func (w *AuditWorker) latestSnapshot(ctx context.Context, queryID string) (*Snapshot, error) {
	var s Snapshot
	var embModelVersion *string
	err := w.db.QueryRow(ctx,
		`SELECT id, query_id, project, memory_ids, scores, embedding_model,
		        embedding_model_version, run_at
		 FROM audit_snapshots
		 WHERE query_id = $1
		 ORDER BY run_at DESC
		 LIMIT 1`,
		queryID,
	).Scan(
		&s.ID, &s.QueryID, &s.Project, &s.MemoryIDs, &s.Scores,
		&s.EmbeddingModel, &embModelVersion, &s.RunAt,
	)
	if err != nil {
		// pgx returns pgx.ErrNoRows when empty — treat as no previous snapshot.
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	if embModelVersion != nil {
		s.EmbeddingModelVersion = *embModelVersion
	}
	return &s, nil
}

// insertSnapshot persists a snapshot to the database.
func (w *AuditWorker) insertSnapshot(ctx context.Context, s *Snapshot) error {
	var embModelVersion *string
	if s.EmbeddingModelVersion != "" {
		embModelVersion = &s.EmbeddingModelVersion
	}
	_, err := w.db.Exec(ctx,
		`INSERT INTO audit_snapshots
		 (id, query_id, project, memory_ids, scores, embedding_model, embedding_model_version,
		  run_at, rbo_vs_prev, jaccard_at_5, jaccard_at_10, jaccard_full, additions, removals)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)`,
		s.ID, s.QueryID, s.Project, s.MemoryIDs, s.Scores,
		s.EmbeddingModel, embModelVersion, s.RunAt,
		s.RBOVsPrev, s.JaccardAt5, s.JaccardAt10, s.JaccardFull,
		s.Additions, s.Removals,
	)
	return err
}

func snapshotToSummary(q CanonicalQuery, s *Snapshot) SnapshotSummary {
	return SnapshotSummary{
		QueryID:    q.ID,
		QueryText:  q.Query,
		Project:    q.Project,
		SnapshotID: s.ID,
		RunAt:      s.RunAt.Format(time.RFC3339),
		RBOVsPrev:  s.RBOVsPrev,
		JaccardAt5: s.JaccardAt5,
		IsBaseline: s.RBOVsPrev == nil,
	}
}
