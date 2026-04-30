package mcp_test

import (
	"archive/zip"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	internalmcp "github.com/petersimmons1972/engram/internal/mcp"
	"github.com/petersimmons1972/engram/internal/ingestqueue"
)

func buildMinimalSlackZip(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "slack-export.zip")
	f, _ := os.Create(path)
	w := zip.NewWriter(f)
	writeFile := func(name, body string) {
		zf, _ := w.Create(name)
		zf.Write([]byte(body))
	}
	writeFile("users.json", `[{"id":"U001","name":"alice","real_name":"Alice"}]`)
	writeFile("channels.json", `[{"id":"C001","name":"general"}]`)
	writeFile("general/2026-01-01.json", `[{"type":"message","user":"U001","text":"Hello","ts":"1700000000.000001"}]`)
	w.Close()
	f.Close()
	return path
}

func TestHandleMemoryIngestExport_SlackZip(t *testing.T) {
	zipPath := buildMinimalSlackZip(t)
	pool := internalmcp.NewTestStorePool(t)
	cfg := internalmcp.Config{DataDir: filepath.Dir(zipPath)}
	result, err := internalmcp.CallHandleMemoryIngestExport(context.Background(), t, pool, "default", cfg, zipPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.MemoriesStored == 0 {
		t.Errorf("want >=1 memory stored, got 0")
	}
	if result.Format != "slack" {
		t.Errorf("want format=slack, got %q", result.Format)
	}
}

func TestHandleMemoryIngestExport_ClaudeAIJSON(t *testing.T) {
	dir := t.TempDir()
	content := `[{"uuid":"abc","name":"Test","created_at":"2026-01-01T00:00:00.000000Z","updated_at":"2026-01-01T01:00:00.000000Z","chat_messages":[{"uuid":"m1","sender":"human","text":"hello","created_at":"2026-01-01T00:00:00.000000Z"}]}]`
	jsonPath := filepath.Join(dir, "conversations.json")
	os.WriteFile(jsonPath, []byte(content), 0o600)
	pool := internalmcp.NewTestStorePool(t)
	cfg := internalmcp.Config{DataDir: dir}
	result, err := internalmcp.CallHandleMemoryIngestExport(context.Background(), t, pool, "default", cfg, jsonPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.MemoriesStored != 1 {
		t.Errorf("want 1 memory, got %d", result.MemoriesStored)
	}
	if result.Format != "claudeai" {
		t.Errorf("want format=claudeai, got %q", result.Format)
	}
}

func TestHandleMemoryIngestExport_UnknownFormat(t *testing.T) {
	dir := t.TempDir()
	txtPath := filepath.Join(dir, "random.txt")
	os.WriteFile(txtPath, []byte("not an export"), 0o600)
	pool := internalmcp.NewTestStorePool(t)
	cfg := internalmcp.Config{DataDir: dir}
	_, err := internalmcp.CallHandleMemoryIngestExport(context.Background(), t, pool, "default", cfg, txtPath)
	if err == nil {
		t.Error("want error for unknown format, got nil")
	}
}

func TestHandleMemoryIngestExport_PathOutsideDataDir(t *testing.T) {
	pool := internalmcp.NewTestStorePool(t)
	cfg := internalmcp.Config{DataDir: t.TempDir()}
	_, err := internalmcp.CallHandleMemoryIngestExport(context.Background(), t, pool, "default", cfg, "/etc/passwd")
	if err == nil {
		t.Error("want path validation error, got nil")
	}
}

// TestHandleMemoryIngestExport_Queued verifies that when an IngestQueue is
// configured the handler returns status=queued with a non-empty job_id rather
// than blocking on the store path.
func TestHandleMemoryIngestExport_Queued(t *testing.T) {
	zipPath := buildMinimalSlackZip(t)
	pool := internalmcp.NewTestStorePool(t)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	q := ingestqueue.New(ctx, ingestqueue.Config{Depth: 16, Workers: 2})

	cfg := internalmcp.Config{
		DataDir:     filepath.Dir(zipPath),
		IngestQueue: q,
	}
	out, err := internalmcp.CallHandleMemoryIngestExportRaw(context.Background(), t, pool, "default", cfg, zipPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := out["status"]; got != "queued" {
		t.Errorf("want status=queued, got %v", got)
	}
	jobID, ok := out["job_id"].(string)
	if !ok || jobID == "" {
		t.Errorf("want non-empty job_id, got %v", out["job_id"])
	}
}

// TestHandleMemoryIngestExport_QueueFull verifies that when the queue is at
// capacity the handler returns status=queue_full without panicking.
func TestHandleMemoryIngestExport_QueueFull(t *testing.T) {
	zipPath := buildMinimalSlackZip(t)
	pool := internalmcp.NewTestStorePool(t)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Use a 1-worker, 1-slot queue and a blocking job to guarantee fullness
	// before the test call — avoids the timing race in a larger queue.
	block := make(chan struct{})
	q := ingestqueue.New(ctx, ingestqueue.Config{Depth: 1, Workers: 1})

	started := make(chan struct{}, 1)
	_ = q.Enqueue(&ingestqueue.Job{
		ID: "worker-holder", Project: "test",
		Work: func(ctx context.Context) error {
			started <- struct{}{}
			<-block
			return nil
		},
	})
	<-started // worker is definitely busy

	// Fill the 1-slot channel.
	_ = q.Enqueue(&ingestqueue.Job{
		ID: "filler", Project: "test",
		Work: func(ctx context.Context) error { return nil },
	})

	// Queue is now full (worker busy + channel slot taken).
	cfg := internalmcp.Config{
		DataDir:     filepath.Dir(zipPath),
		IngestQueue: q,
	}
	out, err := internalmcp.CallHandleMemoryIngestExportRaw(context.Background(), t, pool, "default", cfg, zipPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := out["status"]; got != "queue_full" {
		t.Errorf("want status=queue_full, got %v", got)
	}
	close(block)
}

// TestHandleMemoryIngestStatus_UnknownJob verifies that querying a job ID that
// was never enqueued returns status=unknown.
func TestHandleMemoryIngestStatus_UnknownJob(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	q := ingestqueue.New(ctx, ingestqueue.Config{Depth: 4, Workers: 1})
	cfg := internalmcp.Config{IngestQueue: q}

	out := internalmcp.CallHandleMemoryIngestStatus(ctx, t, cfg, "does-not-exist")
	if got := out["status"]; got != "unknown" {
		t.Errorf("want status=unknown, got %v", got)
	}
}

// TestHandleMemoryIngestStatus_CompletedJob enqueues a fast no-op job, waits
// for it to finish, then verifies the status handler reports status=done.
func TestHandleMemoryIngestStatus_CompletedJob(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	q := ingestqueue.New(ctx, ingestqueue.Config{Depth: 4, Workers: 1})

	done := make(chan struct{}, 1)
	jobID := "fast-job"
	_ = q.Enqueue(&ingestqueue.Job{
		ID: jobID, Project: "test",
		Work: func(ctx context.Context) error {
			done <- struct{}{}
			return nil
		},
	})

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for job to start")
	}
	// Give the worker goroutine a moment to update the result to StatusDone.
	time.Sleep(20 * time.Millisecond)

	cfg := internalmcp.Config{IngestQueue: q}
	out := internalmcp.CallHandleMemoryIngestStatus(ctx, t, cfg, jobID)
	if got := out["status"]; got != string(ingestqueue.StatusDone) {
		t.Errorf("want status=done, got %v", got)
	}
}

// Ensure IngestExportResult round-trips through JSON correctly.
func TestIngestExportResult_JSONRoundtrip(t *testing.T) {
	r := internalmcp.IngestExportResult{
		Format:         "slack",
		MemoriesStored: 3,
		MemoryIDs:      []string{"id1", "id2", "id3"},
	}
	b, err := json.Marshal(r)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got internalmcp.IngestExportResult
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.Format != r.Format || got.MemoriesStored != r.MemoriesStored || len(got.MemoryIDs) != len(r.MemoryIDs) {
		t.Errorf("round-trip mismatch: %+v != %+v", got, r)
	}
}
