package db_test

import (
	"context"
	"testing"
	"time"

	"github.com/petersimmons1972/engram/internal/db"
	"github.com/petersimmons1972/engram/internal/layerb"
	"github.com/petersimmons1972/engram/internal/types"
	"github.com/stretchr/testify/require"
)

func TestLayerBEvents_RoundTripIdempotentAndCascade(t *testing.T) {
	proj := uniqueProject("layerb-roundtrip")
	backend := newTestBackend(t, proj)
	pg, ok := backend.(*db.PostgresBackend)
	require.True(t, ok, "newTestBackend must return *db.PostgresBackend")

	ctx := context.Background()
	validFrom := time.Date(2024, 2, 2, 0, 0, 0, 0, time.UTC)
	mem := &types.Memory{
		ID:         types.NewMemoryID(),
		Content:    "I visited the doctor on Monday.",
		Project:    proj,
		MemoryType: types.MemoryTypeContext,
		Importance: 1,
		ValidFrom:  &validFrom,
	}
	require.NoError(t, backend.StoreMemory(ctx, mem))

	atom := layerb.Atom{
		Project:        proj,
		MemoryID:       mem.ID,
		ProvenanceSpan: "chars:0-31",
		SpanText:       "I visited the doctor on Monday.",
		Statement:      "I visited the doctor on Monday.",
		NormalizedText: "visit doctor monday",
		EventTime:      &validFrom,
	}
	event := layerb.Event{
		Project:        proj,
		MemoryID:       mem.ID,
		ProvenanceSpan: atom.ProvenanceSpan,
		SpanText:       atom.SpanText,
		Anchor:         "visit doctor",
		NormalizedText: atom.NormalizedText,
		EventTime:      atom.EventTime,
	}

	require.NoError(t, pg.UpsertLayerBAtom(ctx, atom))
	require.NoError(t, pg.UpsertLayerBEvent(ctx, event))
	require.NoError(t, pg.UpsertLayerBAtom(ctx, atom))
	require.NoError(t, pg.UpsertLayerBEvent(ctx, event))

	records, err := pg.ListLayerBEvents(ctx, proj, []string{mem.ID})
	require.NoError(t, err)
	require.Len(t, records, 1, "idempotent re-ingest must not duplicate stored events")
	require.Equal(t, atom.ProvenanceSpan, records[0].ProvenanceSpan)
	require.Equal(t, atom.SpanText, records[0].SpanText, "provenance span text must round-trip exactly")
	require.Equal(t, event.Anchor, records[0].Anchor)
	require.NotNil(t, records[0].EventTime)
	require.True(t, records[0].EventTime.Equal(validFrom), "stored event_time must round-trip exactly")

	deleted, err := backend.DeleteMemory(ctx, mem.ID)
	require.NoError(t, err)
	require.True(t, deleted)

	records, err = pg.ListLayerBEvents(ctx, proj, []string{mem.ID})
	require.NoError(t, err)
	require.Empty(t, records, "deleted memories must not leave orphaned Layer B events")
}
