package instinctllm

import (
	"fmt"
	"os"
)

// NewClient constructs the appropriate LLMClient.
//
// Backend selection order:
//  1. cfg.Backend if non-empty (preferred — explicit, race-safe).
//  2. LLM_BACKEND environment variable (backward compat for the consolidator).
//  3. Defaults to "anthropic".
//
// Values:
//   - "anthropic": Anthropic Messages API — the proven path.
//   - "olla": Olla OpenAI-compatible API with dynamic model resolution.
//
// Any other value returns an error so mis-configuration is caught early.
func NewClient(cfg Config) (LLMClient, error) {
	backend := cfg.Backend
	if backend == "" {
		backend = os.Getenv("LLM_BACKEND")
	}
	switch backend {
	case "", "anthropic":
		return NewAnthropicClient(cfg)
	case "olla":
		return NewOllaClient(cfg)
	default:
		return nil, fmt.Errorf("llm: unknown backend %q (valid: anthropic, olla)", backend)
	}
}
