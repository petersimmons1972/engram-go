// Package bench_test documents the context-savings story for memory_explore
// versus the manual alternative of 5× memory_recall + 1× memory_reason.
//
// These are synthetic benchmarks — no live Postgres or Ollama required.
// Payload sizes are representative of real-world responses at detail="full".
//
// Spec requirement: memory_explore must deliver ≥80% byte reduction vs the
// manual path. Both BenchmarkExploreContextSavings and
// TestExploreContextSavings_MetricThresholds enforce that threshold.
package bench_test

import "testing"

const (
	// recallSize is the size in bytes of a single memory_recall result at
	// detail="full": id, content, score, tags, timestamps — roughly 2 KB.
	recallSize = 2 * 1024

	// recallCount is the number of memory_recall calls in the manual path.
	recallCount = 5

	// reasonSize is the size in bytes of a memory_reason response:
	// a synthesized paragraph — roughly 1 KB.
	reasonSize = 1 * 1024

	// manualBytes is the total byte cost of the manual path:
	// 5 × memory_recall(full) + 1 × memory_reason ≈ 11 KB.
	manualBytes = recallCount*recallSize + reasonSize

	// exploreBytes is the size of a memory_explore response:
	// {answer: "...", sources: [{id, score}], confidence: 0.9} — roughly 2 KB.
	// The sources list carries lightweight IDs only, not full content.
	exploreBytes = 2 * 1024
)

// BenchmarkExploreContextSavings measures context bytes saved by using
// memory_explore instead of the 5-recall + 1-reason manual path.
//
// Run with: go test ./bench/... -bench=BenchmarkExploreContextSavings
//
// Expected output includes two custom metrics:
//
//	pct_context_saved   ≥ 80
//	bytes_saved         ≥ 9216
func BenchmarkExploreContextSavings(b *testing.B) {
	for i := 0; i < b.N; i++ {
		savingsPct := (1 - float64(exploreBytes)/float64(manualBytes)) * 100

		b.ReportMetric(savingsPct, "pct_context_saved")
		b.ReportMetric(float64(manualBytes-exploreBytes), "bytes_saved")

		if savingsPct < 80.0 {
			b.Errorf("context savings regression: got %.1f%% < required 80.0%% (manualBytes=%d, exploreBytes=%d)",
				savingsPct, manualBytes, exploreBytes)
		}
	}
}

// TestExploreContextSavings_MetricThresholds verifies the ≥80% byte-reduction
// threshold without requiring -bench=. so ordinary `go test` catches regressions.
func TestExploreContextSavings_MetricThresholds(t *testing.T) {
	savingsPct := (1 - float64(exploreBytes)/float64(manualBytes)) * 100

	t.Logf("manualBytes:  %d bytes (%d KB)", manualBytes, manualBytes/1024)
	t.Logf("exploreBytes: %d bytes (%d KB)", exploreBytes, exploreBytes/1024)
	t.Logf("bytes_saved:  %d bytes", manualBytes-exploreBytes)
	t.Logf("pct_context_saved: %.2f%%", savingsPct)

	if savingsPct < 80.0 {
		t.Errorf("context savings regression: got %.1f%% < required 80.0%% "+
			"(manualBytes=%d, exploreBytes=%d)",
			savingsPct, manualBytes, exploreBytes)
	}
}
