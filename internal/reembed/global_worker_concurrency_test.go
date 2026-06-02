package reembed

import (
	"os"
	"strings"
	"testing"
)

// TestGlobalReembedder_ConcurrencyIsEnvConfigurable verifies that the global
// reembedder concurrency limit is read from ENGRAM_GLOBAL_REEMBED_CONCURRENCY
// rather than hardcoded. This lets operators throttle the W6800 reembed batch
// pipeline so interactive recalls do not starve the embed endpoint and silently
// fall back to BM25-only (#917).
func TestGlobalReembedder_ConcurrencyIsEnvConfigurable(t *testing.T) {
	src, err := os.ReadFile("global_worker.go")
	if err != nil {
		t.Fatalf("read global_worker.go: %v", err)
	}
	text := string(src)

	if strings.Contains(text, "const globalConcurrency = 8") {
		t.Error("global_worker.go still hardcodes globalConcurrency — must be env-configurable via ENGRAM_GLOBAL_REEMBED_CONCURRENCY (#917)")
	}
	if !strings.Contains(text, "ENGRAM_GLOBAL_REEMBED_CONCURRENCY") {
		t.Error("global_worker.go does not read ENGRAM_GLOBAL_REEMBED_CONCURRENCY — concurrency cannot be tuned at deploy time (#917)")
	}
}

// TestGlobalConcurrencyDefault verifies the env-configured default remains 8
// when the variable is unset, preserving prior behavior for deployments that
// do not set it.
func TestGlobalConcurrencyDefault(t *testing.T) {
	if globalConcurrency != 8 {
		t.Errorf("globalConcurrency default = %d, want 8 (unset env should preserve prior behavior)", globalConcurrency)
	}
}
