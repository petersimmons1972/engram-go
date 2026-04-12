package summarize_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/petersimmons1972/engram/internal/summarize"
	"github.com/stretchr/testify/require"
)

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
