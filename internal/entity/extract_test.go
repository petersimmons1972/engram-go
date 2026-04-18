package entity_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/petersimmons1972/engram/internal/entity"
)

// stubCompleter implements entity.ClaudeCompleter for tests.
type stubCompleter struct {
	response string
	err      error
}

func (s *stubCompleter) Complete(ctx context.Context, system, prompt, executorModel, advisorModel string, advisorMaxUses, maxTokens int) (string, error) {
	return s.response, s.err
}

func TestClaudeExtractor_ParsesEntitiesAndRelations(t *testing.T) {
	stub := &stubCompleter{
		response: `{"entities":[{"name":"Kubernetes","aliases":["k8s"]},{"name":"Docker","aliases":[]}],"relations":[{"source_name":"Kubernetes","target_name":"Docker","rel_type":"depends_on","strength":0.9}]}`,
	}
	ext := entity.NewClaudeExtractor(stub)
	entities, relations, err := ext.Extract(context.Background(), "Kubernetes depends on Docker.")
	require.NoError(t, err)
	assert.Len(t, entities, 2)
	assert.Equal(t, "Kubernetes", entities[0].Name)
	assert.Equal(t, []string{"k8s"}, entities[0].Aliases)
	assert.Len(t, relations, 1)
	assert.Equal(t, "depends_on", relations[0].RelType)
	assert.InDelta(t, 0.9, relations[0].Strength, 0.001)
}

func TestClaudeExtractor_EmptyContent(t *testing.T) {
	stub := &stubCompleter{
		response: `{"entities":[],"relations":[]}`,
	}
	ext := entity.NewClaudeExtractor(stub)
	entities, relations, err := ext.Extract(context.Background(), "")
	require.NoError(t, err)
	assert.Empty(t, entities)
	assert.Empty(t, relations)
}

func TestClaudeExtractor_APIError(t *testing.T) {
	stub := &stubCompleter{err: errors.New("api unavailable")}
	ext := entity.NewClaudeExtractor(stub)
	_, _, err := ext.Extract(context.Background(), "some content")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "api unavailable")
}

func TestClaudeExtractor_BadJSON(t *testing.T) {
	stub := &stubCompleter{response: "not json at all"}
	ext := entity.NewClaudeExtractor(stub)
	_, _, err := ext.Extract(context.Background(), "some content")
	assert.Error(t, err)
}

func TestClaudeExtractor_TruncatesLongContent(t *testing.T) {
	// Build content longer than 4000 chars. Verify we don't error on huge input.
	bigContent := strings.Repeat("a", 8000)
	stub := &stubCompleter{
		response: `{"entities":[],"relations":[]}`,
	}
	ext := entity.NewClaudeExtractor(stub)
	_, _, err := ext.Extract(context.Background(), bigContent)
	require.NoError(t, err)
}

func TestClaudeExtractor_PartialJSONWrapped(t *testing.T) {
	// Claude sometimes wraps JSON in markdown code fences.
	stub := &stubCompleter{
		response: "```json\n{\"entities\":[{\"name\":\"OpenAI\",\"aliases\":[]}],\"relations\":[]}\n```",
	}
	ext := entity.NewClaudeExtractor(stub)
	entities, _, err := ext.Extract(context.Background(), "OpenAI released GPT-4.")
	require.NoError(t, err)
	assert.Len(t, entities, 1)
	assert.Equal(t, "OpenAI", entities[0].Name)
}
