package db

import (
	"context"
	"testing"
)

// TestDeleteOldSessionsNilPool verifies that the nil-pool fast path in
// deleteOldSessions returns (0, nil) rather than panicking. This matches the
// documented behaviour: a nil-pool backend is a no-op for cleanup, not an error.
func TestDeleteOldSessionsNilPool(t *testing.T) {
	b := &PostgresBackend{pool: nil, project: "default"}
	n, err := b.deleteOldSessions(context.Background())
	if err != nil {
		t.Errorf("deleteOldSessions with nil pool should return nil error, got %v", err)
	}
	if n != 0 {
		t.Errorf("deleteOldSessions with nil pool should return 0 rows, got %d", n)
	}
}

// TestEfSearchForLimitBoundaries exercises the efSearchForLimit helper for the
// three behavioural regions: below threshold, above threshold but below cap, and
// above the cap boundary (#370).
func TestEfSearchForLimitBoundaries(t *testing.T) {
	cases := []struct {
		limit int
		want  int
	}{
		{limit: 1, want: 2},
		{limit: 64, want: 128},  // at the HNSW default threshold
		{limit: 100, want: 200}, // above threshold, below cap
		{limit: 499, want: 998}, // just below the cap boundary (499*2=998 ≤ 1000)
		{limit: 500, want: 1000}, // exactly at the cap boundary (500*2=1000)
		{limit: 501, want: 1000}, // one above — cap kicks in (501*2=1002 → 1000)
		{limit: 10000, want: 1000}, // pathological large limit
	}
	for _, tc := range cases {
		got := efSearchForLimit(tc.limit)
		if got != tc.want {
			t.Errorf("efSearchForLimit(%d) = %d, want %d", tc.limit, got, tc.want)
		}
	}
}
