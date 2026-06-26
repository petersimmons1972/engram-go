package search_test

import (
	"context"
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
	atoms    []atom.Atom
	filtered []atom.Atom
	lastOpts db.AtomQueryOpts
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
	s.lastOpts = opts
	return s.filtered, nil
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

// ── FormatAtomsAsContext with preference-entity fields (#1181) ────────────────

// makeTestAtomWithEntity builds an atom with Polarity/Entity/Domain set.
func makeTestAtomWithEntity(id, polarity, entity, domain, statement string) atom.Atom {
	return atom.Atom{
		ID:        id,
		Type:      atom.TypePreference,
		Subject:   "the user",
		Predicate: "prefers",
		Value:     entity,
		Statement: statement,
		Scope:     atom.ScopeGlobal,
		Confidence: 0.9,
		Polarity:  polarity,
		Entity:    entity,
		Domain:    domain,
	}
}

// TestFormatAtomsAsContext_LikeEntityRendered verifies that a "like" atom with
// an entity surfaces [LIKES: <entity>] in the formatted output.
func TestFormatAtomsAsContext_LikeEntityRendered(t *testing.T) {
	atoms := []atom.Atom{
		makeTestAtomWithEntity("a1", "like", "dark chocolate", "food",
			"The user prefers dark chocolate over milk chocolate."),
	}
	result := search.FormatAtomsAsContext(atoms)
	assert.Contains(t, result, "[LIKES: dark chocolate]",
		"like-polarity entity must appear as [LIKES: <entity>]")
	assert.Contains(t, result, "The user prefers dark chocolate over milk chocolate.")
}

// TestFormatAtomsAsContext_DislikeEntityRendered verifies [AVOIDS: <entity>] for dislike.
func TestFormatAtomsAsContext_DislikeEntityRendered(t *testing.T) {
	atoms := []atom.Atom{
		makeTestAtomWithEntity("a1", "dislike", "cilantro", "food",
			"The user dislikes cilantro."),
	}
	result := search.FormatAtomsAsContext(atoms)
	assert.Contains(t, result, "[AVOIDS: cilantro]",
		"dislike-polarity entity must appear as [AVOIDS: <entity>]")
	assert.Contains(t, result, "The user dislikes cilantro.")
}

// TestFormatAtomsAsContext_NoEntityFallsBack verifies the old "[type] Statement"
// format is used when Entity is empty (back-compat).
func TestFormatAtomsAsContext_NoEntityFallsBack(t *testing.T) {
	atoms := []atom.Atom{
		makeTestAtom("a1", atom.TypePreference, "The user prefers tea.", 0.9),
	}
	result := search.FormatAtomsAsContext(atoms)
	assert.Contains(t, result, "[preference]",
		"atoms without Entity must fall back to [type] format")
	assert.NotContains(t, result, "[LIKES:", "no entity = no LIKES: tag")
	assert.NotContains(t, result, "[AVOIDS:", "no entity = no AVOIDS: tag")
}

// TestFormatAtomsAsContext_EntityWithEmptyPolarity verifies graceful handling
// when Entity is set but Polarity is empty (edge case: model omits polarity).
func TestFormatAtomsAsContext_EntityWithEmptyPolarity(t *testing.T) {
	atoms := []atom.Atom{
		makeTestAtomWithEntity("a1", "", "oolong tea", "food", "The user likes oolong tea."),
	}
	result := search.FormatAtomsAsContext(atoms)
	// Should fall back to [preference] format since polarity is empty.
	assert.Contains(t, result, "[preference]",
		"empty polarity must use fallback format even if entity is set")
	assert.Contains(t, result, "The user likes oolong tea.")
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
