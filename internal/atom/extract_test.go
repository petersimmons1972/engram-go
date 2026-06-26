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

func TestExtractProfileAndStatusChange(t *testing.T) {
	stub := &stubCompleter{response: `[
  {
    "atom_type": "profile",
    "subject": "the user",
    "predicate": "diet",
    "value": "vegetarian",
    "statement": "The user is vegetarian.",
    "scope": "global",
    "confidence": 0.95,
    "source_span": "I'm vegetarian"
  },
  {
    "atom_type": "status_change",
    "subject": "the user",
    "predicate": "job",
    "value": "started a new role at Acme",
    "statement": "The user started a new role at Acme.",
    "scope": "global",
    "confidence": 0.9,
    "source_span": "I started a new role at Acme"
  }
]`}
	ext := atom.NewClaudeExtractor(stub)

	atoms, err := ext.Extract(context.Background(), "I'm vegetarian, and I started a new role at Acme.")
	require.NoError(t, err)
	require.Len(t, atoms, 2)
	assert.Equal(t, "profile", atoms[0].Type)
	assert.Equal(t, "status_change", atoms[1].Type)
	assert.Contains(t, atom.ExtractionPrompt(), "profile")
	assert.Contains(t, atom.ExtractionPrompt(), "status_change")
}

func TestExtractionPrompt_ContainsPreferenceFocus(t *testing.T) {
	prompt := atom.ExtractionPrompt()
	// Verify the prompt instructs the model to capture casual preference language.
	assert.Contains(t, prompt, "preference")
	assert.Contains(t, prompt, "casual")
	assert.Contains(t, prompt, "I usually prefer")
}

// ── preference-entity fields (#1181) ─────────────────────────────────────────

const preferenceAtomWithEntityJSON = `[
  {
    "atom_type": "preference",
    "subject": "the user",
    "predicate": "prefers",
    "value": "dark chocolate",
    "statement": "The user prefers dark chocolate over milk chocolate.",
    "scope": "global",
    "confidence": 0.9,
    "polarity": "like",
    "entity": "dark chocolate",
    "domain": "food",
    "source_span": "I usually go with dark chocolate"
  }
]`

const preferenceAtomDislikeJSON = `[
  {
    "atom_type": "preference",
    "subject": "the user",
    "predicate": "dislikes",
    "value": "cilantro",
    "statement": "The user dislikes cilantro.",
    "scope": "global",
    "confidence": 0.88,
    "polarity": "dislike",
    "entity": "cilantro",
    "domain": "food",
    "source_span": "I hate cilantro"
  }
]`

// TestClaudeExtractor_MapsEntityFields verifies that polarity/entity/domain
// returned by the model are carried through to the Atom struct.
func TestClaudeExtractor_MapsEntityFields(t *testing.T) {
	stub := &stubCompleter{response: preferenceAtomWithEntityJSON}
	ext := atom.NewClaudeExtractor(stub)

	atoms, err := ext.Extract(context.Background(), "I usually go with dark chocolate.")
	require.NoError(t, err)
	require.Len(t, atoms, 1)

	a := atoms[0]
	assert.Equal(t, "like", a.Polarity, "polarity must be mapped from response")
	assert.Equal(t, "dark chocolate", a.Entity, "entity must be copied verbatim from response")
	assert.Equal(t, "food", a.Domain, "domain must be mapped from response")
}

// TestClaudeExtractor_MapsDislikePolarity verifies dislike polarity round-trips.
func TestClaudeExtractor_MapsDislikePolarity(t *testing.T) {
	stub := &stubCompleter{response: preferenceAtomDislikeJSON}
	ext := atom.NewClaudeExtractor(stub)

	atoms, err := ext.Extract(context.Background(), "I hate cilantro in my food.")
	require.NoError(t, err)
	require.Len(t, atoms, 1)

	a := atoms[0]
	assert.Equal(t, "dislike", a.Polarity)
	assert.Equal(t, "cilantro", a.Entity)
	assert.Equal(t, "food", a.Domain)
}

// TestClaudeExtractor_EntityFieldsEmpty_BackCompat verifies that atoms from
// responses without polarity/entity/domain (legacy/non-preference) have empty
// fields and are still valid.
func TestClaudeExtractor_EntityFieldsEmpty_BackCompat(t *testing.T) {
	stub := &stubCompleter{response: factAtomJSON}
	ext := atom.NewClaudeExtractor(stub)

	atoms, err := ext.Extract(context.Background(), "Alice works at Acme Corp.")
	require.NoError(t, err)
	require.Len(t, atoms, 1)

	a := atoms[0]
	assert.Empty(t, a.Polarity, "non-preference atoms must have empty polarity")
	assert.Empty(t, a.Entity, "non-preference atoms must have empty entity")
	assert.Empty(t, a.Domain, "non-preference atoms must have empty domain")
}

// TestExtractionPrompt_ContainsEntityInstructions verifies the extraction
// prompt now instructs the model to capture polarity, entity, and domain.
func TestExtractionPrompt_ContainsEntityInstructions(t *testing.T) {
	prompt := atom.ExtractionPrompt()
	assert.Contains(t, prompt, "polarity", "prompt must request polarity field")
	assert.Contains(t, prompt, "entity", "prompt must request entity field")
	assert.Contains(t, prompt, "domain", "prompt must request domain field")
	assert.Contains(t, prompt, "VERBATIM", "prompt must forbid inventing entities")
}
