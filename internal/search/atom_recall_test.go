package search_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/petersimmons1972/engram/internal/atom"
	"github.com/petersimmons1972/engram/internal/db"
	"github.com/petersimmons1972/engram/internal/search"
)

// stubAtomBackend satisfies search.AtomBackend for unit tests.
type stubAtomBackend struct {
	atoms         []atom.Atom
	filtered      []atom.Atom
	lastOpts      db.AtomQueryOpts
	filteredCalls int
	err           error
}

func (s *stubAtomBackend) GetActiveAtoms(_ context.Context, _ string, atomType string) ([]atom.Atom, error) {
	if atomType == "" {
		return s.atoms, nil
	}
	var filtered []atom.Atom
	for _, a := range s.atoms {
		if a.Type == atomType {
			filtered = append(filtered, a)
		}
	}
	return filtered, nil
}

func (s *stubAtomBackend) GetActiveAtomsFiltered(_ context.Context, _ string, opts db.AtomQueryOpts) ([]atom.Atom, error) {
	s.filteredCalls++
	s.lastOpts = opts
	filtered := s.filtered
	if opts.Limit > 0 && len(filtered) > opts.Limit {
		filtered = filtered[:opts.Limit]
	}
	return filtered, s.err
}

func makeTestAtom(id, atomType, statement string, confidence float64) atom.Atom {
	return atom.Atom{
		ID:         id,
		Type:       atomType,
		Subject:    "the user",
		Predicate:  "p",
		Value:      "v",
		Statement:  statement,
		Scope:      atom.ScopeGlobal,
		Confidence: confidence,
	}
}

func TestRecallAtoms_FilterByType(t *testing.T) {
	backend := &stubAtomBackend{atoms: []atom.Atom{
		makeTestAtom("a1", atom.TypePreference, "The user prefers tea.", 0.9),
		makeTestAtom("a2", atom.TypeFact, "Alice works at Acme.", 1.0),
		makeTestAtom("a3", atom.TypePreference, "The user dislikes noise.", 0.8),
	}}

	atoms, err := search.RecallAtoms(context.Background(), backend, "proj", search.AtomRecallOpts{
		AtomType: atom.TypePreference,
	})
	require.NoError(t, err)
	assert.Len(t, atoms, 2)
	for _, a := range atoms {
		assert.Equal(t, atom.TypePreference, a.Type)
	}
}

func TestRecallAtoms_NoFilter(t *testing.T) {
	backend := &stubAtomBackend{atoms: []atom.Atom{
		makeTestAtom("a1", atom.TypePreference, "The user prefers tea.", 0.9),
		makeTestAtom("a2", atom.TypeFact, "Alice works at Acme.", 1.0),
	}}

	atoms, err := search.RecallAtoms(context.Background(), backend, "proj", search.AtomRecallOpts{})
	require.NoError(t, err)
	assert.Len(t, atoms, 2)
}

func TestRecallAtoms_SortedByConfidence(t *testing.T) {
	backend := &stubAtomBackend{atoms: []atom.Atom{
		makeTestAtom("a1", atom.TypePreference, "lower conf", 0.5),
		makeTestAtom("a2", atom.TypePreference, "higher conf", 0.95),
		makeTestAtom("a3", atom.TypePreference, "mid conf", 0.75),
	}}

	atoms, err := search.RecallAtoms(context.Background(), backend, "proj", search.AtomRecallOpts{
		AtomType: atom.TypePreference,
	})
	require.NoError(t, err)
	require.Len(t, atoms, 3)
	assert.Greater(t, atoms[0].Confidence, atoms[1].Confidence)
	assert.Greater(t, atoms[1].Confidence, atoms[2].Confidence)
}

func TestRecallAtoms_TopKLimitsResults(t *testing.T) {
	backend := &stubAtomBackend{atoms: []atom.Atom{
		makeTestAtom("a1", atom.TypePreference, "p1", 0.9),
		makeTestAtom("a2", atom.TypePreference, "p2", 0.8),
		makeTestAtom("a3", atom.TypePreference, "p3", 0.7),
		makeTestAtom("a4", atom.TypePreference, "p4", 0.6),
	}}

	atoms, err := search.RecallAtoms(context.Background(), backend, "proj", search.AtomRecallOpts{
		AtomType: atom.TypePreference,
		TopK:     2,
	})
	require.NoError(t, err)
	assert.Len(t, atoms, 2)
	// Should return the top-2 by confidence.
	assert.InDelta(t, 0.9, atoms[0].Confidence, 0.001)
	assert.InDelta(t, 0.8, atoms[1].Confidence, 0.001)
}

func TestRecallAtoms_EmptyBackend(t *testing.T) {
	backend := &stubAtomBackend{}
	atoms, err := search.RecallAtoms(context.Background(), backend, "proj", search.AtomRecallOpts{})
	require.NoError(t, err)
	assert.Empty(t, atoms)
}

func TestFormatAtomsAsContext_ContainsLabels(t *testing.T) {
	atoms := []atom.Atom{
		makeTestAtom("a1", atom.TypePreference, "The user prefers tea.", 0.9),
		makeTestAtom("a2", atom.TypeFact, "Alice works at Acme.", 1.0),
	}

	result := search.FormatAtomsAsContext(atoms)
	assert.Contains(t, result, "[preference]")
	assert.Contains(t, result, "[fact]")
	assert.Contains(t, result, "The user prefers tea.")
	assert.Contains(t, result, "Alice works at Acme.")
	assert.Contains(t, result, "=== Extracted Preference Atoms ===")
}

func TestFormatAtomsAsContext_Empty(t *testing.T) {
	result := search.FormatAtomsAsContext(nil)
	assert.Empty(t, result)
}

func TestFormatAtomsAsContext_NoTrailingGarbage(t *testing.T) {
	atoms := []atom.Atom{
		makeTestAtom("a1", atom.TypePreference, "The user prefers tea.", 0.9),
	}
	result := search.FormatAtomsAsContext(atoms)
	// Should end cleanly (newline after last atom).
	assert.True(t, strings.HasSuffix(result, "\n"), "should end with newline")
}

func TestRecallPreferenceAtoms_LatestAndColdStart(t *testing.T) {
	observedAt := time.Date(2024, 6, 15, 0, 0, 0, 0, time.UTC)
	backend := &stubAtomBackend{
		filtered: []atom.Atom{
			{
				ID:         "a1",
				Type:       atom.TypePreference,
				Subject:    "the user",
				Predicate:  "prefers",
				Value:      "oolong tea",
				Statement:  "The user prefers oolong tea.",
				Scope:      atom.ScopeGlobal,
				Confidence: 0.93,
				ObservedAt: &observedAt,
			},
		},
	}

	asOf := time.Date(2024, 6, 16, 0, 0, 0, 0, time.UTC)
	preamble, err := search.RecallPreferenceAtoms(context.Background(), backend, "proj", "what tea do I like", &asOf)
	require.NoError(t, err)
	require.Contains(t, preamble, "The user prefers oolong tea.")
	require.True(t, backend.lastOpts.LatestOnly, "RecallPreferenceAtoms must request LatestOnly")
	require.Equal(t, atom.TypePreference, backend.lastOpts.AtomType)
	require.NotNil(t, backend.lastOpts.AsOf)
	require.True(t, backend.lastOpts.AsOf.Equal(asOf))

	backend.filtered = nil
	preamble, err = search.RecallPreferenceAtoms(context.Background(), backend, "proj", "what tea do I like", nil)
	require.NoError(t, err)
	assert.Empty(t, preamble, "cold start must return an empty preamble, not an error")
}

func TestRecallEventWindowAtoms_PadsOrdersAndCaps(t *testing.T) {
	windowSince := time.Date(2024, 1, 10, 18, 45, 0, 0, time.FixedZone("west", -5*60*60))
	windowBefore := time.Date(2024, 1, 12, 9, 30, 0, 0, time.FixedZone("east", 9*60*60))
	backend := &stubAtomBackend{}
	for i := 0; i <= 41; i++ {
		validFrom := time.Date(2024, 1, 3+i, 12, 0, 0, 0, time.UTC)
		backend.filtered = append(backend.filtered, atom.Atom{
			ID:         fmt.Sprintf("event-%02d", i),
			Type:       atom.TypeEvent,
			Statement:  fmt.Sprintf("Event %02d", i),
			ValidFrom:  &validFrom,
			Confidence: 1,
		})
	}

	result, err := search.RecallEventWindowIncludingSuperseded(
		context.Background(),
		backend,
		"project-a",
		&windowSince,
		&windowBefore,
	)
	require.NoError(t, err)
	block := result.Context
	require.NotEmpty(t, block)
	require.Contains(t, block, "=== Dated Events (window) ===")
	require.NotContains(t, block, "=== Extracted Preference Atoms ===")
	require.Equal(t, atom.TypeEvent, backend.lastOpts.AtomType)
	require.True(t, backend.lastOpts.IncludeSuperseded)
	require.Equal(t, 41, backend.lastOpts.Limit)
	require.NotNil(t, backend.lastOpts.ValidFromSince)
	require.NotNil(t, backend.lastOpts.ValidFromBefore)
	require.Equal(t, "2024-01-03", backend.lastOpts.ValidFromSince.Format(time.DateOnly))
	require.Equal(t, "2024-01-19", backend.lastOpts.ValidFromBefore.Format(time.DateOnly))
	require.Equal(t, time.UTC, backend.lastOpts.ValidFromSince.Location())
	require.Equal(t, time.UTC, backend.lastOpts.ValidFromBefore.Location())
	require.Less(t, strings.Index(block, "Event 00"), strings.Index(block, "Event 01"))
	require.Contains(t, block, "Event 29")
	require.NotContains(t, block, "Event 30")
	require.Contains(t, block, "(+more events truncated)")
	require.Len(t, result.Atoms, 41, "B4 ledger input must retain the truncation sentinel")
	require.Equal(t, "event-40", result.Atoms[40].ID)
}

func TestRecallEventWindow_ReturnsFetchedAtomsWithContextFromOneQuery(t *testing.T) {
	validFrom := time.Date(2024, 1, 11, 12, 0, 0, 0, time.UTC)
	backend := &stubAtomBackend{filtered: []atom.Atom{{
		ID:        "event-1",
		Type:      atom.TypeEvent,
		Statement: "Visited the dentist.",
		ValidFrom: &validFrom,
	}}}
	since := time.Date(2024, 1, 10, 0, 0, 0, 0, time.UTC)
	before := since.AddDate(0, 0, 2)

	result, err := search.RecallEventWindow(
		context.Background(),
		backend,
		"project-a",
		&since,
		&before,
	)

	require.NoError(t, err)
	require.Contains(t, result.Context, "Visited the dentist.")
	require.Equal(t, backend.filtered, result.Atoms)
	require.Equal(t, 1, backend.filteredCalls, "the context and atoms must share one database query")
	require.False(t, backend.lastOpts.IncludeSuperseded, "B3-only recall must preserve active-only behavior")
	require.Equal(t, 31, backend.lastOpts.Limit, "B3-only recall must preserve its existing query cap")
}

func TestRecallEventWindowAtoms_BypassAndFailurePaths(t *testing.T) {
	t.Run("missing window does not query or change context", func(t *testing.T) {
		backend := &stubAtomBackend{filtered: []atom.Atom{makeTestAtom("a1", atom.TypeEvent, "event", 1)}}

		block, err := search.RecallEventWindowAtoms(context.Background(), backend, "project-a", nil, nil)

		require.NoError(t, err)
		require.Empty(t, block)
		require.Equal(t, db.AtomQueryOpts{}, backend.lastOpts)
	})

	t.Run("no atoms is a quiet no-op", func(t *testing.T) {
		since := time.Date(2024, 1, 10, 0, 0, 0, 0, time.UTC)
		before := since.AddDate(0, 0, 1)
		block, err := search.RecallEventWindowAtoms(context.Background(), &stubAtomBackend{}, "project-a", &since, &before)
		require.NoError(t, err)
		require.Empty(t, block)
	})

	t.Run("backend error fails loudly", func(t *testing.T) {
		since := time.Date(2024, 1, 10, 0, 0, 0, 0, time.UTC)
		before := since.AddDate(0, 0, 1)
		backend := &stubAtomBackend{err: errors.New("query failed")}
		block, err := search.RecallEventWindowAtoms(context.Background(), backend, "project-a", &since, &before)
		require.ErrorContains(t, err, "query failed")
		require.Empty(t, block)
	})
}
