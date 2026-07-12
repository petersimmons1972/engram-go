package atom_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/petersimmons1972/engram/internal/atom"
)

var testNow = time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)

func makeAtom(id, project, subject, predicate, value string) atom.Atom {
	return atom.Atom{
		ID:         id,
		Project:    project,
		Type:       atom.TypePreference,
		Subject:    subject,
		Predicate:  predicate,
		Value:      value,
		Statement:  subject + " " + predicate + " " + value + ".",
		Scope:      atom.ScopeGlobal,
		Confidence: 0.9,
	}
}

func TestDeduplicate_FreshAtom(t *testing.T) {
	existing := []atom.Atom{}
	candidates := []atom.Atom{
		makeAtom("", "proj", "the user", "prefers", "dark chocolate"),
	}

	result := atom.Deduplicate(existing, candidates, testNow)
	assert.Len(t, result.Fresh, 1)
	assert.Empty(t, result.Superseded)
	assert.Equal(t, "dark chocolate", result.Fresh[0].Value)
}

func TestDeduplicate_IdenticalValueReinforced(t *testing.T) {
	existing := []atom.Atom{
		makeAtom("old-1", "proj", "the user", "prefers", "dark chocolate"),
	}
	candidates := []atom.Atom{
		makeAtom("", "proj", "the user", "prefers", "dark chocolate"),
	}

	result := atom.Deduplicate(existing, candidates, testNow)
	// Same value → reinforce only, no new insert.
	assert.Empty(t, result.Fresh)
	assert.Empty(t, result.Superseded)
}

func TestB1CorruptionProbeSameValueDifferentDateEventsAreFresh(t *testing.T) {
	existing := makeAtom("old-1", "proj", "the user", "attended", "Go meetup")
	existing.Type = atom.TypeEvent
	existingDate := time.Date(2026, 7, 4, 0, 0, 0, 0, time.UTC)
	existing.ValidFrom = &existingDate
	candidate := makeAtom("", "proj", "the user", "attended", "Go meetup")
	candidate.Type = atom.TypeEvent
	candidateDate := time.Date(2026, 7, 11, 0, 0, 0, 0, time.UTC)
	candidate.ValidFrom = &candidateDate

	result := atom.Deduplicate([]atom.Atom{existing}, []atom.Atom{candidate}, testNow)

	require.Len(t, result.Fresh, 1, "a recurring event on a different date must be inserted")
	assert.Empty(t, result.Superseded, "a recurring event must not retire the earlier occurrence")
}

func TestDeduplicateSameValueSameDateEventIsReinforced(t *testing.T) {
	eventDate := time.Date(2026, 7, 4, 0, 0, 0, 0, time.UTC)
	existing := makeAtom("old-1", "proj", "the user", "attended", "Go meetup")
	existing.Type = atom.TypeEvent
	existing.ValidFrom = timePointer(eventDate)
	candidate := makeAtom("", "proj", "the user", "attended", "Go meetup")
	candidate.Type = atom.TypeEvent
	candidate.ValidFrom = timePointer(eventDate)

	result := atom.Deduplicate([]atom.Atom{existing}, []atom.Atom{candidate}, testNow)

	assert.Empty(t, result.Fresh)
	assert.Empty(t, result.Superseded)
}

func TestDeduplicateSameValueStatusChangeWithoutExplicitEventDateIsReinforced(t *testing.T) {
	existingDate := time.Date(2026, 7, 4, 0, 0, 0, 0, time.UTC)
	existing := makeAtom("old-1", "proj", "the user", "employment status", "employed")
	existing.Type = atom.TypeStatusChange
	existing.ValidFrom = timePointer(existingDate)
	candidateDate := time.Date(2026, 7, 11, 0, 0, 0, 0, time.UTC)
	candidate := makeAtom("", "proj", "the user", "employment status", "employed")
	candidate.Type = atom.TypeStatusChange
	candidate.ValidFrom = timePointer(candidateDate)

	result := atom.Deduplicate([]atom.Atom{existing}, []atom.Atom{candidate}, testNow)

	assert.Empty(t, result.Fresh, "a restated status change must not create another active row")
	assert.Empty(t, result.Superseded, "a restated status change is reinforcement, not supersession")
}

func TestDeduplicate_SupersessionOnValueChange(t *testing.T) {
	existing := []atom.Atom{
		makeAtom("old-1", "proj", "the user", "prefers", "dark chocolate"),
	}
	candidates := []atom.Atom{
		makeAtom("", "proj", "the user", "prefers", "milk chocolate"),
	}

	result := atom.Deduplicate(existing, candidates, testNow)
	assert.Empty(t, result.Fresh)
	require.Len(t, result.Superseded, 1)

	pair := result.Superseded[0]
	// Old atom should be retired.
	assert.Equal(t, "old-1", pair.Old.ID)
	require.NotNil(t, pair.Old.ValidTo)
	assert.Equal(t, testNow, *pair.Old.ValidTo)
	// New atom should link back to old.
	assert.Equal(t, "old-1", pair.New.Supersedes)
	assert.Equal(t, "milk chocolate", pair.New.Value)
}

func TestDeduplicate_CaseInsensitiveValueMatch(t *testing.T) {
	existing := []atom.Atom{
		makeAtom("old-1", "proj", "the user", "prefers", "Dark Chocolate"),
	}
	candidates := []atom.Atom{
		makeAtom("", "proj", "the user", "prefers", "dark chocolate"),
	}

	result := atom.Deduplicate(existing, candidates, testNow)
	// Case-insensitive match → reinforce only.
	assert.Empty(t, result.Fresh)
	assert.Empty(t, result.Superseded)
}

func TestDeduplicate_CaseInsensitivePredicate(t *testing.T) {
	existing := []atom.Atom{
		makeAtom("old-1", "proj", "The User", "PREFERS", "coffee"),
	}
	candidates := []atom.Atom{
		makeAtom("", "proj", "the user", "prefers", "tea"),
	}

	result := atom.Deduplicate(existing, candidates, testNow)
	// Subject+predicate match (case-insensitive) → supersession.
	require.Len(t, result.Superseded, 1)
	assert.Equal(t, "old-1", result.Superseded[0].Old.ID)
}

func TestDeduplicate_DifferentPredicateBothFresh(t *testing.T) {
	existing := []atom.Atom{
		makeAtom("old-1", "proj", "the user", "prefers", "dark chocolate"),
	}
	candidates := []atom.Atom{
		makeAtom("", "proj", "the user", "dislikes", "spicy food"),
	}

	result := atom.Deduplicate(existing, candidates, testNow)
	// Different predicate → fresh insert.
	assert.Len(t, result.Fresh, 1)
	assert.Empty(t, result.Superseded)
}

func TestDeduplicate_DuplicateCandidateWithinBatch(t *testing.T) {
	// Two candidates with the same (subject, predicate) in the same batch.
	candidates := []atom.Atom{
		makeAtom("", "proj", "the user", "prefers", "coffee"),
		makeAtom("", "proj", "the user", "prefers", "tea"),
	}

	result := atom.Deduplicate(nil, candidates, testNow)
	// First one wins; second is dropped as intra-batch duplicate.
	assert.Len(t, result.Fresh, 1)
	assert.Equal(t, "coffee", result.Fresh[0].Value)
}

func TestDeduplicate_EmptyInputs(t *testing.T) {
	result := atom.Deduplicate(nil, nil, testNow)
	assert.Empty(t, result.Fresh)
	assert.Empty(t, result.Superseded)
}

func TestDeduplicate_MultipleProjects(t *testing.T) {
	// Atoms from different projects should not interfere.
	existing := []atom.Atom{
		makeAtom("old-1", "proj-A", "the user", "prefers", "coffee"),
	}
	candidates := []atom.Atom{
		// Same subject+predicate but different project — should be fresh.
		makeAtom("", "proj-B", "the user", "prefers", "tea"),
	}
	for i := range candidates {
		candidates[i].Project = "proj-B"
	}

	result := atom.Deduplicate(existing, candidates, testNow)
	assert.Len(t, result.Fresh, 1)
	assert.Empty(t, result.Superseded)
}
