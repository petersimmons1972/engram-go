package db

import (
	"context"
	"math"
	"testing"
)

// FM-21 (catalog/failure-modes.md, hindsight issue #112): "public destructive
// methods with caller-side-only guards" — NullAllEmbeddings(Tx), DeleteProject
// and the prune methods historically had NO precondition guard of their own.
// Every safety check (the confirm-threshold in internal/search/engine.go, the
// ENGRAM_ALLOW_PROJECT_DELETE + confirm-string gate in
// internal/mcp/tools_consolidate.go) lived entirely in the caller. A future
// caller — a new admin CLI, a test helper, a fixed script — that invoked these
// methods directly, with a degenerate parameter (empty project, non-positive
// maxAgeHours), got no refusal: the method ran the destructive SQL anyway.
//
// These tests exercise the methods directly, bypassing every existing caller-
// side guard, to prove the method itself now refuses on a degenerate
// parameter rather than relying on convention. Because the guard must fire
// BEFORE any pool access (a nil pool would otherwise panic on
// Begin/Exec), a zero-value *PostgresBackend* is sufficient — no live
// database is required to prove the refusal.

// ---- DeleteProject -----------------------------------------------------

// TestDeleteProject_RefusesEmptyProject_MethodItself is the negative case:
// an unguarded caller that forgets to scope by project (or passes "" by
// accident/bug) must be refused by DeleteProject itself, not merely by the
// MCP-layer confirm-string check in tools_consolidate.go, which this call
// path never touches.
func TestDeleteProject_RefusesEmptyProject_MethodItself(t *testing.T) {
	b := &PostgresBackend{} // nil pool: proves the guard runs before any DB access
	err := b.DeleteProject(context.Background(), "")
	if err == nil {
		t.Fatal("DeleteProject(ctx, \"\") must be refused by the method itself; got nil error " +
			"(before the FM-21 fix this silently 'succeeded' via the nil-pool no-op, giving an " +
			"unguarded caller zero feedback that nothing was scoped)")
	}
}

// TestDeleteProject_GuardedCallProceeds_MethodItself is the positive case: a
// properly-scoped call (non-empty project) must not be blocked by the new
// guard — it proceeds to existing behavior (nil pool → nil error, unchanged).
func TestDeleteProject_GuardedCallProceeds_MethodItself(t *testing.T) {
	b := &PostgresBackend{}
	if err := b.DeleteProject(context.Background(), "some-real-project"); err != nil {
		t.Fatalf("DeleteProject with a valid project must not be refused by the new guard, got: %v", err)
	}
}

// ---- NullAllEmbeddings / NullAllEmbeddingsTx ----------------------------

func TestNullAllEmbeddings_RefusesEmptyProject_MethodItself(t *testing.T) {
	b := &PostgresBackend{}
	_, err := b.NullAllEmbeddings(context.Background(), "")
	if err == nil {
		t.Fatal("NullAllEmbeddings(ctx, \"\") must be refused by the method itself " +
			"(the migrate confirm-threshold guard lives only in internal/search/engine.go, " +
			"never in this method)")
	}
}

func TestNullAllEmbeddingsTx_RefusesEmptyProject_MethodItself(t *testing.T) {
	b := &PostgresBackend{}
	_, err := b.NullAllEmbeddingsTx(context.Background(), nil, "")
	if err == nil {
		t.Fatal("NullAllEmbeddingsTx(ctx, tx, \"\") must be refused by the method itself")
	}
}

// ---- PruneStaleMemories / PruneColdDocuments ----------------------------

func TestPruneStaleMemories_RefusesEmptyProject_MethodItself(t *testing.T) {
	b := &PostgresBackend{}
	_, err := b.PruneStaleMemories(context.Background(), "", 24, 3)
	if err == nil {
		t.Fatal("PruneStaleMemories(ctx, \"\", ...) must be refused by the method itself")
	}
}

func TestPruneStaleMemories_RefusesNonPositiveMaxAge_MethodItself(t *testing.T) {
	b := &PostgresBackend{}
	_, err := b.PruneStaleMemories(context.Background(), "real-project", 0, 3)
	if err == nil {
		t.Fatal("PruneStaleMemories(ctx, project, maxAgeHours=0, ...) must be refused: a zero or " +
			"negative maxAgeHours sets cutoff=now, which prunes nearly every non-immutable row " +
			"immediately. There is no confirm gate for this anywhere today (issue #112 scoping " +
			"receipt: 'no confirm gate at all')")
	}

	_, err = b.PruneStaleMemories(context.Background(), "real-project", -1, 3)
	if err == nil {
		t.Fatal("PruneStaleMemories with negative maxAgeHours must be refused")
	}
}

func TestPruneColdDocuments_RefusesEmptyProject_MethodItself(t *testing.T) {
	b := &PostgresBackend{}
	_, err := b.PruneColdDocuments(context.Background(), "", 24, 3)
	if err == nil {
		t.Fatal("PruneColdDocuments(ctx, \"\", ...) must be refused by the method itself")
	}
}

func TestPruneColdDocuments_RefusesNonPositiveMaxAge_MethodItself(t *testing.T) {
	b := &PostgresBackend{}
	_, err := b.PruneColdDocuments(context.Background(), "real-project", 0, 3)
	if err == nil {
		t.Fatal("PruneColdDocuments(ctx, project, maxAgeHours=0, ...) must be refused")
	}
}

// ---- Round-1 review blockers (PR #1442): degenerate maxAgeHours ---------
//
// Round 1 found that a positive-but-sub-duration maxAgeHours (e.g.
// 1/(2*time.Hour)) passed the old `<= 0` check but converted to a zero
// time.Duration, collapsing the cutoff to time.Now() and pruning nearly
// everything; math.NaN() and math.Inf(1) also passed `<= 0` unrejected.
// These tests construct exactly those bypasses against both prune methods.

func TestPruneStaleMemories_RefusesSubNanosecondPositiveMaxAge_MethodItself(t *testing.T) {
	b := &PostgresBackend{}
	// 1 / (2 * time.Hour) is positive but converts to well under 1 nanosecond
	// once multiplied by float64(time.Hour) and truncated to a time.Duration.
	tinyAge := 1.0 / (2 * float64(3600*1e9)) // hours equivalent of 0.5ns
	_, err := b.PruneStaleMemories(context.Background(), "real-project", tinyAge, 3)
	if err == nil {
		t.Fatal("PruneStaleMemories with a positive sub-nanosecond-duration maxAgeHours must be " +
			"refused: it would collapse the cutoff to time.Now(), pruning nearly every " +
			"non-immutable row immediately (round 1 blocker)")
	}
}

func TestPruneStaleMemories_RefusesNaNMaxAge_MethodItself(t *testing.T) {
	b := &PostgresBackend{}
	_, err := b.PruneStaleMemories(context.Background(), "real-project", math.NaN(), 3)
	if err == nil {
		t.Fatal("PruneStaleMemories with maxAgeHours=NaN must be refused (NaN <= 0 is false, so the " +
			"old guard let it through; round 1 blocker)")
	}
}

func TestPruneStaleMemories_RefusesInfMaxAge_MethodItself(t *testing.T) {
	b := &PostgresBackend{}
	_, err := b.PruneStaleMemories(context.Background(), "real-project", math.Inf(1), 3)
	if err == nil {
		t.Fatal("PruneStaleMemories with maxAgeHours=+Inf must be refused (round 1 blocker)")
	}

	_, err = b.PruneStaleMemories(context.Background(), "real-project", math.Inf(-1), 3)
	if err == nil {
		t.Fatal("PruneStaleMemories with maxAgeHours=-Inf must be refused")
	}
}

func TestPruneColdDocuments_RefusesSubNanosecondPositiveMaxAge_MethodItself(t *testing.T) {
	b := &PostgresBackend{}
	tinyAge := 1.0 / (2 * float64(3600*1e9))
	_, err := b.PruneColdDocuments(context.Background(), "real-project", tinyAge, 3)
	if err == nil {
		t.Fatal("PruneColdDocuments with a positive sub-nanosecond-duration maxAgeHours must be " +
			"refused (round 1 blocker)")
	}
}

func TestPruneColdDocuments_RefusesNaNAndInfMaxAge_MethodItself(t *testing.T) {
	b := &PostgresBackend{}
	if _, err := b.PruneColdDocuments(context.Background(), "real-project", math.NaN(), 3); err == nil {
		t.Fatal("PruneColdDocuments with maxAgeHours=NaN must be refused (round 1 blocker)")
	}
	if _, err := b.PruneColdDocuments(context.Background(), "real-project", math.Inf(1), 3); err == nil {
		t.Fatal("PruneColdDocuments with maxAgeHours=+Inf must be refused (round 1 blocker)")
	}
}

func TestPruneStaleMemories_RefusesExcessiveMaxAge_MethodItself(t *testing.T) {
	b := &PostgresBackend{}
	// A maxAgeHours large enough that maxAgeHours*float64(time.Hour) risks
	// overflowing the int64-nanosecond time.Duration must also be refused.
	_, err := b.PruneStaleMemories(context.Background(), "real-project", math.MaxFloat64, 3)
	if err == nil {
		t.Fatal("PruneStaleMemories with an overflow-risking maxAgeHours must be refused")
	}
}

// ---- Round-1 review blockers (PR #1442): unchecked maxImportance --------
//
// Round 1 found maxImportance was never validated: both DELETE predicates
// use `importance >= maxImportance`, so a caller passing -1 (or any value
// below the valid 0-4 importance domain) broadens the predicate to include
// critical (importance=0) memories — precisely what the guard exists to
// protect. These tests construct that exact bypass against both methods.

func TestPruneStaleMemories_RefusesNegativeMaxImportance_MethodItself(t *testing.T) {
	b := &PostgresBackend{}
	_, err := b.PruneStaleMemories(context.Background(), "real-project", 24, -1)
	if err == nil {
		t.Fatal("PruneStaleMemories with maxImportance=-1 must be refused: 'importance >= -1' " +
			"matches every importance level including critical (importance=0) memories " +
			"(round 1 blocker)")
	}
}

func TestPruneStaleMemories_RefusesOutOfRangeMaxImportance_MethodItself(t *testing.T) {
	b := &PostgresBackend{}
	_, err := b.PruneStaleMemories(context.Background(), "real-project", 24, 5)
	if err == nil {
		t.Fatal("PruneStaleMemories with maxImportance=5 (above the valid 0-4 domain) must be refused")
	}
}

func TestPruneColdDocuments_RefusesNegativeMaxImportance_MethodItself(t *testing.T) {
	b := &PostgresBackend{}
	_, err := b.PruneColdDocuments(context.Background(), "real-project", 24, -1)
	if err == nil {
		t.Fatal("PruneColdDocuments with maxImportance=-1 must be refused: it would broaden the " +
			"destructive DELETE predicate to include critical (importance=0) memories " +
			"(round 1 blocker)")
	}
}

func TestPruneColdDocuments_RefusesOutOfRangeMaxImportance_MethodItself(t *testing.T) {
	b := &PostgresBackend{}
	_, err := b.PruneColdDocuments(context.Background(), "real-project", 24, 5)
	if err == nil {
		t.Fatal("PruneColdDocuments with maxImportance=5 (above the valid 0-4 domain) must be refused")
	}
}

// TestPruneGuard_GuardedCallWithValidImportanceProceeds is the positive case:
// a properly-scoped, in-range call must not be blocked by the new
// maxImportance check.
func TestPruneGuard_GuardedCallWithValidImportanceProceeds(t *testing.T) {
	for _, imp := range []int{0, 1, 2, 3, 4} {
		if err := pruneGuard("real-project", 24, imp); err != nil {
			t.Fatalf("pruneGuard with in-range maxImportance=%d must not be refused, got: %v", imp, err)
		}
	}
}
