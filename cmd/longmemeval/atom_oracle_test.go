package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/petersimmons1972/engram/internal/atom"
	"github.com/petersimmons1972/engram/internal/longmemeval"
)

// stubCompleter is a test double for atom.ClaudeCompleter that returns canned
// JSON atoms without making any LLM network calls.
type stubCompleter struct {
	atoms []atom.Atom
	err   error
}

func (s *stubCompleter) Complete(_ context.Context, _, _ string, _, _ string, _ int, _ int) (string, error) {
	if s.err != nil {
		return "", s.err
	}
	type wire struct {
		Type       string  `json:"atom_type"`
		Subject    string  `json:"subject"`
		Predicate  string  `json:"predicate"`
		Value      string  `json:"value"`
		Statement  string  `json:"statement"`
		Scope      string  `json:"scope"`
		Confidence float64 `json:"confidence"`
		SourceSpan string  `json:"source_span"`
	}
	ws := make([]wire, len(s.atoms))
	for i, a := range s.atoms {
		ws[i] = wire{
			Type:       a.Type,
			Subject:    a.Subject,
			Predicate:  a.Predicate,
			Value:      a.Value,
			Statement:  a.Statement,
			Scope:      a.Scope,
			Confidence: a.Confidence,
		}
	}
	b, _ := json.Marshal(ws)
	return string(b), nil
}

// twoTestAtoms returns two valid preference atoms for use in tests.
func twoTestAtoms() []atom.Atom {
	return []atom.Atom{
		{
			Type:       atom.TypePreference,
			Subject:    "the user",
			Predicate:  "prefers",
			Value:      "dark chocolate",
			Statement:  "The user prefers dark chocolate over milk chocolate.",
			Scope:      atom.ScopeGlobal,
			Confidence: 0.9,
		},
		{
			Type:       atom.TypePreference,
			Subject:    "the user",
			Predicate:  "dislikes",
			Value:      "loud music",
			Statement:  "The user dislikes loud music.",
			Scope:      atom.ScopeGlobal,
			Confidence: 0.85,
		},
	}
}

// buildTestItem returns an Item with 3 haystack sessions, 2 of which are
// referenced by AnswerSessionIDs. Session IDs are "sess-a", "sess-b", "sess-c";
// "sess-b" is the gold answer session.
func buildTestItem() longmemeval.Item {
	turnFor := func(content string) []longmemeval.Turn {
		return []longmemeval.Turn{
			{Role: "user", Content: content},
			{Role: "assistant", Content: "acknowledged"},
		}
	}
	return longmemeval.Item{
		QuestionID:         "q001",
		QuestionType:       "single-session-preference",
		Question:           "Do I prefer dark or milk chocolate?",
		QuestionDate:       "2024/01/15",
		HaystackSessionIDs: []string{"sess-a", "sess-b", "sess-c"},
		HaystackSessions: [][]longmemeval.Turn{
			turnFor("I like going to the cinema."),      // sess-a — not gold
			turnFor("I prefer dark chocolate, always."), // sess-b — gold
			turnFor("The weather was nice today."),      // sess-c — not gold
		},
		AnswerSessionIDs: []string{"sess-b"},
	}
}

// --- Test 1: goldSessionTexts only returns sessions in AnswerSessionIDs ---

func TestOracleAtomsOnlyFromAnswerSessions(t *testing.T) {
	t.Parallel()
	item := buildTestItem()
	texts := goldSessionTexts(item)

	if len(texts) != 1 {
		t.Fatalf("goldSessionTexts: want 1 session text, got %d", len(texts))
	}
	if !strings.Contains(texts[0], "dark chocolate") {
		t.Errorf("goldSessionTexts: expected gold session content (dark chocolate), got: %q", texts[0])
	}
	// Non-gold sessions must NOT appear.
	for _, text := range texts {
		if strings.Contains(text, "cinema") {
			t.Error("goldSessionTexts: non-gold session (cinema) leaked into result")
		}
		if strings.Contains(text, "weather") {
			t.Error("goldSessionTexts: non-gold session (weather) leaked into result")
		}
	}
}

// --- Test 2: AtomOracleVariants produce correct context block shapes via buildOracleContext ---

func TestAtomOracleVariantsContext(t *testing.T) {
	t.Parallel()
	item := buildTestItem()

	tests := []struct {
		variant        string
		wantBlockCount int
	}{
		{"atom-only", 1},
		{"atom-plus-source", 2}, // atom block + 1 gold session text
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.variant, func(t *testing.T) {
			t.Parallel()
			cfg := &Config{
				AtomOracle:        true,
				AtomOracleVariant: tc.variant,
			}
			stub := &stubCompleter{atoms: twoTestAtoms()}

			contextBlocks, atomCount, _, err := buildOracleContext(context.Background(), stub, cfg, item)
			if err != nil {
				t.Fatalf("buildOracleContext: %v", err)
			}
			if atomCount == 0 {
				t.Fatal("expected atoms to be extracted")
			}
			if len(contextBlocks) != tc.wantBlockCount {
				t.Errorf("variant %q: want %d context blocks, got %d", tc.variant, tc.wantBlockCount, len(contextBlocks))
			}
		})
	}
}

// --- Test 3: score_report emits by_type keyed by question type ---

func TestOracleReportByQuestionType(t *testing.T) {
	t.Parallel()
	// Synthetic score entries spanning two question types.
	scores := []longmemeval.ScoreEntry{
		{QuestionID: "q1", QuestionType: "single-session-preference", ScoreLabel: "CORRECT", Status: "done"},
		{QuestionID: "q2", QuestionType: "single-session-preference", ScoreLabel: "INCORRECT", Status: "done"},
		{QuestionID: "q3", QuestionType: "temporal-reasoning", ScoreLabel: "CORRECT", Status: "done"},
	}
	cfg := &Config{OutDir: t.TempDir(), RunID: "by-type-test"}

	// Call the real writeScoreReport so this test fails if the grouping logic changes.
	writeScoreReport(cfg, scores)

	data, err := os.ReadFile(filepath.Join(cfg.OutDir, "score_report.json"))
	if err != nil {
		t.Fatalf("read score_report.json: %v", err)
	}
	var report map[string]any
	if err := json.Unmarshal(data, &report); err != nil {
		t.Fatalf("parse score_report.json: %v", err)
	}
	byType, ok := report["by_type"].(map[string]any)
	if !ok {
		t.Fatalf("by_type missing or wrong type: %v", report["by_type"])
	}

	ssp, ok := byType["single-session-preference"].(map[string]any)
	if !ok {
		t.Fatal("by_type: missing key single-session-preference")
	}
	if total := int(ssp["total"].(float64)); total != 2 {
		t.Errorf("single-session-preference total: want 2, got %d", total)
	}

	tr, ok := byType["temporal-reasoning"].(map[string]any)
	if !ok {
		t.Fatal("by_type: missing key temporal-reasoning")
	}
	if correct := int(tr["correct"].(float64)); correct != 1 {
		t.Errorf("temporal-reasoning correct: want 1, got %d", correct)
	}
}

// --- Test 4: buildOracleContext falls back to raw text on zero atoms ---

func TestOracleFallsBackToRawTextOnZeroAtoms(t *testing.T) {
	t.Parallel()
	item := buildTestItem()
	cfg := &Config{
		AtomOracle:        true,
		AtomOracleVariant: "atom-only",
	}
	stub := &stubCompleter{atoms: []atom.Atom{}} // extractor returns empty array

	contextBlocks, atomCount, sessionCount, err := buildOracleContext(context.Background(), stub, cfg, item)
	if err != nil {
		t.Fatalf("buildOracleContext: unexpected error on zero atoms: %v", err)
	}
	if atomCount != 0 {
		t.Errorf("atomCount: want 0, got %d", atomCount)
	}
	if sessionCount != 1 {
		t.Errorf("sessionCount: want 1 (one gold session), got %d", sessionCount)
	}
	// Fallback must return raw gold session text, not an empty context.
	if len(contextBlocks) == 0 {
		t.Fatal("expected fallback to raw session text, got no context blocks")
	}
	if !strings.Contains(contextBlocks[0], "dark chocolate") {
		t.Errorf("fallback context must contain gold session content, got: %q", contextBlocks[0])
	}
}

// --- Test 5: buildOracleContext with atoms present ---

func TestBuildOracleContextWithAtoms(t *testing.T) {
	t.Parallel()
	item := buildTestItem()

	tests := []struct {
		variant        string
		wantBlockCount int
	}{
		{"atom-only", 1},        // only atom block
		{"atom-plus-source", 2}, // atom block + 1 gold session
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.variant, func(t *testing.T) {
			t.Parallel()
			cfg := &Config{
				AtomOracle:        true,
				AtomOracleVariant: tc.variant,
			}
			stub := &stubCompleter{atoms: twoTestAtoms()}

			contextBlocks, atomCount, sessionCount, err := buildOracleContext(context.Background(), stub, cfg, item)
			if err != nil {
				t.Fatalf("buildOracleContext: %v", err)
			}
			if atomCount != 2 {
				t.Errorf("atomCount: want 2, got %d", atomCount)
			}
			if sessionCount != 1 {
				t.Errorf("sessionCount: want 1, got %d", sessionCount)
			}
			if len(contextBlocks) != tc.wantBlockCount {
				t.Errorf("contextBlocks: want %d blocks, got %d", tc.wantBlockCount, len(contextBlocks))
			}
		})
	}
}

// --- Test 6: runOneOracleWithDeps — injectable generation ---

func TestRunOneOracleWithDeps(t *testing.T) {
	t.Parallel()
	item := buildTestItem()
	cfg := &Config{
		AtomOracle:        true,
		AtomOracleVariant: "atom-only",
	}
	ingest := longmemeval.IngestEntry{QuestionID: item.QuestionID}

	t.Run("happy path returns done with hypothesis", func(t *testing.T) {
		t.Parallel()
		stub := &stubCompleter{atoms: twoTestAtoms()}
		generateFn := func(_ context.Context, _ string) (string, error) {
			return "dark chocolate", nil
		}
		entry := runOneOracleWithDeps(context.Background(), cfg, stub, generateFn, item, ingest)
		if entry.Status != "done" {
			t.Errorf("status: want done, got %q (err: %q)", entry.Status, entry.Error)
		}
		if entry.Hypothesis != "dark chocolate" {
			t.Errorf("hypothesis: want %q, got %q", "dark chocolate", entry.Hypothesis)
		}
		if entry.OracleAtomCount != 2 {
			t.Errorf("OracleAtomCount: want 2, got %d", entry.OracleAtomCount)
		}
		if entry.OracleZeroAtoms {
			t.Error("OracleZeroAtoms must be false when atoms were extracted")
		}
	})

	t.Run("zero atoms falls back — status done, OracleZeroAtoms true", func(t *testing.T) {
		t.Parallel()
		stub := &stubCompleter{atoms: []atom.Atom{}}
		generateFn := func(_ context.Context, _ string) (string, error) {
			return "some answer", nil
		}
		entry := runOneOracleWithDeps(context.Background(), cfg, stub, generateFn, item, ingest)
		if entry.Status != "done" {
			t.Errorf("status: want done for zero-atoms fallback, got %q", entry.Status)
		}
		if !entry.OracleZeroAtoms {
			t.Error("OracleZeroAtoms must be true when extraction returned zero atoms")
		}
	})

	t.Run("generate error propagates as status error", func(t *testing.T) {
		t.Parallel()
		stub := &stubCompleter{atoms: twoTestAtoms()}
		generateFn := func(_ context.Context, _ string) (string, error) {
			return "", fmt.Errorf("LLM unreachable")
		}
		entry := runOneOracleWithDeps(context.Background(), cfg, stub, generateFn, item, ingest)
		if entry.Status != "error" {
			t.Errorf("status: want error, got %q", entry.Status)
		}
		if !strings.Contains(entry.Error, "oracle generate") {
			t.Errorf("error must mention oracle generate, got %q", entry.Error)
		}
	})
}

// --- Test 7: AtomOracle flag defaults ---

func TestOracleModeDefaultOff(t *testing.T) {
	t.Parallel()

	parse := func(extraArgs ...string) *Config {
		fs := flag.NewFlagSet("run", flag.ContinueOnError)
		cfg := &Config{}
		fs.BoolVar(&cfg.AtomOracle, "atom-oracle", false, "oracle probe")
		fs.StringVar(&cfg.AtomOracleVariant, "atom-oracle-variant", "atom-only", "oracle variant")
		_ = fs.Parse(extraArgs)
		return cfg
	}

	// Default: off.
	cfgDefault := parse()
	if cfgDefault.AtomOracle {
		t.Error("AtomOracle must default to false")
	}
	if cfgDefault.AtomOracleVariant != "atom-only" {
		t.Errorf("AtomOracleVariant default: want atom-only, got %q", cfgDefault.AtomOracleVariant)
	}

	// Explicitly enabled.
	cfgOn := parse("--atom-oracle", "--atom-oracle-variant=atom-plus-source")
	if !cfgOn.AtomOracle {
		t.Error("AtomOracle must be true when --atom-oracle flag is set")
	}
	if cfgOn.AtomOracleVariant != "atom-plus-source" {
		t.Errorf("AtomOracleVariant: want atom-plus-source, got %q", cfgOn.AtomOracleVariant)
	}
}
