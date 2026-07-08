package wp05retrofit

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/petersimmons1972/engram/internal/types"
)

// LoadRetrofitMemories returns all memories for a retrofit project as SearchResult
// values for offline Layer B diagnosis.
func LoadRetrofitMemories(ctx context.Context, dsn, project string) ([]types.SearchResult, error) {
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, fmt.Errorf("open retrofit pool: %w", err)
	}
	defer pool.Close()

	rows, err := pool.Query(ctx, `
		SELECT id, content, project
		FROM memories
		WHERE project = $1
		ORDER BY created_at ASC`, project)
	if err != nil {
		return nil, fmt.Errorf("query memories: %w", err)
	}
	defer rows.Close()

	out := make([]types.SearchResult, 0)
	for rows.Next() {
		var id, content, proj string
		if err := rows.Scan(&id, &content, &proj); err != nil {
			return nil, fmt.Errorf("scan memory: %w", err)
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
