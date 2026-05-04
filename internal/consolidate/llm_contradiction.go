package consolidate

// LLM-powered contradiction detection using LiteLLM /v1/chat/completions.

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/petersimmons1972/engram/internal/llm"
)

const (
	// llmCallTimeout is the per-call deadline applied via context.WithTimeout.
	// 30s is generous for a simple YES/NO classification on a small model.
	llmCallTimeout = 30 * time.Second

	llmContradictionPrompt = "Do these two statements contradict each other? Answer only YES or NO.\n\nStatement A: %s\n\nStatement B: %s"
)

// ClassifyContradictionLLM asks a LiteLLM model whether contentA and contentB
// contradict each other. Returns true when the model responds with a string
// that starts with "yes" (case-insensitive). Returns (false, err) on any failure.
func ClassifyContradictionLLM(ctx context.Context, contentA, contentB, litellmURL, model string) (bool, error) {
	callCtx, cancel := context.WithTimeout(ctx, llmCallTimeout)
	defer cancel()
	prompt := fmt.Sprintf(llmContradictionPrompt, contentA, contentB)
	response, err := llm.Complete(callCtx, litellmURL, "", model, prompt)
	if err != nil {
		return false, fmt.Errorf("classifyContradictionLLM: %w", err)
	}
	return strings.HasPrefix(strings.ToLower(strings.TrimSpace(response)), "yes"), nil
}
