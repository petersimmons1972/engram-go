package atom_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
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
	system   string
	prompt   string
}

func (s *stubCompleter) Complete(_ context.Context, system, prompt, _, _ string, _, _ int) (string, error) {
	s.system = system
	s.prompt = prompt
	return s.response, s.err
}

type extractionFidelityFixture struct {
	Name               string   `json:"name"`
	Session            string   `json:"session"`
	ForbiddenUserFacts []string `json:"forbidden_user_facts"`
	ForbiddenHabits    []string `json:"forbidden_habits"`
}

type claudeCLICompleter struct{}

func (claudeCLICompleter) Complete(
	ctx context.Context,
	system string,
	prompt string,
	_ string,
	_ string,
	_ int,
	_ int,
) (string, error) {
	cmd := exec.CommandContext(ctx, "claude", "--print", "--model", "sonnet")
	cmd.Stdin = strings.NewReader(system + "\n\n" + prompt)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", errors.New(string(out))
	}
	return string(out), nil
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

func TestExtractionPrompt_GuardsExtractionFidelity(t *testing.T) {
	fixtures := loadExtractionFidelityFixtures(t)
	require.Len(t, fixtures, 2)

	for _, fixture := range fixtures {
		t.Run(fixture.Name, func(t *testing.T) {
			stub := &stubCompleter{response: `[]`}
			ext := atom.NewClaudeExtractor(stub)

			_, err := ext.Extract(context.Background(), fixture.Session)
			require.NoError(t, err)
			assert.Contains(t, stub.prompt, fixture.Session)
			assert.Contains(t, stub.system, "roleplay personas")
			assert.Contains(t, stub.system, "article or story subjects")
			assert.Contains(t, stub.system, "hypothetical people")
			assert.Contains(t, stub.system, "NOT user facts")
			assert.Contains(t, stub.system, "only repeated or explicitly stated habits")
			assert.Contains(t, stub.system, "one-off events and plans")
			assert.Contains(t, stub.system, "atom_type event")
		})
	}
}

func loadExtractionFidelityFixtures(t *testing.T) []extractionFidelityFixture {
	t.Helper()

	path := filepath.Join("testdata", "extraction_fidelity.json")
	data, err := os.ReadFile(path)
	require.NoError(t, err)

	fixtures := []extractionFidelityFixture{}
	require.NoError(t, json.Unmarshal(data, &fixtures))
	return fixtures
}

// TestManualExtractionFidelity exercises the non-deterministic LLM boundary.
// Run it via bin/manual-eval-atom-extraction.sh; normal unit test runs skip it.
func TestManualExtractionFidelity(t *testing.T) {
	if os.Getenv("ENGRAM_MANUAL_ATOM_EVAL") != "1" {
		t.Skip("set ENGRAM_MANUAL_ATOM_EVAL=1 to run the Claude-backed evaluation")
	}
	if _, err := exec.LookPath("claude"); err != nil {
		t.Skip("claude CLI is not installed")
	}

	fixtures := loadExtractionFidelityFixtures(t)
	for _, fixture := range fixtures {
		t.Run(fixture.Name, func(t *testing.T) {
			ext := atom.NewClaudeExtractor(claudeCLICompleter{})
			atoms, err := ext.Extract(context.Background(), fixture.Session)
			require.NoError(t, err)

			for _, extracted := range atoms {
				text := strings.ToLower(extracted.Statement + " " + extracted.Value)
				if strings.EqualFold(extracted.Subject, "the user") {
					for _, forbidden := range fixture.ForbiddenUserFacts {
						assert.NotContains(t, text, strings.ToLower(forbidden))
					}
				}
				if extracted.Type == atom.TypePreference || extracted.Type == atom.TypeProfile {
					for _, forbidden := range fixture.ForbiddenHabits {
						assert.NotContains(t, text, strings.ToLower(forbidden))
					}
				}
			}
		})
	}
}
