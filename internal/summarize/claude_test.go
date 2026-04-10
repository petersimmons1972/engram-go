package summarize_test

import (
	"context"
	"strings"
	"testing"

	"github.com/petersimmons1972/engram/internal/db"
	"github.com/petersimmons1972/engram/internal/summarize"
	"github.com/petersimmons1972/engram/internal/types"
	"github.com/stretchr/testify/require"
)

// stubCompleter records the last Complete call arguments.
type stubCompleter struct {
	lastSystem    string
	lastPrompt    string
	lastExecModel string
	lastAdvModel  string
	lastMaxUses   int
	returnText    string
	returnErr     error
}

func (s *stubCompleter) Complete(ctx context.Context, system, prompt, execModel, advModel string, advisorMaxUses, maxTokens int) (string, error) {
	s.lastSystem = system
	s.lastPrompt = prompt
	s.lastExecModel = execModel
	s.lastAdvModel = advModel
	s.lastMaxUses = advisorMaxUses
	return s.returnText, s.returnErr
}

func TestClaudeSummarize_CallsComplete(t *testing.T) {
	stub := &stubCompleter{returnText: "a summary"}
	result, err := summarize.ClaudeSummarize(context.Background(), "some content", stub)
	require.NoError(t, err)
	require.Equal(t, "a summary", result)
	require.Equal(t, "claude-sonnet-4-6", stub.lastExecModel)
	require.Equal(t, "claude-opus-4-6", stub.lastAdvModel)
	require.Equal(t, 2, stub.lastMaxUses)
}

func TestClaudeSummarize_TruncatesLongContent(t *testing.T) {
	// 3000-char content should be truncated to 2000 chars before being sent as prompt
	longContent := strings.Repeat("x", 3000)
	stub := &stubCompleter{returnText: "summary"}
	_, err := summarize.ClaudeSummarize(context.Background(), longContent, stub)
	require.NoError(t, err)
	// The prompt sent to Complete is the (possibly truncated) content directly
	require.LessOrEqual(t, len(stub.lastPrompt), 2000,
		"content portion of prompt must not exceed maxContent (2000)")
}

func TestClaudeSummarize_UsesSonnetAndOpus(t *testing.T) {
	// Verify that a Worker created via NewWorkerWithClaude routes through ClaudeSummarize.
	// Since runOnce returns early on nil backend, we test ClaudeSummarize directly
	// with a stub to confirm the correct models are wired.
	stub := &stubCompleter{returnText: "decision recorded"}
	result, err := summarize.ClaudeSummarize(context.Background(), "we decided to use postgres", stub)
	require.NoError(t, err)
	require.Equal(t, "decision recorded", result)
	require.Equal(t, "claude-sonnet-4-6", stub.lastExecModel,
		"executor model must be claude-sonnet-4-6")
	require.Equal(t, "claude-opus-4-6", stub.lastAdvModel,
		"advisor model must be claude-opus-4-6")
}

// fakeBackend satisfies db.Backend for the already-summarized test.
type fakeBackend struct {
	db.Backend
	mem *types.Memory
}

func (f *fakeBackend) GetMemory(_ context.Context, _ string) (*types.Memory, error) {
	return f.mem, nil
}

func (f *fakeBackend) StoreSummary(_ context.Context, _, _ string) error {
	return nil
}

func TestSummarizeOneWithClaude_AlreadySummarized(t *testing.T) {
	existingSummary := "already done"
	mem := &types.Memory{
		ID:      "test-id",
		Content: "some content",
		Summary: &existingSummary,
	}
	backend := &fakeBackend{mem: mem}
	stub := &stubCompleter{returnText: "should not be called"}

	err := summarize.SummarizeOneWithClaude(context.Background(), backend, "test-id", stub)
	require.NoError(t, err)
	// stub should not have been called because summary is already set
	require.Empty(t, stub.lastExecModel,
		"Complete must not be called when summary is already present")
}
