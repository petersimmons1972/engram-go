package layerb

import (
	"strings"

	"github.com/petersimmons1972/engram/internal/aggq"
	"github.com/petersimmons1972/engram/internal/types"
)

// Diagnosis records which BuildSummary gate failed for a candidate recall set.
type Diagnosis struct {
	Anchor                string   `json:"anchor"`
	AnchorTerms           []string `json:"anchor_terms"`
	AnchorTermCount       int      `json:"anchor_term_count"`
	UnionRequired         int      `json:"union_required"`
	RecallCount           int      `json:"recall_count"`
	ContributingMemories  int      `json:"contributing_memories"`
	UnionMatchedTerms     []string `json:"union_matched_terms"`
	UnionMatchedCount     int      `json:"union_matched_count"`
	Connected             bool     `json:"connected"`
	ConnectedComponents   int      `json:"connected_components"`
	FailedGate            string   `json:"failed_gate"`
	WouldFire             bool     `json:"would_fire"`
}

// DiagnoseBuildSummary reports which v4 gate would block BuildSummary for the
// given recall results without persisting atoms or events.
func DiagnoseBuildSummary(q string, results []types.SearchResult) Diagnosis {
	d := Diagnosis{
		RecallCount: len(results),
		FailedGate:  "none",
		WouldFire:   false,
		Connected:   true,
	}
	if !aggq.IsAggregationQuestion(q) {
		d.FailedGate = "not_aggregation"
		return d
	}
	anchor := strings.TrimSpace(aggq.ExtractAggregationAnchor(q))
	d.Anchor = anchor
	anchorTerms := normalizeTerms(anchor)
	d.AnchorTerms = anchorTerms
	d.AnchorTermCount = len(anchorTerms)
	d.UnionRequired = minimumAnchorTermMatches(len(anchorTerms))
	if len(anchorTerms) == 0 {
		d.FailedGate = "empty_anchor"
		return d
	}

	var contributions []memberContribution
	for _, result := range results {
		if result.Memory == nil || strings.TrimSpace(result.Memory.Content) == "" {
			continue
		}
		matches, memMatchedTerms := extractMatchingAtoms(result.Memory, anchorTerms)
		if len(matches) == 0 {
			continue
		}
		contributions = append(contributions, memberContribution{atoms: matches, terms: dedupe(memMatchedTerms)})
	}
	d.ContributingMemories = len(contributions)
	if d.ContributingMemories == 0 {
		d.ConnectedComponents = 0
		d.FailedGate = "no_contributions"
		return d
	}

	var unionTerms []string
	for _, c := range contributions {
		unionTerms = append(unionTerms, c.terms...)
	}
	unionMatched := dedupe(unionTerms)
	d.UnionMatchedTerms = unionMatched
	d.UnionMatchedCount = len(unionMatched)
	if d.UnionMatchedCount < d.UnionRequired {
		d.ConnectedComponents = connectedComponents(contributions)
		d.FailedGate = "collective_coverage"
		return d
	}

	d.ConnectedComponents = connectedComponents(contributions)
	d.Connected = d.ConnectedComponents <= 1
	if !d.Connected {
		d.FailedGate = "connectivity"
		return d
	}

	d.FailedGate = "none"
	d.WouldFire = true
	return d
}

func connectedComponents(contributions []memberContribution) int {
	if len(contributions) <= 1 {
		return len(contributions)
	}
	parent := make([]int, len(contributions))
	for i := range parent {
		parent[i] = i
	}
	find := func(i int) int {
		for parent[i] != i {
			parent[i] = parent[parent[i]]
			i = parent[i]
		}
		return i
	}
	union := func(a, b int) {
		ra, rb := find(a), find(b)
		if ra != rb {
			parent[ra] = rb
		}
	}
	for i := 0; i < len(contributions); i++ {
		for j := i + 1; j < len(contributions); j++ {
			if sharesTerm(contributions[i].terms, contributions[j].terms) {
				union(i, j)
			}
		}
	}
	roots := make(map[int]bool)
	for i := range contributions {
		roots[find(i)] = true
	}
	return len(roots)
}