package db

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/petersimmons1972/engram/internal/metrics"
)

// PoolMetricsInterval controls how often StartPoolMetricsSampler updates the
// Prometheus gauges from pool.Stat(). 5 seconds is fast enough to catch
// saturation episodes in a Grafana scrape, slow enough not to be a hot path.
const PoolMetricsInterval = 5 * time.Second

// StartPoolMetricsSampler launches a background goroutine that periodically
// samples pool.Stat() and updates the engram_db_pool_* gauges in
// internal/metrics. The goroutine exits when ctx is cancelled. #673.
//
// One sampler per pool. Calling this on the shared pool gives operators
// visibility into pool saturation that /health Ping alone cannot.
func StartPoolMetricsSampler(ctx context.Context, pool *pgxpool.Pool) {
	if pool == nil {
		return
	}
	go func() {
		t := time.NewTicker(PoolMetricsInterval)
		defer t.Stop()
		// Sample once at startup so dashboards aren't blank for the first interval.
		samplePoolMetrics(pool)
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				samplePoolMetrics(pool)
			}
		}
	}()
}

// samplePoolMetrics reads pool.Stat() once and updates every gauge.
// Extracted from StartPoolMetricsSampler so it is unit-testable.
func samplePoolMetrics(pool *pgxpool.Pool) {
	s := pool.Stat()
	metrics.DBPoolAcquiredConns.Set(float64(s.AcquiredConns()))
	metrics.DBPoolIdleConns.Set(float64(s.IdleConns()))
	metrics.DBPoolTotalConns.Set(float64(s.TotalConns()))
	metrics.DBPoolMaxConns.Set(float64(s.MaxConns()))
	// AcquireCount + AcquireDuration are gauges that we re-set each sample
	// to the absolute cumulative value (see metrics.go for rationale).
	metrics.DBPoolAcquireCount.Set(float64(s.AcquireCount()))
	metrics.DBPoolAcquireDurationSeconds.Set(s.AcquireDuration().Seconds())
}
