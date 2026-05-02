package summarize_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/petersimmons1972/engram/internal/db"
	"github.com/petersimmons1972/engram/internal/summarize"
	"github.com/petersimmons1972/engram/internal/types"
	"github.com/stretchr/testify/require"
)

// stubBackend is a minimal db.Backend for testing SummarizeOne.
// Only GetMemory and StoreSummary are wired; all other methods panic.
type stubBackend struct {
	db.Backend // embed to satisfy the interface without implementing every method
	mem        *types.Memory
	stored     string
}

func (s *stubBackend) GetMemory(_ context.Context, _ string) (*types.Memory, error) {
	return s.mem, nil
}

func (s *stubBackend) StoreSummary(_ context.Context, _, summary string) error {
	s.stored = summary
	return nil
}

// openAIResponse returns the /v1/chat/completions format that llm.Complete expects.
// Tests that stub the LiteLLM endpoint must use this format.
func openAIResponse(content string) map[string]any {
	return map[string]any{
		"choices": []map[string]any{
			{"message": map[string]string{"content": content}},
		},
	}
}

func TestSummarizeContent(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(openAIResponse("a short summary"))
	}))
	defer srv.Close()

	summary, err := summarize.SummarizeContent(context.Background(), "long content here", srv.URL, "llama3.2")
	require.NoError(t, err)
	require.Equal(t, "a short summary", summary)
}

func TestWorker_StartsAndStops(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(openAIResponse("summary"))
	}))
	defer srv.Close()

	w := summarize.NewWorker(nil, "proj", srv.URL, "llama3.2", true)
	w.Start()
	time.Sleep(50 * time.Millisecond)
	w.Stop()
}

func TestSummarizeContent_404_ReturnsErrModelNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"error":"model not found"}`))
	}))
	defer srv.Close()

	_, err := summarize.SummarizeContent(context.Background(), "content", srv.URL, "ghost-model")
	require.Error(t, err)
	require.ErrorIs(t, err, summarize.ErrModelNotFound, "expected ErrModelNotFound for HTTP 404")
}

// TestSummarizeContent_500_ModelNotFoundInBody covers the LiteLLM path where the
// upstream Ollama reports "model not found" but LiteLLM returns HTTP 500 instead
// of 404. The backoff in runOnce only fires on ErrModelNotFound, so this case
// must be detected and wrapped correctly — otherwise the worker spams WARN logs
// every 30s indefinitely with no backoff.
func TestSummarizeContent_500_ModelNotFoundInBody_ReturnsErrModelNotFound(t *testing.T) {
	// Reproduce the exact LiteLLM error body observed in production.
	litellmBody := `{"error":{"message":"litellm.APIConnectionError: OllamaException - {\"error\":\"model 'llama3.2:3b' not found\"}. Received Model Group=llama3.2:3b\nAvailable Model Group Fallbacks=None","type":null,"param":null,"code":"500"}}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(litellmBody))
	}))
	defer srv.Close()

	_, err := summarize.SummarizeContent(context.Background(), "content", srv.URL, "llama3.2:3b")
	require.Error(t, err)
	require.ErrorIs(t, err, summarize.ErrModelNotFound,
		"HTTP 500 with 'not found' in body must wrap ErrModelNotFound so the 10-minute backoff fires")
}

// TestSummarizeOne_VerbatimCopy verifies that SummarizeOne re-summarizes a memory
// whose summary was set to the verbatim content (the Ollama-down fallback path).
func TestSummarizeOne_VerbatimCopy(t *testing.T) {
	content := "We decided to store memories by project to allow namespace isolation."
	verbatimSummary := content // summary = content — the bug state

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(openAIResponse("short real summary"))
	}))
	defer srv.Close()

	backend := &stubBackend{
		mem: &types.Memory{ID: "abc", Content: content, Summary: &verbatimSummary},
	}

	err := summarize.SummarizeOne(context.Background(), backend, "abc", srv.URL, "llama3.2")
	require.NoError(t, err)
	require.Equal(t, "short real summary", backend.stored,
		"SummarizeOne must re-summarize when summary == content, not skip it")
}

// TestSummarizeOne_AlreadySummarized verifies that SummarizeOne skips memories
// that already have a real (non-verbatim) summary.
func TestSummarizeOne_AlreadySummarized(t *testing.T) {
	content := "We decided to store memories by project to allow namespace isolation."
	realSummary := "Project-scoped memory isolation decision."

	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		json.NewEncoder(w).Encode(map[string]string{"response": "should not be called"})
	}))
	defer srv.Close()

	backend := &stubBackend{
		mem: &types.Memory{ID: "abc", Content: content, Summary: &realSummary},
	}

	err := summarize.SummarizeOne(context.Background(), backend, "abc", srv.URL, "llama3.2")
	require.NoError(t, err)
	require.False(t, called, "Ollama must not be called when memory has a real summary")
	require.Empty(t, backend.stored, "StoreSummary must not be called when memory has a real summary")
}
