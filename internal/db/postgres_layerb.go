package db

import (
	"context"
	"fmt"

	"github.com/petersimmons1972/engram/internal/layerb"
)

// UpsertLayerBAtom inserts or refreshes one deterministic Layer B atom.
func (p *PostgresBackend) UpsertLayerBAtom(ctx context.Context, atom layerb.Atom) error {
	_, err := p.pool.Exec(ctx, `
		INSERT INTO layer_b_atoms (
			project, provenance_memory_id, provenance_span, span_text,
			statement, normalized_text, event_time
		) VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (project, provenance_memory_id, provenance_span, normalized_text)
		DO UPDATE SET
			span_text = EXCLUDED.span_text,
			statement = EXCLUDED.statement,
			event_time = EXCLUDED.event_time`,
		atom.Project, atom.MemoryID, atom.ProvenanceSpan, atom.SpanText,
		atom.Statement, atom.NormalizedText, atom.EventTime,
	)
	if err != nil {
		return fmt.Errorf("UpsertLayerBAtom: %w", err)
	}
	return nil
}

// UpsertLayerBEvent inserts or refreshes one deterministic Layer B event.
func (p *PostgresBackend) UpsertLayerBEvent(ctx context.Context, event layerb.Event) error {
	_, err := p.pool.Exec(ctx, `
		INSERT INTO layer_b_events (
			project, provenance_memory_id, provenance_span, span_text,
			anchor, normalized_text, event_time
		) VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (project, provenance_memory_id, provenance_span, anchor)
		DO UPDATE SET
			span_text = EXCLUDED.span_text,
			normalized_text = EXCLUDED.normalized_text,
			event_time = EXCLUDED.event_time`,
		event.Project, event.MemoryID, event.ProvenanceSpan, event.SpanText,
		event.Anchor, event.NormalizedText, event.EventTime,
	)
	if err != nil {
		return fmt.Errorf("UpsertLayerBEvent: %w", err)
	}
	return nil
}

// ListLayerBEvents returns all Layer B events for the requested memories.
func (p *PostgresBackend) ListLayerBEvents(ctx context.Context, project string, memoryIDs []string) ([]layerb.EventRecord, error) {
	if len(memoryIDs) == 0 {
		return nil, nil
	}
	rows, err := p.pool.Query(ctx, `
		SELECT provenance_memory_id, provenance_span, span_text, anchor, normalized_text, event_time
		FROM layer_b_events
		WHERE project = $1 AND provenance_memory_id = ANY($2)
		ORDER BY event_time ASC NULLS LAST, provenance_span ASC`,
		project, memoryIDs,
	)
	if err != nil {
		return nil, fmt.Errorf("ListLayerBEvents query: %w", err)
	}
	defer rows.Close()

	records := make([]layerb.EventRecord, 0)
	for rows.Next() {
		var rec layerb.EventRecord
		if err := rows.Scan(
			&rec.MemoryID, &rec.ProvenanceSpan, &rec.SpanText,
			&rec.Anchor, &rec.NormalizedText, &rec.EventTime,
		); err != nil {
			return nil, fmt.Errorf("ListLayerBEvents scan: %w", err)
		}
		records = append(records, rec)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("ListLayerBEvents rows: %w", err)
	}
	return records, nil
}
