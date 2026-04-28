package mcp

import (
	"fmt"
	"os"
)

const (
	defaultRecallMaxTopK = 500
	defaultTopK          = 10
)

// recallMaxTopK returns the maximum allowed top_k for memory_recall.
// Configurable via ENGRAM_RECALL_MAX_TOP_K; defaults to 500.
func recallMaxTopK() int {
	if v := os.Getenv("ENGRAM_RECALL_MAX_TOP_K"); v != "" {
		var n int
		if _, err := fmt.Sscanf(v, "%d", &n); err == nil && n > 0 {
			return n
		}
	}
	return defaultRecallMaxTopK
}

// clampTopK ensures topK is within [1, max]. Values < 1 reset to defaultTopK
// (10). Values > max are capped at max rather than silently reset to 10, so
// a caller requesting top_k=150 against a max=500 still gets 150 results.
func clampTopK(topK, max int) int {
	if topK < 1 {
		return defaultTopK
	}
	if topK > max {
		return max
	}
	return topK
}
