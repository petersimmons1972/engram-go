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
