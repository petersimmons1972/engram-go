package runner_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/petersimmons1972/engram/internal/manifest"
	"github.com/petersimmons1972/engram/internal/ollama"
	"github.com/petersimmons1972/engram/internal/runner"
)

func mockOllama(t *testing.T, responseContent string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/tags":
			json.NewEncoder(w).Encode(map[string]any{
				"models": []map[string]any{
					{"name": "mistral:7b", "digest": "sha256:test"},
				},
			})
		case "/api/version":
			json.NewEncoder(w).Encode(map[string]string{"version": "0.3.0"})
		case "/api/chat":
			json.NewEncoder(w).Encode(map[string]any{
				"model": "mistral:7b",
				"message": map[string]string{
					"role":    "assistant",
					"content": responseContent,
				},
				"done": true,
			})
		case "/api/generate":
			json.NewEncoder(w).Encode(map[string]any{"done": true})
		}
	}))
}

func TestRun_ProducesRunResult(t *testing.T) {
	content := `{"patterns":[{"type":"correction","description":"Use xh","domain":"bash","evidence":"curl used twice","tag_signature":"sig-curl-xh","confidence":0.9}]}`
	srv := mockOllama(t, content)
	defer srv.Close()

	model := manifest.Model{
		Name:   "mistral:7b",
		VRAMGB: 4.5,
		Tier:   "4-6GB",
		Vendor: "Mistral AI",
		Family: "mistral",
	}
	client := ollama.NewClient(srv.URL)
	result, err := runner.Run(context.Background(), client, model, "testdata/sample.jsonl", 1)
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	if len(result.Runs) != 1 {
		t.Errorf("want 1 run, got %d", len(result.Runs))
	}
	if result.Runs[0].RawContent != content {
		t.Errorf("unexpected content: %q", result.Runs[0].RawContent)
	}
}

func TestRun_TimedOut(t *testing.T) {
	chatStarted := make(chan struct{}, 1)
	srvDone := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/tags":
			json.NewEncoder(w).Encode(map[string]any{
				"models": []map[string]any{{"name": "mistral:7b", "digest": "sha256:test"}},
			})
		case "/api/chat":
			select {
			case chatStarted <- struct{}{}:
			default:
			}
			// Block until either the server is shutting down or a generous timeout
			// fires — whichever comes first. This keeps the connection open long
			// enough that the client's 2s context expires, but ensures the handler
			// goroutine always exits so srv.Close() does not block.
			select {
			case <-srvDone:
			case <-time.After(10 * time.Second):
			}
		case "/api/generate": // evict
			json.NewEncoder(w).Encode(map[string]any{"done": true})
		}
	}))
	defer func() {
		close(srvDone)
		srv.Close()
	}()

	model := manifest.Model{Name: "mistral:7b", VRAMGB: 4.5, Tier: "4-6GB", Vendor: "Mistral AI", Family: "mistral"}
	client := ollama.NewClient(srv.URL)

	// Use a context with a very short timeout so the chat times out quickly.
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// Run() uses 300s per-call timeout, but ctx cancels in 2s.
	// The parent ctx deadline fires first, making runCtx.Err() non-nil.
	result, _ := runner.Run(ctx, client, model, "testdata/sample.jsonl", 1)

	// Chat handler must have been reached for the timeout path to be exercised.
	select {
	case <-chatStarted:
	case <-time.After(5 * time.Second):
		t.Fatal("chat handler was never reached — timeout path not exercised")
	}

	if len(result.Runs) != 1 {
		t.Fatalf("want 1 run, got %d", len(result.Runs))
	}
	if !result.Runs[0].TimedOut {
		t.Errorf("want TimedOut=true, got Error=%q", result.Runs[0].Error)
	}
}
