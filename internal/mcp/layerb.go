package mcp

import (
	"context"

	"github.com/petersimmons1972/engram/internal/layerb"
	"github.com/petersimmons1972/engram/internal/types"
)

type layerBStore interface {
	UpsertLayerBAtom(ctx context.Context, atom layerb.Atom) error
	UpsertLayerBEvent(ctx context.Context, event layerb.Event) error
	ListLayerBEvents(ctx context.Context, project string, memoryIDs []string) ([]layerb.EventRecord, error)
}

func buildLayerBSummary(ctx context.Context, backend any, query string, results []types.SearchResult) (*layerb.Summary, error) {
	store, ok := backend.(layerBStore)
	if !ok {
		return nil, nil
	}
	return layerb.BuildSummary(ctx, store, query, results)
}
