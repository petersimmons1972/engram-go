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
	// Two candidates with the same normalized key and value in the same batch.
	candidates := []atom.Atom{
		makeAtom("", "proj", "the user", "prefers", "coffee"),
		makeAtom("", "proj", "the user", "prefers", " COFFEE "),
	}

	result := atom.Deduplicate(nil, candidates, testNow)
	// First one wins; second is dropped as an exact intra-batch duplicate.
	assert.Len(t, result.Fresh, 1)
	assert.Equal(t, "coffee", result.Fresh[0].Value)
}

func TestDeduplicate_SameValueEventsOnDifferentDatesWithinBatchAreFresh(t *testing.T) {
	first := makeAtom("", "proj", "Alice", "attended", "Go meetup")
	first.Type = atom.TypeEvent
	first.ValidFrom = timePointer(time.Date(2026, 7, 4, 18, 0, 0, 0, time.FixedZone("EDT", -4*60*60)))
	second := makeAtom("", "proj", "Alice", "attended", "go MEETUP")
	second.Type = atom.TypeEvent
	second.ValidFrom = timePointer(time.Date(2026, 7, 11, 9, 0, 0, 0, time.UTC))

	result := atom.Deduplicate(nil, []atom.Atom{first, second}, testNow)

	assert.Len(t, result.Fresh, 2)
	assert.Empty(t, result.Superseded)
}

func TestDeduplicate_DifferentNonEventValuesWithinBatchChain(t *testing.T) {
	first := makeAtom("first", "proj", "Alice", "employment", "Acme")
	second := makeAtom("second", "proj", "Alice", "employment", "Globex")

	result := atom.Deduplicate(nil, []atom.Atom{first, second}, testNow)

	require.Len(t, result.Fresh, 1)
	require.Len(t, result.Superseded, 1)
	assert.Equal(t, result.Fresh[0].ID, result.Superseded[0].New.Supersedes)
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

func TestDeduplicate_TypeAwareSupersessionKey(t *testing.T) {
	existing := makeAtom("event-1", "proj", "deploy", "state", "started")
	existing.Type = atom.TypeEvent
	candidate := makeAtom("status-1", "proj", "deploy", "state", "complete")
	candidate.Type = atom.TypeStatusChange

	result := atom.Deduplicate([]atom.Atom{existing}, []atom.Atom{candidate}, testNow)

	require.Len(t, result.Fresh, 1)
	assert.Equal(t, "status-1", result.Fresh[0].ID)
	assert.Empty(t, result.Superseded)
}

func TestDeduplicate_ConfidenceGate(t *testing.T) {
	tests := []struct {
		name                string
		candidateConfidence float64
		wantSuperseded      bool
	}{
		{name: "exact boundary supersedes", candidateConfidence: 0.7, wantSuperseded: true},
		{name: "computed boundary supersedes", candidateConfidence: 0.73 - 0.2, wantSuperseded: true},
		{name: "below boundary coexists", candidateConfidence: 0.699, wantSuperseded: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			existing := makeAtom("old-1", "proj", "the user", "prefers", "coffee")
			existing.Confidence = 0.9
			if tt.name == "computed boundary supersedes" {
				existing.Confidence = 0.73
			}
			candidate := makeAtom("new-1", "proj", "the user", "prefers", "tea")
			candidate.Confidence = tt.candidateConfidence

			result := atom.Deduplicate([]atom.Atom{existing}, []atom.Atom{candidate}, testNow)

			if tt.wantSuperseded {
				require.Len(t, result.Superseded, 1)
				assert.Empty(t, result.Fresh)
				return
			}
			require.Len(t, result.Fresh, 1)
			assert.Empty(t, result.Superseded)
			assert.Empty(t, result.Fresh[0].Supersedes)
		})
	}
}

func TestDeduplicate_RetiresAtCandidateAssertionTime(t *testing.T) {
	existingObservedAt := time.Date(2024, 2, 2, 0, 0, 0, 0, time.UTC)
	observedAt := time.Date(2024, 2, 3, 0, 0, 0, 0, time.UTC)
	existing := makeAtom("old-1", "proj", "the user", "prefers", "coffee")
	existing.ObservedAt = &existingObservedAt
	candidate := makeAtom("new-1", "proj", "the user", "prefers", "tea")
	candidate.ObservedAt = &observedAt

	result := atom.Deduplicate([]atom.Atom{existing}, []atom.Atom{candidate}, testNow)

	require.Len(t, result.Superseded, 1)
	require.NotNil(t, result.Superseded[0].Old.ValidTo)
	assert.Equal(t, observedAt, *result.Superseded[0].Old.ValidTo)
}

func TestDeduplicate_StatusChangesChainByAssertionTime(t *testing.T) {
	firstTime := time.Date(2026, 7, 10, 0, 0, 0, 0, time.UTC)
	lastTime := time.Date(2026, 7, 12, 0, 0, 0, 0, time.UTC)
	first := makeAtom("first", "proj", "release", "status", "queued")
	first.Type = atom.TypeStatusChange
	first.ObservedAt = &firstTime
	last := makeAtom("last", "proj", "release", "status", "complete")
	last.Type = atom.TypeStatusChange
	last.ObservedAt = &lastTime
	middle := makeAtom("middle", "proj", "release", "status", "running")
	middle.Type = atom.TypeStatusChange
	middle.ObservedAt = &firstTime

	result := atom.Deduplicate(nil, []atom.Atom{last, first, middle}, testNow)

	require.Len(t, result.Superseded, 2)
	assert.Equal(t, "first", result.Superseded[0].Old.ID)
	assert.Equal(t, "first", result.Superseded[0].New.Supersedes)
	assert.Equal(t, "middle", result.Superseded[0].New.ID, "equal assertion times preserve input order")
	assert.Equal(t, "middle", result.Superseded[1].Old.ID)
	assert.Equal(t, "middle", result.Superseded[1].New.Supersedes)
	assert.Equal(t, "last", result.Superseded[1].New.ID)
	assert.Equal(t, firstTime, *result.Superseded[0].Old.ValidTo)
	assert.Equal(t, lastTime, *result.Superseded[1].Old.ValidTo)
	require.Len(t, result.Fresh, 1, "the first status must be inserted before it can be retired")
	assert.Equal(t, "first", result.Fresh[0].ID)
}

func TestDeduplicate_EventsNeverSupersedeExistingDifferentValue(t *testing.T) {
	existing := makeAtom("event-1", "proj", "release", "deployed", "v1")
	existing.Type = atom.TypeEvent
	candidate := makeAtom("event-2", "proj", "release", "deployed", "v2")
	candidate.Type = atom.TypeEvent

	result := atom.Deduplicate([]atom.Atom{existing}, []atom.Atom{candidate}, testNow)

	require.Len(t, result.Fresh, 1)
	assert.Empty(t, result.Superseded)
	assert.Empty(t, result.Fresh[0].Supersedes)
}

func TestDeduplicate_NonStatusCandidatesChainWithinBatch(t *testing.T) {
	oldTime := time.Date(2026, 7, 9, 0, 0, 0, 0, time.UTC)
	firstTime := time.Date(2026, 7, 10, 0, 0, 0, 0, time.UTC)
	secondTime := time.Date(2026, 7, 11, 0, 0, 0, 0, time.UTC)
	existing := makeAtom("old", "proj", "user", "drink", "coffee")
	existing.ObservedAt = &oldTime
	first := makeAtom("first", "proj", "user", "drink", "tea")
	first.ObservedAt = &firstTime
	second := makeAtom("second", "proj", "user", "drink", "water")
	second.ObservedAt = &secondTime

	result := atom.Deduplicate([]atom.Atom{existing}, []atom.Atom{first, second}, testNow)

	require.Len(t, result.Superseded, 2)
	assert.Equal(t, "old", result.Superseded[0].Old.ID)
	assert.Equal(t, "old", result.Superseded[0].New.Supersedes)
	assert.Equal(t, "first", result.Superseded[1].Old.ID)
	assert.Equal(t, "first", result.Superseded[1].New.Supersedes)
}

func TestDeduplicate_BackfilledNonStatusCandidateCoexists(t *testing.T) {
	validFrom := time.Date(2026, 7, 12, 0, 0, 0, 0, time.UTC)
	backfilledAt := time.Date(2026, 7, 10, 0, 0, 0, 0, time.UTC)
	existing := makeAtom("current", "proj", "service", "region", "east")
	existing.Type = atom.TypeAttribute
	existing.ValidFrom = &validFrom
	candidate := makeAtom("backfilled", "proj", "service", "region", "west")
	candidate.Type = atom.TypeAttribute
	candidate.ObservedAt = &backfilledAt

	result := atom.Deduplicate([]atom.Atom{existing}, []atom.Atom{candidate}, testNow)

	require.Len(t, result.Fresh, 1)
	assert.Equal(t, "backfilled", result.Fresh[0].ID)
	assert.Empty(t, result.Fresh[0].Supersedes)
	assert.Empty(t, result.Superseded)
}

func TestDeduplicate_StaleStatusDoesNotSupersedeCurrentStatus(t *testing.T) {
	currentTime := time.Date(2026, 7, 12, 0, 0, 0, 0, time.UTC)
	staleTime := time.Date(2026, 7, 10, 0, 0, 0, 0, time.UTC)
	current := makeAtom("current", "proj", "release", "status", "complete")
	current.Type = atom.TypeStatusChange
	current.ObservedAt = &currentTime
	stale := makeAtom("stale", "proj", "release", "status", "queued")
	stale.Type = atom.TypeStatusChange
	stale.ObservedAt = &staleTime

	result := atom.Deduplicate([]atom.Atom{current}, []atom.Atom{stale}, testNow)

	require.Len(t, result.Fresh, 1)
	assert.Equal(t, "stale", result.Fresh[0].ID)
	assert.Empty(t, result.Superseded)
}

func TestDeduplicate_StaleLowConfidenceStatusCoexists(t *testing.T) {
	currentTime := time.Date(2026, 7, 12, 0, 0, 0, 0, time.UTC)
	staleTime := time.Date(2026, 7, 10, 0, 0, 0, 0, time.UTC)
	current := makeAtom("current", "proj", "release", "status", "complete")
	current.Type = atom.TypeStatusChange
	current.Confidence = 0.9
	current.ObservedAt = &currentTime
	stale := makeAtom("stale-low", "proj", "release", "status", "queued")
	stale.Type = atom.TypeStatusChange
	stale.Confidence = 0.1
	stale.ObservedAt = &staleTime

	result := atom.Deduplicate([]atom.Atom{current}, []atom.Atom{stale}, testNow)

	require.Len(t, result.Fresh, 1)
	assert.Equal(t, "stale-low", result.Fresh[0].ID)
	assert.Empty(t, result.Superseded)
}

func TestDeduplicate_SelectsNewestActivePredecessor(t *testing.T) {
	oldTime := time.Date(2026, 7, 9, 0, 0, 0, 0, time.UTC)
	newTime := time.Date(2026, 7, 11, 0, 0, 0, 0, time.UTC)
	candidateTime := time.Date(2026, 7, 12, 0, 0, 0, 0, time.UTC)
	old := makeAtom("old", "proj", "service", "region", "east")
	old.Type = atom.TypeAttribute
	old.ObservedAt = &oldTime
	current := makeAtom("current", "proj", "service", "region", "west")
	current.Type = atom.TypeAttribute
	current.ObservedAt = &newTime
	candidate := makeAtom("candidate", "proj", "service", "region", "central")
	candidate.Type = atom.TypeAttribute
	candidate.ObservedAt = &candidateTime

	result := atom.Deduplicate([]atom.Atom{current, old}, []atom.Atom{candidate}, testNow)

	require.Len(t, result.Superseded, 1)
	assert.Equal(t, "current", result.Superseded[0].Old.ID)
	assert.Equal(t, "current", result.Superseded[0].New.Supersedes)
}
