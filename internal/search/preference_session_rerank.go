package search

import (
	"sort"

	"github.com/petersimmons1972/engram/internal/types"
)

const (
	preferenceSessionAnchorBonus = 3.0
	preferenceSessionTypeBonus   = 1.0
)

type preferenceSessionGroup struct {
	key         string
	members     []types.SearchResult
	score       float64
	evidence    float64
	bestScore   float64
	insertOrder int
}

func preferenceSessionRerank(results []types.SearchResult, topicAnchorTokens []string, enabled bool) []types.SearchResult {
	if !enabled || len(results) == 0 || len(topicAnchorTokens) == 0 {
		return results
	}

	groupIndex := make(map[string]int)
	groups := make([]preferenceSessionGroup, 0, len(results))
	for _, r := range results {
		key := preferenceSessionKey(r)
		idx, ok := groupIndex[key]
		if !ok {
			idx = len(groups)
			groupIndex[key] = idx
			groups = append(groups, preferenceSessionGroup{key: key, insertOrder: idx})
		}
		groups[idx].members = append(groups[idx].members, r)
	}

	for i := range groups {
		groups[i].bestScore = bestScore(groups[i].members)
		groups[i].evidence = preferenceSessionEvidenceScore(groups[i].members, topicAnchorTokens)
		groups[i].score = groups[i].bestScore + groups[i].evidence
	}

	var evidenceGroups []preferenceSessionGroup
	evidenceKeys := make(map[string]struct{})
	for _, g := range groups {
		if g.evidence > 0 {
			evidenceGroups = append(evidenceGroups, g)
			evidenceKeys[g.key] = struct{}{}
		}
	}
	if len(evidenceGroups) == 0 {
		return results
	}

	sort.SliceStable(evidenceGroups, func(i, j int) bool {
		if evidenceGroups[i].score != evidenceGroups[j].score {
			return evidenceGroups[i].score > evidenceGroups[j].score
		}
		if evidenceGroups[i].bestScore != evidenceGroups[j].bestScore {
			return evidenceGroups[i].bestScore > evidenceGroups[j].bestScore
		}
		return evidenceGroups[i].insertOrder < evidenceGroups[j].insertOrder
	})

	out := make([]types.SearchResult, 0, len(results))
	for _, g := range evidenceGroups {
		sortPreferenceSessionMembers(g.members, topicAnchorTokens)
		out = append(out, g.members...)
	}
	for _, r := range results {
		if _, promoted := evidenceKeys[preferenceSessionKey(r)]; !promoted {
			out = append(out, r)
		}
	}
	return out
}

func preferenceSessionKey(r types.SearchResult) string {
	if r.Memory == nil {
		return "\x00no-sid:nil"
	}
	if sid := extractSessionID(r.Memory.Tags); sid != "" {
		return sid
	}
	return "\x00no-sid:" + r.Memory.ID
}

func preferenceSessionEvidenceScore(members []types.SearchResult, topicAnchorTokens []string) float64 {
	best := 0.0
	for _, r := range members {
		if r.Memory == nil {
			continue
		}
		score := 0.0
		anchorHit := contentContainsTopicAnchor(r.Memory.Content, topicAnchorTokens)
		if anchorHit {
			score += preferenceSessionAnchorBonus
			if r.Memory.MemoryType == "preference" {
				score += preferenceSessionTypeBonus
			}
		}
		if score > best {
			best = score
		}
	}
	return best
}

func sortPreferenceSessionMembers(members []types.SearchResult, topicAnchorTokens []string) {
	sort.SliceStable(members, func(i, j int) bool {
		iMem := members[i].Memory
		jMem := members[j].Memory
		iAnchor := iMem != nil && contentContainsTopicAnchor(iMem.Content, topicAnchorTokens)
		jAnchor := jMem != nil && contentContainsTopicAnchor(jMem.Content, topicAnchorTokens)
		if iAnchor != jAnchor {
			return iAnchor
		}
		iPref := iMem != nil && iMem.MemoryType == "preference"
		jPref := jMem != nil && jMem.MemoryType == "preference"
		if iPref != jPref {
			return iPref
		}
		return members[i].Score > members[j].Score
	})
}
