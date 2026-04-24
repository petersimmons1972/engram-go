// Package entity provides entity extraction, deduplication, and graph-building for memories.
package entity

import "strings"

// Deduplicate matches candidates against existing entities by name/alias (case-insensitive).
// Returns (updated existing entities that matched, genuinely new entities).
func Deduplicate(existing []Entity, candidates []Entity) (merged []Entity, fresh []Entity) {
	// Build case-insensitive lookup: lower(name/alias) → index in existing
	index := make(map[string]int, len(existing)*3)
	for i, e := range existing {
		index[strings.ToLower(e.Name)] = i
		for _, a := range e.Aliases {
			index[strings.ToLower(a)] = i
		}
	}

	// Deep-copy existing so we can mutate Aliases without aliasing the caller's slices.
	updated := make([]Entity, len(existing))
	copy(updated, existing)
	for i := range updated {
		src := existing[i].Aliases
		if len(src) > 0 {
			dst := make([]string, len(src))
			copy(dst, src)
			updated[i].Aliases = dst
		}
	}

	// Track which existing indices were matched (for dedup of merged return).
	mergedSet := make(map[int]bool)
	// Track which new names were already added to fresh (key → index in fresh).
	freshIndex := make(map[string]int)

	for _, c := range candidates {
		key := strings.ToLower(c.Name)
		if idx, ok := index[key]; ok {
			// Matched an existing entity.
			ent := updated[idx]
			if !containsAlias(ent.Aliases, c.Name) && strings.ToLower(ent.Name) != key {
				ent.Aliases = append(ent.Aliases, c.Name)
				updated[idx] = ent
			}
			mergedSet[idx] = true
		} else if _, seen := freshIndex[key]; seen {
			// Duplicate new candidate already in fresh — skip.
		} else {
			fresh = append(fresh, c)
			freshIndex[key] = len(fresh) - 1
		}
	}

	// Collect merged results once per matched existing entity (no duplicates).
	for idx := range mergedSet {
		merged = append(merged, updated[idx])
	}
	return merged, fresh
}

func containsAlias(aliases []string, name string) bool {
	lower := strings.ToLower(name)
	for _, a := range aliases {
		if strings.ToLower(a) == lower {
			return true
		}
	}
	return false
}
