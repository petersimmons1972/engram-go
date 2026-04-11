package claude

// Feature 7: Conflict-Aware Reasoning.
// Provides DiagnoseMemories — a pure function that builds an EvidenceMap from
// a set of recalled memories and their known relationship edges. No LLM required.

import (
	"fmt"
	"strings"

	"github.com/petersimmons1972/engram/internal/types"
)

// ConflictPair identifies two memories connected by a contradicts edge.
type ConflictPair struct {
	MemoryAID string  `json:"memory_a_id"`
	MemoryBID string  `json:"memory_b_id"`
	Strength  float64 `json:"strength"`
}

// EvidenceMap is the structured output of DiagnoseMemories.
type EvidenceMap struct {
	Memories           []*types.Memory `json:"memories"`
	Conflicts          []ConflictPair  `json:"conflicts"`
	InvalidatedSources []string        `json:"invalidated_sources"`
	// Confidence is 1.0 when there are no conflicts among the recalled memories,
	// reduced proportionally to the number of conflicting pairs.
	Confidence float64 `json:"confidence"`
}

// DiagnoseMemories builds an EvidenceMap for a set of recalled memories.
// rels should contain all relationships among (or referencing) the memories —
// only edges whose SourceID and TargetID are both in the memory set AND whose
// RelType is "contradicts" are counted as conflicts.
//
// This is a pure function — no DB calls, no LLM. The caller is responsible for
// fetching the relevant relationships from the backend before calling.
func DiagnoseMemories(memories []*types.Memory, rels []types.Relationship) EvidenceMap {
	// Index memory IDs for O(1) presence checks.
	memSet := make(map[string]bool, len(memories))
	for _, m := range memories {
		memSet[m.ID] = true
	}

	// Find invalidated sources.
	var invalidated []string
	for _, m := range memories {
		if m.ValidTo != nil {
			invalidated = append(invalidated, m.ID)
		}
	}

	// Find contradicts edges within the memory set.
	var conflicts []ConflictPair
	for _, rel := range rels {
		if rel.RelType != types.RelTypeContradicts {
			continue
		}
		if !memSet[rel.SourceID] || !memSet[rel.TargetID] {
			continue
		}
		conflicts = append(conflicts, ConflictPair{
			MemoryAID: rel.SourceID,
			MemoryBID: rel.TargetID,
			Strength:  rel.Strength,
		})
	}

	// Confidence: 1.0 with no conflicts; decreases as conflicting pairs grow.
	// Formula: 1 / (1 + number_of_conflicts) — asymptotes toward 0.
	confidence := 1.0
	if len(conflicts) > 0 {
		confidence = 1.0 / (1.0 + float64(len(conflicts)))
	}

	return EvidenceMap{
		Memories:           memories,
		Conflicts:          conflicts,
		InvalidatedSources: invalidated,
		Confidence:         confidence,
	}
}

// BuildConflictAwarePrompt builds a reasoning prompt that explicitly annotates
// known conflicts so Claude can acknowledge uncertainty rather than silently
// picking one version.
func BuildConflictAwarePrompt(question string, ev EvidenceMap) string {
	var sb strings.Builder

	fmt.Fprintf(&sb, "Question: %s\n\n", question)

	if len(ev.Conflicts) > 0 {
		sb.WriteString("CONFLICT NOTICE: The following memories contain contradicting claims:\n") // #95: no emoji
		for _, c := range ev.Conflicts {
			fmt.Fprintf(&sb, "  CONFLICT: memory %s contradicts memory %s (strength %.2f)\n",
				c.MemoryAID, c.MemoryBID, c.Strength)
		}
		fmt.Fprintf(&sb, "  Overall confidence in this evidence set: %.0f%%\n\n", ev.Confidence*100)
	}

	if len(ev.InvalidatedSources) > 0 {
		sb.WriteString("INVALIDATED SOURCES (may be outdated): ") // #95: no emoji
		sb.WriteString(strings.Join(ev.InvalidatedSources, ", "))
		sb.WriteString("\n\n")
	}

	sb.WriteString("Memories:\n")
	for i, m := range ev.Memories {
		content := m.Content
		if len(content) > maxMemoryContentInReason {
			content = content[:maxMemoryContentInReason]
		}
		fmt.Fprintf(&sb, "[%d] ID: %s\n%s\n\n", i+1, m.ID, content)
	}

	return strings.TrimRight(sb.String(), "\n")
}
