package claude

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/petersimmons1972/engram/internal/types"
)

const maxMergeBatch = 10

// MergeCandidate is a pair of memories that may be near-duplicates.
type MergeCandidate struct {
	MemoryA    *types.Memory
	MemoryB    *types.Memory
	Similarity float64
}

// MergeDecision is Claude's verdict on whether to merge a candidate pair.
type MergeDecision struct {
	MemoryAID     string `json:"memory_a_id"`
	MemoryBID     string `json:"memory_b_id"`
	ShouldMerge   bool   `json:"should_merge"`
	Reason        string `json:"reason"`
	MergedContent string `json:"merged_content,omitempty"`
}

const mergeCandidateSystem = "You are a memory consolidation engine. Given pairs of memories, decide whether they should be merged into one. Be conservative: only merge when both memories express the same core fact and merging produces a strictly better result. Respond with a JSON array of merge decisions, one per candidate pair."

// ReviewMergeCandidates asks Claude to decide which candidate pairs should be
// merged. Returns nil, nil when candidates is empty (no HTTP call is made).
// At most maxMergeBatch candidates are evaluated per call; any extras are
// silently dropped.
func (c *Client) ReviewMergeCandidates(ctx context.Context, candidates []MergeCandidate) ([]MergeDecision, error) {
	if len(candidates) == 0 {
		return nil, nil
	}

	if len(candidates) > maxMergeBatch {
		candidates = candidates[:maxMergeBatch]
	}

	prompt := buildMergePrompt(candidates)

	raw, err := c.Complete(ctx, mergeCandidateSystem, prompt, "claude-sonnet-4-6", "claude-opus-4-6", 2, 2048)
	if err != nil {
		return nil, err
	}

	cleaned := extractJSON(raw)

	var decisions []MergeDecision
	if err := json.Unmarshal([]byte(cleaned), &decisions); err != nil {
		return nil, fmt.Errorf("parse merge decisions: %w", err)
	}

	return decisions, nil
}

// buildMergePrompt constructs the user-facing prompt describing each candidate.
func buildMergePrompt(candidates []MergeCandidate) string {
	prompt := "Review these memory pairs and decide whether each should be merged:\n\n"
	for i, c := range candidates {
		contentA := c.MemoryA.Content
		if len(contentA) > 500 {
			contentA = contentA[:500]
		}
		contentB := c.MemoryB.Content
		if len(contentB) > 500 {
			contentB = contentB[:500]
		}
		prompt += fmt.Sprintf(
			"Pair %d:\n  memory_a_id: %s\n  memory_b_id: %s\n  similarity: %.4f\n  content_a: %s\n  content_b: %s\n\n",
			i+1, c.MemoryA.ID, c.MemoryB.ID, c.Similarity, contentA, contentB,
		)
	}
	return prompt
}
