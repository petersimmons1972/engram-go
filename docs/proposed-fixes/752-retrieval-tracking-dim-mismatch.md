> **SUPERSEDED** — Implemented in PR #756 (merged 2026-05-20). This document retained for design context only.

# Issue #752 — test: TestRetrievalTracking_PrecisionUpdated — verify embed-dim fixture alignment

**Severity:** nice-to-have
**Area:** testing
**Status:** Design only — not yet implemented

## Root cause

The task brief cited a 1024 vs 768 embed-dim mismatch failure in `TestRetrievalTracking_PrecisionUpdated` (`internal/search/retrieval_precision_test.go:123`). As of the 2026-05-19 investigation the test **passes** (`go test ./internal/search/ -run TestRetrievalTracking_PrecisionUpdated` exits 0). The root cause of the original failure is unconfirmed — likely one of:

1. A live-DB fixture had memories at 1024 dims (from a prior embedder), and a test run against the shared dev DB picked up that mismatch. See `internal/search/engine_test.go:420-440` for an existing dim-mismatch guard at the engine level.
2. The failure was environment-specific and was resolved by re-embedding or DB reset.

The gap is not the test failure itself but the **absence of a dim-mismatch test at the retrieval-precision layer** — `retrieval_precision_test.go` creates a `newTestEngine` with the default fake client (768 dims) but never exercises the path where a memory stored at one dim is later queried by an engine with a different dim.

## Repro

```bash
# Current state: test passes
cd /home/psimmons/projects/engram-go
go test ./internal/search/ -run TestRetrievalTracking_PrecisionUpdated -v -count=1
# Output: ok

# To reproduce the hypothetical failure:
# 1. Store a memory using an engine with 1024-dim embedder
# 2. Re-open the project with a 768-dim engine
# 3. Call RecallWithEvent + FeedbackWithEvent
# 4. Expected: error returned, not silent empty recall + phantom precision score
```

## Proposed patch

Add a dim-mismatch guard test in `internal/search/retrieval_precision_test.go`:

```diff
--- a/internal/search/retrieval_precision_test.go
+++ b/internal/search/retrieval_precision_test.go
@@ -151,0 +152,30 @@
+
+// TestRetrievalTracking_DimMismatch verifies that opening a project with a
+// different embedding dimension than it was stored with returns a clear error
+// from RecallWithEvent rather than silently returning zero results and
+// recording phantom precision data.
+func TestRetrievalTracking_DimMismatch(t *testing.T) {
+	ctx := context.Background()
+	proj := uniqueProject("test-rt-dim-mismatch")
+
+	// Store a memory using a 1024-dim engine.
+	engine1024 := newEngineWithDims(t, proj, 1024)
+	t.Cleanup(func() { engine1024.Close() })
+	m := &types.Memory{
+		Content:     "dim mismatch test memory",
+		MemoryType:  types.MemoryTypeDecision,
+		Project:     proj,
+		Importance:  1,
+		StorageMode: "focused",
+	}
+	require.NoError(t, engine1024.Store(ctx, m))
+
+	// Attempt recall using a 768-dim engine — must fail clearly, not silently.
+	engine768 := newEngineWithDims(t, proj, 768)
+	t.Cleanup(func() { engine768.Close() })
+	_, _, err := engine768.RecallWithEvent(ctx, "dim mismatch test memory", 5, "normal")
+	require.Error(t, err, "RecallWithEvent with dim mismatch must return an error")
+	assert.Contains(t, err.Error(), "dim", "error must mention embedding dimension mismatch")
+}
```

The `newEngineWithDims` helper:

```go
func newEngineWithDims(t *testing.T, proj string, dims int) *search.Engine {
    t.Helper()
    backend := newTestBackend(t)
    return search.New(ctx, backend, &fakeClient{dims: dims}, proj, search.DefaultConfig())
}
```

## TDD scenarios

1. **dim_match_precision_tracking_works** — Given consistent 768-dim engine across store + recall + feedback, when 5 cycles complete, then `RetrievalPrecision` is non-nil and near 1.0 (existing test, verify still passes).
2. **dim_mismatch_returns_error** — Given memory stored at 1024 dims, when a 768-dim engine calls `RecallWithEvent`, then an error is returned containing "dim" (or "dimension" or similar) rather than returning empty results silently.
3. **dim_mismatch_no_phantom_precision** — Given dim mismatch error from RecallWithEvent, when the error path is taken, then `times_retrieved` on the memory is NOT incremented (no phantom tracking data).

## Risk notes

- The proposed `newEngineWithDims` helper already has a structural counterpart in `engine_test.go:420-440`; the pattern is established.
- If the search engine does not currently return an error on dim mismatch (it may just return empty results), the fix may require a production-code change in `internal/search/engine.go` — this design doc does not prescribe that fix, only the test that would detect it.
- Blast radius: test-only change unless the production guard is also added.

## Rollout

Test-only change. No schema migrations, no binary rebuild needed beyond running tests.

## Out of scope (followups)

- If dim-mismatch currently silently returns zero results in production, file a separate blocker: `memory_recall` silently returning empty when the DB has a different dim than the current embedder is a production correctness issue.
