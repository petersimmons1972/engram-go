package longmemeval_test

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
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

func TestCheckpointReadersFailClosedOnMalformedJSONL(t *testing.T) {
	dir := t.TempDir()

	tests := []struct {
		name string
		read func(string) error
	}{
		{
			name: "skip set",
			read: func(path string) error {
				_, err := longmemeval.ReadSkipSet(path)
				return err
			},
		},
		{
			name: "scored labels",
			read: func(path string) error {
				_, err := longmemeval.ReadScoredLabels(path)
				return err
			},
		},
		{
			name: "all ingest",
			read: func(path string) error {
				_, err := longmemeval.ReadAllIngest(path)
				return err
			},
		},
		{
			name: "all run",
			read: func(path string) error {
				_, err := longmemeval.ReadAllRun(path)
				return err
			},
		},
		{
			name: "all score",
			read: func(path string) error {
				_, err := longmemeval.ReadAllScore(path)
				return err
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(dir, strings.ReplaceAll(tt.name, " ", "-")+".jsonl")
			data := []byte("{\"question_id\":\"q001\",\"status\":\"done\"}\n{bad-json\n")
			if err := os.WriteFile(path, data, 0o644); err != nil {
				t.Fatal(err)
			}

			if err := tt.read(path); err == nil {
				t.Fatal("expected malformed checkpoint line to fail closed")
			}
		})
	}
}

func TestReadScoredLabels(t *testing.T) {
	f, _ := os.CreateTemp(t.TempDir(), "score-*.jsonl")
	fmt.Fprintln(f, `{"question_id":"a","status":"done","score_label":"CORRECT"}`)
	fmt.Fprintln(f, `{"question_id":"b","status":"done","score_label":"PARTIALLY_CORRECT"}`)
	fmt.Fprintln(f, `{"question_id":"c","status":"error","score_label":""}`)
	f.Close()
	labels, err := longmemeval.ReadScoredLabels(f.Name())
	if err != nil {
		t.Fatal(err)
	}
	if labels["a"] != "CORRECT" {
		t.Errorf("want CORRECT got %q", labels["a"])
	}
	if labels["b"] != "PARTIALLY_CORRECT" {
		t.Errorf("want PARTIALLY_CORRECT got %q", labels["b"])
	}
	if _, ok := labels["c"]; ok {
		t.Error("error entries must not appear in labels map")
	}
	// non-existent file → empty map, no error
	labels2, err2 := longmemeval.ReadScoredLabels("/tmp/does-not-exist-lme-xyz.jsonl")
	if err2 != nil {
		t.Fatalf("missing file must not error: %v", err2)
	}
	if len(labels2) != 0 {
		t.Error("missing file must return empty map")
	}
}

func TestReadScoredLabels_LastWriteWinsClearsDoneLabel(t *testing.T) {
	f, _ := os.CreateTemp(t.TempDir(), "score-*.jsonl")
	fmt.Fprintln(f, `{"question_id":"a","status":"done","score_label":"CORRECT"}`)
	fmt.Fprintln(f, `{"question_id":"a","status":"error","score_label":"","error":"retry failed"}`)
	fmt.Fprintln(f, `{"question_id":"b","status":"error","score_label":"","error":"first failed"}`)
	fmt.Fprintln(f, `{"question_id":"b","status":"done","score_label":"INCORRECT"}`)
	f.Close()

	labels, err := longmemeval.ReadScoredLabels(f.Name())
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := labels["a"]; ok {
		t.Error("later error entry must clear stale done label")
	}
	if labels["b"] != "INCORRECT" {
		t.Errorf("later done entry should win for b; got %q", labels["b"])
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

func TestWriteCheckpointReturnsOpenError(t *testing.T) {
	ch := make(chan longmemeval.IngestEntry)
	close(ch)

	err := longmemeval.WriteCheckpoint(filepath.Join(t.TempDir(), "missing", "ckpt.jsonl"), ch)
	if err == nil {
		t.Fatal("expected open failure to be returned")
	}
	if errors.Is(err, os.ErrNotExist) {
		return
	}
	t.Fatalf("expected not-exist error, got %v", err)
}
