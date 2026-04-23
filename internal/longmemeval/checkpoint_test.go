package longmemeval_test

import (
	"path/filepath"
	"testing"

	"github.com/petersimmons1972/engram/internal/longmemeval"
)

func TestCheckpointRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.jsonl")

	ch := make(chan longmemeval.IngestEntry, 4)
	done := make(chan struct{})
	go func() {
		defer close(done)
		longmemeval.WriteCheckpoint(path, ch)
	}()

	ch <- longmemeval.IngestEntry{QuestionID: "q001", Status: "done", SessionCount: 3}
	ch <- longmemeval.IngestEntry{QuestionID: "q002", Status: "error", Error: "timeout"}
	close(ch)
	<-done

	skip, err := longmemeval.ReadSkipSet(path)
	if err != nil {
		t.Fatalf("ReadSkipSet: %v", err)
	}
	if !skip["q001"] {
		t.Error("q001 (done) should be in skip set")
	}
	if skip["q002"] {
		t.Error("q002 (error) should NOT be in skip set")
	}
}

func TestReadSkipSet_Missing(t *testing.T) {
	skip, err := longmemeval.ReadSkipSet("/tmp/nonexistent-ckpt-xyz.jsonl")
	if err != nil {
		t.Fatalf("ReadSkipSet on missing file: %v", err)
	}
	if len(skip) != 0 {
		t.Errorf("expected empty skip set, got %d entries", len(skip))
	}
}

func TestReadAllIngest(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "ingest.jsonl")

	ch := make(chan longmemeval.IngestEntry, 2)
	done := make(chan struct{})
	go func() {
		defer close(done)
		longmemeval.WriteCheckpoint(path, ch)
	}()
	ch <- longmemeval.IngestEntry{QuestionID: "q001", Status: "done", Project: "lme-x-q001", MemoryMap: map[string]string{"m1": "s1"}}
	close(ch)
	<-done

	entries, err := longmemeval.ReadAllIngest(path)
	if err != nil {
		t.Fatalf("ReadAllIngest: %v", err)
	}
	if len(entries) != 1 || entries[0].Project != "lme-x-q001" {
		t.Errorf("unexpected entries: %+v", entries)
	}
}
