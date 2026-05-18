package llm_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/petersimmons1972/engram/cmd/instinct/llm"
)

// goldenAnthropicResponse returns a minimal Anthropic Messages API response
// wrapping the provided text as the assistant's content.
func goldenAnthropicResponse(text string) string {
	return fmt.Sprintf(`{"content":[{"type":"text","text":%q}],"usage":{"input_tokens":10,"output_tokens":50}}`, text)
}

func newAnthropicClient(t *testing.T, endpoint string) llm.LLMClient {
	t.Helper()
	cfg := llm.Config{
		APIKey:   "sk-ant-fake",
		Endpoint: endpoint,
		Timeout:  5 * time.Second,
	}
	c, err := llm.NewAnthropicClient(cfg)
	if err != nil {
		t.Fatalf("NewAnthropicClient: %v", err)
	}
	return c
}

// TestAnthropicHappyPath: server returns a golden Anthropic response;
// client returns expected string without error.
func TestAnthropicHappyPath(t *testing.T) {
	const want = `[{"type":"workflow","description":"test","domain":"git","evidence":"e","tag_signature":"sig-t"}]`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, goldenAnthropicResponse(want))
	}))
	defer srv.Close()

	c := newAnthropicClient(t, srv.URL+"/v1/messages")
	got, err := c.Complete(context.Background(), "system prompt", "user prompt")
	if err != nil {
		t.Fatalf("Complete() error: %v", err)
	}
	if got != want {
		t.Errorf("Complete() = %q, want %q", got, want)
	}
}

// TestAnthropicErrorPropagation: server returns 500; client returns error,
// does not panic.
func TestAnthropicErrorPropagation(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	c := newAnthropicClient(t, srv.URL+"/v1/messages")
	_, err := c.Complete(context.Background(), "system", "user")
	if err == nil {
		t.Error("Complete() should return error on 500")
	}
}

// TestAnthropicContextCancellation: context cancelled before request completes;
// client returns ctx.Err().
func TestAnthropicContextCancellation(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Block until client cancels.
		<-r.Context().Done()
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	c := newAnthropicClient(t, srv.URL+"/v1/messages")

	done := make(chan error, 1)
	go func() {
		_, err := c.Complete(ctx, "system", "user")
		done <- err
	}()
	cancel()

	err := <-done
	if err == nil {
		t.Error("Complete() should return error when context is cancelled")
	}
}

// TestAnthropicEmptyPrompts: empty system or user prompt — still sends the
// request, does not short-circuit (domain logic lives in caller, not client).
func TestAnthropicEmptyPrompts(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, goldenAnthropicResponse("[]"))
	}))
	defer srv.Close()

	c := newAnthropicClient(t, srv.URL+"/v1/messages")

	// Empty system prompt
	got, err := c.Complete(context.Background(), "", "user msg")
	if err != nil {
		t.Errorf("empty system prompt: unexpected error: %v", err)
	}
	if got != "[]" {
		t.Errorf("empty system prompt: got %q, want []", got)
	}

	// Empty user prompt
	got, err = c.Complete(context.Background(), "system msg", "")
	if err != nil {
		t.Errorf("empty user prompt: unexpected error: %v", err)
	}
	if got != "[]" {
		t.Errorf("empty user prompt: got %q, want []", got)
	}
}
