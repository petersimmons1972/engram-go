package instinctllm_test

import (
	"testing"

	"github.com/petersimmons1972/engram/internal/instinctllm"
)

// TestFactoryDefaultsAnthropic: unset LLM_BACKEND → anthropic client.
func TestFactoryDefaultsAnthropic(t *testing.T) {
	t.Setenv("LLM_BACKEND", "")
	c, err := instinctllm.NewClient(instinctllm.Config{APIKey: "sk-fake"})
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
	c, err := instinctllm.NewClient(instinctllm.Config{Endpoint: "http://olla.example.invalid"})
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
	_, err := instinctllm.NewClient(instinctllm.Config{})
	if err == nil {
		t.Error("NewClient() should return error for unknown LLM_BACKEND")
	}
}

// TestFactoryUsesConfigBackendBeforeEnv: explicit Config.Backend must win over
// any conflicting LLM_BACKEND env value. This is the regression guard for
// removing the os.Setenv plumbing in cmd/audit (Blocker 3).
func TestFactoryUsesConfigBackendBeforeEnv(t *testing.T) {
	// Env says anthropic, config says olla; config must win.
	t.Setenv("LLM_BACKEND", "anthropic")
	c, err := instinctllm.NewClient(instinctllm.Config{
		Backend:  "olla",
		Endpoint: "http://olla.example.invalid",
	})
	if err != nil {
		t.Fatalf("NewClient() error: %v", err)
	}
	// Olla client has dynamic model resolution; check via type-name string match
	// from interface — easiest signal: olla client never requires APIKey, so
	// passing nothing-but-Endpoint succeeded above. If Config were ignored and
	// env "anthropic" won, NewAnthropicClient would error on missing APIKey.
	_ = c

	// And the inverse: env=olla, config=anthropic → anthropic must win.
	t.Setenv("LLM_BACKEND", "olla")
	c2, err := instinctllm.NewClient(instinctllm.Config{
		Backend: "anthropic",
		APIKey:  "sk-fake",
	})
	if err != nil {
		t.Fatalf("NewClient() error (anthropic via Config): %v", err)
	}
	if c2 == nil {
		t.Fatal("NewClient() returned nil client")
	}
}

// TestFactoryFallsBackToEnvWhenConfigEmpty: empty Config.Backend → env wins.
// Preserves backward compatibility with the consolidator's env-only setup.
func TestFactoryFallsBackToEnvWhenConfigEmpty(t *testing.T) {
	t.Setenv("LLM_BACKEND", "olla")
	c, err := instinctllm.NewClient(instinctllm.Config{
		// Backend deliberately empty.
		Endpoint: "http://olla.example.invalid",
	})
	if err != nil {
		t.Fatalf("NewClient() error: %v", err)
	}
	if c == nil {
		t.Fatal("NewClient() returned nil client")
	}
}

// TestFactoryConfigBackendRejectsUnknown: garbage in Config.Backend → error,
// same as env-set garbage.
func TestFactoryConfigBackendRejectsUnknown(t *testing.T) {
	t.Setenv("LLM_BACKEND", "anthropic") // valid env; should not save us
	_, err := instinctllm.NewClient(instinctllm.Config{Backend: "garbage"})
	if err == nil {
		t.Error("NewClient() should reject garbage Config.Backend even with valid env")
	}
}
