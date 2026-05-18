// Package instinctllm provides a generic LLM completion interface and concrete
// backends for Anthropic and Olla, used by the instinct consolidator and audit
// binaries. It is named distinctly from internal/llm (the OpenAI-compatible
// chat client used by summarize/consolidate) to avoid identifier collision when
// both packages are imported together.
//
// # Design rationale
//
// The interface is intentionally generic — Complete(ctx, system, user) string —
// rather than domain-specific (e.g. Detect(events) patterns). This was an
// explicit founder ruling (captured in the instinct migration campaign log):
//
//   - A domain-specific interface couples the transport layer to one use-case and
//     requires churn when new use-cases emerge (judge, audit, summarise, …).
//   - A generic completion interface lets the caller own prompt construction and
//     response parsing, keeping the client a thin HTTP wrapper.
//   - Both Anthropic and Olla implement this same interface, enabling a factory
//     that switches backends with zero changes to callers.
//
// Domain logic (prompt text, JSON parsing, pattern validation) lives in
// cmd/instinct/consolidator, not here. This package was moved from
// cmd/instinct/llm to internal/instinctllm to fix a cross-cmd import from
// cmd/audit.
package instinctllm

import (
	"context"
	"time"
)

// LLMClient is a generic text completion client.
// Callers provide a system prompt and a user prompt; the client returns the
// model's raw response string.  Domain logic (prompt construction, output
// parsing, retry) belongs in the caller, not here.
type LLMClient interface {
	// Complete sends system+user prompts to the underlying LLM and returns
	// the model's response text.  ctx is mandatory: it controls cancellation
	// and, for the Anthropic backend, propagates prompt-caching headers.
	// Returns an error on HTTP failure; callers should treat error as a
	// signal to skip LLM-dependent work, not crash.
	Complete(ctx context.Context, systemPrompt, userPrompt string) (string, error)
}

// Config holds endpoint and authentication details for an LLM backend.
// Fields are intentionally flat (no sub-structs) so the factory can build
// either backend from the same Config without branching.
type Config struct {
	// Endpoint is the full URL of the completion endpoint.
	// Anthropic: https://api.anthropic.com/v1/messages (or override for tests).
	// Olla: the base host, e.g. https://olla.example.invalid (client appends paths).
	Endpoint string

	// APIKey is the API key or bearer token for authentication.
	// Olla does not currently require a key; pass "" for Olla.
	APIKey string

	// Model is the model identifier.
	// Anthropic: e.g. "claude-haiku-4-5-20251001".
	// Olla: dynamically resolved; this field is ignored if set.
	Model string

	// Timeout caps each individual LLM call.  Zero means no timeout beyond
	// what the caller's context imposes.
	Timeout time.Duration

	// Backend selects which concrete client to construct: "anthropic" or "olla".
	// When empty, the factory falls back to the LLM_BACKEND environment variable
	// (default "anthropic"). Setting Backend explicitly is preferred — it keeps
	// configuration in the call chain and is race-safe under parallel tests.
	Backend string
}
