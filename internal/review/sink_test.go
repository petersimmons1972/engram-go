package review

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestFingerprintStable(t *testing.T) {
	a := Fingerprint("title", "source", "body")
	b := Fingerprint("title", "source", "body")
	if a != b {
		t.Fatalf("fingerprint not stable: %q != %q", a, b)
	}
}

func TestLocalSinkDedupedByCaller(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "drafts.jsonl")
	sink := &dedupeSink{next: &localSink{path: path}}
	e := Event{Title: "t", Body: "b", Source: "s", Fingerprint: "fp"}
	if err := sink.Record(context.Background(), e); err != nil {
		t.Fatalf("first record: %v", err)
	}
	if err := sink.Record(context.Background(), e); err != nil {
		t.Fatalf("second record: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read draft: %v", err)
	}
	if got := len(splitLines(string(data))); got != 1 {
		t.Fatalf("want 1 line, got %d", got)
	}
}

func splitLines(s string) []string {
	out := []string{}
	cur := ""
	for _, r := range s {
		if r == '\n' {
			out = append(out, cur)
			cur = ""
			continue
		}
		cur += string(r)
	}
	if cur != "" {
		out = append(out, cur)
	}
	return out
}
