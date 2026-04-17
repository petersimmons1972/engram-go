package rag

import (
	"context"

	"github.com/petersimmons1972/engram/internal/types"
)

// Recaller is a minimal interface for recall — allows testability.
type Recaller interface {
	Recall(ctx context.Context, query string, topK int, detail string) ([]types.SearchResult, error)
}

// ClaudeCompleter is a minimal interface wrapping *claude.Client.
type ClaudeCompleter interface {
	Complete(ctx context.Context, system, prompt, executorModel, advisorModel string, advisorMaxUses, maxTokens int) (string, error)
}

// Asker performs retrieval-augmented question answering over the memory store.
type Asker struct {
	Engine Recaller
	Client ClaudeCompleter
	Budget ContextBudget
	TopK   int // default 10 if 0
}

// Ask recalls relevant memories, trims them to budget, and asks Claude to answer.
func (a Asker) Ask(ctx context.Context, question string) (*AskResult, error) {
	topK := a.TopK
	if topK <= 0 {
		topK = 10
	}

	results, err := a.Engine.Recall(ctx, question, topK, "full")
	if err != nil {
		return nil, err
	}

	if len(results) == 0 {
		return &AskResult{
			Answer:    "No relevant memories found.",
			Citations: []Citation{},
		}, nil
	}

	trimmed := a.Budget.Trim(results)

	// Count context tokens from trimmed chunks.
	contextTokens := 0
	for _, chunk := range trimmed {
		contextTokens += len(chunk.MatchedChunk) / 4
	}

	prompt := AssemblePrompt(question, trimmed)

	answer, err := a.Client.Complete(ctx, systemPrompt, prompt, "claude-sonnet-4-6", "", 0, 2048)
	if err != nil {
		return nil, err
	}

	return &AskResult{
		Answer:            answer,
		Citations:         BuildCitations(trimmed),
		ContextTokensUsed: contextTokens,
	}, nil
}
