package db

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/petersimmons1972/engram/internal/atom"
	pgvector "github.com/pgvector/pgvector-go"
)

// AtomQueryOpts controls filtered active-atom queries.
type AtomQueryOpts struct {
	AtomType        string
	AsOf            *time.Time
	ValidFromSince  *time.Time
	ValidFromBefore *time.Time
	LatestOnly      bool
	OrderValidFrom  bool
	Limit           int
}

// InsertAtom inserts a new atom into the atoms table. A UUIDv4 ID is generated
// if a.ID is empty. The atom's Project must be set by the caller.
// Embedding is NOT written here — that is a separate step via InsertAtomEmbedding.
func (p *PostgresBackend) InsertAtom(ctx context.Context, a *atom.Atom) error {
	_, err := insertAtom(ctx, p.pool, a)
	return err
}

type atomExecer interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
}

func insertAtom(ctx context.Context, execer atomExecer, a *atom.Atom) (bool, error) {
	if a.ID == "" {
		a.ID = uuid.New().String()
	}
	if a.CreatedAt.IsZero() {
		a.CreatedAt = time.Now().UTC()
	}
	if a.ObservedAt == nil {
		observedAt := a.CreatedAt
		a.ObservedAt = &observedAt
	}
	tag, err := execer.Exec(ctx, `
		INSERT INTO atoms (
			id, project, atom_type, subject, predicate, value,
			statement, scope, valid_from, valid_to, confidence,
			provenance_memory_id, provenance_span, supersedes, observed_at, created_at
		) VALUES (
			$1, $2, $3, $4, $5, $6,
			$7, $8, $9, $10, $11,
			$12, $13, $14, $15, $16
		) ON CONFLICT (id) DO NOTHING`,
		a.ID, a.Project, a.Type, a.Subject, a.Predicate, a.Value,
		a.Statement, a.Scope, a.ValidFrom, a.ValidTo, a.Confidence,
		nullableString(a.ProvenanceMemoryID), nullableString(a.ProvenanceSpan),
		nullableString(a.Supersedes), a.ObservedAt, a.CreatedAt,
	)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() == 1, nil
}

// InsertAtomEmbedding upserts the vector embedding for the given atom into the
// atom_embeddings table. The caller must have already inserted the atom via
// InsertAtom. Uses ON CONFLICT DO UPDATE so re-embedding an atom (e.g. after a
// model change) overwrites the stale vector rather than silently skipping it.
func (p *PostgresBackend) InsertAtomEmbedding(ctx context.Context, atomID string, vec []float32) error {
	// pgvector.NewVector wraps the []float32 so pgx encodes it as the pgvector
	// `vector` type rather than a Postgres float array literal. Passing the raw
	// slice yields "{...}" which the vector column rejects (SQLSTATE 22P02).
	// Mirrors UpdateChunkEmbedding / StoreChunks.
	_, err := p.pool.Exec(ctx, `
		INSERT INTO atom_embeddings (atom_id, embedding, embedder, created_at)
		VALUES ($1, $2, 'olla', NOW())
		ON CONFLICT (atom_id) DO UPDATE
			SET embedding = EXCLUDED.embedding,
			    embedder  = EXCLUDED.embedder,
			    created_at = NOW()`,
		atomID, pgvector.NewVector(vec))
	return err
}

// RetireAtom atomically inserts superseding with its backward link and then
// sets valid_to on the active predecessor. The INSERT precedes the sole UPDATE
// inside the transaction, so no reader can observe an unlinked active overlap.
func (p *PostgresBackend) RetireAtom(
	ctx context.Context,
	atomID string,
	validTo time.Time,
	superseding *atom.Atom,
) error {
	tx, err := p.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("begin atom supersession: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	inserted, err := insertAtom(ctx, tx, superseding)
	if err != nil {
		return fmt.Errorf("insert superseding atom: %w", err)
	}
	if !inserted {
		return fmt.Errorf("insert superseding atom %q: ID already exists", superseding.ID)
	}
	tag, err := tx.Exec(ctx,
		`UPDATE atoms SET valid_to = $1 WHERE id = $2 AND valid_to IS NULL`,
		validTo, atomID)
	if err != nil {
		return fmt.Errorf("retire superseded atom: %w", err)
	}
	if tag.RowsAffected() != 1 {
		return fmt.Errorf("retire superseded atom %q: active row not found", atomID)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit atom supersession: %w", err)
	}
	return nil
}

// GetActiveAtoms returns all atoms for the project with valid_to IS NULL.
// If atomType is non-empty, results are filtered to that type.
func (p *PostgresBackend) GetActiveAtoms(ctx context.Context, project string, atomType string) ([]atom.Atom, error) {
	return p.GetActiveAtomsFiltered(ctx, project, AtomQueryOpts{AtomType: atomType})
}

// GetActiveAtomsFiltered returns active atoms with optional type/as-of/latest filtering.
func (p *PostgresBackend) GetActiveAtomsFiltered(ctx context.Context, project string, opts AtomQueryOpts) ([]atom.Atom, error) {
	query, args := buildActiveAtomsQuery(project, opts)
	rows, err := p.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	return scanAtomRows(rows)
}

func buildActiveAtomsQuery(project string, opts AtomQueryOpts) (string, []interface{}) {
	where := []string{"project = $1", "valid_to IS NULL"}
	args := []interface{}{project}
	nextArg := 2

	if opts.AtomType != "" {
		where = append(where, fmt.Sprintf("atom_type = $%d", nextArg))
		args = append(args, opts.AtomType)
		nextArg++
	}
	if opts.AsOf != nil {
		where = append(where, fmt.Sprintf("observed_at <= $%d", nextArg))
		args = append(args, *opts.AsOf)
		nextArg++
	}
	if opts.ValidFromSince != nil {
		where = append(where, fmt.Sprintf("valid_from >= $%d", nextArg))
		args = append(args, *opts.ValidFromSince)
		nextArg++
	}
	if opts.ValidFromBefore != nil {
		where = append(where, fmt.Sprintf("valid_from < $%d", nextArg))
		args = append(args, *opts.ValidFromBefore)
		nextArg++
	}

	selectClause := `
		SELECT id, project, atom_type, subject, predicate, value,
		       statement, scope, valid_from, valid_to, observed_at, confidence,
		       COALESCE(provenance_memory_id,''), COALESCE(provenance_span,''),
		       COALESCE(supersedes,''), created_at
		FROM atoms`
	if opts.LatestOnly {
		selectClause = `
		SELECT DISTINCT ON (subject, predicate)
		       id, project, atom_type, subject, predicate, value,
		       statement, scope, valid_from, valid_to, observed_at, confidence,
		       COALESCE(provenance_memory_id,''), COALESCE(provenance_span,''),
		       COALESCE(supersedes,''), created_at
		FROM atoms`
	}

	orderBy := "ORDER BY observed_at DESC NULLS LAST, created_at DESC"
	if opts.LatestOnly {
		orderBy = "ORDER BY subject, predicate, observed_at DESC NULLS LAST, created_at DESC"
	} else if opts.OrderValidFrom {
		orderBy = "ORDER BY valid_from ASC, created_at ASC"
	}

	limitClause := ""
	if opts.Limit > 0 {
		limitClause = fmt.Sprintf("LIMIT $%d", nextArg)
		args = append(args, opts.Limit)
	}

	query := `
		` + selectClause + `
		WHERE ` + strings.Join(where, " AND ") + `
		` + orderBy + `
		` + limitClause
	return query, args
}

// EnqueueAtomExtractionJob inserts a pending job for the given memory.
// ON CONFLICT DO NOTHING mirrors entity_extraction_jobs behaviour.
func (p *PostgresBackend) EnqueueAtomExtractionJob(ctx context.Context, memoryID, project string) error {
	_, err := p.pool.Exec(ctx, `
		INSERT INTO atom_extraction_jobs (id, memory_id, project)
		VALUES ($1, $2, $3)
		ON CONFLICT ON CONSTRAINT uq_atom_jobs_pending DO NOTHING`,
		uuid.New().String(), memoryID, project)
	return err
}

// ClaimAtomExtractionJobs atomically marks up to limit pending jobs as
// 'processing' for the given project and returns them.
func (p *PostgresBackend) ClaimAtomExtractionJobs(ctx context.Context, project string, limit int) ([]atom.ExtractionJob, error) {
	var (
		pgRows interface {
			Close()
			Next() bool
			Scan(dest ...interface{}) error
			Err() error
		}
		err error
	)
	if project == "" {
		pgRows, err = p.pool.Query(ctx, `
			UPDATE atom_extraction_jobs
			SET status = 'processing'
			WHERE id IN (
				SELECT id FROM atom_extraction_jobs
				WHERE status = 'pending'
				ORDER BY created_at ASC
				LIMIT $1
				FOR UPDATE SKIP LOCKED
			)
			RETURNING id, memory_id, project`, limit)
	} else {
		pgRows, err = p.pool.Query(ctx, `
			UPDATE atom_extraction_jobs
			SET status = 'processing'
			WHERE id IN (
				SELECT id FROM atom_extraction_jobs
				WHERE project = $1 AND status = 'pending'
				ORDER BY created_at ASC
				LIMIT $2
				FOR UPDATE SKIP LOCKED
			)
			RETURNING id, memory_id, project`, project, limit)
	}
	if err != nil {
		return nil, err
	}
	defer pgRows.Close()
	var jobs []atom.ExtractionJob
	for pgRows.Next() {
		var j atom.ExtractionJob
		if err := pgRows.Scan(&j.ID, &j.MemoryID, &j.Project); err != nil {
			return nil, err
		}
		jobs = append(jobs, j)
	}
	return jobs, pgRows.Err()
}

// CompleteAtomExtractionJob marks a job as done or failed.
func (p *PostgresBackend) CompleteAtomExtractionJob(ctx context.Context, jobID string, jobErr error) error {
	errMsg := ""
	status := "done"
	if jobErr != nil {
		errMsg = truncateString(strings.TrimSpace(jobErr.Error()), 500)
		status = "failed"
	}
	_, err := p.pool.Exec(ctx,
		`UPDATE atom_extraction_jobs SET status=$1, error=$2, processed_at=NOW() WHERE id=$3`,
		status, errMsg, jobID)
	return err
}

// ── helpers ───────────────────────────────────────────────────────────────────

// nullableString returns nil for an empty string so SQL columns receive NULL.
func nullableString(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}

func scanAtomRows(rows interface{ Close() }) ([]atom.Atom, error) {
	// The parameter type is interface{Close()} rather than pgx.Rows to avoid
	// forcing tests to stub pgx. We use a type assertion to get Next/Scan/Err.
	type scannable interface {
		Next() bool
		Scan(dest ...interface{}) error
		Err() error
		Close()
	}
	r, ok := rows.(scannable)
	if !ok {
		return nil, nil
	}
	defer r.Close()

	var atoms []atom.Atom
	for r.Next() {
		var a atom.Atom
		if err := r.Scan(
			&a.ID, &a.Project, &a.Type, &a.Subject, &a.Predicate, &a.Value,
			&a.Statement, &a.Scope, &a.ValidFrom, &a.ValidTo, &a.ObservedAt, &a.Confidence,
			&a.ProvenanceMemoryID, &a.ProvenanceSpan,
			&a.Supersedes, &a.CreatedAt,
		); err != nil {
			return nil, err
		}
		atoms = append(atoms, a)
	}
	return atoms, r.Err()
}
