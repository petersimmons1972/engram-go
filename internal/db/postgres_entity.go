package db

import (
	"context"
	"strings"

	"github.com/google/uuid"
	"github.com/petersimmons1972/engram/internal/entity"
)

func (p *PostgresBackend) UpsertEntity(ctx context.Context, e *entity.Entity) (string, error) {
	if e.ID == "" {
		e.ID = uuid.New().String()
	}
	var id string
	err := p.pool.QueryRow(ctx, `
		INSERT INTO canonical_entities (id, name, aliases, project, updated_at)
		VALUES ($1, $2, $3, $4, NOW())
		ON CONFLICT (project, lower(name)) DO UPDATE
			SET aliases = ARRAY(SELECT DISTINCT unnest(canonical_entities.aliases || EXCLUDED.aliases)),
			    updated_at = NOW()
		RETURNING id`,
		e.ID, e.Name, e.Aliases, e.Project).Scan(&id)
	if err != nil {
		return "", err
	}
	e.ID = id
	return id, nil
}

func (p *PostgresBackend) GetEntitiesByProject(ctx context.Context, project string) ([]entity.Entity, error) {
	rows, err := p.pool.Query(ctx,
		`SELECT id, name, aliases, project FROM canonical_entities WHERE project = $1`,
		project)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var entities []entity.Entity
	for rows.Next() {
		var e entity.Entity
		if err := rows.Scan(&e.ID, &e.Name, &e.Aliases, &e.Project); err != nil {
			return nil, err
		}
		entities = append(entities, e)
	}
	return entities, rows.Err()
}

func (p *PostgresBackend) EnqueueExtractionJob(ctx context.Context, memoryID, project string) error {
	_, err := p.pool.Exec(ctx, `
		INSERT INTO entity_extraction_jobs (id, memory_id, project)
		VALUES ($1, $2, $3)
		ON CONFLICT ON CONSTRAINT uq_entity_jobs_pending DO NOTHING`,
		uuid.New().String(), memoryID, project)
	return err
}

func (p *PostgresBackend) ClaimExtractionJobs(ctx context.Context, project string, limit int) ([]ExtractionJob, error) {
	rows, err := p.pool.Query(ctx, `
		UPDATE entity_extraction_jobs
		SET status = 'processing'
		WHERE id IN (
			SELECT id FROM entity_extraction_jobs
			WHERE project = $1 AND status = 'pending'
			ORDER BY created_at ASC
			LIMIT $2
			FOR UPDATE SKIP LOCKED
		)
		RETURNING id, memory_id, project`, project, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var jobs []ExtractionJob
	for rows.Next() {
		var j ExtractionJob
		if err := rows.Scan(&j.ID, &j.MemoryID, &j.Project); err != nil {
			return nil, err
		}
		jobs = append(jobs, j)
	}
	return jobs, rows.Err()
}

func truncateString(s string, maxRunes int) string {
	r := []rune(s)
	if len(r) > maxRunes {
		return string(r[:maxRunes])
	}
	return s
}

func (p *PostgresBackend) CompleteExtractionJob(ctx context.Context, jobID string, jobErr error) error {
	errMsg := ""
	status := "done"
	if jobErr != nil {
		errMsg = truncateString(strings.TrimSpace(jobErr.Error()), 500)
		status = "failed"
	}
	_, err := p.pool.Exec(ctx,
		`UPDATE entity_extraction_jobs SET status=$1, error=$2, processed_at=NOW() WHERE id=$3`,
		status, errMsg, jobID)
	return err
}
