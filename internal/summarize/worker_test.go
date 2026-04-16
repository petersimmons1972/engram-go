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

func TestSummarizeContent(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]string{"response": "a short summary"})
	}))
	defer srv.Close()

	summary, err := summarize.SummarizeContent(context.Background(), "long content here", srv.URL, "llama3.2")
	require.NoError(t, err)
	require.Equal(t, "a short summary", summary)
}

func TestWorker_StartsAndStops(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]string{"response": "summary"})
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

// TestSummarizeOne_VerbatimCopy verifies that SummarizeOne re-summarizes a memory
// whose summary was set to the verbatim content (the Ollama-down fallback path).
func TestSummarizeOne_VerbatimCopy(t *testing.T) {
	content := "We decided to store memories by project to allow namespace isolation."
	verbatimSummary := content // summary = content — the bug state

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]string{"response": "short real summary"})
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
