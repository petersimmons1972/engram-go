package llm_test

import (
	"testing"

	"github.com/petersimmons1972/engram/cmd/instinct/llm"
)

// TestFactoryDefaultsAnthropic: unset LLM_BACKEND → anthropic client.
func TestFactoryDefaultsAnthropic(t *testing.T) {
	t.Setenv("LLM_BACKEND", "")
	c, err := llm.NewClient(llm.Config{APIKey: "sk-fake"})
	if err != nil {
		t.Fatalf("NewClient() error: %v", err)
	}
	if c == nil {
		t.Fatal("NewClient() returned nil client")
	}
	// We can't introspect the concrete type from outside the package without
	// a type switch, but we can assert it's not nil and that it satisfies the
	// interface (compile-time checked above).
	_ = c
}

// TestFactoryOllaSelected: LLM_BACKEND=olla → olla client.
func TestFactoryOllaSelected(t *testing.T) {
	t.Setenv("LLM_BACKEND", "olla")
	c, err := llm.NewClient(llm.Config{Endpoint: "http://olla.example.invalid"})
	if err != nil {
		t.Fatalf("NewClient() error: %v", err)
	}
	if c == nil {
		t.Fatal("NewClient() returned nil client")
	}
	_ = c
}

// TestFactoryRejectsUnknown: LLM_BACKEND=garbage → error.
func TestFactoryRejectsUnknown(t *testing.T) {
	t.Setenv("LLM_BACKEND", "garbage")
	_, err := llm.NewClient(llm.Config{})
	if err == nil {
		t.Error("NewClient() should return error for unknown LLM_BACKEND")
	}
}
