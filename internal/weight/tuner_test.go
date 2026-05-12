package weight

import (
	"context"
	"fmt"
	"math"
	"math/rand"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

func TestNormalizeWeights_AlreadyNormalized(t *testing.T) {
	w := DefaultWeights()
	got, ok := normalizeWeights(w)
	if !ok {
		t.Fatal("normalizeWeights: expected ok=true for defaults")
	}
	sum := got.Vector + got.BM25 + got.Recency + got.Precision
	if math.Abs(sum-1.0) > 0.01 {
		t.Errorf("sum want 1.0, got %f", sum)
	}
}

func TestNormalizeWeights_ClampAndNormalize(t *testing.T) {
	// Weights that need clamping.
	w := Weights{Vector: 0.80, BM25: 0.80, Recency: 0.80, Precision: 0.80}
	got, ok := normalizeWeights(w)
	if !ok {
		t.Fatal("normalizeWeights: expected ok=true")
	}
	sum := got.Vector + got.BM25 + got.Recency + got.Precision
	if math.Abs(sum-1.0) > 0.02 {
		t.Errorf("sum want ~1.0, got %f", sum)
	}
	// Each weight should be within bounds.
	if got.Vector < minVector || got.Vector > maxVector {
		t.Errorf("vector %f out of [%f,%f]", got.Vector, minVector, maxVector)
	}
	if got.BM25 < minBM25 || got.BM25 > maxBM25 {
		t.Errorf("bm25 %f out of [%f,%f]", got.BM25, minBM25, maxBM25)
	}
}

func TestNormalizeWeights_BelowMin(t *testing.T) {
	// Very small weights — should be clamped up.
	w := Weights{Vector: 0.01, BM25: 0.01, Recency: 0.01, Precision: 0.01}
	got, ok := normalizeWeights(w)
	if !ok {
		t.Fatal("normalizeWeights: expected ok=true")
	}
	if got.Vector < minVector {
		t.Errorf("vector below min: %f < %f", got.Vector, minVector)
	}
}

func TestComputeDelta_StaleRanking(t *testing.T) {
	d := computeDelta("stale_ranking")
	if d == nil {
		t.Fatal("expected non-nil delta for stale_ranking")
	}
	if d.Recency <= 0 {
		t.Errorf("stale_ranking: expected positive recency delta, got %f", d.Recency)
	}
	if d.Vector >= 0 {
		t.Errorf("stale_ranking: expected negative vector delta, got %f", d.Vector)
	}
}

func TestComputeDelta_VocabularyMismatch(t *testing.T) {
	d := computeDelta("vocabulary_mismatch")
	if d == nil {
		t.Fatal("expected non-nil delta for vocabulary_mismatch")
	}
	if d.Vector <= 0 {
		t.Errorf("vocabulary_mismatch: expected positive vector delta, got %f", d.Vector)
	}
	if d.BM25 >= 0 {
		t.Errorf("vocabulary_mismatch: expected negative BM25 delta, got %f", d.BM25)
	}
}

func TestComputeDelta_ScopeMismatch(t *testing.T) {
	d := computeDelta("scope_mismatch")
	if d == nil {
		t.Fatal("expected non-nil delta for scope_mismatch")
	}
	if d.Precision <= 0 {
		t.Errorf("scope_mismatch: expected positive precision delta, got %f", d.Precision)
	}
}

func TestComputeDelta_NoChange(t *testing.T) {
	for _, class := range []string{"aggregation_failure", "missing_content", "other", ""} {
		if computeDelta(class) != nil {
			t.Errorf("class %q: expected nil delta (no weight change), got non-nil", class)
		}
	}
}

func TestApplyDelta(t *testing.T) {
	base := DefaultWeights()
	delta := Weights{Vector: 0.05, Recency: -0.05}
	got := applyDelta(base, delta)
	wantVector := base.Vector + 0.05
	wantRecency := base.Recency - 0.05
	if math.Abs(got.Vector-wantVector) > 1e-9 {
		t.Errorf("vector: want %f, got %f", wantVector, got.Vector)
	}
	if math.Abs(got.Recency-wantRecency) > 1e-9 {
		t.Errorf("recency: want %f, got %f", wantRecency, got.Recency)
	}
}

func TestDefaultWeights_SumToOne(t *testing.T) {
	d := DefaultWeights()
	sum := d.Vector + d.BM25 + d.Recency + d.Precision
	if math.Abs(sum-1.0) > 1e-9 {
		t.Errorf("default weights sum want 1.0, got %f", sum)
	}
}

// --- DB stubs for tuner tests ---

type tunerStubRow struct {
	vals []any
	err  error
}

func (r *tunerStubRow) Scan(dest ...any) error {
	if r.err != nil {
		return r.err
	}
	for i, d := range dest {
		if i >= len(r.vals) {
			break
		}
		switch ptr := d.(type) {
		case **time.Time:
			if v, ok := r.vals[i].(time.Time); ok {
				*ptr = &v
			} else if r.vals[i] == nil {
				*ptr = nil
			}
		case *float64:
			if v, ok := r.vals[i].(float64); ok {
				*ptr = v
			}
		case *string:
			if v, ok := r.vals[i].(string); ok {
				*ptr = v
			}
		}
	}
	return nil
}

type tunerStubRows struct {
	rows [][]any
	pos  int
}

func newTunerRows(rows [][]any) *tunerStubRows { return &tunerStubRows{rows: rows, pos: -1} }

func (r *tunerStubRows) Close() {}
func (r *tunerStubRows) Err() error { return nil }
func (r *tunerStubRows) CommandTag() pgconn.CommandTag { return pgconn.CommandTag{} }
func (r *tunerStubRows) FieldDescriptions() []pgconn.FieldDescription { return nil }
func (r *tunerStubRows) Conn() *pgx.Conn { return nil }
func (r *tunerStubRows) RawValues() [][]byte { return nil }
func (r *tunerStubRows) Values() ([]any, error) { return nil, nil }
func (r *tunerStubRows) Next() bool {
	r.pos++
	return r.pos < len(r.rows)
}
func (r *tunerStubRows) Scan(dest ...any) error {
	if r.pos < 0 || r.pos >= len(r.rows) {
		return fmt.Errorf("no current row")
	}
	row := r.rows[r.pos]
	for i, d := range dest {
		if i >= len(row) {
			break
		}
		switch ptr := d.(type) {
		case *string:
			if v, ok := row[i].(string); ok {
				*ptr = v
			}
		case **string:
			if row[i] == nil {
				*ptr = nil
			} else if v, ok := row[i].(string); ok {
				*ptr = &v
			}
		case *int:
			if v, ok := row[i].(int); ok {
				*ptr = v
			}
		case *int64:
			if v, ok := row[i].(int64); ok {
				*ptr = v
			}
		case *float64:
			if v, ok := row[i].(float64); ok {
				*ptr = v
			}
		case *time.Time:
			if v, ok := row[i].(time.Time); ok {
				*ptr = v
			}
		case *[]byte:
			if row[i] == nil {
				*ptr = nil
			} else if v, ok := row[i].([]byte); ok {
				*ptr = v
			}
		}
	}
	return nil
}

type tunerStubDB struct {
	queryRowFn func(sql string, args ...any) pgx.Row
	queryFn    func(sql string, args ...any) (pgx.Rows, error)
	execFn     func(sql string, args ...any) (pgconn.CommandTag, error)
}

func (s *tunerStubDB) Exec(_ context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	if s.execFn != nil {
		return s.execFn(sql, args...)
	}
	return pgconn.NewCommandTag("UPDATE 1"), nil
}

func (s *tunerStubDB) Query(_ context.Context, sql string, args ...any) (pgx.Rows, error) {
	if s.queryFn != nil {
		return s.queryFn(sql, args...)
	}
	return newTunerRows(nil), nil
}

func (s *tunerStubDB) QueryRow(_ context.Context, sql string, args ...any) pgx.Row {
	if s.queryRowFn != nil {
		return s.queryRowFn(sql, args...)
	}
	return &tunerStubRow{err: pgx.ErrNoRows}
}

// makeTunerWorker creates a TunerWorker with stub DB and no real pool.
func makeTunerWorker(db *tunerStubDB) *TunerWorker {
	w := &TunerWorker{
		pool:     nil,
		db:       db,
		interval: time.Hour,
	}
	return w
}

// --- maybeAdjust tests ---

func TestMaybeAdjust_BelowThreshold(t *testing.T) {
	// Total failure events = 10, below minEventsBeforeTuning (20) → no tuning.
	db := &tunerStubDB{
		queryRowFn: func(_ string, _ ...any) pgx.Row {
			return &tunerStubRow{err: pgx.ErrNoRows} // no cooldown history
		},
		queryFn: func(_ string, _ ...any) (pgx.Rows, error) {
			// 10 stale_ranking events
			return newTunerRows([][]any{{"stale_ranking", 10}}), nil
		},
	}
	applied := false
	w := makeTunerWorker(db)
	w.applyFn = func(_ context.Context, _ string, _ Weights, _ []byte, _ string) error {
		applied = true
		return nil
	}
	if err := w.maybeAdjust(context.Background(), "proj1"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if applied {
		t.Error("BelowThreshold: applyFn should not have been called")
	}
}

func TestMaybeAdjust_NonDominant(t *testing.T) {
	// Two classes at 30% each — neither reaches 40% threshold.
	db := &tunerStubDB{
		queryRowFn: func(_ string, _ ...any) pgx.Row {
			return &tunerStubRow{err: pgx.ErrNoRows}
		},
		queryFn: func(_ string, _ ...any) (pgx.Rows, error) {
			// 30 stale_ranking + 30 vocabulary_mismatch = 60 total, top = 50%... wait, need <40%
			// 25 + 25 = 50 total, 25/50 = 50% — that is dominant. Let me use 20+20+20+5=65, top=20/65=30.8%
			return newTunerRows([][]any{
				{"stale_ranking", 20},
				{"vocabulary_mismatch", 20},
				{"aggregation_failure", 20},
				{"other", 5},
			}), nil
		},
	}
	applied := false
	w := makeTunerWorker(db)
	w.applyFn = func(_ context.Context, _ string, _ Weights, _ []byte, _ string) error {
		applied = true
		return nil
	}
	if err := w.maybeAdjust(context.Background(), "proj1"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if applied {
		t.Error("NonDominant: applyFn should not have been called")
	}
}

func TestMaybeAdjust_MarginTooSmall(t *testing.T) {
	// Top class = 45%, runner-up = 40% — margin = 5pp < 10pp threshold.
	db := &tunerStubDB{
		queryRowFn: func(_ string, _ ...any) pgx.Row {
			return &tunerStubRow{err: pgx.ErrNoRows}
		},
		queryFn: func(_ string, _ ...any) (pgx.Rows, error) {
			// 45 stale_ranking + 40 vocabulary_mismatch = 85 total
			return newTunerRows([][]any{
				{"stale_ranking", 45},
				{"vocabulary_mismatch", 40},
			}), nil
		},
	}
	applied := false
	w := makeTunerWorker(db)
	w.applyFn = func(_ context.Context, _ string, _ Weights, _ []byte, _ string) error {
		applied = true
		return nil
	}
	if err := w.maybeAdjust(context.Background(), "proj1"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if applied {
		t.Error("MarginTooSmall: applyFn should not have been called")
	}
}

func TestMaybeAdjust_CooldownActive(t *testing.T) {
	recent := time.Now().Add(-24 * time.Hour) // 1 day ago, within 3-day cooldown
	db := &tunerStubDB{
		queryRowFn: func(_ string, _ ...any) pgx.Row {
			return &tunerStubRow{vals: []any{recent}}
		},
	}
	applied := false
	w := makeTunerWorker(db)
	w.applyFn = func(_ context.Context, _ string, _ Weights, _ []byte, _ string) error {
		applied = true
		return nil
	}
	if err := w.maybeAdjust(context.Background(), "proj1"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if applied {
		t.Error("CooldownActive: applyFn should not have been called")
	}
}

func TestMaybeAdjust_StaleRanking_WritesWeights(t *testing.T) {
	// stale_ranking dominant, cooldown clear, enough events → should call applyFn.
	// LoadWeights returns defaults (no row).
	callCount := 0
	db := &tunerStubDB{
		queryRowFn: func(_ string, _ ...any) pgx.Row {
			callCount++
			if callCount == 1 {
				// cooldown check: no history
				return &tunerStubRow{err: pgx.ErrNoRows}
			}
			// LoadWeights: no row → defaults
			return &tunerStubRow{err: pgx.ErrNoRows}
		},
		queryFn: func(_ string, _ ...any) (pgx.Rows, error) {
			// 60 stale_ranking, 10 other → dominant at 85.7%, margin > 10pp
			return newTunerRows([][]any{
				{"stale_ranking", 60},
				{"aggregation_failure", 10},
			}), nil
		},
	}
	var appliedProject string
	var appliedWeights Weights
	w := makeTunerWorker(db)
	w.applyFn = func(_ context.Context, project string, wt Weights, _ []byte, _ string) error {
		appliedProject = project
		appliedWeights = wt
		return nil
	}
	if err := w.maybeAdjust(context.Background(), "proj1"); err != nil {
		t.Fatalf("StaleRanking: unexpected error: %v", err)
	}
	if appliedProject != "proj1" {
		t.Errorf("StaleRanking: expected project=proj1, got %q", appliedProject)
	}
	// stale_ranking → recency increased, vector decreased
	def := DefaultWeights()
	if appliedWeights.Recency <= def.Recency {
		t.Errorf("StaleRanking: expected recency increased, got %f (default %f)", appliedWeights.Recency, def.Recency)
	}
	if appliedWeights.Vector >= def.Vector {
		t.Errorf("StaleRanking: expected vector decreased, got %f (default %f)", appliedWeights.Vector, def.Vector)
	}
}

func TestMaybeAdjust_VocabularyMismatch_WritesWeights(t *testing.T) {
	callCount := 0
	db := &tunerStubDB{
		queryRowFn: func(_ string, _ ...any) pgx.Row {
			callCount++
			return &tunerStubRow{err: pgx.ErrNoRows}
		},
		queryFn: func(_ string, _ ...any) (pgx.Rows, error) {
			return newTunerRows([][]any{
				{"vocabulary_mismatch", 60},
				{"aggregation_failure", 5},
			}), nil
		},
	}
	var appliedWeights Weights
	w := makeTunerWorker(db)
	w.applyFn = func(_ context.Context, _ string, wt Weights, _ []byte, _ string) error {
		appliedWeights = wt
		return nil
	}
	if err := w.maybeAdjust(context.Background(), "proj1"); err != nil {
		t.Fatalf("VocabularyMismatch: unexpected error: %v", err)
	}
	def := DefaultWeights()
	if appliedWeights.Vector <= def.Vector {
		t.Errorf("VocabularyMismatch: expected vector increased, got %f (default %f)", appliedWeights.Vector, def.Vector)
	}
	if appliedWeights.BM25 >= def.BM25 {
		t.Errorf("VocabularyMismatch: expected BM25 decreased, got %f (default %f)", appliedWeights.BM25, def.BM25)
	}
}

func TestMaybeAdjust_AggregationFailure_NoChange(t *testing.T) {
	db := &tunerStubDB{
		queryRowFn: func(_ string, _ ...any) pgx.Row {
			return &tunerStubRow{err: pgx.ErrNoRows}
		},
		queryFn: func(_ string, _ ...any) (pgx.Rows, error) {
			// aggregation_failure dominant — no weight change expected
			return newTunerRows([][]any{
				{"aggregation_failure", 60},
				{"stale_ranking", 5},
			}), nil
		},
	}
	applied := false
	w := makeTunerWorker(db)
	w.applyFn = func(_ context.Context, _ string, _ Weights, _ []byte, _ string) error {
		applied = true
		return nil
	}
	if err := w.maybeAdjust(context.Background(), "proj1"); err != nil {
		t.Fatalf("AggregationFailure: unexpected error: %v", err)
	}
	if applied {
		t.Error("AggregationFailure: applyFn should not be called for non-actionable class")
	}
}

func TestLoadWeights_NoRow(t *testing.T) {
	db := &tunerStubDB{
		queryRowFn: func(_ string, _ ...any) pgx.Row {
			return &tunerStubRow{err: pgx.ErrNoRows}
		},
	}
	w := makeTunerWorker(db)
	wt, err := w.LoadWeights(context.Background(), "proj1")
	if err != nil {
		t.Fatalf("LoadWeights NoRow: unexpected error: %v", err)
	}
	if wt != DefaultWeights() {
		t.Errorf("LoadWeights NoRow: expected defaults, got %+v", wt)
	}
}

func TestLoadWeights_WithRow(t *testing.T) {
	want := Weights{Vector: 0.40, BM25: 0.35, Recency: 0.12, Precision: 0.13}
	db := &tunerStubDB{
		queryRowFn: func(_ string, _ ...any) pgx.Row {
			return &tunerStubRow{vals: []any{want.Vector, want.BM25, want.Recency, want.Precision}}
		},
	}
	w := makeTunerWorker(db)
	wt, err := w.LoadWeights(context.Background(), "proj1")
	if err != nil {
		t.Fatalf("LoadWeights WithRow: unexpected error: %v", err)
	}
	if wt != want {
		t.Errorf("LoadWeights WithRow: got %+v, want %+v", wt, want)
	}
}

func TestNormalizeWeights_SumEqualsOne_Property(t *testing.T) {
	rng := rand.New(rand.NewSource(42))
	for i := 0; i < 100; i++ {
		w := Weights{
			Vector:    minVector + rng.Float64()*(maxVector-minVector+0.2)-0.1,
			BM25:      minBM25 + rng.Float64()*(maxBM25-minBM25+0.2)-0.1,
			Recency:   minRecency + rng.Float64()*(maxRecency-minRecency+0.1)-0.05,
			Precision: minPrecision + rng.Float64()*(maxPrecision-minPrecision+0.1)-0.05,
		}
		got, ok := normalizeWeights(w)
		if !ok {
			continue // infeasible input — skip, not a failure
		}
		sum := got.Vector + got.BM25 + got.Recency + got.Precision
		if math.Abs(sum-1.0) > 1e-9 {
			t.Errorf("property[%d]: sum = %f, want 1.0 (input: %+v, output: %+v)", i, sum, w, got)
		}
		// All within bounds.
		if got.Vector < minVector || got.Vector > maxVector {
			t.Errorf("property[%d]: vector %f out of [%f,%f]", i, got.Vector, minVector, maxVector)
		}
		if got.BM25 < minBM25 || got.BM25 > maxBM25 {
			t.Errorf("property[%d]: bm25 %f out of [%f,%f]", i, got.BM25, minBM25, maxBM25)
		}
		if got.Recency < minRecency || got.Recency > maxRecency {
			t.Errorf("property[%d]: recency %f out of [%f,%f]", i, got.Recency, minRecency, maxRecency)
		}
		if got.Precision < minPrecision || got.Precision > maxPrecision {
			t.Errorf("property[%d]: precision %f out of [%f,%f]", i, got.Precision, minPrecision, maxPrecision)
		}
	}
}

func TestNewTunerWorkerWithDB_ConstructorSetsFields(t *testing.T) {
	db := &tunerStubDB{}
	w := NewTunerWorkerWithDB(nil, db, time.Hour)
	if w == nil {
		t.Fatal("NewTunerWorkerWithDB returned nil")
	}
	if w.db != db {
		t.Error("NewTunerWorkerWithDB: db field not set")
	}
	if w.interval != time.Hour {
		t.Errorf("NewTunerWorkerWithDB: interval want %v, got %v", time.Hour, w.interval)
	}
}

func TestGetHistory_Empty(t *testing.T) {
	db := &tunerStubDB{
		queryFn: func(_ string, _ ...any) (pgx.Rows, error) {
			return newTunerRows(nil), nil
		},
	}
	w := makeTunerWorker(db)
	history, err := w.GetHistory(context.Background(), "proj1", 10)
	if err != nil {
		t.Fatalf("GetHistory empty: unexpected error: %v", err)
	}
	if len(history) != 0 {
		t.Errorf("GetHistory empty: expected 0 items, got %d", len(history))
	}
}

func TestGetHistory_WithRows(t *testing.T) {
	now := time.Now().Truncate(time.Second)
	noteStr := "auto-tuned"
	db := &tunerStubDB{
		queryFn: func(_ string, _ ...any) (pgx.Rows, error) {
			return newTunerRows([][]any{
				{"uuid-1", "proj1", now, 0.45, 0.30, 0.15, 0.10, []byte(`{}`), noteStr},
				{"uuid-2", "proj1", now, 0.40, 0.35, 0.15, 0.10, nil, nil},
			}), nil
		},
	}
	w := makeTunerWorker(db)
	history, err := w.GetHistory(context.Background(), "proj1", 20)
	if err != nil {
		t.Fatalf("GetHistory rows: unexpected error: %v", err)
	}
	if len(history) != 2 {
		t.Fatalf("GetHistory rows: expected 2 items, got %d", len(history))
	}
	if history[0].ID != "uuid-1" {
		t.Errorf("GetHistory rows[0].ID want uuid-1, got %q", history[0].ID)
	}
	if history[0].Notes != "auto-tuned" {
		t.Errorf("GetHistory rows[0].Notes want %q, got %q", noteStr, history[0].Notes)
	}
	if string(history[0].TriggerData) != "{}" {
		t.Errorf("GetHistory rows[0].TriggerData want {}, got %q", string(history[0].TriggerData))
	}
	if history[1].Notes != "" {
		t.Errorf("GetHistory rows[1].Notes should be empty, got %q", history[1].Notes)
	}
}

func TestGetHistory_DefaultLimit(t *testing.T) {
	// Passing limit=0 should default to 20 (doesn't error).
	db := &tunerStubDB{
		queryFn: func(_ string, _ ...any) (pgx.Rows, error) {
			return newTunerRows(nil), nil
		},
	}
	w := makeTunerWorker(db)
	_, err := w.GetHistory(context.Background(), "proj1", 0)
	if err != nil {
		t.Fatalf("GetHistory zero limit: unexpected error: %v", err)
	}
}

func TestResetToDefaults_Success(t *testing.T) {
	execCalled := false
	db := &tunerStubDB{
		execFn: func(_ string, _ ...any) (pgconn.CommandTag, error) {
			execCalled = true
			return pgconn.NewCommandTag("DELETE 1"), nil
		},
	}
	w := makeTunerWorker(db)
	if err := w.ResetToDefaults(context.Background(), "proj1"); err != nil {
		t.Fatalf("ResetToDefaults: unexpected error: %v", err)
	}
	if !execCalled {
		t.Error("ResetToDefaults: expected exec to be called")
	}
}

func TestResetToDefaults_Error(t *testing.T) {
	db := &tunerStubDB{
		execFn: func(_ string, _ ...any) (pgconn.CommandTag, error) {
			return pgconn.CommandTag{}, fmt.Errorf("db unavailable")
		},
	}
	w := makeTunerWorker(db)
	if err := w.ResetToDefaults(context.Background(), "proj1"); err == nil {
		t.Error("ResetToDefaults: expected error, got nil")
	}
}

func TestAdjustWeightsForProject_DelegatesTo_MaybeAdjust(t *testing.T) {
	// Verify the public entry-point calls through to maybeAdjust without error.
	db := &tunerStubDB{
		queryRowFn: func(_ string, _ ...any) pgx.Row {
			return &tunerStubRow{err: pgx.ErrNoRows}
		},
		queryFn: func(_ string, _ ...any) (pgx.Rows, error) {
			return newTunerRows([][]any{{"stale_ranking", 10}}), nil
		},
	}
	w := makeTunerWorker(db)
	if err := w.AdjustWeightsForProject(context.Background(), "proj1"); err != nil {
		t.Fatalf("AdjustWeightsForProject: unexpected error: %v", err)
	}
}
