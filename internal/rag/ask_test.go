package rag_test

import (
	"context"
	"testing"
	"time"

	"github.com/petersimmons1972/engram/internal/rag"
	"github.com/petersimmons1972/engram/internal/types"
	"github.com/stretchr/testify/require"
)

// stubRecaller implements rag.Recaller. It returns a fixed list of results.
type stubRecaller struct {
	results []types.SearchResult
	err     error
}

func (s *stubRecaller) Recall(_ context.Context, _ string, _ int, _ string) ([]types.SearchResult, error) {
	return s.results, s.err
}

// stubCompleter implements rag.ClaudeCompleter. It returns a fixed string and
// records the model argument passed to Complete.
type stubCompleter struct {
	response      string
	err           error
	capturedModel string
}

func (s *stubCompleter) Complete(_ context.Context, _, _, executorModel, _ string, _, _ int) (string, error) {
	s.capturedModel = executorModel
	return s.response, s.err
}

// TestAsker_ReturnsAnswerAndCitations verifies the happy path: two search
// results produce an answer from the completer and two citations.
func TestAsker_ReturnsAnswerAndCitations(t *testing.T) {
	ts := time.Date(2024, time.January, 1, 0, 0, 0, 0, time.UTC)
	results := []types.SearchResult{
		makeResult("mem-001", "First excerpt.", 0.9, ts),
		makeResult("mem-002", "Second excerpt.", 0.8, ts),
	}

	asker := rag.Asker{
		Engine: &stubRecaller{results: results},
		Client: &stubCompleter{response: "The answer is X"},
		Budget: rag.ContextBudget{MaxTokens: 1000},
	}

	got, err := asker.Ask(context.Background(), "What is X?")

	require.NoError(t, err)
	require.NotNil(t, got)
	require.Equal(t, "The answer is X", got.Answer)
	require.Len(t, got.Citations, 2, "one Citation per search result")
}

// TestAsker_EmptyRecall_GracefulAnswer verifies that when Recaller returns no
// results, Ask returns a graceful "no memories" answer rather than an error.
func TestAsker_EmptyRecall_GracefulAnswer(t *testing.T) {
	asker := rag.Asker{
		Engine: &stubRecaller{results: []types.SearchResult{}},
		Client: &stubCompleter{response: "should not be used"},
		Budget: rag.ContextBudget{MaxTokens: 1000},
	}

	got, err := asker.Ask(context.Background(), "Anything?")

	require.NoError(t, err)
	require.NotNil(t, got)
	require.Equal(t, "No relevant memories found.", got.Answer)
	require.Len(t, got.Citations, 0)
}

// TestAsker_ContextTokensUsed verifies that ContextTokensUsed is set to
// len(MatchedChunk) / 4 for a single result (the standard token estimate).
func TestAsker_ContextTokensUsed(t *testing.T) {
	// "helloworld!!" = 12 chars => 12/4 = 3 tokens
	chunk := "helloworld!!"
	result := types.SearchResult{
		Memory:       &types.Memory{ID: "mem-tok", Content: chunk},
		Score:        0.9,
		MatchedChunk: chunk,
	}

	asker := rag.Asker{
		Engine: &stubRecaller{results: []types.SearchResult{result}},
		Client: &stubCompleter{response: "token answer"},
		Budget: rag.ContextBudget{MaxTokens: 1000},
	}

	got, err := asker.Ask(context.Background(), "How many tokens?")

	require.NoError(t, err)
	require.NotNil(t, got)
	expected := len(chunk) / 4
	require.Equal(t, expected, got.ContextTokensUsed,
		"ContextTokensUsed must equal len(MatchedChunk)/4")
}

// TestAsker_UsesConfiguredModel verifies that Asker.Model is forwarded to the
// completer. When Model is empty the default "claude-sonnet-4-6" is used.
func TestAsker_UsesConfiguredModel(t *testing.T) {
	chunk := types.SearchResult{
		Memory:       &types.Memory{ID: "mem-m", Content: "model test"},
		Score:        0.9,
		MatchedChunk: "model test",
	}
	completer := &stubCompleter{response: "ok"}
	asker := rag.Asker{
		Engine: &stubRecaller{results: []types.SearchResult{chunk}},
		Client: completer,
		Budget: rag.ContextBudget{MaxTokens: 100},
		Model:  "custom-model-id",
	}

	_, err := asker.Ask(context.Background(), "model?")

	require.NoError(t, err)
	require.Equal(t, "custom-model-id", completer.capturedModel,
		"Asker.Model must be forwarded to the completer")
}

// TestAsker_DefaultModel verifies that when Model is empty, the default model
// is used rather than an empty string.
func TestAsker_DefaultModel(t *testing.T) {
	chunk := types.SearchResult{
		Memory:       &types.Memory{ID: "mem-d", Content: "default model test"},
		Score:        0.9,
		MatchedChunk: "default model test",
	}
	completer := &stubCompleter{response: "ok"}
	asker := rag.Asker{
		Engine: &stubRecaller{results: []types.SearchResult{chunk}},
		Client: completer,
		Budget: rag.ContextBudget{MaxTokens: 100},
		// Model intentionally left empty
	}

	_, err := asker.Ask(context.Background(), "default?")

	require.NoError(t, err)
	require.NotEmpty(t, completer.capturedModel,
		"default model must not be empty string")
}

// These assertions confirm that our stubs satisfy the interfaces Asker requires.
// They will compile only once those interfaces are defined in the rag package.
var _ rag.Recaller = (*stubRecaller)(nil)
var _ rag.ClaudeCompleter = (*stubCompleter)(nil)

