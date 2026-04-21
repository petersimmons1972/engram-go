package search

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// pgPooler is a narrow interface satisfied by db.PostgresBackend.
// It allows the engine to obtain a *pgxpool.Pool for the weight cache
// without importing the db package's concrete type.
type pgPooler interface {
	PgxPool() *pgxpool.Pool
}

const weightCacheTTL = 15 * time.Minute

// weightCacheEntry holds a cached weight set with an expiry timestamp.
type weightCacheEntry struct {
	weights   Weights
	expiresAt time.Time
}

// WeightCache provides a per-process cache of project weights loaded from
// the weight_config table. Refreshes entries after weightCacheTTL (15 min).
// Thread-safe.
type WeightCache struct {
	mu      sync.Mutex
	entries map[string]weightCacheEntry
	pool    *pgxpool.Pool
}

// NewWeightCache creates a WeightCache backed by pool.
func NewWeightCache(pool *pgxpool.Pool) *WeightCache {
	return &WeightCache{
		entries: make(map[string]weightCacheEntry),
		pool:    pool,
	}
}

// Get returns cached weights for a project, refreshing from the DB if stale.
// Falls back to DefaultWeights() on any DB error.
func (c *WeightCache) Get(ctx context.Context, project string) Weights {
	c.mu.Lock()
	defer c.mu.Unlock()

	if e, ok := c.entries[project]; ok && time.Now().Before(e.expiresAt) {
		return e.weights
	}

	w := c.loadFromDB(ctx, project)
	c.entries[project] = weightCacheEntry{
		weights:   w,
		expiresAt: time.Now().Add(weightCacheTTL),
	}
	return w
}

// Invalidate removes the cached entry for a project so the next Get
// reloads from the DB. Used when weights are updated programmatically.
func (c *WeightCache) Invalidate(project string) {
	c.mu.Lock()
	delete(c.entries, project)
	c.mu.Unlock()
}

// loadFromDB queries weight_config and returns the stored weights, or defaults
// if no row exists or the query fails.
func (c *WeightCache) loadFromDB(ctx context.Context, project string) Weights {
	var w Weights
	err := c.pool.QueryRow(ctx,
		`SELECT weight_vector, weight_bm25, weight_recency, weight_precision
		 FROM weight_config WHERE project = $1`,
		project,
	).Scan(&w.Vector, &w.BM25, &w.Recency, &w.Precision)
	if err != nil {
		// No row or query error — fall back to compile-time defaults.
		if err.Error() != "no rows in result set" {
			slog.Warn("weight_cache: DB load failed, using defaults",
				"project", project, "err", err)
		}
		return DefaultWeights()
	}
	return w
}
