package llmclient_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/petersimmons1972/engram/internal/llmclient"
)

// goldenAnthropicResponse returns a minimal Anthropic Messages API response
// wrapping the provided text as the assistant's content.
func goldenAnthropicResponse(text string) string {
	return fmt.Sprintf(`{"content":[{"type":"text","text":%q}],"usage":{"input_tokens":10,"output_tokens":50}}`, text)
}

func newAnthropicClient(t *testing.T, endpoint string) llmclient.LLMClient {
	t.Helper()
	cfg := llmclient.Config{
		APIKey:   "sk-ant-fake",
		Endpoint: endpoint,
		Timeout:  5 * time.Second,
	}
	c, err := llmclient.NewAnthropicClient(cfg)
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

// TestAnthropicEmptyContentReturnsError: server returns valid JSON but with an
// empty content array (e.g. content filter triggered, or upstream model
// returned nothing). The client must return a non-nil error — but must NOT
// wrap ErrBackendUnavailable, because an empty response is a model-behaviour
// condition, not transport unavailability. Updated per re-review Fix 2.
func TestAnthropicEmptyContentReturnsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// Valid JSON, but Content array is empty.
		fmt.Fprint(w, `{"content":[],"usage":{"input_tokens":10,"output_tokens":0}}`)
	}))
	defer srv.Close()

	c := newAnthropicClient(t, srv.URL+"/v1/messages")
	_, err := c.Complete(context.Background(), "system", "user")
	if err == nil {
		t.Fatal("Complete() should return error on empty content array")
	}
}

// TestAnthropicEmptyContentNotSentinel: empty content array must NOT wrap
// ErrBackendUnavailable — callers need to distinguish "backend down" from
// "model returned nothing". Re-review Fix 2.
func TestAnthropicEmptyContentNotSentinel(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"content":[],"usage":{"input_tokens":10,"output_tokens":0}}`)
	}))
	defer srv.Close()

	c := newAnthropicClient(t, srv.URL+"/v1/messages")
	_, err := c.Complete(context.Background(), "system", "user")
	if errors.Is(err, llmclient.ErrBackendUnavailable) {
		t.Errorf("Complete() err = %v, empty content must NOT be ErrBackendUnavailable", err)
	}
}

// TestAnthropicParseFailureIsNotSentinel: server returns malformed JSON.
// That is a parse error, NOT backend unavailability — must NOT match the
// sentinel (caller should surface, not skip-and-continue).
func TestAnthropicParseFailureIsNotSentinel(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `not valid json at all`)
	}))
	defer srv.Close()

	c := newAnthropicClient(t, srv.URL+"/v1/messages")
	_, err := c.Complete(context.Background(), "system", "user")
	if err == nil {
		t.Fatal("Complete() should return error on malformed JSON")
	}
	if errors.Is(err, llmclient.ErrBackendUnavailable) {
		t.Errorf("Complete() err = %v, parse failure must NOT be ErrBackendUnavailable", err)
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

// ---------------------------------------------------------------------------
// #772 — transport errors must wrap ErrBackendUnavailable
// ---------------------------------------------------------------------------

// TestAnthropicTransportError_WrapsBackendUnavailable: server is closed before
// the request lands (connection refused). The error must wrap ErrBackendUnavailable
// so errors.Is(err, ErrBackendUnavailable) returns true.
func TestAnthropicTransportError_WrapsBackendUnavailable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	// Close immediately so subsequent request gets connection refused.
	srv.Close()

	c := newAnthropicClient(t, srv.URL+"/v1/messages")
	_, err := c.Complete(context.Background(), "system", "user")
	if err == nil {
		t.Fatal("Complete() should return error when server is unavailable")
	}
	if !errors.Is(err, llmclient.ErrBackendUnavailable) {
		t.Errorf("transport error must wrap ErrBackendUnavailable; got: %v", err)
	}
}

// TestAnthropicNon200_WrapsBackendUnavailable: server returns 503; error must
// wrap ErrBackendUnavailable so callers can use errors.Is to skip gracefully.
func TestAnthropicNon200_WrapsBackendUnavailable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	c := newAnthropicClient(t, srv.URL+"/v1/messages")
	_, err := c.Complete(context.Background(), "system", "user")
	if err == nil {
		t.Fatal("Complete() should return error on 503")
	}
	if !errors.Is(err, llmclient.ErrBackendUnavailable) {
		t.Errorf("non-200 status must wrap ErrBackendUnavailable; got: %v", err)
	}
}

// TestAnthropicReadBodyError_WrapsBackendUnavailable: server closes the
// connection mid-response (hijack), so ReadAll fails. The error must wrap
// ErrBackendUnavailable.
func TestAnthropicReadBodyError_WrapsBackendUnavailable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Send a valid 200 header but then hijack and close so body read fails.
		hj, ok := w.(http.Hijacker)
		if !ok {
			t.Error("ResponseWriter does not implement Hijacker")
			return
		}
		conn, _, err := hj.Hijack()
		if err != nil {
			t.Errorf("Hijack: %v", err)
			return
		}
		// Write minimal HTTP/1.1 200 headers then close mid-body.
		_, _ = conn.Write([]byte("HTTP/1.1 200 OK\r\nContent-Type: application/json\r\nContent-Length: 100\r\n\r\n{"))
		conn.Close()
	}))
	defer srv.Close()

	c := newAnthropicClient(t, srv.URL+"/v1/messages")
	_, err := c.Complete(context.Background(), "system", "user")
	if err == nil {
		t.Fatal("Complete() should return error on truncated body")
	}
	if !errors.Is(err, llmclient.ErrBackendUnavailable) {
		t.Errorf("read-body failure must wrap ErrBackendUnavailable; got: %v", err)
	}
}
