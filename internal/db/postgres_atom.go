package db

import (
	"context"
	"strings"
	"time"

	"github.com/google/uuid"
	pgvector "github.com/pgvector/pgvector-go"
	"github.com/petersimmons1972/engram/internal/atom"
)

// InsertAtom inserts a new atom into the atoms table. A UUIDv4 ID is generated
// if a.ID is empty. The atom's Project must be set by the caller.
// Embedding is NOT written here — that is a separate step via InsertAtomEmbedding.
func (p *PostgresBackend) InsertAtom(ctx context.Context, a *atom.Atom) error {
	if a.ID == "" {
		a.ID = uuid.New().String()
	}
	if a.CreatedAt.IsZero() {
		a.CreatedAt = time.Now().UTC()
	}
	_, err := p.pool.Exec(ctx, `
		INSERT INTO atoms (
			id, project, atom_type, subject, predicate, value,
			statement, scope, valid_from, valid_to, confidence,
			provenance_memory_id, provenance_span, supersedes, created_at
		) VALUES (
			$1, $2, $3, $4, $5, $6,
			$7, $8, $9, $10, $11,
			$12, $13, $14, $15
		) ON CONFLICT (id) DO NOTHING`,
		a.ID, a.Project, a.Type, a.Subject, a.Predicate, a.Value,
		a.Statement, a.Scope, a.ValidFrom, a.ValidTo, a.Confidence,
		nullableString(a.ProvenanceMemoryID), nullableString(a.ProvenanceSpan),
		nullableString(a.Supersedes), a.CreatedAt,
	)
	return err
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

// RetireAtom sets valid_to on the atom with the given ID, effectively marking
// it as superseded. Idempotent: a second call with the same ID is a no-op if
// valid_to is already set.
func (p *PostgresBackend) RetireAtom(ctx context.Context, atomID string, validTo time.Time) error {
	_, err := p.pool.Exec(ctx,
		`UPDATE atoms SET valid_to = $1 WHERE id = $2 AND valid_to IS NULL`,
		validTo, atomID)
	return err
}

// GetActiveAtoms returns all atoms for the project with valid_to IS NULL.
// If atomType is non-empty, results are filtered to that type.
func (p *PostgresBackend) GetActiveAtoms(ctx context.Context, project string, atomType string) ([]atom.Atom, error) {
	var rows interface{ Close() }
	var err error

	if atomType == "" {
		rows, err = p.pool.Query(ctx, `
			SELECT id, project, atom_type, subject, predicate, value,
			       statement, scope, valid_from, valid_to, confidence,
			       COALESCE(provenance_memory_id,''), COALESCE(provenance_span,''),
			       COALESCE(supersedes,''), created_at
			FROM atoms
			WHERE project = $1 AND valid_to IS NULL
			ORDER BY created_at DESC`,
			project)
	} else {
		rows, err = p.pool.Query(ctx, `
			SELECT id, project, atom_type, subject, predicate, value,
			       statement, scope, valid_from, valid_to, confidence,
			       COALESCE(provenance_memory_id,''), COALESCE(provenance_span,''),
			       COALESCE(supersedes,''), created_at
			FROM atoms
			WHERE project = $1 AND atom_type = $2 AND valid_to IS NULL
			ORDER BY created_at DESC`,
			project, atomType)
	}
	if err != nil {
		return nil, err
	}
	return scanAtomRows(rows)
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
	pgRows, err := p.pool.Query(ctx, `
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

// rowScanner is the minimal interface for scanning pgx rows, shared by Query
// results (pgx.Rows) and the stub in tests. Avoids importing pgx in tests.
type rowScanner interface {
	Next() bool
	Scan(dest ...interface{}) error
	Err() error
	Close()
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
			&a.Statement, &a.Scope, &a.ValidFrom, &a.ValidTo, &a.Confidence,
			&a.ProvenanceMemoryID, &a.ProvenanceSpan,
			&a.Supersedes, &a.CreatedAt,
		); err != nil {
			return nil, err
		}
		atoms = append(atoms, a)
	}
	return atoms, r.Err()
}
