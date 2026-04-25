package ollama_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/petersimmons1972/engram/internal/ollama"
)

func TestVersion(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/version" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		json.NewEncoder(w).Encode(map[string]string{"version": "0.3.14"})
	}))
	defer srv.Close()

	c := ollama.NewClient(srv.URL)
	v, err := c.Version(context.Background())
	if err != nil {
		t.Fatalf("Version() error: %v", err)
	}
	if v != "0.3.14" {
		t.Errorf("want 0.3.14, got %q", v)
	}
}

func TestIsAvailable_True(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"models": []map[string]any{
				{"name": "mistral:7b", "digest": "sha256:abc123"},
			},
		})
	}))
	defer srv.Close()

	c := ollama.NewClient(srv.URL)
	ok, digest, err := c.IsAvailable(context.Background(), "mistral:7b")
	if err != nil {
		t.Fatalf("IsAvailable() error: %v", err)
	}
	if !ok {
		t.Error("want available=true")
	}
	if digest != "sha256:abc123" {
		t.Errorf("want digest sha256:abc123, got %q", digest)
	}
}

func TestIsAvailable_False(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"models": []any{}})
	}))
	defer srv.Close()

	c := ollama.NewClient(srv.URL)
	ok, _, err := c.IsAvailable(context.Background(), "missing:7b")
	if err != nil {
		t.Fatalf("IsAvailable() error: %v", err)
	}
	if ok {
		t.Error("want available=false")
	}
}

func TestEvict(t *testing.T) {
	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		if r.URL.Path != "/api/chat" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		if body["keep_alive"] != float64(0) {
			t.Errorf("want keep_alive=0, got %v", body["keep_alive"])
		}
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]any{"done": true})
	}))
	defer srv.Close()

	c := ollama.NewClient(srv.URL)
	if err := c.Evict(context.Background(), "mistral:7b"); err != nil {
		t.Fatalf("Evict() error: %v", err)
	}
	if !called {
		t.Error("expected HTTP call to Ollama")
	}
}
