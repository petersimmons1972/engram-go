package main

import (
	"os"
	"testing"
)

// TestBenchmarkSetsEmbedTimeout1500 verifies that longmemeval defaults the MCP
// recall timeout to 1500ms for benchmark run paths, and stores the chosen value
// in ENGRAM_EMBED_RECALL_TIMEOUT_MS so the recall engine uses it at construction.
func TestBenchmarkSetsEmbedTimeout1500(t *testing.T) {
	t.Setenv("LME_EMBED_RECALL_TIMEOUT_MS", "")
	t.Setenv("ENGRAM_EMBED_RECALL_TIMEOUT_MS", "unset")

	cfg := &Config{
		EmbedRecallTimeoutMS: envInt("LME_EMBED_RECALL_TIMEOUT_MS", 1500),
	}
	setBenchmarkEmbedTimeoutMS(cfg.EmbedRecallTimeoutMS)

	if got := os.Getenv("ENGRAM_EMBED_RECALL_TIMEOUT_MS"); got != "1500" {
		t.Fatalf("after benchmark setup, ENGRAM_EMBED_RECALL_TIMEOUT_MS=%q, want 1500", got)
	}
}
