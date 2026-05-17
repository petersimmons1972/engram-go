package db

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"

	"github.com/petersimmons1972/engram/internal/metrics"
)

// TestStartPoolMetricsSampler_NilPool — must not panic when given a nil pool.
func TestStartPoolMetricsSampler_NilPool(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	StartPoolMetricsSampler(ctx, nil) // must be a no-op
}

// TestSamplePoolMetrics_UpdatesGauges — given a real pool, sampling once
// populates the gauges with the pool's current Stat() values.
func TestSamplePoolMetrics_UpdatesGauges(t *testing.T) {
	dsn := testDSN(t)
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}

	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatalf("ParseConfig: %v", err)
	}
	cfg.MaxConns = 7
	pool, err := pgxpool.NewWithConfig(context.Background(), cfg)
	if err != nil {
		t.Fatalf("NewWithConfig: %v", err)
	}
	defer pool.Close()
	if err := pool.Ping(context.Background()); err != nil {
		t.Skipf("ping: %v (test DB unreachable)", err)
	}

	samplePoolMetrics(pool)

	maxConns := testutil.ToFloat64(metrics.DBPoolMaxConns)
	if maxConns != 7 {
		t.Errorf("DBPoolMaxConns = %v, want 7", maxConns)
	}

	// Verify all 6 gauges have been registered with the global registry.
	mf, err := prometheus.DefaultGatherer.Gather()
	if err != nil {
		t.Fatalf("gather: %v", err)
	}
	want := []string{
		"engram_db_pool_acquired_conns",
		"engram_db_pool_idle_conns",
		"engram_db_pool_total_conns",
		"engram_db_pool_max_conns",
		"engram_db_pool_acquire_count_total",
		"engram_db_pool_acquire_duration_seconds_total",
	}
	have := map[string]bool{}
	for _, f := range mf {
		have[f.GetName()] = true
	}
	for _, w := range want {
		if !have[w] {
			t.Errorf("metric %q not registered", w)
		}
	}
}

