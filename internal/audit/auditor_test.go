package audit

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// --- manual stubs for AuditQuerier ---

// stubRow implements pgx.Row.
type stubRow struct {
	vals []any
	err  error
}

func (r *stubRow) Scan(dest ...any) error {
	if r.err != nil {
		return r.err
	}
	for i, d := range dest {
		if i >= len(r.vals) {
			break
		}
		switch ptr := d.(type) {
		case *bool:
			if v, ok := r.vals[i].(bool); ok {
				*ptr = v
			}
		case *string:
			if v, ok := r.vals[i].(string); ok {
				*ptr = v
			}
		case *[]string:
			if v, ok := r.vals[i].([]string); ok {
				*ptr = v
			}
		case **float64:
			if v, ok := r.vals[i].(*float64); ok {
				*ptr = v
			}
		case *time.Time:
			if v, ok := r.vals[i].(time.Time); ok {
				*ptr = v
			}
		case **string:
			if v, ok := r.vals[i].(*string); ok {
				*ptr = v
			} else if r.vals[i] == nil {
				*ptr = nil
			}
		}
	}
	return nil
}

// stubRows implements pgx.Rows.
type stubRows struct {
	rows [][]any
	pos  int
	err  error
}

func newStubRows(rows [][]any) *stubRows {
	return &stubRows{rows: rows, pos: -1}
}

func (r *stubRows) Close() {}
func (r *stubRows) Err() error { return r.err }
func (r *stubRows) CommandTag() pgconn.CommandTag { return pgconn.CommandTag{} }
func (r *stubRows) FieldDescriptions() []pgconn.FieldDescription { return nil }
func (r *stubRows) Conn() *pgx.Conn { return nil }
func (r *stubRows) RawValues() [][]byte { return nil }
func (r *stubRows) Values() ([]any, error) {
	if r.pos < 0 || r.pos >= len(r.rows) {
		return nil, nil
	}
	return r.rows[r.pos], nil
}

func (r *stubRows) Next() bool {
	r.pos++
	return r.pos < len(r.rows)
}

func (r *stubRows) Scan(dest ...any) error {
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
		case *bool:
			if v, ok := row[i].(bool); ok {
				*ptr = v
			}
		case **float64:
			if v, ok := row[i].(*float64); ok {
				*ptr = v
			} else if row[i] == nil {
				*ptr = nil
			}
		case *time.Time:
			if v, ok := row[i].(time.Time); ok {
				*ptr = v
			}
		case **string:
			if v, ok := row[i].(*string); ok {
				*ptr = v
			} else if row[i] == nil {
				*ptr = nil
			}
		case *[]string:
			if v, ok := row[i].([]string); ok {
				*ptr = v
			} else if row[i] == nil {
				*ptr = []string{}
			}
		}
	}
	return nil
}

// stubQuerier implements AuditQuerier for testing.
type stubQuerier struct {
	execFn     func(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	queryFn    func(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	queryRowFn func(ctx context.Context, sql string, args ...any) pgx.Row
}

func (s *stubQuerier) Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	if s.execFn != nil {
		return s.execFn(ctx, sql, args...)
	}
	return pgconn.NewCommandTag("UPDATE 1"), nil
}

func (s *stubQuerier) Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
	if s.queryFn != nil {
		return s.queryFn(ctx, sql, args...)
	}
	return newStubRows(nil), nil
}

func (s *stubQuerier) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	if s.queryRowFn != nil {
		return s.queryRowFn(ctx, sql, args...)
	}
	return &stubRow{err: pgx.ErrNoRows}
}

// stubRecaller satisfies the Recaller interface for audit tests.
type stubRecaller struct {
	ids []string
	err error
}

func (r *stubRecaller) Recall(_ context.Context, _, _ string, _ int) ([]string, error) {
	return r.ids, r.err
}

// makeWorker creates an AuditWorker with a stub querier (no real DB needed).
func makeWorker(db AuditQuerier, recaller Recaller) *AuditWorker {
	return &AuditWorker{
		pool:       nil,
		db:         db,
		recaller:   recaller,
		embedModel: "test-model",
		interval:   time.Hour,
	}
}

// --- tests ---

func TestRegisterQuery(t *testing.T) {
	var captured struct {
		sql  string
		args []any
	}
	db := &stubQuerier{
		execFn: func(_ context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
			captured.sql = sql
			captured.args = args
			return pgconn.NewCommandTag("INSERT 0 1"), nil
		},
	}
	w := makeWorker(db, nil)
	id, err := w.RegisterQuery(context.Background(), "proj1", "test query", "desc")
	if err != nil {
		t.Fatalf("RegisterQuery: unexpected error: %v", err)
	}
	if id == "" {
		t.Error("RegisterQuery: expected non-empty id")
	}
	if len(captured.args) < 4 {
		t.Errorf("RegisterQuery: expected 4 args, got %d", len(captured.args))
	}
}

func TestListQueries_AllActive(t *testing.T) {
	now := time.Now()
	rows := [][]any{
		{"id1", "proj1", "query one", (*string)(nil), true, (*float64)(nil), now},
		{"id2", "proj1", "query two", (*string)(nil), false, (*float64)(nil), now},
	}
	db := &stubQuerier{
		queryFn: func(_ context.Context, _ string, _ ...any) (pgx.Rows, error) {
			return newStubRows(rows), nil
		},
	}
	w := makeWorker(db, nil)
	queries, err := w.ListQueries(context.Background(), "proj1")
	if err != nil {
		t.Fatalf("ListQueries: unexpected error: %v", err)
	}
	if len(queries) != 2 {
		t.Errorf("ListQueries: expected 2 queries, got %d", len(queries))
	}
	if queries[0].ID != "id1" {
		t.Errorf("ListQueries: first query ID = %q, want %q", queries[0].ID, "id1")
	}
}

func TestDeactivateQuery_Success(t *testing.T) {
	db := &stubQuerier{
		execFn: func(_ context.Context, _ string, _ ...any) (pgconn.CommandTag, error) {
			return pgconn.NewCommandTag("UPDATE 1"), nil
		},
	}
	w := makeWorker(db, nil)
	if err := w.DeactivateQuery(context.Background(), "some-id"); err != nil {
		t.Errorf("DeactivateQuery: unexpected error: %v", err)
	}
}

func TestDeactivateQuery_NotFound(t *testing.T) {
	db := &stubQuerier{
		execFn: func(_ context.Context, _ string, _ ...any) (pgconn.CommandTag, error) {
			return pgconn.NewCommandTag("UPDATE 0"), nil
		},
	}
	w := makeWorker(db, nil)
	err := w.DeactivateQuery(context.Background(), "missing-id")
	if err == nil {
		t.Error("DeactivateQuery: expected error for missing id, got nil")
	}
}

func TestGetSnapshots_Empty(t *testing.T) {
	db := &stubQuerier{
		queryFn: func(_ context.Context, _ string, _ ...any) (pgx.Rows, error) {
			return newStubRows(nil), nil
		},
	}
	w := makeWorker(db, nil)
	snaps, err := w.GetSnapshots(context.Background(), "query-id", 5)
	if err != nil {
		t.Fatalf("GetSnapshots: unexpected error: %v", err)
	}
	if snaps == nil {
		snaps = []Snapshot{}
	}
	if len(snaps) != 0 {
		t.Errorf("GetSnapshots: expected 0 snapshots, got %d", len(snaps))
	}
}

func TestRunProjectAudit_Baseline(t *testing.T) {
	now := time.Now()
	// loadQueriesByProject returns one active query
	queryRows := [][]any{
		{"q1", "proj1", "test query", (*string)(nil), true, (*float64)(nil), now},
	}
	queryCalled := 0
	db := &stubQuerier{
		queryFn: func(_ context.Context, sql string, _ ...any) (pgx.Rows, error) {
			queryCalled++
			if queryCalled == 1 {
				// loadQueriesByProject
				return newStubRows(queryRows), nil
			}
			// any other query returns empty
			return newStubRows(nil), nil
		},
		queryRowFn: func(_ context.Context, _ string, _ ...any) pgx.Row {
			// latestSnapshot returns no rows (baseline)
			return &stubRow{err: pgx.ErrNoRows}
		},
		execFn: func(_ context.Context, _ string, _ ...any) (pgconn.CommandTag, error) {
			// insertSnapshot
			return pgconn.NewCommandTag("INSERT 0 1"), nil
		},
	}
	recaller := &stubRecaller{ids: []string{"mem1", "mem2", "mem3"}}
	w := makeWorker(db, recaller)

	summaries, err := w.RunProjectAudit(context.Background(), "proj1")
	if err != nil {
		t.Fatalf("RunProjectAudit: unexpected error: %v", err)
	}
	if len(summaries) != 1 {
		t.Fatalf("RunProjectAudit: expected 1 summary, got %d", len(summaries))
	}
	if !summaries[0].IsBaseline {
		t.Error("RunProjectAudit: expected IsBaseline=true for first run")
	}
}

func TestRunProjectAudit_Delta(t *testing.T) {
	now := time.Now().Add(-time.Hour)
	queryRows := [][]any{
		{"q1", "proj1", "test query", (*string)(nil), true, (*float64)(nil), now},
	}
	// Previous snapshot has memory IDs A, B, C
	prevMemIDs := []string{"A", "B", "C"}
	var prevScores []float64
	queryCalled := 0
	db := &stubQuerier{
		queryFn: func(_ context.Context, _ string, _ ...any) (pgx.Rows, error) {
			queryCalled++
			if queryCalled == 1 {
				return newStubRows(queryRows), nil
			}
			return newStubRows(nil), nil
		},
		queryRowFn: func(_ context.Context, _ string, _ ...any) pgx.Row {
			// Return a previous snapshot with IDs A, B, C
			return &stubRow{vals: []any{
				"snap-prev", "q1", "proj1",
				prevMemIDs, prevScores,
				"test-model", (*string)(nil), now,
			}}
		},
		execFn: func(_ context.Context, _ string, _ ...any) (pgconn.CommandTag, error) {
			return pgconn.NewCommandTag("INSERT 0 1"), nil
		},
	}
	// Recall returns B, C, D — so addition=[D], removal=[A]
	recaller := &stubRecaller{ids: []string{"B", "C", "D"}}
	w := makeWorker(db, recaller)

	summaries, err := w.RunProjectAudit(context.Background(), "proj1")
	if err != nil {
		t.Fatalf("RunProjectAudit delta: unexpected error: %v", err)
	}
	if len(summaries) != 1 {
		t.Fatalf("RunProjectAudit delta: expected 1 summary, got %d", len(summaries))
	}
	if summaries[0].IsBaseline {
		t.Error("RunProjectAudit delta: expected IsBaseline=false when previous snapshot exists")
	}
	if summaries[0].RBOVsPrev == nil {
		t.Error("RunProjectAudit delta: expected non-nil RBOVsPrev")
	}
}

func TestSnapshotToSummary_Baseline(t *testing.T) {
	q := CanonicalQuery{
		ID:      "q1",
		Project: "proj1",
		Query:   "test",
	}
	s := &Snapshot{
		ID:        "snap1",
		RunAt:     time.Now(),
		RBOVsPrev: nil, // baseline
	}
	summary := snapshotToSummary(q, s)
	if !summary.IsBaseline {
		t.Error("snapshotToSummary: nil RBOVsPrev should produce IsBaseline=true")
	}
}

func TestListQueries_EmptyProject_ReturnsAll(t *testing.T) {
	now := time.Now()
	rows := [][]any{
		{"id1", "proj1", "q1", (*string)(nil), true, (*float64)(nil), now},
		{"id2", "proj2", "q2", (*string)(nil), false, (*float64)(nil), now},
	}
	var capturedSQL string
	db := &stubQuerier{
		queryFn: func(_ context.Context, sql string, _ ...any) (pgx.Rows, error) {
			capturedSQL = sql
			return newStubRows(rows), nil
		},
	}
	w := makeWorker(db, nil)
	queries, err := w.ListQueries(context.Background(), "")
	if err != nil {
		t.Fatalf("ListQueries empty project: unexpected error: %v", err)
	}
	if len(queries) != 2 {
		t.Errorf("ListQueries empty: expected 2, got %d", len(queries))
	}
	// Verify the query does NOT have a WHERE clause filtering by project or active
	if containsSubstr(capturedSQL, "WHERE project") {
		t.Errorf("ListQueries empty: SQL should not filter by project, got: %s", capturedSQL)
	}
	if containsSubstr(capturedSQL, "WHERE active") {
		t.Errorf("ListQueries empty: SQL should not filter by active status, got: %s", capturedSQL)
	}
}

func containsSubstr(s, substr string) bool {
	if len(substr) > len(s) {
		return false
	}
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
