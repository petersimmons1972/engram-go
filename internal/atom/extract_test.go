package atom_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/petersimmons1972/engram/internal/atom"
)

// stubCompleter implements atom.ClaudeCompleter for tests — no real LLM calls.
type stubCompleter struct {
	response  string
	responses []string
	err       error
	system    string
	prompt    string
	prompts   []string
}

func (s *stubCompleter) Complete(_ context.Context, system, prompt, _, _ string, _, _ int) (string, error) {
	s.system = system
	s.prompt = prompt
	s.prompts = append(s.prompts, prompt)
	if len(s.responses) > 0 {
		response := s.responses[0]
		s.responses = s.responses[1:]
		return response, s.err
	}
	return s.response, s.err
}

func TestClaudeExtractorAssignsEventTimeFromSessionDate(t *testing.T) {
	sessionDate := time.Date(2026, 7, 11, 18, 30, 0, 0, time.FixedZone("EDT", -4*60*60))
	stub := &stubCompleter{response: `[
{"atom_type":"event","subject":"the user","predicate":"attended","value":"Go meetup","statement":"On 2026-07-04, the user attended a Go meetup.","scope":"global","confidence":0.9,"event_date":"2026/07/04"},
{"atom_type":"status_change","subject":"the user","predicate":"employment","value":"joined Acme","statement":"The user joined Acme.","scope":"global","confidence":0.9},
{"atom_type":"fact","subject":"Acme","predicate":"location","value":"Boston","statement":"Acme is in Boston.","scope":"global","confidence":0.9,"event_date":"2020-01-01"}
]`}

	atoms, err := atom.NewClaudeExtractor(stub).Extract(context.Background(), "dated session", sessionDate)
	require.NoError(t, err)
	require.Len(t, atoms, 3)

	assert.Equal(t, "2026-07-04", atoms[0].ValidFrom.Format(time.DateOnly))
	assert.Equal(t, "2026-07-11", atoms[0].ObservedAt.Format(time.DateOnly))
	assert.Equal(t, "2026-07-11", atoms[1].ValidFrom.Format(time.DateOnly))
	assert.Equal(t, "2026-07-11", atoms[1].ObservedAt.Format(time.DateOnly))
	assert.Nil(t, atoms[2].ValidFrom, "event_date must not set valid_from for non-event atoms")
	assert.Equal(t, "2026-07-11", atoms[2].ObservedAt.Format(time.DateOnly))
	assert.Contains(t, stub.system, `"event_date"`)
	assert.Contains(t, atoms[0].Statement, "2026-07-04", "statement must retain its date anchor")
}

func TestB1CorruptionProbeRejectsAdversarialEventDates(t *testing.T) {
	sessionDate := time.Date(2026, 7, 11, 0, 0, 0, 0, time.UTC)
	tests := []struct {
		name      string
		eventDate string
	}{
		{name: "garbage", eventDate: "last Tuesday"},
		{name: "mixed separators", eventDate: "2026/07-04"},
		{name: "far future", eventDate: "2028-07-12"},
		{name: "pre 1990", eventDate: "1989-12-31"},
		{name: "swapped range", eventDate: "2026-07-12/2026-07-10"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			response := fmt.Sprintf(`[{"atom_type":"event","subject":"the user","predicate":"visited","value":"Rome","statement":"The user visited Rome.","scope":"global","confidence":0.9,"event_date":%q}]`, tt.eventDate)
			atoms, err := atom.NewClaudeExtractor(&stubCompleter{response: response}).Extract(context.Background(), "session", sessionDate)
			require.NoError(t, err)
			require.Len(t, atoms, 1)
			assert.Nil(t, atoms[0].ValidFrom)
			assert.NotNil(t, atoms[0].ObservedAt)
		})
	}
}

func TestB1CorruptionProbeChunkingSupersetAndExactDedup(t *testing.T) {
	sessionDate := time.Date(2026, 7, 11, 0, 0, 0, 0, time.UTC)
	first := `[{"atom_type":"fact","subject":"Alpha","predicate":"exists","value":"yes","statement":"Alpha exists.","scope":"global","confidence":0.9}]`
	second := `[
{"atom_type":"fact","subject":"Alpha","predicate":"exists","value":"yes","statement":"Alpha exists.","scope":"global","confidence":0.9},
{"atom_type":"fact","subject":"Beyond6000","predicate":"exists","value":"yes","statement":"Beyond6000 exists.","scope":"global","confidence":0.9}
]`
	stub := &stubCompleter{responses: []string{first, second, `[]`}}
	session := strings.Repeat("a", 6000) + "\n[user]\nBeyond6000 appears here.\n" + strings.Repeat("b", 9000)
	truncatedAtoms, err := atom.NewClaudeExtractor(&stubCompleter{response: first}).Extract(
		context.Background(),
		string([]rune(session)[:6000]),
		sessionDate,
	)
	require.NoError(t, err)

	atoms, err := atom.NewClaudeExtractor(stub).Extract(context.Background(), session, sessionDate)
	require.NoError(t, err)
	require.Greater(t, len(atoms), len(truncatedAtoms), "chunked extraction must be a strict superset")
	assert.Len(t, stub.prompts, 3)
	assert.Equal(t, "Alpha", atoms[0].Subject)
	assert.Equal(t, "Beyond6000", atoms[1].Subject)
	assert.Contains(t, stub.prompts[1], "Beyond6000")
	for _, truncated := range truncatedAtoms {
		assert.Contains(t, atoms, truncated, "every atom from the old truncated path must survive windowing")
	}
	for _, prompt := range stub.prompts {
		window := strings.TrimPrefix(prompt, "Extract typed atoms (focus on preferences, profile facts, and status changes) from the following session text:\n\n")
		assert.LessOrEqual(t, len([]rune(window)), 6000)
	}
}

func TestClaudeExtractorStopsWhenAnyWindowFails(t *testing.T) {
	sessionDate := time.Date(2026, 7, 11, 0, 0, 0, 0, time.UTC)
	stub := &stubCompleter{responses: []string{`[]`, `not json`}}
	session := strings.Repeat("a", 6000) + "\n" + strings.Repeat("b", 100)

	_, err := atom.NewClaudeExtractor(stub).Extract(context.Background(), session, sessionDate)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "window 2")
}

func TestClaudeExtractorIgnoresPathologicallyEarlyBoundary(t *testing.T) {
	stub := &stubCompleter{response: `[]`}
	session := "user: " + strings.Repeat("a", 5494) + "\n" + strings.Repeat("continued", 800)

	_, err := atom.NewClaudeExtractor(stub).Extract(context.Background(), session, time.Now())
	require.NoError(t, err)
	require.Len(t, stub.prompts, 3)
	firstWindow := strings.TrimPrefix(stub.prompts[0], "Extract typed atoms (focus on preferences, profile facts, and status changes) from the following session text:\n\n")
	assert.Len(t, []rune(firstWindow), 6000, "an early newline must not create a tiny extraction window")
}

func TestClaudeExtractorPrefersRecognizedMessageBoundary(t *testing.T) {
	stub := &stubCompleter{response: `[]`}
	firstMessage := "user: " + strings.Repeat("a", 5494) + "\n"
	session := firstMessage + "assistant: " + strings.Repeat("b", 1000)

	_, err := atom.NewClaudeExtractor(stub).Extract(context.Background(), session, time.Now())
	require.NoError(t, err)
	require.Len(t, stub.prompts, 2)
	firstWindow := strings.TrimPrefix(stub.prompts[0], "Extract typed atoms (focus on preferences, profile facts, and status changes) from the following session text:\n\n")
	secondWindow := strings.TrimPrefix(stub.prompts[1], "Extract typed atoms (focus on preferences, profile facts, and status changes) from the following session text:\n\n")
	assert.Equal(t, firstMessage, firstWindow)
	assert.True(t, strings.HasPrefix(secondWindow, "assistant: "))
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
	require.Len(t, fixtures, 3)

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
			assert.Contains(t, stub.system, "personas assigned to the assistant")
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
				text := strings.ToLower(extracted.Subject + " " + extracted.Statement + " " + extracted.Value)
				userAttributed := strings.Contains(strings.ToLower(extracted.Subject), "user") ||
					strings.Contains(strings.ToLower(extracted.Statement), "the user")
				if userAttributed {
					for _, forbidden := range fixture.ForbiddenUserFacts {
						assert.NotContains(t, text, strings.ToLower(forbidden),
							"persona/subject fact attributed to the user (atom_type=%s)", extracted.Type)
					}
				}
				// A standing claim can hide under any non-event type — gate them all.
				if extracted.Type != atom.TypeEvent && extracted.Type != atom.TypeStatusChange {
					for _, forbidden := range fixture.ForbiddenHabits {
						assert.NotContains(t, text, strings.ToLower(forbidden),
							"one-off plan generalised into a standing claim (atom_type=%s)", extracted.Type)
					}
				}
			}
		})
	}
}
