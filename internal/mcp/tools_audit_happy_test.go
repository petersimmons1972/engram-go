package mcp

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/petersimmons1972/engram/internal/audit"
)

// --- stubs implementing audit.AuditQuerier ---

type happyTestRow struct {
	vals []any
	err  error
}

func (r *happyTestRow) Scan(dest ...any) error {
	if r.err != nil {
		return r.err
	}
	for i, d := range dest {
		if i >= len(r.vals) {
			break
		}
		switch ptr := d.(type) {
		case *string:
			if v, ok := r.vals[i].(string); ok {
				*ptr = v
			}
		case *bool:
			if v, ok := r.vals[i].(bool); ok {
				*ptr = v
			}
		case **float64:
			if v, ok := r.vals[i].(*float64); ok {
				*ptr = v
			} else if r.vals[i] == nil {
				*ptr = nil
			}
		case *time.Time:
			if v, ok := r.vals[i].(time.Time); ok {
				*ptr = v
			}
		case **string:
			if r.vals[i] == nil {
				*ptr = nil
			} else if v, ok := r.vals[i].(string); ok {
				*ptr = &v
			}
		case *[]string:
			if v, ok := r.vals[i].([]string); ok {
				*ptr = v
			} else {
				*ptr = []string{}
			}
		case *[]float64:
			if v, ok := r.vals[i].([]float64); ok {
				*ptr = v
			} else {
				*ptr = nil
			}
		}
	}
	return nil
}

type happyTestRows struct {
	rows [][]any
	pos  int
}

func newHappyRows(rows [][]any) *happyTestRows { return &happyTestRows{rows: rows, pos: -1} }

func (r *happyTestRows) Close() {}
func (r *happyTestRows) Err() error { return nil }
func (r *happyTestRows) CommandTag() pgconn.CommandTag { return pgconn.CommandTag{} }
func (r *happyTestRows) FieldDescriptions() []pgconn.FieldDescription { return nil }
func (r *happyTestRows) Conn() *pgx.Conn { return nil }
func (r *happyTestRows) RawValues() [][]byte { return nil }
func (r *happyTestRows) Values() ([]any, error) { return nil, nil }
func (r *happyTestRows) Next() bool {
	r.pos++
	return r.pos < len(r.rows)
}
func (r *happyTestRows) Scan(dest ...any) error {
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
			if row[i] == nil {
				*ptr = nil
			} else if v, ok := row[i].(string); ok {
				*ptr = &v
			}
		case *[]string:
			if v, ok := row[i].([]string); ok {
				*ptr = v
			} else {
				*ptr = []string{}
			}
		case *[]float64:
			if v, ok := row[i].([]float64); ok {
				*ptr = v
			} else {
				*ptr = nil
			}
		}
	}
	return nil
}

type happyTestQuerier struct {
	execFn     func(sql string, args ...any) (pgconn.CommandTag, error)
	queryFn    func(sql string, args ...any) (pgx.Rows, error)
	queryRowFn func(sql string, args ...any) pgx.Row
}

func (q *happyTestQuerier) Exec(_ context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	if q.execFn != nil {
		return q.execFn(sql, args...)
	}
	return pgconn.NewCommandTag("INSERT 0 1"), nil
}

func (q *happyTestQuerier) Query(_ context.Context, sql string, args ...any) (pgx.Rows, error) {
	if q.queryFn != nil {
		return q.queryFn(sql, args...)
	}
	return newHappyRows(nil), nil
}

func (q *happyTestQuerier) QueryRow(_ context.Context, sql string, args ...any) pgx.Row {
	if q.queryRowFn != nil {
		return q.queryRowFn(sql, args...)
	}
	return &happyTestRow{err: pgx.ErrNoRows}
}

// makeTestConfig creates a Config with an injected stub DB and nil PgPool.
func makeTestConfig(db audit.AuditQuerier) Config {
	return Config{
		testAuditDB: db,
		PgPool:      nil,
		EmbedModel:  "test-model",
	}
}

// makeNilEnginePool creates an EnginePool that returns nil handles.
func makeNilEnginePool() *EnginePool {
	return NewEnginePool(func(_ context.Context, _ string) (*EngineHandle, error) {
		return nil, nil
	})
}

// --- handler happy path tests ---

func TestHandleAuditAddQuery_HappyPath(t *testing.T) {
	var capturedID string
	db := &happyTestQuerier{
		execFn: func(_ string, args ...any) (pgconn.CommandTag, error) {
			if len(args) > 0 {
				if id, ok := args[0].(string); ok {
					capturedID = id
				}
			}
			return pgconn.NewCommandTag("INSERT 0 1"), nil
		},
	}
	cfg := makeTestConfig(db)
	pool := makeNilEnginePool()

	result, err := handleMemoryAuditAddQuery(context.Background(), pool,
		auditHandlerRequest(map[string]any{
			"project": "proj1",
			"query":   "test query",
		}), cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	m := extractAuditResult(t, result)
	if m["id"] == "" || m["id"] == nil {
		t.Errorf("expected non-empty id in result")
	}
	if capturedID == "" {
		t.Error("expected exec to be called with an ID")
	}
}

func TestHandleAuditAddQuery_MissingProject(t *testing.T) {
	cfg := makeTestConfig(&happyTestQuerier{})
	pool := makeNilEnginePool()
	_, err := handleMemoryAuditAddQuery(context.Background(), pool,
		auditHandlerRequest(map[string]any{"query": "q"}), cfg)
	if err == nil {
		t.Error("expected error for missing project")
	}
}

func TestHandleAuditAddQuery_MissingQuery(t *testing.T) {
	cfg := makeTestConfig(&happyTestQuerier{})
	pool := makeNilEnginePool()
	_, err := handleMemoryAuditAddQuery(context.Background(), pool,
		auditHandlerRequest(map[string]any{"project": "proj1"}), cfg)
	if err == nil {
		t.Error("expected error for missing query")
	}
}

func TestHandleAuditListQueries_HappyPath(t *testing.T) {
	now := time.Now()
	rows := [][]any{
		{"q1", "proj1", "query 1", nil, true, nil, now},
		{"q2", "proj1", "query 2", nil, false, nil, now},
	}
	db := &happyTestQuerier{
		queryFn: func(_ string, _ ...any) (pgx.Rows, error) {
			return newHappyRows(rows), nil
		},
	}
	cfg := makeTestConfig(db)
	pool := makeNilEnginePool()

	result, err := handleMemoryAuditListQueries(context.Background(), pool,
		auditHandlerRequest(map[string]any{"project": "proj1"}), cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	m := extractAuditResult(t, result)
	count, _ := m["count"].(float64)
	if int(count) != 2 {
		t.Errorf("expected count=2, got %v", m["count"])
	}
}

func TestHandleAuditDeactivateQuery_HappyPath(t *testing.T) {
	db := &happyTestQuerier{
		execFn: func(_ string, _ ...any) (pgconn.CommandTag, error) {
			return pgconn.NewCommandTag("UPDATE 1"), nil
		},
	}
	cfg := makeTestConfig(db)
	pool := makeNilEnginePool()

	result, err := handleMemoryAuditDeactivateQuery(context.Background(), pool,
		auditHandlerRequest(map[string]any{"query_id": "q1"}), cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	m := extractAuditResult(t, result)
	if m["status"] != "deactivated" {
		t.Errorf("expected status=deactivated, got %v", m["status"])
	}
}

func TestHandleAuditDeactivateQuery_UnknownID(t *testing.T) {
	db := &happyTestQuerier{
		execFn: func(_ string, _ ...any) (pgconn.CommandTag, error) {
			return pgconn.NewCommandTag("UPDATE 0"), nil
		},
	}
	cfg := makeTestConfig(db)
	pool := makeNilEnginePool()

	_, err := handleMemoryAuditDeactivateQuery(context.Background(), pool,
		auditHandlerRequest(map[string]any{"query_id": "missing"}), cfg)
	if err == nil {
		t.Error("expected error for unknown query_id")
	}
}

func TestHandleAuditRun_NoQueries(t *testing.T) {
	db := &happyTestQuerier{
		queryFn: func(_ string, _ ...any) (pgx.Rows, error) {
			return newHappyRows(nil), nil
		},
	}
	cfg := makeTestConfig(db)
	pool := makeNilEnginePool()

	result, err := handleMemoryAuditRun(context.Background(), pool,
		auditHandlerRequest(map[string]any{"project": "proj1"}), cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	m := extractAuditResult(t, result)
	count, _ := m["count"].(float64)
	if int(count) != 0 {
		t.Errorf("expected count=0, got %v", m["count"])
	}
}

func TestHandleAuditRun_HappyPath(t *testing.T) {
	now := time.Now()
	queryRows := [][]any{
		{"q1", "proj1", "test query", nil, true, nil, now},
	}
	queryCalled := 0
	db := &happyTestQuerier{
		queryFn: func(_ string, _ ...any) (pgx.Rows, error) {
			queryCalled++
			if queryCalled == 1 {
				return newHappyRows(queryRows), nil
			}
			return newHappyRows(nil), nil
		},
		queryRowFn: func(_ string, _ ...any) pgx.Row {
			return &happyTestRow{err: pgx.ErrNoRows} // baseline: no prev snapshot
		},
		execFn: func(_ string, _ ...any) (pgconn.CommandTag, error) {
			return pgconn.NewCommandTag("INSERT 0 1"), nil
		},
	}
	// Need an engine pool that provides IDs for the recaller.
	pool := NewEnginePool(func(_ context.Context, _ string) (*EngineHandle, error) {
		return nil, fmt.Errorf("no engine in test") // recaller will fail
	})
	cfg := makeTestConfig(db)

	// Even if the recaller fails, the handler should not error — it logs and continues.
	result, err := handleMemoryAuditRun(context.Background(), pool,
		auditHandlerRequest(map[string]any{"project": "proj1"}), cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	m := extractAuditResult(t, result)
	if m["project"] != "proj1" {
		t.Errorf("expected project=proj1, got %v", m["project"])
	}
}

func TestHandleAuditCompare_HappyPath(t *testing.T) {
	now := time.Now()
	snapRows := [][]any{
		{"s1", "q1", "proj1", []string{"m1", "m2"}, nil, "test-model", nil, now, nil, nil, nil, nil, nil, nil},
		{"s2", "q1", "proj1", []string{"m2", "m3"}, nil, "test-model", nil, now.Add(-time.Hour), nil, nil, nil, nil, nil, nil},
	}
	db := &happyTestQuerier{
		queryFn: func(_ string, _ ...any) (pgx.Rows, error) {
			return newHappyRows(snapRows), nil
		},
	}
	cfg := makeTestConfig(db)
	pool := makeNilEnginePool()

	result, err := handleMemoryAuditCompare(context.Background(), pool,
		auditHandlerRequest(map[string]any{"query_id": "q1"}), cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	m := extractAuditResult(t, result)
	count, _ := m["count"].(float64)
	if int(count) != 2 {
		t.Errorf("expected count=2, got %v", m["count"])
	}
}

func TestHandleAuditCompare_SingleSnapshot(t *testing.T) {
	now := time.Now()
	snapRows := [][]any{
		{"s1", "q1", "proj1", []string{"m1"}, nil, "test-model", nil, now, nil, nil, nil, nil, nil, nil},
	}
	db := &happyTestQuerier{
		queryFn: func(_ string, _ ...any) (pgx.Rows, error) {
			return newHappyRows(snapRows), nil
		},
	}
	cfg := makeTestConfig(db)
	pool := makeNilEnginePool()

	result, err := handleMemoryAuditCompare(context.Background(), pool,
		auditHandlerRequest(map[string]any{"query_id": "q1"}), cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	m := extractAuditResult(t, result)
	count, _ := m["count"].(float64)
	if int(count) != 1 {
		t.Errorf("expected count=1, got %v", m["count"])
	}
}

func TestHandleAuditCompare_UnknownQueryID(t *testing.T) {
	// Unknown query_id — returns empty list (not an error).
	db := &happyTestQuerier{
		queryFn: func(_ string, _ ...any) (pgx.Rows, error) {
			return newHappyRows(nil), nil
		},
	}
	cfg := makeTestConfig(db)
	pool := makeNilEnginePool()

	result, err := handleMemoryAuditCompare(context.Background(), pool,
		auditHandlerRequest(map[string]any{"query_id": "unknown"}), cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	m := extractAuditResult(t, result)
	count, _ := m["count"].(float64)
	if int(count) != 0 {
		t.Errorf("expected count=0 for unknown query, got %v", m["count"])
	}
}
