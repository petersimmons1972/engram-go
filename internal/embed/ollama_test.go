package embed_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/petersimmons1972/engram/internal/embed"
	"github.com/stretchr/testify/require"
)

// fakeOllama returns an httptest.Server that serves minimal Ollama responses.
func fakeOllama(t *testing.T, tagsModels []string, embedDims int) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()

	mux.HandleFunc("/api/tags", func(w http.ResponseWriter, _ *http.Request) {
		type model struct {
			Name string `json:"name"`
		}
		type resp struct {
			Models []model `json:"models"`
		}
		models := make([]model, len(tagsModels))
		for i, name := range tagsModels {
			models[i] = model{Name: name}
		}
		json.NewEncoder(w).Encode(resp{Models: models})
	})

	mux.HandleFunc("/api/embed", func(w http.ResponseWriter, _ *http.Request) {
		vec := make([]float32, embedDims)
		for i := range vec {
			vec[i] = float32(i) / float32(embedDims)
		}
		type resp struct {
			Embeddings [][]float32 `json:"embeddings"`
		}
		json.NewEncoder(w).Encode(resp{Embeddings: [][]float32{vec}})
	})

	mux.HandleFunc("/api/pull", func(w http.ResponseWriter, _ *http.Request) {
		json.NewEncoder(w).Encode(map[string]string{"status": "success"})
	})

	return httptest.NewServer(mux)
}

// newTestClient wires NewOllamaClientWithTransport to a local httptest.Server,
// bypassing the production SSRF guard (which correctly blocks 127.0.0.1).
// Use only in tests.
func newTestClient(t *testing.T, srv *httptest.Server, model string) (*embed.OllamaClient, error) {
	t.Helper()
	return embed.NewOllamaClientWithTransport(context.Background(), srv.URL, model, srv.Client().Transport)
}

func TestNewOllamaClient_ModelPresent(t *testing.T) {
	srv := fakeOllama(t, []string{"nomic-embed-text:latest"}, 768)
	defer srv.Close()

	c, err := newTestClient(t, srv, "nomic-embed-text")
	require.NoError(t, err)
	require.Equal(t, "nomic-embed-text", c.Name())
}

func TestNewOllamaClient_ModelAbsent_TriggersPull(t *testing.T) {
	srv := fakeOllama(t, []string{}, 768) // no models — pull will be triggered
	defer srv.Close()

	_, err := newTestClient(t, srv, "nomic-embed-text")
	require.NoError(t, err)
}

func TestOllamaClient_Embed(t *testing.T) {
	srv := fakeOllama(t, []string{"nomic-embed-text:latest"}, 768)
	defer srv.Close()

	c, err := newTestClient(t, srv, "nomic-embed-text")
	require.NoError(t, err)

	vec, err := c.Embed(context.Background(), "hello world")
	require.NoError(t, err)
	require.Len(t, vec, 768)
	require.Equal(t, 768, c.Dimensions())
}
