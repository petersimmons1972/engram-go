package reembed

import (
	"os"
	"strings"
	"testing"
)

// TestGlobalWorker_NullGuard_SQLContainsAndEmbeddingIsNull verifies the UPDATE
// in global_worker.go's runBatch includes AND embedding IS NULL so a second
// drainer racing the same row gets rows_affected==0 and cannot overwrite an
// already-committed embedding. This is Patch A of #1087 (no-overwrite invariant).
//
// Deterministic SQL-string assertion (no DB required). The companion integration
// test (TestGlobalWorker_NullGuard_RowsAffectedZeroOnRace, //go:build integration)
// covers the live rows_affected==0 guarantee.
func TestGlobalWorker_NullGuard_SQLContainsAndEmbeddingIsNull(t *testing.T) {
	src, err := os.ReadFile("global_worker.go")
	if err != nil {
		t.Fatalf("read global_worker.go: %v", err)
	}
	text := string(src)

	// Verify the guard is present on the write path.
	if !strings.Contains(text, "AND embedding IS NULL") {
		t.Error("global_worker.go UPDATE chunks does not include AND embedding IS NULL — " +
			"concurrent drainers can overwrite each other's committed embeddings (#1087 Patch A)")
	}

	// Verify the unguarded form is gone (catches partial edits).
	const unguarded = `"UPDATE chunks SET embedding=$1 WHERE id=$2"`
	if strings.Contains(text, unguarded) {
		t.Error("global_worker.go still contains unguarded UPDATE (no AND embedding IS NULL) on the write path (#1087 Patch A)")
	}
}
