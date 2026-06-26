package db_test

import (
	"context"
	"os"
	"strings"
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

// ── preference-entity DB fields (#1181) — structural tests (no DB required) ──

// TestPostgresAtom_InsertSQL_ContainsEntityFields verifies that postgres_atom.go
// includes polarity/entity/domain in its INSERT statement. Structural source check
// — no DB connection needed. Mirrors TestAtomMode_FlagRegistered pattern.
func TestPostgresAtom_InsertSQL_ContainsEntityFields(t *testing.T) {
	src, err := os.ReadFile("postgres_atom.go")
	if err != nil {
		t.Fatalf("read postgres_atom.go: %v", err)
	}
	text := string(src)
	for _, field := range []string{"polarity", "entity", "domain"} {
		if !strings.Contains(text, field) {
			t.Errorf("postgres_atom.go missing %q — preference-entity fields not added to DB layer (#1181)", field)
		}
	}
}

// TestPostgresAtom_ScanAtomRows_ContainsEntityFields verifies the scan order
// includes the three new columns (structural, no DB).
func TestPostgresAtom_ScanAtomRows_ContainsEntityFields(t *testing.T) {
	src, err := os.ReadFile("postgres_atom.go")
	if err != nil {
		t.Fatalf("read postgres_atom.go: %v", err)
	}
	text := string(src)
	// Must reference the Atom struct fields from scanAtomRows.
	for _, field := range []string{"a.Polarity", "a.Entity", "a.Domain"} {
		if !strings.Contains(text, field) {
			t.Errorf("postgres_atom.go scanAtomRows missing %q — scan will not populate field (#1181)", field)
		}
	}
}
