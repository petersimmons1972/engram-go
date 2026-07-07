package wp05retrofit

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/petersimmons1972/engram/internal/types"
)

// LoadGreenfieldMemories returns all raw_memories for a greenfield project as
// SearchResult values (score 0) for offline Layer B diagnosis.
func LoadGreenfieldMemories(ctx context.Context, dsn, project string) ([]types.SearchResult, error) {
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, fmt.Errorf("open greenfield pool: %w", err)
	}
	defer pool.Close()

	rows, err := pool.Query(ctx, `
		SELECT id, content, project
		FROM raw_memories
		WHERE project = $1
		ORDER BY created_at ASC`, project)
	if err != nil {
		return nil, fmt.Errorf("query raw_memories: %w", err)
	}
	defer rows.Close()

	out := make([]types.SearchResult, 0)
	for rows.Next() {
		var id, content, proj string
		if err := rows.Scan(&id, &content, &proj); err != nil {
			return nil, fmt.Errorf("scan raw_memory: %w", err)
		}
		out = append(out, types.SearchResult{
			Memory: &types.Memory{ID: id, Content: content, Project: proj},
			Score:  0,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}