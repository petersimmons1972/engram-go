package db

import (
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
