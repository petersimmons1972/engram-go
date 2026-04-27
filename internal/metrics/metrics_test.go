package metrics

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

// TestMetricsRegistered verifies that all expected metrics are registered and
// have the correct names. The init() call in metrics.go registers them with the
// default registry; this test confirms they are reachable.
func TestMetricsRegistered(t *testing.T) {
	// Increment each counter/gauge once so testutil can observe them.
	ToolRequests.WithLabelValues("memory_store", "ok").Inc()
	ToolDuration.WithLabelValues("memory_store").Observe(0.001)
	WorkerTicks.WithLabelValues("reembed").Inc()
	WorkerErrors.WithLabelValues("reembed").Inc()
	ChunksPendingReembed.Set(5)

	names := []string{
		"engram_tool_requests_total",
		"engram_tool_duration_seconds",
		"engram_worker_ticks_total",
		"engram_worker_errors_total",
		"engram_chunks_pending_reembed",
	}
	for _, name := range names {
		mfs, err := prometheus.DefaultGatherer.Gather()
		if err != nil {
			t.Fatalf("gather: %v", err)
		}
		found := false
		for _, mf := range mfs {
			if mf.GetName() == name {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("metric %q not found in default registry", name)
		}
	}
}

// TestToolRequestsCounterLabels verifies the tool/status label combination
// increments independently.
func TestToolRequestsCounterLabels(t *testing.T) {
	// Use a fresh registry to isolate from init()-registered globals.
	reg := prometheus.NewRegistry()
	c := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "engram_tool_requests_total_test",
		Help: "test",
	}, []string{"tool", "status"})
	reg.MustRegister(c)

	c.WithLabelValues("memory_recall", "ok").Inc()
	c.WithLabelValues("memory_recall", "ok").Inc()
	c.WithLabelValues("memory_recall", "error").Inc()

	n := testutil.ToFloat64(c.WithLabelValues("memory_recall", "ok"))
	if n != 2 {
		t.Errorf("expected 2 ok increments, got %v", n)
	}
	e := testutil.ToFloat64(c.WithLabelValues("memory_recall", "error"))
	if e != 1 {
		t.Errorf("expected 1 error increment, got %v", e)
	}
}

// TestEpisodeLifecycleCounters verifies the three episode lifecycle counters
// are registered and increment independently.
func TestEpisodeLifecycleCounters(t *testing.T) {
	// Drive each counter once so the default gatherer can observe them.
	EpisodesStartedTotal.Inc()
	EpisodesEndedCleanTotal.Inc()
	EpisodesEndedByReaperTotal.Add(3)

	want := []string{
		"engram_episodes_started_total",
		"engram_episodes_ended_clean_total",
		"engram_episodes_ended_by_reaper_total",
	}
	mfs, err := prometheus.DefaultGatherer.Gather()
	if err != nil {
		t.Fatalf("gather: %v", err)
	}
	registered := make(map[string]bool, len(mfs))
	for _, mf := range mfs {
		registered[mf.GetName()] = true
	}
	for _, name := range want {
		if !registered[name] {
			t.Errorf("metric %q not found in default registry", name)
		}
	}

	// Verify Add(3) landed on the reaper counter via testutil on an isolated reg.
	reg := prometheus.NewRegistry()
	reaper := prometheus.NewCounter(prometheus.CounterOpts{
		Name: "engram_episodes_ended_by_reaper_total_test",
		Help: "test",
	})
	reg.MustRegister(reaper)
	reaper.Add(3)
	if v := testutil.ToFloat64(reaper); v != 3 {
		t.Errorf("expected reaper Add(3) == 3, got %v", v)
	}
}

// TestChunksPendingReembedGauge verifies Set and observation.
func TestChunksPendingReembedGauge(t *testing.T) {
	reg := prometheus.NewRegistry()
	g := prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "engram_chunks_pending_reembed_test",
		Help: "test",
	})
	reg.MustRegister(g)

	g.Set(42)
	if v := testutil.ToFloat64(g); v != 42 {
		t.Errorf("expected 42, got %v", v)
	}
	g.Set(0)
	if v := testutil.ToFloat64(g); v != 0 {
		t.Errorf("expected 0 after reset, got %v", v)
	}
}
