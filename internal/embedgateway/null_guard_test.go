package embedgateway

import (
	"os"
	"strings"
	"testing"
)

// TestDrain_NullGuard_SQLContainsAndEmbeddingIsNull verifies the UPDATE
// in drain.go includes AND embedding IS NULL so a second drainer racing
// the same row gets rows_affected==0 and cannot corrupt an already-committed
// embedding. This is the mixed-drainer corruption gate for #1087 Patch A.
//
// Deterministic SQL-string assertion (no DB required).
func TestDrain_NullGuard_SQLContainsAndEmbeddingIsNull(t *testing.T) {
	src, err := os.ReadFile("drain.go")
	if err != nil {
		t.Fatalf("read drain.go: %v", err)
	}
	text := string(src)

	// Verify the guard is present.
	if !strings.Contains(text, "AND embedding IS NULL") {
		t.Error("drain.go UPDATE chunks does not include AND embedding IS NULL — " +
			"a second drainer racing after FOR UPDATE SKIP LOCKED commit can overwrite " +
			"the first committed embedding (#1087 Patch A, mixed-drainer corruption gate)")
	}

	// Verify the unguarded form is gone.
	const unguarded = `"UPDATE chunks SET embedding=$1 WHERE id=$2"`
	if strings.Contains(text, unguarded) {
		t.Error("drain.go still contains unguarded UPDATE (no AND embedding IS NULL) on the write path (#1087 Patch A)")
	}
}
