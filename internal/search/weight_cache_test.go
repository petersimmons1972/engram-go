package search

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

// stubWCRow implements pgx.Row for weight_cache tests.
type stubWCRow struct {
	vals []any
	err  error
}

func (r *stubWCRow) Scan(dest ...any) error {
	if r.err != nil {
		return r.err
	}
	for i, d := range dest {
		if i >= len(r.vals) {
			break
		}
		if ptr, ok := d.(*float64); ok {
			if v, ok2 := r.vals[i].(float64); ok2 {
				*ptr = v
			}
		}
	}
	return nil
}

// stubWCQuerier implements weightQuerier for weight_cache tests.
type stubWCQuerier struct {
	callCount int
	rowFn     func(callCount int) pgx.Row
}

func (s *stubWCQuerier) QueryRow(_ context.Context, _ string, _ ...any) pgx.Row {
	s.callCount++
	if s.rowFn != nil {
		return s.rowFn(s.callCount)
	}
	return &stubWCRow{err: pgx.ErrNoRows}
}

func TestWeightCache_NoRow(t *testing.T) {
	q := &stubWCQuerier{
		rowFn: func(_ int) pgx.Row {
			return &stubWCRow{err: pgx.ErrNoRows}
		},
	}
	cache := newWeightCacheWithQuerier(q)
	w := cache.Get(context.Background(), "proj1")
	if w != DefaultWeights() {
		t.Errorf("WeightCache NoRow: expected defaults, got %+v", w)
	}
}

func TestWeightCache_RowReturned(t *testing.T) {
	want := Weights{Vector: 0.40, BM25: 0.35, Recency: 0.12, Precision: 0.13}
	q := &stubWCQuerier{
		rowFn: func(_ int) pgx.Row {
			return &stubWCRow{vals: []any{want.Vector, want.BM25, want.Recency, want.Precision}}
		},
	}
	cache := newWeightCacheWithQuerier(q)
	w := cache.Get(context.Background(), "proj1")
	if w != want {
		t.Errorf("WeightCache RowReturned: got %+v, want %+v", w, want)
	}
}

func TestWeightCache_Cached(t *testing.T) {
	callCount := 0
	q := &stubWCQuerier{
		rowFn: func(_ int) pgx.Row {
			callCount++
			return &stubWCRow{vals: []any{0.40, 0.35, 0.12, 0.13}}
		},
	}
	cache := newWeightCacheWithQuerier(q)
	_ = cache.Get(context.Background(), "proj1")
	_ = cache.Get(context.Background(), "proj1")
	// Second call must use the cache, not hit the DB again.
	if callCount != 1 {
		t.Errorf("WeightCache Cached: expected 1 DB call, got %d", callCount)
	}
}

func TestWeightCache_TTLExpiry(t *testing.T) {
	callCount := 0
	q := &stubWCQuerier{
		rowFn: func(_ int) pgx.Row {
			callCount++
			return &stubWCRow{vals: []any{0.40, 0.35, 0.12, 0.13}}
		},
	}
	cache := newWeightCacheWithQuerier(q)
	// Prime the cache.
	_ = cache.Get(context.Background(), "proj1")

	// Manually expire the entry.
	cache.mu.Lock()
	e := cache.entries["proj1"]
	e.expiresAt = time.Now().Add(-time.Second) // already expired
	cache.entries["proj1"] = e
	cache.mu.Unlock()

	// Second call should reload from DB.
	_ = cache.Get(context.Background(), "proj1")
	if callCount != 2 {
		t.Errorf("WeightCache TTLExpiry: expected 2 DB calls after expiry, got %d", callCount)
	}
}

func TestWeightCache_Invalidate(t *testing.T) {
	callCount := 0
	q := &stubWCQuerier{
		rowFn: func(_ int) pgx.Row {
			callCount++
			return &stubWCRow{vals: []any{0.40, 0.35, 0.12, 0.13}}
		},
	}
	cache := newWeightCacheWithQuerier(q)
	_ = cache.Get(context.Background(), "proj1")
	cache.Invalidate("proj1")
	_ = cache.Get(context.Background(), "proj1")
	if callCount != 2 {
		t.Errorf("WeightCache Invalidate: expected 2 DB calls after invalidation, got %d", callCount)
	}
}

func TestWeightCache_DBError(t *testing.T) {
	q := &stubWCQuerier{
		rowFn: func(_ int) pgx.Row {
			return &stubWCRow{err: fmt.Errorf("connection refused")}
		},
	}
	cache := newWeightCacheWithQuerier(q)
	w := cache.Get(context.Background(), "proj1")
	// Non-ErrNoRows error should fall back to defaults.
	if w != DefaultWeights() {
		t.Errorf("WeightCache DBError: expected defaults on error, got %+v", w)
	}
}
