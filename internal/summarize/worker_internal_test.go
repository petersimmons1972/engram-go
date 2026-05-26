package summarize

import (
	"testing"
	"time"
)

func TestBatchTimeoutSupportsLongSummaries(t *testing.T) {
	if batchTimeout < 10*time.Minute {
		t.Fatalf("batchTimeout = %s, want at least 10m for long-running summaries", batchTimeout)
	}
}
