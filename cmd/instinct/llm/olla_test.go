package llm_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/petersimmons1972/engram/cmd/instinct/llm"
)

// ollaModelList builds a minimal Olla /olla/models response.
type ollaModelEntry struct {
	id       string
	family   string
	caps     []string
	states   []string // availability states
}

func buildOllaModelsResponse(models []ollaModelEntry) string {
	type availEntry struct {
		State string `json:"state"`
	}
	type ollaInfo struct {
		Family       string       `json:"family"`
		Capabilities []string     `json:"capabilities"`
		Availability []availEntry `json:"availability"`
	}
	type modelEntry struct {
		ID   string   `json:"id"`
		Olla ollaInfo `json:"olla"`
	}
	type response struct {
		Data []modelEntry `json:"data"`
	}

	var data []modelEntry
	for _, m := range models {
		var avail []availEntry
		for _, s := range m.states {
			avail = append(avail, availEntry{State: s})
		}
		data = append(data, modelEntry{
			ID: m.id,
			Olla: ollaInfo{
				Family:       m.family,
				Capabilities: m.caps,
				Availability: avail,
			},
		})
	}
	b, _ := json.Marshal(response{Data: data})
	return string(b)
}

func goldenOllaCompletionResponse(text string) string {
	return fmt.Sprintf(`{"choices":[{"message":{"role":"assistant","content":%q}}]}`, text)
}

// newOllaTestServer wires up an httptest server that serves the Olla model
// discovery endpoint and the chat completions endpoint.
// onCompletion is called with the decoded request body when /v1/chat/completions
// is hit; pass nil to ignore.
func newOllaTestServer(
	t *testing.T,
	modelsBody string,
	completionBody string,
	onCompletion func(body map[string]any),
) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/olla/models":
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, modelsBody)
		case "/v1/chat/completions":
			if onCompletion != nil {
				var body map[string]any
				if err := json.NewDecoder(r.Body).Decode(&body); err == nil {
					onCompletion(body)
				}
			}
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, completionBody)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
}

func newOllaClient(t *testing.T, endpoint string) llm.LLMClient {
	t.Helper()
	cfg := llm.Config{
		Endpoint: endpoint,
		Timeout:  5 * time.Second,
	}
	c, err := llm.NewOllaClient(cfg)
	if err != nil {
		t.Fatalf("NewOllaClient: %v", err)
	}
	return c
}

// TestOllaDynamicModelResolution: model list has one text-generation-capable
// available model and one qwen3 model; client picks the first, skips qwen3.
func TestOllaDynamicModelResolution(t *testing.T) {
	models := buildOllaModelsResponse([]ollaModelEntry{
		{id: "qwen3:7b", family: "qwen3", caps: []string{"text-generation"}, states: []string{"available"}},
		{id: "llama3.2:3b", family: "llama3", caps: []string{"text-generation"}, states: []string{"available"}},
	})
	var selectedModel string
	srv := newOllaTestServer(t, models, goldenOllaCompletionResponse("[]"), func(body map[string]any) {
		if m, ok := body["model"].(string); ok {
			selectedModel = m
		}
	})
	defer srv.Close()

	c := newOllaClient(t, srv.URL)
	_, err := c.Complete(context.Background(), "sys", "user")
	if err != nil {
		t.Fatalf("Complete() error: %v", err)
	}
	if selectedModel != "llama3.2:3b" {
		t.Errorf("selected model = %q, want llama3.2:3b (qwen3 should be skipped)", selectedModel)
	}
}

// TestOllaSkipsUnavailableModels: model with state != "available" is excluded.
func TestOllaSkipsUnavailableModels(t *testing.T) {
	models := buildOllaModelsResponse([]ollaModelEntry{
		{id: "busy-model:7b", family: "llama", caps: []string{"text-generation"}, states: []string{"loading"}},
		{id: "ready-model:7b", family: "mistral", caps: []string{"text-generation"}, states: []string{"available"}},
	})
	var selectedModel string
	srv := newOllaTestServer(t, models, goldenOllaCompletionResponse("[]"), func(body map[string]any) {
		if m, ok := body["model"].(string); ok {
			selectedModel = m
		}
	})
	defer srv.Close()

	c := newOllaClient(t, srv.URL)
	_, err := c.Complete(context.Background(), "sys", "user")
	if err != nil {
		t.Fatalf("Complete() error: %v", err)
	}
	if selectedModel != "ready-model:7b" {
		t.Errorf("selected model = %q, want ready-model:7b (unavailable model skipped)", selectedModel)
	}
}

// TestOllaOpenAICompatRequestShape: assert POST body matches {model, messages, temperature}.
func TestOllaOpenAICompatRequestShape(t *testing.T) {
	models := buildOllaModelsResponse([]ollaModelEntry{
		{id: "llama3.2:3b", family: "llama3", caps: []string{"text-generation"}, states: []string{"available"}},
	})
	var capturedBody map[string]any
	srv := newOllaTestServer(t, models, goldenOllaCompletionResponse("[]"), func(body map[string]any) {
		capturedBody = body
	})
	defer srv.Close()

	c := newOllaClient(t, srv.URL)
	if _, err := c.Complete(context.Background(), "system msg", "user msg"); err != nil {
		t.Fatalf("Complete() error: %v", err)
	}

	// Must have model
	if _, ok := capturedBody["model"]; !ok {
		t.Error("request body missing 'model' field")
	}
	// Must have messages array
	msgs, ok := capturedBody["messages"].([]any)
	if !ok || len(msgs) != 2 {
		t.Errorf("messages: got %v, want 2-element array", capturedBody["messages"])
	} else {
		roles := []string{}
		for _, m := range msgs {
			if msg, ok := m.(map[string]any); ok {
				if r, ok := msg["role"].(string); ok {
					roles = append(roles, r)
				}
			}
		}
		if len(roles) != 2 || roles[0] != "system" || roles[1] != "user" {
			t.Errorf("message roles = %v, want [system user]", roles)
		}
	}
	// Must have temperature
	if _, ok := capturedBody["temperature"]; !ok {
		t.Error("request body missing 'temperature' field")
	}
}

// TestOllaNoModelAvailable: model list returns no suitable model;
// Complete returns empty string + nil error (fail-quiet, matches Python).
func TestOllaNoModelAvailable(t *testing.T) {
	models := buildOllaModelsResponse([]ollaModelEntry{
		{id: "image-model:7b", family: "clip", caps: []string{"image-generation"}, states: []string{"available"}},
	})
	srv := newOllaTestServer(t, models, "", nil)
	defer srv.Close()

	c := newOllaClient(t, srv.URL)
	got, err := c.Complete(context.Background(), "sys", "user")
	if err != nil {
		t.Errorf("Complete() returned error on no-model-available, want nil: %v", err)
	}
	if got != "" {
		t.Errorf("Complete() = %q, want empty string on no-model-available", got)
	}
}

// TestOllaStripsMarkdownFences: server returns response wrapped in triple-backtick
// fences; client strips them before returning.
func TestOllaStripsMarkdownFences(t *testing.T) {
	fenced := "```json\n[{\"type\":\"workflow\"}]\n```"
	models := buildOllaModelsResponse([]ollaModelEntry{
		{id: "llama3.2:3b", family: "llama3", caps: []string{"text-generation"}, states: []string{"available"}},
	})
	srv := newOllaTestServer(t, models, goldenOllaCompletionResponse(fenced), nil)
	defer srv.Close()

	c := newOllaClient(t, srv.URL)
	got, err := c.Complete(context.Background(), "sys", "user")
	if err != nil {
		t.Fatalf("Complete() error: %v", err)
	}
	if strings.Contains(got, "```") {
		t.Errorf("Complete() result still contains markdown fences: %q", got)
	}
}

// TestOllaModelDiscoveryFailure: model list endpoint returns 500;
// Complete returns empty string + nil error (fail-quiet).
func TestOllaModelDiscoveryFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	c := newOllaClient(t, srv.URL)
	got, err := c.Complete(context.Background(), "sys", "user")
	if err != nil {
		t.Errorf("Complete() returned error on discovery failure, want nil: %v", err)
	}
	if got != "" {
		t.Errorf("Complete() = %q, want empty string on discovery failure", got)
	}
}
