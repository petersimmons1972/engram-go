// Package weight implements adaptive weight tuning for composite retrieval scoring.
// It analyzes retrieval failure events and adjusts per-project weights within
// defined guardrails when a dominant failure class is detected.
package weight

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Default weights — must match the compile-time constants in internal/search/score.go.
const (
	DefaultVector    = 0.45
	DefaultBM25      = 0.30
	DefaultRecency   = 0.10
	DefaultPrecision = 0.15
)

// Guardrail bounds for each weight.
const (
	minVector    = 0.30
	maxVector    = 0.65
	minBM25      = 0.15
	maxBM25      = 0.45
	minRecency   = 0.05
	maxRecency   = 0.20
	minPrecision = 0.05
	maxPrecision = 0.25
)

// Tuning policy constants.
const (
	minEventsBeforeTuning = 50    // minimum failure events in window before firing
	dominantThreshold     = 0.40  // ≥40% of relevant events
	dominantMargin        = 0.10  // ≥10pp above runner-up
	tuningCooldownDays    = 7     // max once per 7 days per project
	lockKey               = 7332  // advisory lock key
)

// Weights holds one complete set of scoring weights.
type Weights struct {
	Vector    float64 `json:"weight_vector"`
	BM25      float64 `json:"weight_bm25"`
	Recency   float64 `json:"weight_recency"`
	Precision float64 `json:"weight_precision"`
}

// DefaultWeights returns the compile-time weight constants.
func DefaultWeights() Weights {
	return Weights{
		Vector:    DefaultVector,
		BM25:      DefaultBM25,
		Recency:   DefaultRecency,
		Precision: DefaultPrecision,
	}
}

// WeightHistory is one recorded weight adjustment.
type WeightHistory struct {
	ID          string          `json:"id"`
	Project     string          `json:"project"`
	AppliedAt   time.Time       `json:"applied_at"`
	Weights     Weights         `json:"weights"`
	TriggerData json.RawMessage `json:"trigger_data,omitempty"`
	Notes       string          `json:"notes,omitempty"`
}

// TunerWorker periodically checks failure event distributions and applies
// weight adjustments when a dominant failure class is detected.
type TunerWorker struct {
	pool     *pgxpool.Pool
	interval time.Duration
}

// NewTunerWorker creates a TunerWorker.
func NewTunerWorker(pool *pgxpool.Pool, interval time.Duration) *TunerWorker {
	return &TunerWorker{pool: pool, interval: interval}
}

// Run starts the background tuning loop. Call as a goroutine.
func (w *TunerWorker) Run(ctx context.Context) {
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			func() {
				defer func() {
					if r := recover(); r != nil {
						slog.Error("weight tuner: panic", "err", r)
					}
				}()
				if err := w.RunPass(ctx); err != nil {
					slog.Error("weight tuner: pass failed", "err", err)
				}
			}()
		}
	}
}

// RunPass examines all projects with recent failure events and applies
// weight adjustments where warranted.
func (w *TunerWorker) RunPass(ctx context.Context) error {
	var locked bool
	if err := w.pool.QueryRow(ctx, "SELECT pg_try_advisory_lock($1)", lockKey).Scan(&locked); err != nil {
		return fmt.Errorf("advisory lock: %w", err)
	}
	if !locked {
		slog.Info("weight tuner: another pass is running, skipping")
		return nil
	}
	defer func() {
		if err := w.pool.QueryRow(ctx, "SELECT pg_advisory_unlock($1)", lockKey).Scan(new(bool)); err != nil {
			slog.Warn("weight tuner: advisory unlock failed", "err", err)
		}
	}()

	projects, err := w.projectsWithRecentEvents(ctx)
	if err != nil {
		return fmt.Errorf("list projects: %w", err)
	}

	for _, project := range projects {
		if err := w.maybeAdjust(ctx, project); err != nil {
			slog.Error("weight tuner: adjust failed", "project", project, "err", err)
		}
	}
	return nil
}

// AdjustWeightsForProject is the externally-callable version for tests and
// on-demand triggering. It respects the same guardrails as the background loop.
func (w *TunerWorker) AdjustWeightsForProject(ctx context.Context, project string) error {
	return w.maybeAdjust(ctx, project)
}

// LoadWeights loads weights for a project from the DB.
// Returns defaults if no entry exists.
func (w *TunerWorker) LoadWeights(ctx context.Context, project string) (Weights, error) {
	var wt Weights
	err := w.pool.QueryRow(ctx,
		`SELECT weight_vector, weight_bm25, weight_recency, weight_precision
		 FROM weight_config WHERE project = $1`,
		project,
	).Scan(&wt.Vector, &wt.BM25, &wt.Recency, &wt.Precision)
	if err != nil {
		if err.Error() == "no rows in result set" {
			return DefaultWeights(), nil
		}
		return Weights{}, fmt.Errorf("load weights: %w", err)
	}
	return wt, nil
}

// GetHistory returns weight history for a project, newest first.
func (w *TunerWorker) GetHistory(ctx context.Context, project string, limit int) ([]WeightHistory, error) {
	if limit <= 0 {
		limit = 20
	}
	rows, err := w.pool.Query(ctx,
		`SELECT id, project, applied_at, weight_vector, weight_bm25, weight_recency, weight_precision,
		        trigger_data, notes
		 FROM weight_history
		 WHERE project = $1
		 ORDER BY applied_at DESC
		 LIMIT $2`,
		project, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("query weight_history: %w", err)
	}
	defer rows.Close()

	var history []WeightHistory
	for rows.Next() {
		var h WeightHistory
		var triggerData []byte
		var notes *string
		if err := rows.Scan(
			&h.ID, &h.Project, &h.AppliedAt,
			&h.Weights.Vector, &h.Weights.BM25, &h.Weights.Recency, &h.Weights.Precision,
			&triggerData, &notes,
		); err != nil {
			return nil, fmt.Errorf("scan weight_history: %w", err)
		}
		if triggerData != nil {
			h.TriggerData = triggerData
		}
		if notes != nil {
			h.Notes = *notes
		}
		history = append(history, h)
	}
	return history, rows.Err()
}

// ResetToDefaults resets weight_config to defaults for a project.
// Used during embedder migration to clear learned weights.
func (w *TunerWorker) ResetToDefaults(ctx context.Context, pool *pgxpool.Pool, project string) error {
	_, err := pool.Exec(ctx, `DELETE FROM weight_config WHERE project = $1`, project)
	return err
}

// --- internal methods ---

// maybeAdjust checks whether to adjust weights for a single project.
func (w *TunerWorker) maybeAdjust(ctx context.Context, project string) error {
	// Check cooldown: skip if a tuning was applied in the last 7 days.
	var lastApplied *time.Time
	err := w.pool.QueryRow(ctx,
		`SELECT applied_at FROM weight_history WHERE project = $1 ORDER BY applied_at DESC LIMIT 1`,
		project,
	).Scan(&lastApplied)
	if err != nil && err.Error() != "no rows in result set" {
		return fmt.Errorf("check cooldown: %w", err)
	}
	if lastApplied != nil && time.Since(*lastApplied) < tuningCooldownDays*24*time.Hour {
		slog.Debug("weight tuner: cooldown active", "project", project,
			"last_applied", lastApplied.Format(time.RFC3339))
		return nil
	}

	// Aggregate failure events for the last 30 days.
	type classCount struct {
		class string
		count int
	}
	rows, err := w.pool.Query(ctx,
		`SELECT failure_class, COUNT(*) AS cnt
		 FROM retrieval_events
		 WHERE project = $1
		   AND failure_class IS NOT NULL
		   AND created_at >= NOW() - INTERVAL '30 days'
		 GROUP BY failure_class
		 ORDER BY cnt DESC`,
		project,
	)
	if err != nil {
		return fmt.Errorf("aggregate failure classes: %w", err)
	}
	defer rows.Close()

	var counts []classCount
	total := 0
	for rows.Next() {
		var cc classCount
		if err := rows.Scan(&cc.class, &cc.count); err != nil {
			return fmt.Errorf("scan failure class: %w", err)
		}
		counts = append(counts, cc)
		total += cc.count
	}
	if err := rows.Err(); err != nil {
		return err
	}

	if total < minEventsBeforeTuning {
		return nil // not enough data
	}

	if len(counts) == 0 {
		return nil
	}

	// Determine dominant failure class.
	dominant := counts[0]
	domFrac := float64(dominant.count) / float64(total)

	var runnerUpFrac float64
	if len(counts) > 1 {
		runnerUpFrac = float64(counts[1].count) / float64(total)
	}

	if domFrac < dominantThreshold {
		return nil // not dominant enough
	}
	if domFrac-runnerUpFrac < dominantMargin {
		return nil // not enough margin over runner-up
	}

	// Only actionable classes trigger weight changes.
	delta := computeDelta(dominant.class)
	if delta == nil {
		return nil
	}

	// Load current weights.
	current, err := w.LoadWeights(ctx, project)
	if err != nil {
		return err
	}

	// Apply delta and clamp within guardrails.
	proposed := applyDelta(current, *delta)
	proposed, ok := normalizeWeights(proposed)
	if !ok {
		slog.Warn("weight tuner: normalization infeasible, skipping", "project", project)
		return nil
	}

	// Persist.
	triggerJSON, _ := json.Marshal(map[string]any{
		"dominant_class":   dominant.class,
		"dominant_frac":    domFrac,
		"runner_up_frac":   runnerUpFrac,
		"total_events":     total,
		"window_days":      30,
	})
	notes := fmt.Sprintf("auto-tuned: %s dominant at %.0f%% of %d events",
		dominant.class, domFrac*100, total)

	if err := w.applyWeights(ctx, project, proposed, triggerJSON, notes); err != nil {
		return fmt.Errorf("apply weights: %w", err)
	}
	slog.Info("weight tuner: weights adjusted",
		"project", project,
		"class", dominant.class,
		"new_weights", proposed,
	)
	return nil
}

// computeDelta returns the weight delta for an actionable failure class, or nil.
func computeDelta(class string) *Weights {
	switch class {
	case "stale_ranking":
		// Boost recency, reduce vector.
		return &Weights{Vector: -0.05, Recency: +0.05}
	case "vocabulary_mismatch":
		// BM25 failed (vocabulary couldn't match), boost vector.
		return &Weights{Vector: +0.05, BM25: -0.05}
	case "scope_mismatch":
		// Boost precision signal, reduce bm25.
		return &Weights{Precision: +0.03, BM25: -0.03}
	default:
		// aggregation_failure, missing_content, other: no weight change.
		return nil
	}
}

// applyDelta adds a delta to current weights without clamping.
func applyDelta(current, delta Weights) Weights {
	return Weights{
		Vector:    current.Vector + delta.Vector,
		BM25:      current.BM25 + delta.BM25,
		Recency:   current.Recency + delta.Recency,
		Precision: current.Precision + delta.Precision,
	}
}

// normalizeWeights clamps each weight within its guardrail bounds and
// normalizes so the sum equals 1.0. Returns (weights, ok). ok=false if
// the guardrail constraints make it impossible for the weights to sum to 1.0.
func normalizeWeights(w Weights) (Weights, bool) {
	w.Vector = math.Max(minVector, math.Min(maxVector, w.Vector))
	w.BM25 = math.Max(minBM25, math.Min(maxBM25, w.BM25))
	w.Recency = math.Max(minRecency, math.Min(maxRecency, w.Recency))
	w.Precision = math.Max(minPrecision, math.Min(maxPrecision, w.Precision))

	// Check feasibility: min sum and max sum within bounds.
	minSum := minVector + minBM25 + minRecency + minPrecision // 0.55
	maxSum := maxVector + maxBM25 + maxRecency + maxPrecision // 1.55
	if minSum > 1.0 || maxSum < 1.0 {
		return Weights{}, false
	}

	sum := w.Vector + w.BM25 + w.Recency + w.Precision
	if math.Abs(sum-1.0) < 1e-9 {
		return w, true
	}
	if sum == 0 {
		return Weights{}, false
	}

	// Scale proportionally.
	factor := 1.0 / sum
	w.Vector *= factor
	w.BM25 *= factor
	w.Recency *= factor
	w.Precision *= factor

	// After scaling, re-clamp to ensure we didn't violate bounds.
	w.Vector = math.Max(minVector, math.Min(maxVector, w.Vector))
	w.BM25 = math.Max(minBM25, math.Min(maxBM25, w.BM25))
	w.Recency = math.Max(minRecency, math.Min(maxRecency, w.Recency))
	w.Precision = math.Max(minPrecision, math.Min(maxPrecision, w.Precision))

	// Final sum check — if still out of tolerance, abort.
	finalSum := w.Vector + w.BM25 + w.Recency + w.Precision
	if math.Abs(finalSum-1.0) > 0.01 {
		return Weights{}, false
	}
	return w, true
}

// applyWeights persists new weights to weight_config and records the history entry.
func (w *TunerWorker) applyWeights(ctx context.Context, project string, wt Weights,
	triggerData []byte, notes string) error {
	tx, err := w.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	_, err = tx.Exec(ctx,
		`INSERT INTO weight_config (project, weight_vector, weight_bm25, weight_recency, weight_precision, updated_at)
		 VALUES ($1,$2,$3,$4,$5,NOW())
		 ON CONFLICT (project) DO UPDATE SET
		   weight_vector=$2, weight_bm25=$3, weight_recency=$4, weight_precision=$5, updated_at=NOW()`,
		project, wt.Vector, wt.BM25, wt.Recency, wt.Precision,
	)
	if err != nil {
		return fmt.Errorf("upsert weight_config: %w", err)
	}

	_, err = tx.Exec(ctx,
		`INSERT INTO weight_history (id, project, weight_vector, weight_bm25, weight_recency, weight_precision, trigger_data, notes)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`,
		uuid.New().String(), project, wt.Vector, wt.BM25, wt.Recency, wt.Precision, triggerData, notes,
	)
	if err != nil {
		return fmt.Errorf("insert weight_history: %w", err)
	}

	return tx.Commit(ctx)
}

// projectsWithRecentEvents returns distinct project names with failure events
// in the last 30 days.
func (w *TunerWorker) projectsWithRecentEvents(ctx context.Context) ([]string, error) {
	rows, err := w.pool.Query(ctx,
		`SELECT DISTINCT project FROM retrieval_events
		 WHERE failure_class IS NOT NULL
		   AND created_at >= NOW() - INTERVAL '30 days'`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var projects []string
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err != nil {
			return nil, err
		}
		projects = append(projects, p)
	}
	return projects, rows.Err()
}
