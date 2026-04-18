package entity

import "strings"

// Deduplicate matches candidates against existing entities by name/alias (case-insensitive).
// Returns (updated existing entities that matched, genuinely new entities).
func Deduplicate(existing []Entity, candidates []Entity) (merged []Entity, fresh []Entity) {
	index := make(map[string]int, len(existing)*3)
	for i, e := range existing {
		index[strings.ToLower(e.Name)] = i
		for _, a := range e.Aliases {
			index[strings.ToLower(a)] = i
		}
	}

	updated := make([]Entity, len(existing))
	copy(updated, existing)

	for _, c := range candidates {
		key := strings.ToLower(c.Name)
		if idx, ok := index[key]; ok {
			ent := updated[idx]
			if !containsAlias(ent.Aliases, c.Name) && strings.ToLower(ent.Name) != key {
				ent.Aliases = append(ent.Aliases, c.Name)
				updated[idx] = ent
			}
			merged = append(merged, updated[idx])
		} else {
			fresh = append(fresh, c)
			index[key] = -1
		}
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
