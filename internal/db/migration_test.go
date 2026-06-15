package db_test

import (
	"context"
	"testing"

	"github.com/petersimmons1972/engram/internal/db"
	"github.com/petersimmons1972/engram/internal/testutil"
	"github.com/stretchr/testify/require"
)

func TestMigration029AtomsObservedAt(t *testing.T) {
	proj := uniqueProject("migration-029-atoms")
	ctx := context.Background()

	backend, err := db.NewPostgresBackend(ctx, proj, testutil.DSN(t))
	require.NoError(t, err)
	t.Cleanup(func() { backend.Close() })

	var hasObservedAt bool
	err = backend.Pool().QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM information_schema.columns
			WHERE table_name = 'atoms'
			  AND column_name = 'observed_at'
		)`).
		Scan(&hasObservedAt)
	require.NoError(t, err)
	require.True(t, hasObservedAt, "migration 029 must add atoms.observed_at")

	var allowsProfile bool
	err = backend.Pool().QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM pg_constraint
			WHERE conrelid = 'atoms'::regclass
			  AND pg_get_constraintdef(oid) ILIKE '%profile%'
			  AND pg_get_constraintdef(oid) ILIKE '%status_change%'
		)`).
		Scan(&allowsProfile)
	require.NoError(t, err)
	require.True(t, allowsProfile, "migration 029 must widen the atom_type CHECK to include profile and status_change")

	var hasLatestOnlyIndex bool
	err = backend.Pool().QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM pg_indexes
			WHERE tablename = 'atoms'
			  AND indexdef ILIKE '%(project, atom_type, observed_at DESC)%'
			  AND indexdef ILIKE '%valid_to IS NULL%'
		)`).
		Scan(&hasLatestOnlyIndex)
	require.NoError(t, err)
	require.True(t, hasLatestOnlyIndex, "migration 029 must create the LatestOnly observed_at index")
}
