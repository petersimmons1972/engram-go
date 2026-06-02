package hookdaemon

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMCPTokenStore_LoadAndStore(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "mcp_servers.json")
	initial := `{
  "mcpServers": {
    "engram": {
      "type": "sse",
      "url": "http://127.0.0.1:8788/sse",
      "headers": {"Authorization": "Bearer old-token"}
    },
    "other": {"url": "http://x"}
  }
}`
	if err := os.WriteFile(path, []byte(initial), 0o600); err != nil {
		t.Fatal(err)
	}
	s := NewMCPTokenStore(path)

	tok, err := s.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if tok != "old-token" {
		t.Fatalf("expected old-token, got %q", tok)
	}

	if err := s.Store("new-token"); err != nil {
		t.Fatalf("Store: %v", err)
	}
	got, _ := s.Load()
	if got != "new-token" {
		t.Fatalf("expected new-token after Store, got %q", got)
	}
	// other server entry must be preserved
	b, _ := os.ReadFile(path)
	if !strings.Contains(string(b), `"other"`) {
		t.Fatalf("Store clobbered unrelated config:\n%s", b)
	}
}

func TestFileMemoryWriter_ReplacesRecallSection(t *testing.T) {
	path := filepath.Join(t.TempDir(), "MEMORY.md")
	os.WriteFile(path, []byte("# Memory\n\nbody text\n\n## Engram Session Recall\n\nold stuff\n"), 0o600)
	w := NewFileMemoryWriter(path)
	if err := w.WriteRecallSection("\n## Engram Session Recall\n\nnew stuff\n"); err != nil {
		t.Fatalf("write: %v", err)
	}
	b, _ := os.ReadFile(path)
	out := string(b)
	if strings.Contains(out, "old stuff") {
		t.Fatalf("old recall section not replaced:\n%s", out)
	}
	if !strings.Contains(out, "new stuff") || !strings.Contains(out, "body text") {
		t.Fatalf("section replacement lost content:\n%s", out)
	}
	if strings.Count(out, recallHeading) != 1 {
		t.Fatalf("expected exactly one recall heading:\n%s", out)
	}
}

func TestFileFallbackStore_AppendsAtomically(t *testing.T) {
	path := filepath.Join(t.TempDir(), "fallback.md")
	s := NewFileFallbackStore(path)
	if err := s.Append([]string{"- one", "- two"}); err != nil {
		t.Fatalf("append: %v", err)
	}
	if err := s.Append([]string{"- three"}); err != nil {
		t.Fatalf("append 2: %v", err)
	}
	b, _ := os.ReadFile(path)
	out := string(b)
	for _, want := range []string{"- one", "- two", "- three"} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in:\n%s", want, out)
		}
	}
}

func TestHTTPEngramClient_HealthAndAuth(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/health":
			w.WriteHeader(http.StatusOK)
		case "/quick-recall":
			if r.Header.Get("Authorization") == "Bearer good" {
				w.WriteHeader(http.StatusOK)
				w.Write([]byte(`{"results":[]}`))
			} else {
				w.WriteHeader(http.StatusUnauthorized)
			}
		case "/quick-store":
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer ts.Close()

	c := NewHTTPEngramClient(ts.URL)
	if err := c.Health(context.Background()); err != nil {
		t.Fatalf("health: %v", err)
	}
	ok, err := c.CheckAuth(context.Background(), "good")
	if err != nil || !ok {
		t.Fatalf("expected auth ok, got ok=%v err=%v", ok, err)
	}
	ok, _ = c.CheckAuth(context.Background(), "bad")
	if ok {
		t.Fatal("expected auth rejected for bad token")
	}
	if _, err := c.Recall(context.Background(), "good", "q", "global", 1); err != nil {
		t.Fatalf("recall: %v", err)
	}
	if err := c.QuickStore(context.Background(), "good", []byte(`{}`)); err != nil {
		t.Fatalf("quick-store: %v", err)
	}
}

func TestHTTPEngramClient_HealthDown(t *testing.T) {
	c := NewHTTPEngramClient("http://127.0.0.1:1") // nothing listening
	if err := c.Health(context.Background()); err == nil {
		t.Fatal("expected health error when server down")
	}
}

// L3 — atomicWrite error paths.

// atomicWrite should succeed on a writable directory.
func TestAtomicWrite_HappyPath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "out.md")
	if err := atomicWrite(path, []byte("hello\n")); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	b, err := os.ReadFile(path)
	if err != nil || string(b) != "hello\n" {
		t.Fatalf("unexpected content: %q %v", b, err)
	}
}

// atomicWrite should fail when the directory does not exist (temp-create error).
func TestAtomicWrite_NoSuchDir(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nonexistent", "out.md")
	if err := atomicWrite(path, []byte("x")); err == nil {
		t.Fatal("expected error for missing parent directory")
	}
}

// atomicWrite should overwrite an existing file atomically.
func TestAtomicWrite_OverwritesExisting(t *testing.T) {
	path := filepath.Join(t.TempDir(), "out.md")
	if err := os.WriteFile(path, []byte("old\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := atomicWrite(path, []byte("new\n")); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	b, _ := os.ReadFile(path)
	if string(b) != "new\n" {
		t.Fatalf("expected new content, got %q", b)
	}
}
