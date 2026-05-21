package reembed

import (
	"os"
	"strings"
	"testing"
)

// TestGlobalReembedder_HasSafeRunBatch — #708: the global reembedder loop
// must wrap its batch call in a safeRunBatch-equivalent panic-recovery
// pattern that increments the WorkerPanics counter and continues, mirroring
// the per-project Worker.safeRunBatch. Otherwise a single bad chunk panics
// the goroutine and the reembed pipeline silently stalls.
func TestGlobalReembedder_HasSafeRunBatch(t *testing.T) {
	src, err := os.ReadFile("global_worker.go")
	if err != nil {
		t.Fatalf("read global_worker.go: %v", err)
	}
	text := string(src)
	if !strings.Contains(text, "safeRunBatch") {
		t.Errorf("global_worker.go missing safeRunBatch wrapper — panics will kill the goroutine (#708)")
	}
	if !strings.Contains(text, `WorkerPanics.WithLabelValues("global_reembed")`) {
		t.Errorf("global_worker.go missing WorkerPanics counter increment for 'global_reembed' worker (#708)")
	}
}
