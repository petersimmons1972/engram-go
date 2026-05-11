package llm

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestComplete_SuccessAndAuthHeader(t *testing.T) {
	var auth string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth = r.Header.Get("Authorization")
		if r.URL.Path != "/v1/chat/completions" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"  hello world  "}}]}`))
	}))
	defer ts.Close()

	got, err := Complete(context.Background(), ts.URL+"/", "secret", "model", "prompt")
	if err != nil {
		t.Fatalf("Complete returned error: %v", err)
	}
	if got != "hello world" {
		t.Fatalf("got %q, want trimmed response", got)
	}
	if auth != "Bearer secret" {
		t.Fatalf("unexpected auth header: %q", auth)
	}
}

func TestComplete_Non200SurfacesBody(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "upstream exploded", http.StatusBadGateway)
	}))
	defer ts.Close()

	_, err := Complete(context.Background(), ts.URL, "", "model", "prompt")
	if err == nil || !strings.Contains(err.Error(), "upstream exploded") {
		t.Fatalf("expected body in error, got %v", err)
	}
}

func TestComplete_MalformedJSON(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"choices":[`))
	}))
	defer ts.Close()

	_, err := Complete(context.Background(), ts.URL, "", "model", "prompt")
	if err == nil || !strings.Contains(err.Error(), "decode") {
		t.Fatalf("expected decode error, got %v", err)
	}
}

func TestComplete_EmptyChoices(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"choices":[]}`))
	}))
	defer ts.Close()

	_, err := Complete(context.Background(), ts.URL, "", "model", "prompt")
	if err == nil || !strings.Contains(err.Error(), "empty response") {
		t.Fatalf("expected empty response error, got %v", err)
	}
}
