package embed

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestProbeUsesModelsEndpointNotEmbeddings(t *testing.T) {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/models":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"object": "list",
				"data": []map[string]any{
					{"id": "bge-m3-Q8_0.gguf", "object": "model", "owned_by": "olla"},
				},
			})
		case "/v1/embeddings":
			t.Fatal("Probe must not call /v1/embeddings")
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := NewLiteLLMClientNoProbe(server.URL, "bge-m3-Q8_0.gguf", "", 1024)
	ok, reason := client.Probe(context.Background())
	if !ok {
		t.Fatalf("Probe returned unhealthy: %q", reason)
	}
	if reason != "" {
		t.Fatalf("Probe reason = %q, want empty", reason)
	}
}

func TestProbeSucceedsWhenEmbeddingsWouldBeTooSlow(t *testing.T) {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/models":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"object": "list",
				"data": []map[string]any{
					{"id": "bge-m3-Q8_0.gguf", "object": "model", "owned_by": "olla"},
				},
			})
		case "/v1/embeddings":
			time.Sleep(250 * time.Millisecond)
			encodeEmbeddingResponse(w, []float32{0.1, 0.2, 0.3})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := NewLiteLLMClientNoProbe(server.URL, "bge-m3-Q8_0.gguf", "", 1024)
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	ok, reason := client.Probe(ctx)
	if !ok {
		t.Fatalf("Probe returned unhealthy under short timeout: %q", reason)
	}
}

func TestProbeFailsWhenModelMissing(t *testing.T) {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"object": "list",
			"data": []map[string]any{
				{"id": "other-model", "object": "model", "owned_by": "olla"},
			},
		})
	}))
	defer server.Close()

	client := NewLiteLLMClientNoProbe(server.URL, "bge-m3-Q8_0.gguf", "", 1024)
	ok, reason := client.Probe(context.Background())
	if ok {
		t.Fatal("Probe succeeded with missing model; want unhealthy")
	}
	if !strings.Contains(reason, "not advertised") {
		t.Fatalf("Probe reason = %q, want missing-model detail", reason)
	}
}
