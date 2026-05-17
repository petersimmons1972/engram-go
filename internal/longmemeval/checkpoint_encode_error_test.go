package longmemeval

// Internal-package test for #670: WriteCheckpoint must log encode failures
// so silent loss of expensive LLM-call results is detectable.
// Lives in its own file (separate from the external-package checkpoint_test.go)
// to avoid disturbing the existing round-trip tests.

import (
	"bytes"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestWriteCheckpoint_LogsEncodeErrors(t *testing.T) {
	tmp := filepath.Join(t.TempDir(), "ckpt.jsonl")

	var buf bytes.Buffer
	orig := log.Writer()
	log.SetOutput(&buf)
	t.Cleanup(func() { log.SetOutput(orig) })

	// Two-channel pattern: the second entry contains a chan-typed value that
	// the JSON encoder refuses, exercising the error path.
	ch := make(chan map[string]any, 4)
	ch <- map[string]any{"id": "ok-1", "value": 1}
	ch <- map[string]any{"id": "bad", "value": make(chan int)}
	close(ch)

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		WriteCheckpoint(tmp, ch)
	}()
	wg.Wait()

	out := buf.String()
	if !strings.Contains(out, "WriteCheckpoint") {
		t.Errorf("expected WriteCheckpoint log line on encode failure; got: %q", out)
	}
	if !strings.Contains(out, "encode") {
		t.Errorf("log should identify the encode failure; got: %q", out)
	}

	// First entry must still be on disk despite second's failure.
	data, err := os.ReadFile(tmp)
	if err != nil {
		t.Fatalf("read checkpoint: %v", err)
	}
	if !strings.Contains(string(data), "ok-1") {
		t.Errorf("checkpoint missing first entry: %q", data)
	}
}
