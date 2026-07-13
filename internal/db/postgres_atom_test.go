package db_test

import (
	"context"
	"testing"
	"time"

	"github.com/petersimmons1972/engram/internal/atom"
	"github.com/petersimmons1972/engram/internal/db"
	"github.com/stretchr/testify/require"
)

func TestGetActiveAtomsFiltered_LatestOnly_AsOf(t *testing.T) {
	proj := uniqueProject("atoms-filtered")
	ctx := context.Background()

	backend, err := db.NewPostgresBackend(ctx, proj, testDSN(t))
	require.NoError(t, err)
	t.Cleanup(func() { backend.Close() })

	earlier := time.Date(2024, 1, 10, 9, 0, 0, 0, time.UTC)
	later := time.Date(2024, 2, 20, 9, 0, 0, 0, time.UTC)
	other := time.Date(2024, 2, 5, 12, 0, 0, 0, time.UTC)

	require.NoError(t, backend.InsertAtom(ctx, &atom.Atom{
		Project:    proj,
		Type:       atom.TypePreference,
		Subject:    "the user",
		Predicate:  "prefers_drink",
		Value:      "coffee",
		Statement:  "The user prefers coffee.",
		Scope:      atom.ScopeGlobal,
		Confidence: 0.9,
		ObservedAt: &earlier,
	}))
	require.NoError(t, backend.InsertAtom(ctx, &atom.Atom{
		Project:    proj,
		Type:       atom.TypePreference,
		Subject:    "the user",
		Predicate:  "prefers_drink",
		Value:      "tea",
		Statement:  "The user prefers tea.",
		Scope:      atom.ScopeGlobal,
		Confidence: 0.95,
		ObservedAt: &later,
	}))
	require.NoError(t, backend.InsertAtom(ctx, &atom.Atom{
		Project:    proj,
		Type:       atom.TypePreference,
		Subject:    "the user",
		Predicate:  "favorite_color",
		Value:      "green",
		Statement:  "The user's favorite color is green.",
		Scope:      atom.ScopeGlobal,
		Confidence: 0.8,
		ObservedAt: &other,
	}))

	atoms, err := backend.GetActiveAtomsFiltered(ctx, proj, db.AtomQueryOpts{
		AtomType:   atom.TypePreference,
		LatestOnly: true,
	})
	require.NoError(t, err)
	require.Len(t, atoms, 2, "LatestOnly must return one row per (subject,predicate)")
	require.Equal(t, "tea", valueForPredicate(t, atoms, "prefers_drink"))
	require.Equal(t, "green", valueForPredicate(t, atoms, "favorite_color"))

	cutoff := time.Date(2024, 1, 31, 0, 0, 0, 0, time.UTC)
	asOfAtoms, err := backend.GetActiveAtomsFiltered(ctx, proj, db.AtomQueryOpts{
		AtomType:   atom.TypePreference,
		AsOf:       &cutoff,
		LatestOnly: true,
	})
	require.NoError(t, err)
	require.Len(t, asOfAtoms, 1, "AsOf must exclude atoms observed after the cutoff")
	require.Equal(t, "coffee", valueForPredicate(t, asOfAtoms, "prefers_drink"))
}

func TestGetActiveAtomsFiltered_ValidFromBoundsWithLatestOnlyAndAsOf(t *testing.T) {
	proj := uniqueProject("atoms-valid-from-filtered")
	ctx := context.Background()

	backend, err := db.NewPostgresBackend(ctx, proj, testDSN(t))
	require.NoError(t, err)
	t.Cleanup(func() { backend.Close() })

	since := time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC)
	before := time.Date(2024, 3, 1, 0, 0, 0, 0, time.UTC)
	asOf := time.Date(2024, 2, 20, 0, 0, 0, 0, time.UTC)
	fixtures := []struct {
		value      string
		validFrom  time.Time
		observedAt time.Time
	}{
		{value: "too-early", validFrom: since.Add(-time.Hour), observedAt: asOf.Add(-48 * time.Hour)},
		{value: "in-window-old", validFrom: since, observedAt: asOf.Add(-24 * time.Hour)},
		{value: "in-window-new", validFrom: before.Add(-time.Hour), observedAt: asOf.Add(-time.Hour)},
		{value: "observed-too-late", validFrom: since.Add(24 * time.Hour), observedAt: asOf.Add(time.Hour)},
		{value: "at-before", validFrom: before, observedAt: asOf.Add(-time.Hour)},
	}
	for _, fixture := range fixtures {
		validFrom := fixture.validFrom
		observedAt := fixture.observedAt
		require.NoError(t, backend.InsertAtom(ctx, &atom.Atom{
			Project: proj, Type: atom.TypeEvent, Subject: "the user", Predicate: "visited",
			Value: fixture.value, Statement: fixture.value, Scope: atom.ScopeGlobal,
			Confidence: 1, ValidFrom: &validFrom, ObservedAt: &observedAt,
		}))
	}

	atoms, err := backend.GetActiveAtomsFiltered(ctx, proj, db.AtomQueryOpts{
		AtomType: atom.TypeEvent, AsOf: &asOf, ValidFromSince: &since,
		ValidFromBefore: &before, LatestOnly: true,
	})
	require.NoError(t, err)
	require.Len(t, atoms, 1)
	require.Equal(t, "in-window-new", atoms[0].Value)
}

func valueForPredicate(t *testing.T, atoms []atom.Atom, predicate string) string {
	t.Helper()
	for _, a := range atoms {
		if a.Predicate == predicate {
			return a.Value
		}
	}
	t.Fatalf("predicate %q not found in result set", predicate)
	return ""
}

func TestGetChronoLedgerAtoms_IncludesSupersededAndFiltersTimelineTypes(t *testing.T) {
	project := uniqueProject("chrono-ledger")
	ctx := context.Background()
	backend, err := db.NewPostgresBackend(ctx, project, testDSN(t))
	require.NoError(t, err)
	t.Cleanup(func() { backend.Close() })

	jan1 := time.Date(2024, 1, 1, 9, 0, 0, 0, time.UTC)
	jan2 := time.Date(2024, 1, 2, 9, 0, 0, 0, time.UTC)
	jan3 := time.Date(2024, 1, 3, 9, 0, 0, 0, time.UTC)
	oldStatus := &atom.Atom{
		Project: project, Type: atom.TypeStatusChange, Subject: "deploy", Predicate: "status",
		Value: "running", Statement: "The deploy was running.", Scope: atom.ScopeGlobal,
		Confidence: 1, ValidFrom: &jan1,
	}
	require.NoError(t, backend.InsertAtom(ctx, oldStatus))
	require.NoError(t, backend.RetireAtom(ctx, oldStatus.ID, jan2, &atom.Atom{
		Project: project, Type: atom.TypeStatusChange, Subject: "deploy", Predicate: "status",
		Value: "done", Statement: "The deploy was done.", Scope: atom.ScopeGlobal,
		Confidence: 1, ValidFrom: &jan2, Supersedes: oldStatus.ID,
	}))
	require.NoError(t, backend.InsertAtom(ctx, &atom.Atom{
		Project: project, Type: atom.TypeEvent, Subject: "user", Predicate: "visited",
		Value: "Boston", Statement: "The user visited Boston.", Scope: atom.ScopeGlobal,
		Confidence: 1, ValidFrom: &jan3,
	}))
	require.NoError(t, backend.InsertAtom(ctx, &atom.Atom{
		Project: project, Type: atom.TypePreference, Subject: "user", Predicate: "likes",
		Value: "tea", Statement: "The user likes tea.", Scope: atom.ScopeGlobal,
		Confidence: 1, ValidFrom: &jan1,
	}))
	require.NoError(t, backend.InsertAtom(ctx, &atom.Atom{
		Project: project, Type: atom.TypeEvent, Subject: "user", Predicate: "planned",
		Value: "trip", Statement: "The user planned a trip.", Scope: atom.ScopeGlobal,
		Confidence: 1,
	}))

	atoms, err := backend.GetChronoLedgerAtoms(ctx, project, 41)
	require.NoError(t, err)
	require.Len(t, atoms, 3)
	require.Equal(t, "The deploy was running.", atoms[0].Statement)
	require.NotNil(t, atoms[0].ValidTo, "superseded status row must be present")
	require.Equal(t, "The deploy was done.", atoms[1].Statement)
	require.Equal(t, "The user visited Boston.", atoms[2].Statement)
}
