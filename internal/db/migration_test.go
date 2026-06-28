package db_test

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/petersimmons1972/engram/internal/db"
	"github.com/petersimmons1972/engram/internal/testutil"
	"github.com/stretchr/testify/require"
)

// TestConcurrentMigrations_NoRace verifies that concurrent NewPostgresBackend calls
// using different project slugs do not race on shared DDL and produce pg_type
// duplicate-key errors (SQLSTATE 23505). This was the bug in #1140: the old
// per-project advisory lock allowed different project slugs to run CREATE TABLE
// concurrently; the fix uses a single-arg global lock.
func TestConcurrentMigrations_NoRace(t *testing.T) {
	dsn := testutil.DSN(t) // skips test if TEST_DATABASE_URL is not set
	const n = 5
	ctx := context.Background()

	var wg sync.WaitGroup
	errs := make([]error, n)

	for i := 0; i < n; i++ {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			proj := fmt.Sprintf("race-test-%d", i)
			backend, err := db.NewPostgresBackend(ctx, proj, dsn)
			if err != nil {
				errs[i] = err
				return
			}
			backend.Close()
		}()
	}
	wg.Wait()

	for i, err := range errs {
		if err == nil {
			continue
		}
		msg := err.Error()
		if strings.Contains(msg, "pg_type") || strings.Contains(msg, "23505") {
			t.Errorf("goroutine %d: concurrent migration race detected (pg_type/23505): %v", i, err)
		} else {
			require.NoError(t, err, "goroutine %d: unexpected migration error", i)
		}
	}
}

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

func TestMigration030AtomsPreferenceFields(t *testing.T) {
	proj := uniqueProject("migration-030-atoms-pref")
	ctx := context.Background()

	backend, err := db.NewPostgresBackend(ctx, proj, testutil.DSN(t))
	require.NoError(t, err)
	t.Cleanup(func() { backend.Close() })

	for _, col := range []string{"polarity", "entity", "domain"} {
		col := col
		t.Run(col, func(t *testing.T) {
			var exists bool
			err = backend.Pool().QueryRow(ctx, `
				SELECT EXISTS (
					SELECT 1
					FROM information_schema.columns
					WHERE table_name  = 'atoms'
					  AND column_name = $1
				)`, col).Scan(&exists)
			require.NoError(t, err)
			require.True(t, exists, "migration 030 must add atoms.%s column", col)
		})
	}
}
