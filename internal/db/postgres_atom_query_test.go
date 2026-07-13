package db

import (
	"strings"
	"testing"
	"time"

	"github.com/petersimmons1972/engram/internal/atom"
	"github.com/stretchr/testify/require"
)

func TestBuildActiveAtomsQuery_PlaceholderOrderingAndLimit(t *testing.T) {
	asOf := time.Date(2024, 3, 1, 0, 0, 0, 0, time.UTC)
	since := time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC)
	before := time.Date(2024, 2, 29, 0, 0, 0, 0, time.UTC)

	query, args := buildActiveAtomsQuery("project-a", AtomQueryOpts{
		AtomType: atom.TypeEvent, AsOf: &asOf, ValidFromSince: &since,
		ValidFromBefore: &before, LatestOnly: true, Limit: 31,
	})

	require.Contains(t, query, "atom_type = $2")
	require.Contains(t, query, "observed_at <= $3")
	require.Contains(t, query, "valid_from >= $4")
	require.Contains(t, query, "valid_from < $5")
	require.Contains(t, query, "LIMIT $6")
	require.Equal(t, []interface{}{"project-a", atom.TypeEvent, asOf, since, before, 31}, args)
}

func TestBuildChronoLedgerAtomsQuery_FiltersDeduplicatesOrdersAndCaps(t *testing.T) {
	query, args := buildChronoLedgerAtomsQuery("project-a", 41)

	required := []string{
		"project = $1",
		"atom_type IN ('event', 'status_change')",
		"valid_from IS NOT NULL",
		"DISTINCT ON",
		"ORDER BY valid_from ASC",
		"LIMIT $2",
	}
	for _, fragment := range required {
		if !strings.Contains(query, fragment) {
			t.Errorf("query missing %q:\n%s", fragment, query)
		}
	}
	if strings.Contains(query, "valid_to IS NULL") {
		t.Fatalf("chronology query excludes superseded atoms:\n%s", query)
	}
	require.Equal(t, []interface{}{"project-a", 41}, args)
}
