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

// ── preference-entity DB fields (#1181) ──────────────────────────────────────

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

// TestInsertAtom_PreferenceEntityFieldsRoundTrip is an integration + adversarial
// test: polarity/entity/domain must survive InsertAtom → GetActiveAtoms, and
// empty optional fields must not break legacy fact atoms in the same project.
func TestInsertAtom_PreferenceEntityFieldsRoundTrip(t *testing.T) {
	proj := uniqueProject("atoms-pref-entity")
	ctx := context.Background()

	backend, err := db.NewPostgresBackend(ctx, proj, testDSN(t))
	require.NoError(t, err)
	t.Cleanup(func() { backend.Close() })

	require.NoError(t, backend.InsertAtom(ctx, &atom.Atom{
		Project:    proj,
		Type:       atom.TypePreference,
		Subject:    "the user",
		Predicate:  "prefers",
		Value:      "dark chocolate",
		Statement:  "The user prefers dark chocolate.",
		Scope:      atom.ScopeGlobal,
		Confidence: 0.91,
		Polarity:   "like",
		Entity:     "dark chocolate",
		Domain:     "food",
	}))
	require.NoError(t, backend.InsertAtom(ctx, &atom.Atom{
		Project:    proj,
		Type:       atom.TypePreference,
		Subject:    "the user",
		Predicate:  "dislikes",
		Value:      "cilantro",
		Statement:  "The user dislikes cilantro.",
		Scope:      atom.ScopeGlobal,
		Confidence: 0.88,
		Polarity:   "dislike",
		Entity:     "cilantro",
		Domain:     "food",
	}))
	// Legacy atom without entity fields — empty must round-trip as empty.
	require.NoError(t, backend.InsertAtom(ctx, &atom.Atom{
		Project:    proj,
		Type:       atom.TypeFact,
		Subject:    "Alice",
		Predicate:  "works at",
		Value:      "Acme",
		Statement:  "Alice works at Acme.",
		Scope:      atom.ScopeGlobal,
		Confidence: 1.0,
	}))

	prefs, err := backend.GetActiveAtoms(ctx, proj, atom.TypePreference)
	require.NoError(t, err)
	require.Len(t, prefs, 2)

	byEntity := map[string]atom.Atom{}
	for _, a := range prefs {
		byEntity[a.Entity] = a
	}
	like := byEntity["dark chocolate"]
	require.Equal(t, "like", like.Polarity)
	require.Equal(t, "food", like.Domain)
	dislike := byEntity["cilantro"]
	require.Equal(t, "dislike", dislike.Polarity)
	require.Equal(t, "food", dislike.Domain)

	facts, err := backend.GetActiveAtoms(ctx, proj, atom.TypeFact)
	require.NoError(t, err)
	require.Len(t, facts, 1)
	require.Empty(t, facts[0].Polarity)
	require.Empty(t, facts[0].Entity)
	require.Empty(t, facts[0].Domain)
}

// TestInsertAtom_InvalidPolarityFailsLoudly is adversarial: the DB CHECK must
// reject out-of-set polarity values rather than silently accepting them.
func TestInsertAtom_InvalidPolarityFailsLoudly(t *testing.T) {
	proj := uniqueProject("atoms-pref-bad-polarity")
	ctx := context.Background()

	backend, err := db.NewPostgresBackend(ctx, proj, testDSN(t))
	require.NoError(t, err)
	t.Cleanup(func() { backend.Close() })

	err = backend.InsertAtom(ctx, &atom.Atom{
		Project:    proj,
		Type:       atom.TypePreference,
		Subject:    "the user",
		Predicate:  "prefers",
		Value:      "matcha",
		Statement:  "The user prefers matcha.",
		Scope:      atom.ScopeGlobal,
		Confidence: 0.9,
		Polarity:   "love", // not in {like,dislike,""}
		Entity:     "matcha",
		Domain:     "food",
	})
	require.Error(t, err, "invalid polarity must fail the INSERT (CHECK constraint)")
}
