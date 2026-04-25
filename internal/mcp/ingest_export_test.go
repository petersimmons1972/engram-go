package mcp_test

import (
	"archive/zip"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	internalmcp "github.com/petersimmons1972/engram/internal/mcp"
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
