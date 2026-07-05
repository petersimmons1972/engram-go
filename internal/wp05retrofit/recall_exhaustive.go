package wp05retrofit

import (
	"context"
	"fmt"
	"strings"

	"github.com/petersimmons1972/engram/internal/layerb"
	"github.com/petersimmons1972/engram/internal/longmemeval"
	"github.com/petersimmons1972/engram/internal/types"
)

const projectSweepLimit = 500

type recallCall struct {
	query string
	topK  int
}

// layerbStubStore is an in-memory Layer B store for client-side BuildSummary.
type layerbStubStore struct {
	atoms  map[string]layerb.Atom
	events map[string]layerb.Event
}

func (s *layerbStubStore) UpsertLayerBAtom(_ context.Context, atom layerb.Atom) error {
	if s.atoms == nil {
		s.atoms = map[string]layerb.Atom{}
	}
	s.atoms[atom.MemoryID+"|"+atom.ProvenanceSpan+"|"+atom.NormalizedText] = atom
	return nil
}

func (s *layerbStubStore) UpsertLayerBEvent(_ context.Context, event layerb.Event) error {
	if s.events == nil {
		s.events = map[string]layerb.Event{}
	}
	s.events[event.MemoryID+"|"+event.ProvenanceSpan+"|"+event.Anchor] = event
	return nil
}

func (s *layerbStubStore) ListLayerBEvents(_ context.Context, _ string, memoryIDs []string) ([]layerb.EventRecord, error) {
	allow := make(map[string]bool, len(memoryIDs))
	for _, id := range memoryIDs {
		allow[id] = true
	}
	out := make([]layerb.EventRecord, 0, len(s.events))
	for _, event := range s.events {
		if !allow[event.MemoryID] {
			continue
		}
		out = append(out, layerb.EventRecord{
			MemoryID:       event.MemoryID,
			ProvenanceSpan: event.ProvenanceSpan,
			SpanText:       event.SpanText,
			Anchor:         event.Anchor,
			NormalizedText: event.NormalizedText,
			EventTime:      event.EventTime,
		})
	}
	return out, nil
}

// mergeSearchResults unions recall hits by memory ID, keeps the maximum score per
// ID, and preserves first-seen order across the input batches.
func mergeSearchResults(batches ...[]types.SearchResult) []types.SearchResult {
	type mergedHit struct {
		result types.SearchResult
		order  int
	}
	merged := make(map[string]mergedHit)
	order := 0
	for _, batch := range batches {
		for _, hit := range batch {
			if hit.Memory == nil || strings.TrimSpace(hit.Memory.ID) == "" {
				continue
			}
			id := hit.Memory.ID
			current, ok := merged[id]
			if !ok {
				merged[id] = mergedHit{result: hit, order: order}
				order++
				continue
			}
			if hit.Score > current.result.Score {
				current.result.Score = hit.Score
			}
			// Recall uses detail=summary (500-char cap); memory_list carries full
			// session text. Keep the longer content so client-side Layer B sees
			// the same evidence the server uses when building layer_b in full mode.
			preferLongerMemoryContent(&current.result, hit)
			merged[id] = current
		}
	}
	out := make([]mergedHit, 0, len(merged))
	for _, hit := range merged {
		out = append(out, hit)
	}
	// Stable sort by original discovery order; recall-ranked items stay ahead of
	// zero-score project-list entries when scores tie.
	for i := 0; i < len(out); i++ {
		for j := i + 1; j < len(out); j++ {
			if out[j].order < out[i].order {
				out[i], out[j] = out[j], out[i]
			}
		}
	}
	results := make([]types.SearchResult, 0, len(out))
	for _, hit := range out {
		results = append(results, hit.result)
	}
	return results
}

func preferLongerMemoryContent(dst *types.SearchResult, src types.SearchResult) {
	if dst.Memory == nil {
		if src.Memory != nil {
			dst.Memory = src.Memory
		}
		return
	}
	if src.Memory == nil {
		return
	}
	if len(src.Memory.Content) > len(dst.Memory.Content) {
		dst.Memory.Content = src.Memory.Content
	}
}

func idsFromSearchResults(results []types.SearchResult) []string {
	ids := make([]string, 0, len(results))
	for _, r := range results {
		if r.Memory != nil && r.Memory.ID != "" {
			ids = append(ids, r.Memory.ID)
		}
	}
	return ids
}

func recallItemExhaustive(ctx context.Context, client Client, project string, item Item, limit int) (longmemeval.RecallResult, error) {
	runOpts := longmemeval.RunOpts{ExhaustiveAggregation: true}
	effectiveTopK := runOpts.EffectiveRecallTopK(item.Question, limit)

	primary, err := client.RecallFullResult(ctx, project, item.Question, effectiveTopK)
	if err != nil {
		return longmemeval.RecallResult{}, err
	}

	batches := [][]types.SearchResult{primary.Results}
	anchor := strings.TrimSpace(longmemeval.ExtractAggregationAnchor(item.Question))
	if anchor != "" && !strings.EqualFold(anchor, strings.TrimSpace(item.Question)) {
		anchorResult, anchorErr := client.RecallFullResult(ctx, project, anchor, effectiveTopK)
		if anchorErr != nil {
			return longmemeval.RecallResult{}, fmt.Errorf("anchor recall: %w", anchorErr)
		}
		batches = append(batches, anchorResult.Results)
	}

	listed, listErr := client.ListProjectMemories(ctx, project, projectSweepLimit)
	if listErr != nil {
		return longmemeval.RecallResult{}, fmt.Errorf("project sweep: %w", listErr)
	}
	batches = append(batches, listed)

	merged := mergeSearchResults(batches...)
	store := &layerbStubStore{}
	layerBSummary, err := layerb.BuildSummary(ctx, store, item.Question, merged)
	if err != nil {
		return longmemeval.RecallResult{}, fmt.Errorf("client layer_b build: %w", err)
	}

	return longmemeval.RecallResult{
		IDs:     idsFromSearchResults(merged),
		Results: merged,
		LayerB:  layerBSummary,
	}, nil
}