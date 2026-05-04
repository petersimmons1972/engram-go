package audit

// Tests for the alert_threshold feature (F2, closes #275).
// Verifies that slog.Error is called when RBO drops below the threshold and
// is NOT called when RBO is at or above it.
//
// Because slog.Error writes to the global logger, we intercept it by
// installing a custom slog.Handler for the duration of each test.

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// captureHandler records every slog.Record that passes its level gate.
type captureHandler struct {
	level   slog.Level
	records []slog.Record
}

func (h *captureHandler) Enabled(_ context.Context, l slog.Level) bool { return l >= h.level }
func (h *captureHandler) Handle(_ context.Context, r slog.Record) error {
	h.records = append(h.records, r)
	return nil
}
func (h *captureHandler) WithAttrs(_ []slog.Attr) slog.Handler { return h }
func (h *captureHandler) WithGroup(_ string) slog.Handler      { return h }

// withCaptureLogger installs a capturing handler as the default slog logger
// for the duration of the test, then restores the original handler.
func withCaptureLogger(t *testing.T) *captureHandler {
	t.Helper()
	original := slog.Default()
	h := &captureHandler{level: slog.LevelError}
	slog.SetDefault(slog.New(h))
	t.Cleanup(func() { slog.SetDefault(original) })
	return h
}

// buildAlertScenario constructs the worker and DB stub for alert-threshold tests.
func buildAlertScenario(t *testing.T, threshold, rboResult float64, prevIDs, newIDs []string) (*AuditWorker, *captureHandler) {
	t.Helper()

	now := time.Now()
	queryRows := [][]any{
		// id, project, query, description, active, alert_threshold, created_at
		{"q1", "proj1", "test query", (*string)(nil), true, &threshold, now},
	}

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
			// latestSnapshot returns prevIDs so RBO is computed against them.
			var scores []float64
			return &stubRow{vals: []any{
				"snap-prev", "q1", "proj1",
				prevIDs, scores,
				"test-model", (*string)(nil), now.Add(-time.Hour),
			}}
		},
		execFn: func(_ context.Context, _ string, _ ...any) (pgconn.CommandTag, error) {
			return pgconn.NewCommandTag("INSERT 0 1"), nil
		},
	}

	recaller := &stubRecaller{ids: newIDs}
	w := makeWorker(db, recaller)

	h := withCaptureLogger(t)
	return w, h
}

// TestAlertThreshold_FiresWhenBelow verifies that slog.Error is called when
// the computed RBO is below the configured alert threshold.
//
// We engineer a large ranking change: prevIDs = [A,B,C,D,E] vs newIDs = [F,G,H,I,J]
// (completely disjoint). RBO(p=0.9) over completely disjoint lists is 0.
// A threshold of 0.5 is above 0, so the alert must fire.
func TestAlertThreshold_FiresWhenBelow(t *testing.T) {
	prevIDs := []string{"A", "B", "C", "D", "E"}
	newIDs := []string{"F", "G", "H", "I", "J"}
	threshold := 0.5 // RBO will be 0.0 — below threshold

	w, h := buildAlertScenario(t, threshold, 0, prevIDs, newIDs)

	summaries, err := w.RunProjectAudit(context.Background(), "proj1")
	if err != nil {
		t.Fatalf("RunProjectAudit: %v", err)
	}
	if len(summaries) != 1 {
		t.Fatalf("expected 1 summary, got %d", len(summaries))
	}
	if summaries[0].RBOVsPrev == nil {
		t.Fatal("expected non-nil RBOVsPrev")
	}

	// Verify the alert fired.
	found := false
	for _, rec := range h.records {
		if rec.Level == slog.LevelError {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected slog.Error alert when RBO=%.2f < threshold=%.2f, but no ERROR record captured",
			*summaries[0].RBOVsPrev, threshold)
	}
}

// TestAlertThreshold_SilentWhenAbove verifies that slog.Error is NOT called
// when the computed RBO is at or above the configured alert threshold.
//
// We use identical ID lists: prev = [A,B,C,D,E], new = [A,B,C,D,E].
// RBO(p=0.9) for identical 5-item lists ≈ 0.41. We set threshold=0.1 so
// that RBO (≈0.41) is clearly above the threshold and no alert fires.
func TestAlertThreshold_SilentWhenAbove(t *testing.T) {
	ids := []string{"A", "B", "C", "D", "E"}
	threshold := 0.1 // RBO for identical lists (~0.41) is above this threshold

	w, h := buildAlertScenario(t, threshold, 0.0, ids, ids)

	summaries, err := w.RunProjectAudit(context.Background(), "proj1")
	if err != nil {
		t.Fatalf("RunProjectAudit: %v", err)
	}
	if len(summaries) != 1 {
		t.Fatalf("expected 1 summary, got %d", len(summaries))
	}

	// Verify no ERROR record was emitted.
	for _, rec := range h.records {
		if rec.Level == slog.LevelError {
			t.Errorf("unexpected slog.Error when RBO=1.0 >= threshold=%.2f: %s", threshold, rec.Message)
		}
	}
}

// TestAlertThreshold_NilThresholdIsNoOp verifies that when AlertThreshold is nil
// the alert is never fired, even for a 0.0 RBO.
func TestAlertThreshold_NilThresholdIsNoOp(t *testing.T) {
	prevIDs := []string{"A", "B", "C"}
	newIDs := []string{"X", "Y", "Z"}

	now := time.Now()
	queryRows := [][]any{
		// alert_threshold is nil
		{"q1", "proj1", "test query", (*string)(nil), true, (*float64)(nil), now},
	}
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
			var scores []float64
			return &stubRow{vals: []any{
				"snap-prev", "q1", "proj1",
				prevIDs, scores,
				"test-model", (*string)(nil), now.Add(-time.Hour),
			}}
		},
		execFn: func(_ context.Context, _ string, _ ...any) (pgconn.CommandTag, error) {
			return pgconn.NewCommandTag("INSERT 0 1"), nil
		},
	}

	w := makeWorker(db, &stubRecaller{ids: newIDs})
	h := withCaptureLogger(t)

	if _, err := w.RunProjectAudit(context.Background(), "proj1"); err != nil {
		t.Fatalf("RunProjectAudit: %v", err)
	}
	for _, rec := range h.records {
		if rec.Level == slog.LevelError {
			t.Errorf("unexpected slog.Error when threshold is nil: %s", rec.Message)
		}
	}
	_ = h
}
