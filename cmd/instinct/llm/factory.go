package llm

import (
	"fmt"
	"os"
)

// NewClient constructs the appropriate LLMClient based on the LLM_BACKEND
// environment variable.
//
// Values:
//   - "anthropic" (or unset/empty): Anthropic Messages API — the proven path.
//   - "olla": Olla OpenAI-compatible API with dynamic model resolution.
//
// Any other value returns an error so mis-configuration is caught early.
func NewClient(cfg Config) (LLMClient, error) {
	backend := os.Getenv("LLM_BACKEND")
	switch backend {
	case "", "anthropic":
		return NewAnthropicClient(cfg)
	case "olla":
		return NewOllaClient(cfg)
	default:
		return nil, fmt.Errorf("llm: unknown LLM_BACKEND %q (valid: anthropic, olla)", backend)
	}
}
