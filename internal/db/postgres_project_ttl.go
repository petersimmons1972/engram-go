package db

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

// SetProjectTTL upserts (project, created_at, expires_at) into project_ttl.
// A nil expiresAt means the project is durable (no expiry).
// Uses ON CONFLICT DO UPDATE so repeated calls are idempotent.
func (b *PostgresBackend) SetProjectTTL(ctx context.Context, project string, createdAt time.Time, expiresAt *time.Time) error {
	var pgExpires pgtype.Timestamptz
	if expiresAt != nil {
		pgExpires = pgtype.Timestamptz{Time: expiresAt.UTC(), Valid: true}
	} else {
		pgExpires = pgtype.Timestamptz{Valid: false}
	}
	_, err := b.pool.Exec(ctx, `
		INSERT INTO project_ttl (project, created_at, expires_at)
		VALUES ($1, $2, $3)
		ON CONFLICT (project) DO UPDATE
		  SET created_at = EXCLUDED.created_at,
		      expires_at = EXCLUDED.expires_at
	`, project, createdAt.UTC(), pgExpires)
	return err
}

// ListExpiredProjects queries project_ttl for rows whose expires_at is
// non-NULL and strictly before cutoff, filtered to names LIKE prefix+'%'.
// When limit > 0, at most limit rows are returned.
func (b *PostgresBackend) ListExpiredProjects(ctx context.Context, prefix string, cutoff time.Time, limit int) ([]string, error) {
	pattern := escapeLike(prefix) + "%"

	var (
		query string
		args  []any
	)
	if limit > 0 {
		query = `
			SELECT project FROM project_ttl
			WHERE expires_at IS NOT NULL
			  AND expires_at < $1
			  AND project LIKE $2
			ORDER BY expires_at
			LIMIT $3`
		args = []any{cutoff.UTC(), pattern, limit}
	} else {
		query = `
			SELECT project FROM project_ttl
			WHERE expires_at IS NOT NULL
			  AND expires_at < $1
			  AND project LIKE $2
			ORDER BY expires_at`
		args = []any{cutoff.UTC(), pattern}
	}

	rows, err := b.pool.Query(ctx, query, args...)
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
