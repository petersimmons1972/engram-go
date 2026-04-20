package db

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/petersimmons1972/engram/internal/types"
)

// escapeLike escapes the three PostgreSQL LIKE metacharacters so that a filter
// value is treated as a literal substring rather than a pattern.
// Order matters: escape the escape character first, then % and _.
func escapeLike(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `%`, `\%`)
	s = strings.ReplaceAll(s, `_`, `\_`)
	return s
}

// AggregateMemories dispatches to the appropriate aggregate helper based on
// the by parameter. Supported values are "tag" and "type".
// by="failure_class" is handled at the engine layer via AggregateFailureClasses.
func (b *PostgresBackend) AggregateMemories(ctx context.Context, project, by, filter string, limit int) ([]types.AggregateRow, error) {
	if limit <= 0 {
		limit = 20
	}
	switch by {
	case "tag":
		return b.aggregateByTag(ctx, project, filter, limit)
	case "type":
		return b.aggregateByType(ctx, project, limit)
	default:
		return nil, fmt.Errorf("invalid aggregate by %q: must be tag, type, or failure_class", by)
	}
}

// aggregateByTag returns tag-level counts for active memories in the given
// project. An optional ILIKE filter is applied inside a subquery so that
// PostgreSQL resolves the column alias unambiguously.
func (b *PostgresBackend) aggregateByTag(ctx context.Context, project, filter string, limit int) ([]types.AggregateRow, error) {
	const q = `
SELECT label, count, oldest, newest
FROM (
    SELECT
        jsonb_array_elements_text(tags) AS label,
        COUNT(*)::int                   AS count,
        MIN(created_at)                 AS oldest,
        MAX(created_at)                 AS newest
    FROM memories
    WHERE project = $1 AND valid_to IS NULL
    GROUP BY label
) sub
WHERE ($2 = '' OR label ILIKE '%' || $2 || '%' ESCAPE '\')
ORDER BY count DESC
LIMIT $3`

	rows, err := b.pool.Query(ctx, q, project, escapeLike(filter), limit)
	if err != nil {
		return nil, fmt.Errorf("aggregateByTag query: %w", err)
	}
	defer rows.Close()

	result := make([]types.AggregateRow, 0)
	for rows.Next() {
		var row types.AggregateRow
		var oldest, newest time.Time
		if err := rows.Scan(&row.Label, &row.Count, &oldest, &newest); err != nil {
			return nil, fmt.Errorf("aggregateByTag scan: %w", err)
		}
		row.Oldest = oldest
		row.Newest = newest
		result = append(result, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("aggregateByTag rows: %w", err)
	}
	return result, nil
}

// aggregateByType returns memory-type-level counts for active memories in the
// given project.
func (b *PostgresBackend) aggregateByType(ctx context.Context, project string, limit int) ([]types.AggregateRow, error) {
	const q = `
SELECT
    memory_type         AS label,
    COUNT(*)::int       AS count,
    MIN(created_at)     AS oldest,
    MAX(created_at)     AS newest
FROM memories
WHERE project = $1 AND valid_to IS NULL
GROUP BY memory_type
ORDER BY count DESC
LIMIT $2`

	rows, err := b.pool.Query(ctx, q, project, limit)
	if err != nil {
		return nil, fmt.Errorf("aggregateByType query: %w", err)
	}
	defer rows.Close()

	result := make([]types.AggregateRow, 0)
	for rows.Next() {
		var row types.AggregateRow
		var oldest, newest time.Time
		if err := rows.Scan(&row.Label, &row.Count, &oldest, &newest); err != nil {
			return nil, fmt.Errorf("aggregateByType scan: %w", err)
		}
		row.Oldest = oldest
		row.Newest = newest
		result = append(result, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("aggregateByType rows: %w", err)
	}
	return result, nil
}

// AggregateFailureClasses returns failure-class-level counts from the
// retrieval_events table for the given project.
func (b *PostgresBackend) AggregateFailureClasses(ctx context.Context, project string, limit int) ([]types.AggregateRow, error) {
	if limit <= 0 {
		limit = 20
	}

	const q = `
SELECT
    failure_class       AS label,
    COUNT(*)::int       AS count,
    MIN(created_at)     AS oldest,
    MAX(created_at)     AS newest
FROM retrieval_events
WHERE project = $1 AND failure_class IS NOT NULL
GROUP BY failure_class
ORDER BY count DESC
LIMIT $2`

	rows, err := b.pool.Query(ctx, q, project, limit)
	if err != nil {
		return nil, fmt.Errorf("aggregateFailureClasses query: %w", err)
	}
	defer rows.Close()

	result := make([]types.AggregateRow, 0)
	for rows.Next() {
		var row types.AggregateRow
		var oldest, newest time.Time
		if err := rows.Scan(&row.Label, &row.Count, &oldest, &newest); err != nil {
			return nil, fmt.Errorf("aggregateFailureClasses scan: %w", err)
		}
		row.Oldest = oldest
		row.Newest = newest
		result = append(result, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("aggregateFailureClasses rows: %w", err)
	}
	return result, nil
}
