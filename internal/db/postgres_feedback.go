package db

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/petersimmons1972/engram/internal/types"
)

// Begin starts a new transaction.
func (b *PostgresBackend) Begin(ctx context.Context) (Tx, error) {
	tx, err := b.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	return &pgxTx{tx: tx}, nil
}

// StoreRetrievalEvent persists a new retrieval event.
func (b *PostgresBackend) StoreRetrievalEvent(ctx context.Context, event *types.RetrievalEvent) error {
	resultIDsJSON, err := json.Marshal(event.ResultIDs)
	if err != nil {
		return fmt.Errorf("marshal result_ids: %w", err)
	}
	_, err = b.pool.Exec(ctx, `
		INSERT INTO retrieval_events (id, project, query, result_ids, created_at)
		VALUES ($1, $2, $3, $4, $5)`,
		event.ID, event.Project, event.Query, resultIDsJSON, event.CreatedAt,
	)
	return err
}

// GetRetrievalEvent fetches a retrieval event by ID. Returns nil, nil if not found.
func (b *PostgresBackend) GetRetrievalEvent(ctx context.Context, id string) (*types.RetrievalEvent, error) {
	var event types.RetrievalEvent
	var resultIDsJSON, feedbackIDsJSON []byte
	var failureClass *string
	err := b.pool.QueryRow(ctx, `
		SELECT id, project, query, result_ids, feedback_ids, created_at, feedback_at, failure_class
		FROM retrieval_events WHERE id=$1`,
		id,
	).Scan(&event.ID, &event.Project, &event.Query, &resultIDsJSON, &feedbackIDsJSON,
		&event.CreatedAt, &event.FeedbackAt, &failureClass)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if failureClass != nil {
		event.FailureClass = *failureClass
	}
	if len(resultIDsJSON) > 0 {
		if err := json.Unmarshal(resultIDsJSON, &event.ResultIDs); err != nil {
			return nil, fmt.Errorf("unmarshal result_ids: %w", err)
		}
	}
	if event.ResultIDs == nil {
		event.ResultIDs = []string{}
	}
	if len(feedbackIDsJSON) > 0 {
		if err := json.Unmarshal(feedbackIDsJSON, &event.FeedbackIDs); err != nil {
			return nil, fmt.Errorf("unmarshal feedback_ids: %w", err)
		}
	}
	return &event, nil
}

// RecordFeedback updates the retrieval event with feedback_ids, sets feedback_at=NOW(),
// increments times_retrieved on all result memories, increments times_useful on useful
// memories, and recomputes retrieval_precision once times_retrieved >= 5.
// All four writes are wrapped in a single transaction so a mid-operation crash cannot
// corrupt the spaced-repetition schedule (fix for #101).
func (b *PostgresBackend) RecordFeedback(ctx context.Context, eventID string, usefulIDs []string) error {
	event, err := b.GetRetrievalEvent(ctx, eventID)
	if err != nil {
		return fmt.Errorf("RecordFeedback get event: %w", err)
	}
	if event == nil {
		return fmt.Errorf("retrieval event %q not found", eventID)
	}

	feedbackIDsJSON, err := json.Marshal(usefulIDs)
	if err != nil {
		return fmt.Errorf("marshal feedback_ids: %w", err)
	}

	tx, err := b.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("RecordFeedback begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx, `
		UPDATE retrieval_events SET feedback_ids=$1, feedback_at=NOW() WHERE id=$2`,
		feedbackIDsJSON, eventID,
	); err != nil {
		return fmt.Errorf("update retrieval event: %w", err)
	}

	// Increment times_retrieved on all result memories.
	if len(event.ResultIDs) > 0 {
		if _, err := tx.Exec(ctx, `
			UPDATE memories SET times_retrieved = COALESCE(times_retrieved, 0) + 1 WHERE id = ANY($1)`,
			event.ResultIDs,
		); err != nil {
			return fmt.Errorf("increment times_retrieved: %w", err)
		}
	}

	// Increment times_useful on useful memories.
	if len(usefulIDs) > 0 {
		if _, err := tx.Exec(ctx, `
			UPDATE memories SET times_useful = COALESCE(times_useful, 0) + 1 WHERE id = ANY($1)`,
			usefulIDs,
		); err != nil {
			return fmt.Errorf("increment times_useful: %w", err)
		}
	}

	// Recompute precision for result memories that have reached the threshold.
	if len(event.ResultIDs) > 0 {
		if _, err := tx.Exec(ctx, `
			UPDATE memories
			SET retrieval_precision = CAST(times_useful AS DOUBLE PRECISION) / times_retrieved
			WHERE id = ANY($1) AND times_retrieved >= 5`,
			event.ResultIDs,
		); err != nil {
			return fmt.Errorf("recompute precision: %w", err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("RecordFeedback commit: %w", err)
	}
	return nil
}

// RecordFeedbackWithClass is like RecordFeedback but also sets failure_class on
// the retrieval event. An empty failureClass stores NULL. The function does NOT
// perform the edge boost present in RecordFeedback — wrong memories must not be
// reinforced.
func (b *PostgresBackend) RecordFeedbackWithClass(ctx context.Context, eventID string, usefulIDs []string, failureClass string) error {
	event, err := b.GetRetrievalEvent(ctx, eventID)
	if err != nil {
		return fmt.Errorf("RecordFeedbackWithClass get event: %w", err)
	}
	if event == nil {
		return fmt.Errorf("retrieval event %q not found", eventID)
	}

	feedbackIDsJSON, err := json.Marshal(usefulIDs)
	if err != nil {
		return fmt.Errorf("marshal feedback_ids: %w", err)
	}

	tx, err := b.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("RecordFeedbackWithClass begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var fcParam interface{}
	if failureClass != "" {
		fcParam = failureClass
	}
	if _, err := tx.Exec(ctx, `
		UPDATE retrieval_events SET feedback_ids=$1, feedback_at=NOW(), failure_class=$3 WHERE id=$2`,
		feedbackIDsJSON, eventID, fcParam,
	); err != nil {
		return fmt.Errorf("update retrieval event: %w", err)
	}

	// Increment times_retrieved on all result memories.
	if len(event.ResultIDs) > 0 {
		if _, err := tx.Exec(ctx, `
			UPDATE memories SET times_retrieved = COALESCE(times_retrieved, 0) + 1 WHERE id = ANY($1)`,
			event.ResultIDs,
		); err != nil {
			return fmt.Errorf("increment times_retrieved: %w", err)
		}
	}

	// Increment times_useful on useful memories.
	if len(usefulIDs) > 0 {
		if _, err := tx.Exec(ctx, `
			UPDATE memories SET times_useful = COALESCE(times_useful, 0) + 1 WHERE id = ANY($1)`,
			usefulIDs,
		); err != nil {
			return fmt.Errorf("increment times_useful: %w", err)
		}
	}

	// Recompute precision for result memories that have reached the threshold.
	if len(event.ResultIDs) > 0 {
		if _, err := tx.Exec(ctx, `
			UPDATE memories
			SET retrieval_precision = CAST(times_useful AS DOUBLE PRECISION) / times_retrieved
			WHERE id = ANY($1) AND times_retrieved >= 5`,
			event.ResultIDs,
		); err != nil {
			return fmt.Errorf("recompute precision: %w", err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("RecordFeedbackWithClass commit: %w", err)
	}
	return nil
}

// IncrementTimesRetrieved increments times_retrieved by 1 on each of the given memory IDs.
func (b *PostgresBackend) IncrementTimesRetrieved(ctx context.Context, ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	_, err := b.pool.Exec(ctx, `
		UPDATE memories SET times_retrieved = COALESCE(times_retrieved, 0) + 1 WHERE id = ANY($1)`,
		ids,
	)
	return err
}

// UpdateDynamicImportance atomically adjusts dynamic_importance by delta and,
// when intervalFactor > 0, advances retrieval_interval_hrs *= intervalFactor and
// sets next_review_at = now + new interval.
func (b *PostgresBackend) UpdateDynamicImportance(ctx context.Context, id string, delta float64, intervalFactor float64) error {
	if intervalFactor > 0 {
		_, err := b.pool.Exec(ctx, `
			UPDATE memories
			SET dynamic_importance     = GREATEST(0.1, COALESCE(dynamic_importance, 1.0) + $1),
			    retrieval_interval_hrs = GREATEST(1, COALESCE(retrieval_interval_hrs, 168) * $2),
			    next_review_at         = NOW() + (GREATEST(1, COALESCE(retrieval_interval_hrs, 168) * $2) * INTERVAL '1 hour')
			WHERE id = $3`,
			delta, intervalFactor, id,
		)
		return err
	}
	_, err := b.pool.Exec(ctx, `
		UPDATE memories
		SET dynamic_importance = GREATEST(0.1, COALESCE(dynamic_importance, 1.0) + $1)
		WHERE id = $2`,
		delta, id,
	)
	return err
}
