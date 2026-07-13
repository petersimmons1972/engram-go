package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/petersimmons1972/engram/internal/atom"
	"github.com/petersimmons1972/engram/internal/longmemeval"
)

type chronoLedgerFetcher struct {
	atoms   []atom.Atom
	err     error
	calls   int
	project string
	limit   int
}

func (f *chronoLedgerFetcher) FetchChronoLedgerAtoms(
	_ context.Context,
	project string,
	limit int,
) ([]atom.Atom, error) {
	f.calls++
	f.project = project
	f.limit = limit
	return f.atoms, f.err
}

func TestLoadChronoLedger_B4AloneFetchesFullProjectTimeline(t *testing.T) {
	newer := time.Date(2024, 3, 12, 9, 0, 0, 0, time.UTC)
	older := time.Date(2023, 11, 5, 17, 0, 0, 0, time.UTC)
	retired := time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC)
	fetcher := &chronoLedgerFetcher{atoms: []atom.Atom{
		{ID: "status-1", Type: atom.TypeStatusChange, Statement: "Started a new role.", ValidFrom: &newer, ValidTo: &retired},
		{ID: "event-1", Type: atom.TypeEvent, Statement: "Moved to Boston.", ValidFrom: &older},
	}}

	ledger, err := loadChronoLedger(
		context.Background(),
		true,
		"temporal-reasoning",
		fetcher,
		"project-a",
	)
	if err != nil {
		t.Fatalf("loadChronoLedger: %v", err)
	}
	if fetcher.calls != 1 || fetcher.project != "project-a" || fetcher.limit != chronoLedgerLineCap+1 {
		t.Fatalf("fetch calls=%d project=%q limit=%d, want one project fetch capped at %d", fetcher.calls, fetcher.project, fetcher.limit, chronoLedgerLineCap+1)
	}
	if !strings.Contains(ledger, "=== Event Timeline") {
		t.Fatalf("ledger missing timeline header: %q", ledger)
	}
	if strings.Index(ledger, "Moved to Boston.") >= strings.Index(ledger, "Started a new role.") {
		t.Fatalf("ledger is not chronological: %q", ledger)
	}
	if !strings.Contains(ledger, "[superseded 2024-06-01]") {
		t.Fatalf("ledger missing supersession annotation: %q", ledger)
	}
}

func TestLoadChronoLedger_EmptyProjectIsNoOp(t *testing.T) {
	fetcher := &chronoLedgerFetcher{}
	ledger, err := loadChronoLedger(
		context.Background(),
		true,
		"temporal-reasoning",
		fetcher,
		"empty-project",
	)
	if err != nil {
		t.Fatalf("loadChronoLedger: %v", err)
	}
	if ledger != "" {
		t.Fatalf("empty project ledger = %q, want empty", ledger)
	}
	if fetcher.calls != 1 {
		t.Fatalf("fetch calls = %d, want 1", fetcher.calls)
	}
	if got := prependChronoLedger([]string{"memory context"}, ledger); len(got) != 1 || got[0] != "memory context" {
		t.Fatalf("empty ledger changed context: %q", got)
	}
}

func TestLoadChronoLedger_GatesDoNotFetch(t *testing.T) {
	tests := []struct {
		name         string
		enabled      bool
		questionType string
	}{
		{name: "flag off", questionType: "temporal-reasoning"},
		{name: "non-temporal question", enabled: true, questionType: "single-session-user"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fetcher := &chronoLedgerFetcher{}
			ledger, err := loadChronoLedger(
				context.Background(),
				tt.enabled,
				tt.questionType,
				fetcher,
				"project-a",
			)
			if err != nil {
				t.Fatalf("loadChronoLedger: %v", err)
			}
			if ledger != "" || fetcher.calls != 0 {
				t.Fatalf("ledger=%q calls=%d, want empty ledger and no fetch", ledger, fetcher.calls)
			}
		})
	}
}

func TestLoadChronoLedger_FetchFailureIsReturned(t *testing.T) {
	fetcher := &chronoLedgerFetcher{err: errors.New("atom service unavailable")}
	ledger, err := loadChronoLedger(
		context.Background(),
		true,
		"temporal-reasoning",
		fetcher,
		"project-a",
	)
	if err == nil || !strings.Contains(err.Error(), "fetch project chronology") {
		t.Fatalf("error = %v, want contextual fetch failure", err)
	}
	if ledger != "" {
		t.Fatalf("ledger = %q on fetch failure, want empty", ledger)
	}
}

func TestFormatChronoLedger_AtomTypeAndDateAllowSet(t *testing.T) {
	date := time.Date(2024, 1, 10, 0, 0, 0, 0, time.UTC)
	tests := []struct {
		name     string
		atomType string
		date     *time.Time
		want     bool
	}{
		{name: "event", atomType: atom.TypeEvent, date: &date, want: true},
		{name: "status change", atomType: atom.TypeStatusChange, date: &date, want: true},
		{name: "preference", atomType: atom.TypePreference, date: &date},
		{name: "fact", atomType: atom.TypeFact, date: &date},
		{name: "attribute", atomType: atom.TypeAttribute, date: &date},
		{name: "relationship", atomType: atom.TypeRelationship, date: &date},
		{name: "profile", atomType: atom.TypeProfile, date: &date},
		{name: "undated event", atomType: atom.TypeEvent},
		{name: "undated status change", atomType: atom.TypeStatusChange},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatChronoLedger([]atom.Atom{{
				ID:        tt.name,
				Type:      tt.atomType,
				Statement: "Timeline candidate.",
				ValidFrom: tt.date,
			}})
			if (got != "") != tt.want {
				t.Fatalf("formatChronoLedger() non-empty = %v, want %v: %q", got != "", tt.want, got)
			}
		})
	}
}

func TestFormatChronoLedger_DeduplicatesSortsAndCapsAfterFiltering(t *testing.T) {
	base := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	atoms := make([]atom.Atom, 0, 45)
	for i := 41; i >= 0; i-- {
		validFrom := base.AddDate(0, 0, i)
		atomType := atom.TypeEvent
		if i%2 == 1 {
			atomType = atom.TypeStatusChange
		}
		atoms = append(atoms, atom.Atom{
			ID:        fmt.Sprintf("atom-%02d", i),
			Type:      atomType,
			Statement: fmt.Sprintf("Timeline item %02d.", i),
			ValidFrom: &validFrom,
		})
	}
	duplicateDate := base
	atoms = append(atoms,
		atom.Atom{ID: "duplicate", Type: atom.TypeEvent, Statement: "Timeline item 00.", ValidFrom: &duplicateDate},
		atom.Atom{ID: "wrong-type", Type: atom.TypeFact, Statement: "Must not consume cap.", ValidFrom: &duplicateDate},
		atom.Atom{ID: "undated", Type: atom.TypeEvent, Statement: "Must not consume cap."},
	)

	got := formatChronoLedger(atoms)
	if strings.Count(got, " [current]\n") != chronoLedgerLineCap {
		t.Fatalf("timeline line count = %d, want %d", strings.Count(got, " [current]\n"), chronoLedgerLineCap)
	}
	if strings.Count(got, "Timeline item 00.") != 1 {
		t.Fatalf("duplicate timeline entry was not removed: %q", got)
	}
	if !strings.Contains(got, "2024-01-01: Timeline item 00.") || strings.Contains(got, "Timeline item 40.") {
		t.Fatalf("timeline did not keep the earliest 40 entries: %q", got)
	}
	if strings.Contains(got, "Must not consume cap.") || !strings.Contains(got, "(+more events truncated)") {
		t.Fatalf("filtering/truncation mismatch: %q", got)
	}
}

func TestChronoLedger_PromptContainsTimelineBeforeMemoryContext(t *testing.T) {
	date := time.Date(2024, 1, 11, 0, 0, 0, 0, time.UTC)
	ledger := formatChronoLedger([]atom.Atom{{
		ID:        "event-1",
		Type:      atom.TypeEvent,
		Statement: "Visited the dentist.",
		ValidFrom: &date,
	}})
	blocks := prependChronoLedger([]string{"raw session"}, ledger)
	item := longmemeval.Item{
		Question:     "What happened yesterday?",
		QuestionType: "temporal-reasoning",
		QuestionDate: "2024/01/12",
	}
	prompt := selectGenerationPrompt(&Config{}, longmemeval.RunOpts{}, item, blocks)

	timelineAt := strings.Index(prompt, "=== Event Timeline")
	memoryAt := strings.Index(prompt, "raw session")
	if timelineAt < 0 || memoryAt < 0 || timelineAt >= memoryAt {
		t.Fatalf("timeline=%d memory=%d in prompt %q", timelineAt, memoryAt, prompt)
	}
}

func TestInjectChronoLedger_TwoItemsB4AloneHaveTimelineWithoutEmptyLog(t *testing.T) {
	date := time.Date(2024, 1, 11, 0, 0, 0, 0, time.UTC)
	fetcher := &chronoLedgerFetcher{atoms: []atom.Atom{{
		ID:        "event-1",
		Type:      atom.TypeEvent,
		Statement: "Visited the dentist.",
		ValidFrom: &date,
	}}}

	var logs bytes.Buffer
	previousWriter := log.Writer()
	log.SetOutput(&logs)
	defer log.SetOutput(previousWriter)

	for _, questionID := range []string{"q-1", "q-2"} {
		item := longmemeval.Item{
			QuestionID:   questionID,
			Question:     "What happened yesterday?",
			QuestionType: "temporal-reasoning",
			QuestionDate: "2024/01/12",
		}
		blocks, err := injectChronoLedger(
			context.Background(),
			fetcher,
			chronoLedgerRunRequest{
				enabled:      true,
				questionType: item.QuestionType,
				project:      "project-a",
				questionID:   item.QuestionID,
			},
			[]string{"raw session"},
		)
		if err != nil {
			t.Fatalf("injectChronoLedger(%s): %v", questionID, err)
		}
		prompt := selectGenerationPrompt(&Config{}, longmemeval.RunOpts{}, item, blocks)
		if !strings.Contains(prompt, "=== Event Timeline") {
			t.Fatalf("prompt for %s missing timeline: %q", questionID, prompt)
		}
	}
	if fetcher.calls != 2 {
		t.Fatalf("fetch calls = %d, want one per item", fetcher.calls)
	}
	if strings.Contains(logs.String(), "chronology ledger found no event atoms") {
		t.Fatalf("non-empty two-item run logged empty ledger: %q", logs.String())
	}
}

func TestInjectChronoLedger_EmptyProjectLogsAndDoesNotInject(t *testing.T) {
	fetcher := &chronoLedgerFetcher{}
	var logs bytes.Buffer
	previousWriter := log.Writer()
	log.SetOutput(&logs)
	defer log.SetOutput(previousWriter)

	baseline := []string{"raw session"}
	got, err := injectChronoLedger(
		context.Background(),
		fetcher,
		chronoLedgerRunRequest{
			enabled:      true,
			questionType: "temporal-reasoning",
			project:      "empty-project",
			questionID:   "q-empty",
		},
		baseline,
	)
	if err != nil {
		t.Fatalf("injectChronoLedger: %v", err)
	}
	if len(got) != 1 || got[0] != baseline[0] {
		t.Fatalf("empty project changed context: %q", got)
	}
	if !strings.Contains(logs.String(), "chronology ledger found no event atoms") {
		t.Fatalf("empty project log = %q", logs.String())
	}
}

func TestInjectChronoLedger_ComposesIndependentlyAfterB3Context(t *testing.T) {
	date := time.Date(2024, 1, 11, 0, 0, 0, 0, time.UTC)
	fetcher := &chronoLedgerFetcher{atoms: []atom.Atom{{
		ID:        "event-1",
		Type:      atom.TypeEvent,
		Statement: "Project-wide event.",
		ValidFrom: &date,
	}}}
	b3Blocks := appendEventWindowContext(
		[]string{"raw session"},
		"=== Dated Events (window) ===\nWindow event.\n",
	)

	got, err := injectChronoLedger(
		context.Background(),
		fetcher,
		chronoLedgerRunRequest{
			enabled:      true,
			questionType: "temporal-reasoning",
			project:      "project-a",
			questionID:   "q-compose",
		},
		b3Blocks,
	)
	if err != nil {
		t.Fatalf("injectChronoLedger: %v", err)
	}
	if len(got) != 3 || !strings.Contains(got[0], "=== Event Timeline") || got[1] != "raw session" || !strings.Contains(got[2], "=== Dated Events (window)") {
		t.Fatalf("B3+B4 context order = %q", got)
	}
}

func TestChronoLedgerInject_FlagDefaultsOffAndIsRegistered(t *testing.T) {
	cfg := Config{}
	if cfg.ChronoLedgerInject {
		t.Fatal("ChronoLedgerInject default = true, want false")
	}

	src, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatalf("read main.go: %v", err)
	}
	if !strings.Contains(string(src), `BoolVar(&cfg.ChronoLedgerInject, "chrono-ledger-inject", false`) {
		t.Fatal("--chrono-ledger-inject is not registered as an opt-in boolean flag")
	}
}

func TestChronoLedgerInject_FeatureFlagPersistedOnlyWhenEnabled(t *testing.T) {
	flags := buildFeatureFlags(&Config{ChronoLedgerInject: true})
	if enabled, ok := flags["chrono_ledger_inject"].(bool); !ok || !enabled {
		t.Fatalf("buildFeatureFlags missing chrono_ledger_inject=true: %#v", flags)
	}
	if _, ok := buildFeatureFlags(&Config{})["chrono_ledger_inject"]; ok {
		t.Fatal("buildFeatureFlags persisted chrono_ledger_inject while disabled")
	}
}
