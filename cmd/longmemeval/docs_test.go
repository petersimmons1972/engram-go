package main

import (
	"os"
	"strings"
	"testing"
)

func TestLMEBenchmarkLearningsScratchTTLIngestExampleUsesCurrentFlags(t *testing.T) {
	data, err := os.ReadFile("../../docs/lme-benchmark-learnings.md")
	if err != nil {
		t.Fatalf("read docs/lme-benchmark-learnings.md: %v", err)
	}
	doc := string(data)
	section := between(t, doc, "### Stamping TTL at ingest time", "### Running the prune sweep")

	for _, stale := range []string{"--data-file", "--out-dir", "--database-url"} {
		if strings.Contains(section, stale) {
			t.Fatalf("scratch TTL ingest example contains stale flag %q:\n%s", stale, section)
		}
	}
	for _, current := range []string{"--data questions.json", "--out /tmp/lme-run-001", "--scratch-ttl 168h"} {
		if !strings.Contains(section, current) {
			t.Fatalf("scratch TTL ingest example missing current flag text %q:\n%s", current, section)
		}
	}
}

func between(t *testing.T, s, start, end string) string {
	t.Helper()
	startIdx := strings.Index(s, start)
	if startIdx < 0 {
		t.Fatalf("missing start marker %q", start)
	}
	rest := s[startIdx:]
	endIdx := strings.Index(rest, end)
	if endIdx < 0 {
		t.Fatalf("missing end marker %q after %q", end, start)
	}
	return rest[:endIdx]
}
