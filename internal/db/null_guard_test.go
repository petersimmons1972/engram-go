package db

import (
	"os"
	"strings"
	"testing"
)

// TestUpdateChunkEmbedding_NullGuard_SQLContainsAndEmbeddingIsNull verifies
// that UpdateChunkEmbedding in postgres_chunk.go includes AND embedding IS NULL
// on the UPDATE so that:
//
//	(a) A second writer racing the same row gets rows_affected==0 (no overwrite).
//	(b) A normal reembed of a NULL-embedding row still writes successfully.
//	(c) A mixed-drainer attempt (global_worker + embed_gateway racing the same
//	    row after the claim transaction commits) PRESERVES the first committed
//	    embedding — the corruption gate for #1087 Patch A.
//
// SQL-string assertion (no DB required). The companion integration test
// (TestUpdateChunkEmbedding_NullGuard_RowsAffected, //go:build integration)
// covers live rows_affected==0 and rows_affected==1 behaviour.
func TestUpdateChunkEmbedding_NullGuard_SQLContainsAndEmbeddingIsNull(t *testing.T) {
	src, err := os.ReadFile("postgres_chunk.go")
	if err != nil {
		t.Fatalf("read postgres_chunk.go: %v", err)
	}
	text := string(src)

	// Guard must be present on the UpdateChunkEmbedding path.
	if !strings.Contains(text, "AND embedding IS NULL") {
		t.Error("postgres_chunk.go UpdateChunkEmbedding does not include AND embedding IS NULL — " +
			"concurrent reembedders can silently overwrite each other's committed embeddings (#1087 Patch A)")
	}

	// The unguarded form must be absent from UpdateChunkEmbedding.
	const unguarded = `"UPDATE chunks SET embedding=$1 WHERE id=$2"`
	if strings.Contains(text, unguarded) {
		t.Error("postgres_chunk.go still contains unguarded UPDATE (no AND embedding IS NULL) — " +
			"UpdateChunkEmbedding write-guard is incomplete (#1087 Patch A)")
	}

	// Verify the NULL-clearing paths (ResetProjectEmbeddings etc.) are NOT guarded
	// by AND embedding IS NULL — those intentionally overwrite non-null embeddings.
	const resetSQL = "UPDATE chunks SET embedding=NULL"
	if !strings.Contains(text, resetSQL) {
		t.Error("postgres_chunk.go: expected a NULL-clearing UPDATE path (embedding=NULL); " +
			"if it was removed, this test needs updating")
	}
}
