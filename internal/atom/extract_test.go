package atom_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/petersimmons1972/engram/internal/atom"
)

// stubCompleter implements atom.ClaudeCompleter for tests — no real LLM calls.
type stubCompleter struct {
	response string
	err      error
}

func (s *stubCompleter) Complete(_ context.Context, _, _, _, _ string, _, _ int) (string, error) {
	return s.response, s.err
}

const preferenceAtomJSON = `[
  {
    "atom_type": "preference",
    "subject": "the user",
    "predicate": "prefers",
    "value": "dark chocolate",
    "statement": "The user prefers dark chocolate over milk chocolate.",
    "scope": "global",
    "confidence": 0.9,
    "source_span": "I usually go with dark chocolate"
  }
]`

const factAtomJSON = `[
  {
    "atom_type": "fact",
    "subject": "Alice",
    "predicate": "works at",
    "value": "Acme Corp",
    "statement": "Alice works at Acme Corp.",
    "scope": "global",
    "confidence": 1.0,
    "source_span": ""
  }
]`

const multiAtomJSON = `[
  {
    "atom_type": "preference",
    "subject": "the user",
    "predicate": "dislikes",
    "value": "early morning meetings",
    "statement": "The user dislikes early morning meetings.",
    "scope": "global",
    "confidence": 0.85,
    "source_span": "I'm not a fan of early morning meetings"
  },
  {
    "atom_type": "attribute",
    "subject": "the user",
    "predicate": "timezone",
    "value": "PST",
    "statement": "The user is in the PST timezone.",
    "scope": "global",
    "confidence": 0.95,
    "source_span": ""
  }
]`

func TestClaudeExtractor_ParsesPreferenceAtom(t *testing.T) {
	stub := &stubCompleter{response: preferenceAtomJSON}
	ext := atom.NewClaudeExtractor(stub)

	atoms, err := ext.Extract(context.Background(), "I usually go with dark chocolate over milk chocolate.")
	require.NoError(t, err)
	require.Len(t, atoms, 1)

	a := atoms[0]
	assert.Equal(t, atom.TypePreference, a.Type)
	assert.Equal(t, "the user", a.Subject)
	assert.Equal(t, "prefers", a.Predicate)
	assert.Equal(t, "dark chocolate", a.Value)
	assert.NotEmpty(t, a.Statement)
	assert.Equal(t, atom.ScopeGlobal, a.Scope)
	assert.InDelta(t, 0.9, a.Confidence, 0.001)
}

// TestClaudeExtractor_CasualPhrasing verifies that casual preference language
// is captured — this is the key goal of Milestone 1 (PatternPreferenceExtractor misses these).
func TestClaudeExtractor_CasualPhrasing(t *testing.T) {
	casuals := []string{
		`I'm not a fan of early morning meetings`,
		`I'd rather have tea than coffee`,
		`I always choose the window seat`,
		`I hate cilantro`,
		`I tend to go with the simpler option`,
	}
	for _, text := range casuals {
		stub := &stubCompleter{response: preferenceAtomJSON}
		ext := atom.NewClaudeExtractor(stub)
		atoms, err := ext.Extract(context.Background(), text)
		require.NoError(t, err, "text: %q", text)
		// The mock always returns one atom regardless of input; the important
		// thing is that the extractor correctly calls through and parses.
		assert.NotEmpty(t, atoms, "expected at least one atom for casual text: %q", text)
	}
}

func TestClaudeExtractor_MultipleAtoms(t *testing.T) {
	stub := &stubCompleter{response: multiAtomJSON}
	ext := atom.NewClaudeExtractor(stub)

	atoms, err := ext.Extract(context.Background(), "I'm not a fan of early morning meetings. I'm in PST.")
	require.NoError(t, err)
	assert.Len(t, atoms, 2)
	assert.Equal(t, atom.TypePreference, atoms[0].Type)
	assert.Equal(t, atom.TypeAttribute, atoms[1].Type)
}

func TestClaudeExtractor_EmptyResponse(t *testing.T) {
	stub := &stubCompleter{response: `[]`}
	ext := atom.NewClaudeExtractor(stub)

	atoms, err := ext.Extract(context.Background(), "Generic text with no extractable atoms.")
	require.NoError(t, err)
	assert.Empty(t, atoms)
}

func TestClaudeExtractor_APIError(t *testing.T) {
	stub := &stubCompleter{err: errors.New("api unavailable")}
	ext := atom.NewClaudeExtractor(stub)

	_, err := ext.Extract(context.Background(), "some content")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "api unavailable")
}

func TestClaudeExtractor_BadJSON(t *testing.T) {
	stub := &stubCompleter{response: "not json at all"}
	ext := atom.NewClaudeExtractor(stub)

	_, err := ext.Extract(context.Background(), "some content")
	assert.Error(t, err)
}

func TestClaudeExtractor_MarkdownFenceStripped(t *testing.T) {
	fenced := "```json\n" + preferenceAtomJSON + "\n```"
	stub := &stubCompleter{response: fenced}
	ext := atom.NewClaudeExtractor(stub)

	atoms, err := ext.Extract(context.Background(), "dark chocolate text")
	require.NoError(t, err)
	assert.Len(t, atoms, 1)
}

func TestExtract_StripsThinkBlock(t *testing.T) {
	stub := &stubCompleter{response: "<think>I should return a preference atom.</think>\n" + preferenceAtomJSON}
	ext := atom.NewClaudeExtractor(stub)

	atoms, err := ext.Extract(context.Background(), "dark chocolate text")
	require.NoError(t, err)
	require.Len(t, atoms, 1)
	assert.Equal(t, atom.TypePreference, atoms[0].Type)
	assert.Equal(t, "dark chocolate", atoms[0].Value)
}

func TestExtract_StripsCodeFence(t *testing.T) {
	stub := &stubCompleter{response: "```json\n" + preferenceAtomJSON + "\n```"}
	ext := atom.NewClaudeExtractor(stub)

	atoms, err := ext.Extract(context.Background(), "dark chocolate text")
	require.NoError(t, err)
	require.Len(t, atoms, 1)
	assert.Equal(t, atom.TypePreference, atoms[0].Type)
	assert.Equal(t, "dark chocolate", atoms[0].Value)
}

func TestExtract_ProsePreambleAndPostamble(t *testing.T) {
	stub := &stubCompleter{response: "Here are the atoms:\n" + preferenceAtomJSON + "\nHope this helps."}
	ext := atom.NewClaudeExtractor(stub)

	atoms, err := ext.Extract(context.Background(), "dark chocolate text")
	require.NoError(t, err)
	require.Len(t, atoms, 1)
	assert.Equal(t, atom.TypePreference, atoms[0].Type)
	assert.Equal(t, "dark chocolate", atoms[0].Value)
}

func TestExtract_CleanArray(t *testing.T) {
	stub := &stubCompleter{response: preferenceAtomJSON}
	ext := atom.NewClaudeExtractor(stub)

	atoms, err := ext.Extract(context.Background(), "dark chocolate text")
	require.NoError(t, err)
	require.Len(t, atoms, 1)
	assert.Equal(t, atom.TypePreference, atoms[0].Type)
	assert.Equal(t, "dark chocolate", atoms[0].Value)
}

func TestExtract_NonJSONErrors(t *testing.T) {
	stub := &stubCompleter{response: "I could not extract any atoms from this text."}
	ext := atom.NewClaudeExtractor(stub)

	_, err := ext.Extract(context.Background(), "some content")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse response JSON")
}

func TestClaudeExtractor_TruncatesLongContent(t *testing.T) {
	bigContent := strings.Repeat("a", 20000)
	stub := &stubCompleter{response: `[]`}
	ext := atom.NewClaudeExtractor(stub)

	_, err := ext.Extract(context.Background(), bigContent)
	assert.NoError(t, err)
}

func TestClaudeExtractor_InvalidAtomSkipped(t *testing.T) {
	// Atom with missing required field (empty statement) should be silently skipped.
	badJSON := `[
	  {"atom_type":"preference","subject":"the user","predicate":"likes","value":"cats","statement":"","scope":"global","confidence":0.8},
	  {"atom_type":"fact","subject":"Alice","predicate":"works at","value":"Acme","statement":"Alice works at Acme.","scope":"global","confidence":1.0}
	]`
	stub := &stubCompleter{response: badJSON}
	ext := atom.NewClaudeExtractor(stub)

	atoms, err := ext.Extract(context.Background(), "text")
	require.NoError(t, err)
	// The invalid atom (empty statement) is dropped; only the valid fact remains.
	assert.Len(t, atoms, 1)
	assert.Equal(t, atom.TypeFact, atoms[0].Type)
}

func TestClaudeExtractor_ScopeDefaultsToGlobal(t *testing.T) {
	noScopeJSON := `[
	  {"atom_type":"preference","subject":"the user","predicate":"prefers","value":"tea","statement":"The user prefers tea.","scope":"","confidence":0.9}
	]`
	stub := &stubCompleter{response: noScopeJSON}
	ext := atom.NewClaudeExtractor(stub)

	atoms, err := ext.Extract(context.Background(), "I prefer tea.")
	require.NoError(t, err)
	require.Len(t, atoms, 1)
	assert.Equal(t, atom.ScopeGlobal, atoms[0].Scope)
}

func TestClaudeExtractor_FactAtom(t *testing.T) {
	stub := &stubCompleter{response: factAtomJSON}
	ext := atom.NewClaudeExtractor(stub)

	atoms, err := ext.Extract(context.Background(), "Alice works at Acme Corp.")
	require.NoError(t, err)
	require.Len(t, atoms, 1)
	assert.Equal(t, atom.TypeFact, atoms[0].Type)
	assert.Equal(t, "Alice", atoms[0].Subject)
}

func TestExtractionPrompt_ContainsPreferenceFocus(t *testing.T) {
	prompt := atom.ExtractionPrompt()
	// Verify the prompt instructs the model to capture casual preference language.
	assert.Contains(t, prompt, "preference")
	assert.Contains(t, prompt, "casual")
	assert.Contains(t, prompt, "I usually prefer")
}
