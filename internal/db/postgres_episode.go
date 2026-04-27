package db

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/petersimmons1972/engram/internal/types"
)

func (b *PostgresBackend) StartEpisode(ctx context.Context, project, description string) (*types.Episode, error) {
	ep := &types.Episode{
		ID:          types.NewMemoryID(),
		Project:     project,
		Description: description,
		StartedAt:   time.Now().UTC(),
	}
	_, err := b.pool.Exec(ctx, `
		INSERT INTO episodes (id, project, description, started_at)
		VALUES ($1, $2, $3, $4)`,
		ep.ID, ep.Project, ep.Description, ep.StartedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("StartEpisode: %w", err)
	}
	return ep, nil
}

func (b *PostgresBackend) EndEpisode(ctx context.Context, id, summary string) error {
	_, err := b.pool.Exec(ctx, `
		UPDATE episodes SET ended_at = NOW(), summary = $1 WHERE id = $2`,
		summary, id,
	)
	return err
}

func (b *PostgresBackend) ListEpisodes(ctx context.Context, project string, limit int) ([]*types.Episode, error) {
	rows, err := b.pool.Query(ctx, `
		SELECT id, project, description, started_at, COALESCE(ended_at, '0001-01-01'::timestamptz), COALESCE(summary, '')
		FROM episodes
		WHERE project = $1
		ORDER BY started_at DESC
		LIMIT $2`,
		project, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("ListEpisodes: %w", err)
	}
	defer rows.Close()
	var eps []*types.Episode
	for rows.Next() {
		var ep types.Episode
		if err := rows.Scan(&ep.ID, &ep.Project, &ep.Description, &ep.StartedAt, &ep.EndedAt, &ep.Summary); err != nil {
			return nil, err
		}
		// Normalise the zero-time sentinel back.
		if ep.EndedAt.Year() == 1 {
			ep.EndedAt = time.Time{}
		}
		eps = append(eps, &ep)
	}
	return eps, rows.Err()
}

// CloseStaleEpisodes closes all open episodes whose started_at is older than
// olderThan. Returns the number of rows updated. Designed for use by a
// background reaper that handles crash-orphaned sessions.
func (b *PostgresBackend) CloseStaleEpisodes(ctx context.Context, olderThan time.Duration) (int64, error) {
	tag, err := b.pool.Exec(ctx, `
		UPDATE episodes
		   SET ended_at = NOW(),
		       summary  = COALESCE(NULLIF(summary, ''), 'auto-closed: stale')
		 WHERE ended_at IS NULL
		   AND started_at < NOW() - $1::interval`,
		fmt.Sprintf("%d seconds", int(olderThan.Seconds())))
	if err != nil {
		return 0, fmt.Errorf("CloseStaleEpisodes: %w", err)
	}
	return tag.RowsAffected(), nil
}

func (b *PostgresBackend) RecallEpisode(ctx context.Context, episodeID string) ([]*types.Memory, error) {
	rows, err := b.pool.Query(ctx, `
		SELECT * FROM memories
		WHERE episode_id = $1 AND valid_to IS NULL
		ORDER BY created_at ASC`,
		episodeID,
	)
	if err != nil {
		return nil, fmt.Errorf("RecallEpisode: %w", err)
	}
	return pgx.CollectRows(rows, rowToMemory)
}
