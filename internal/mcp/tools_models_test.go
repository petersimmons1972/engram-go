package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	mcpgo "github.com/mark3labs/mcp-go/mcp"
	"github.com/petersimmons1972/engram/internal/embed"
)

// TestSuggestedModelsEnrichment verifies the installed-flag detection logic.
func TestSuggestedModelsEnrichment(t *testing.T) {
	installed := map[string]bool{
		"nomic-embed-text:latest": true,
	}
	for _, spec := range embed.SuggestedModels {
		isInstalled := installed[spec.Name] || installed[spec.Name+":latest"]
		if spec.Name == "nomic-embed-text" && !isInstalled {
			t.Errorf("nomic-embed-text should be detected as installed")
		}
		if spec.Name == "mxbai-embed-large" && isInstalled {
			t.Errorf("mxbai-embed-large should not be detected as installed")
		}
	}
}

func TestCosineSim32(t *testing.T) {
	a := []float32{1.0, 0.0, 0.0}
	b := []float32{0.9, 0.1, 0.0}
	c := []float32{0.0, 1.0, 0.0}
	query := []float32{1.0, 0.0, 0.0}

	simAQ := cosineSim32(query, a)
	simBQ := cosineSim32(query, b)
	simCQ := cosineSim32(query, c)

	if simAQ < simBQ {
		t.Errorf("a should be more similar to query than b: simA=%.4f simB=%.4f", simAQ, simBQ)
	}
	if simBQ < simCQ {
		t.Errorf("b should be more similar to query than c: simB=%.4f simC=%.4f", simBQ, simCQ)
	}
}

func TestCosineSim32ZeroMagnitude(t *testing.T) {
	zero := []float32{0.0, 0.0, 0.0}
	a := []float32{1.0, 0.0, 0.0}
	if got := cosineSim32(zero, a); got != 0 {
		t.Errorf("cosineSim32(zero, a) = %v, want 0", got)
	}
	if got := cosineSim32(a, zero); got != 0 {
		t.Errorf("cosineSim32(a, zero) = %v, want 0", got)
	}
}

func TestCosineSim32LengthMismatch(t *testing.T) {
	a := []float32{1.0, 0.0}
	b := []float32{1.0, 0.0, 0.0}
	if got := cosineSim32(a, b); got != 0 {
		t.Errorf("cosineSim32 length mismatch = %v, want 0", got)
	}
}

// --- fetchInstalledOllamaModels ---

func TestFetchInstalledOllamaModels_HappyPath(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"models":[{"name":"nomic-embed-text:latest"}]}`)
	}))
	defer srv.Close()

	names, err := fetchInstalledOllamaModels(context.Background(), srv.URL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !names["nomic-embed-text:latest"] {
		t.Error("expected nomic-embed-text:latest to be present")
	}
	if !names["nomic-embed-text"] {
		t.Error("expected bare nomic-embed-text to be present via :latest stripping")
	}
	if names["mxbai-embed-large"] {
		t.Error("mxbai-embed-large should not be present")
	}
}

func TestFetchInstalledOllamaModels_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "internal server error", http.StatusInternalServerError)
	}))
	defer srv.Close()

	_, err := fetchInstalledOllamaModels(context.Background(), srv.URL)
	if err == nil {
		t.Error("expected error from 500 response")
	}
}

func TestFetchInstalledOllamaModels_MalformedJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{not valid json}`)
	}))
	defer srv.Close()

	_, err := fetchInstalledOllamaModels(context.Background(), srv.URL)
	if err == nil {
		t.Error("expected error from malformed JSON")
	}
}

func TestFetchInstalledOllamaModels_Unreachable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	url := srv.URL
	srv.Close()

	_, err := fetchInstalledOllamaModels(context.Background(), url)
	if err == nil {
		t.Error("expected error from closed server")
	}
}

// --- handleMemoryModels ---

func parseModelsResult(t *testing.T, result *mcpgo.CallToolResult) map[string]any {
	t.Helper()
	text, ok := result.Content[0].(mcpgo.TextContent)
	if !ok {
		t.Fatalf("expected TextContent, got %T", result.Content[0])
	}
	var out map[string]any
	if err := json.Unmarshal([]byte(text.Text), &out); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	return out
}

func TestHandleMemoryModels_HappyPath(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"models":[{"name":"nomic-embed-text:latest"}]}`)
	}))
	defer srv.Close()

	cfg := Config{OllamaURL: srv.URL, EmbedModel: "nomic-embed-text"}
	result, err := handleMemoryModels(context.Background(), nil, mcpgo.CallToolRequest{}, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := parseModelsResult(t, result)
	if out["current"] != "nomic-embed-text" {
		t.Errorf("current = %v, want nomic-embed-text", out["current"])
	}
	suggested, ok := out["suggested"].([]any)
	if !ok || len(suggested) == 0 {
		t.Fatal("suggested list is empty or wrong type")
	}
	for _, s := range suggested {
		m := s.(map[string]any)
		switch m["name"] {
		case "nomic-embed-text":
			if m["installed"] != true {
				t.Error("nomic-embed-text should be marked installed")
			}
		case "mxbai-embed-large":
			if m["installed"] != false {
				t.Error("mxbai-embed-large should not be marked installed")
			}
		}
	}
}

func TestHandleMemoryModels_OllamaUnreachable_GracefulDegradation(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	url := srv.URL
	srv.Close()

	cfg := Config{OllamaURL: url, EmbedModel: "nomic-embed-text"}
	result, err := handleMemoryModels(context.Background(), nil, mcpgo.CallToolRequest{}, cfg)
	if err != nil {
		t.Fatalf("expected graceful degradation, got error: %v", err)
	}

	out := parseModelsResult(t, result)
	suggested, _ := out["suggested"].([]any)
	for _, s := range suggested {
		m := s.(map[string]any)
		if m["installed"] != false {
			t.Errorf("model %v should not be installed when Ollama unreachable", m["name"])
		}
	}
}

// ollamaMockServer returns an httptest.Server that handles the Ollama API
// endpoints needed by embed.NewOllamaClient: GET /api/tags (reports models
// as present to skip auto-pull) and POST /api/embed (returns a fixed 3-D vector).
func ollamaMockServer(models []string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/tags":
			type modelEntry struct {
				Name string `json:"name"`
			}
			entries := make([]modelEntry, 0, len(models))
			for _, m := range models {
				entries = append(entries, modelEntry{Name: m + ":latest"})
			}
			out := map[string]any{"models": entries}
			_ = json.NewEncoder(w).Encode(out)
		case "/api/embed":
			fmt.Fprint(w, `{"embeddings":[[0.1,0.2,0.3]]}`)
		default:
			http.Error(w, "not found", http.StatusNotFound)
		}
	}))
}

// --- handleMemoryEmbeddingEval ---

func TestHandleMemoryEmbeddingEval_SameModelsRejected(t *testing.T) {
	req := mcpgo.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"model_a": "nomic-embed-text",
		"model_b": "nomic-embed-text",
	}
	cfg := Config{OllamaURL: "http://127.0.0.1:0", EmbedModel: "nomic-embed-text"}
	_, err := handleMemoryEmbeddingEval(context.Background(), nil, req, cfg)
	if err == nil {
		t.Fatal("expected error when model_a == model_b")
	}
	if !strings.Contains(err.Error(), "must differ") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestHandleMemoryEmbeddingEval_ModelADefaultsFromConfig(t *testing.T) {
	// When model_a is not provided it should default to cfg.EmbedModel.
	// Providing model_b equal to cfg.EmbedModel triggers the must-differ guard,
	// proving the default was sourced from cfg rather than a hardcoded string.
	req := mcpgo.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"model_b": "mxbai-embed-large",
	}
	cfg := Config{OllamaURL: "http://127.0.0.1:0", EmbedModel: "mxbai-embed-large"}
	_, err := handleMemoryEmbeddingEval(context.Background(), nil, req, cfg)
	if err == nil {
		t.Fatal("expected must-differ error: model_a defaulted to cfg.EmbedModel should equal explicit model_b")
	}
	if !strings.Contains(err.Error(), "must differ") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestHandleMemoryEmbeddingEval_HappyPath(t *testing.T) {
	srv := ollamaMockServer([]string{"nomic-embed-text", "mxbai-embed-large"})
	defer srv.Close()

	req := mcpgo.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"model_a": "nomic-embed-text",
		"model_b": "mxbai-embed-large",
	}
	cfg := Config{OllamaURL: srv.URL, EmbedModel: "nomic-embed-text"}
	result, err := handleMemoryEmbeddingEval(context.Background(), nil, req, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	text, ok := result.Content[0].(mcpgo.TextContent)
	if !ok {
		t.Fatalf("expected TextContent, got %T", result.Content[0])
	}
	var out map[string]any
	if err := json.Unmarshal([]byte(text.Text), &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	for _, key := range []string{"model_a", "model_b", "recommendation", "probe_count"} {
		if _, ok := out[key]; !ok {
			t.Errorf("result missing field %q", key)
		}
	}
	if out["recommendation"] != "nomic-embed-text" && out["recommendation"] != "mxbai-embed-large" {
		t.Errorf("recommendation = %v, want one of the two model names", out["recommendation"])
	}
}

func TestHandleMemoryEmbeddingEval_OllamaUnreachable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	url := srv.URL
	srv.Close()

	req := mcpgo.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"model_a": "nomic-embed-text",
		"model_b": "mxbai-embed-large",
	}
	cfg := Config{OllamaURL: url, EmbedModel: "nomic-embed-text"}
	_, err := handleMemoryEmbeddingEval(context.Background(), nil, req, cfg)
	if err == nil {
		t.Error("expected error when Ollama is unreachable")
	}
}
