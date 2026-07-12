package atom

import (
	"strings"
	"time"
)

// SupersessionKey is the dedup key for an atom: (project, subject, predicate).
// Two atoms with the same key represent competing beliefs about the same property.
type SupersessionKey struct {
	Project   string
	Subject   string
	Predicate string
}

// supersessionKey returns the dedup key for a.
func supersessionKey(a *Atom) SupersessionKey {
	return SupersessionKey{
		Project:   a.Project,
		Subject:   strings.ToLower(strings.TrimSpace(a.Subject)),
		Predicate: strings.ToLower(strings.TrimSpace(a.Predicate)),
	}
}

// DeduplicationResult is the output of Deduplicate.
type DeduplicationResult struct {
	// Fresh contains atoms with no existing match — ready to insert.
	Fresh []Atom
	// Superseded contains (old, new) pairs where the old atom should have its
	// valid_to set and the new atom should link supersedes=old.ID.
	// The new atom is already updated in-place (Supersedes field set).
	Superseded []SupersessionPair
}

// SupersessionPair holds a (old, new) atom pair for a supersession update.
type SupersessionPair struct {
	Old Atom
	New Atom
}

// Deduplicate checks incoming candidates against existing active atoms for the
// same project. For each candidate:
//   - If no existing atom has the same (subject, predicate), it is "fresh".
//   - If an existing atom matches on (subject, predicate) AND the value is
//     identical (case-insensitive), the candidate is dropped as a duplicate.
//   - If an existing atom matches on (subject, predicate) but the value
//     differs, a supersession pair is created: the old atom is retired (caller
//     sets valid_to = now) and the new atom links supersedes = old.ID.
//
// now is injected so tests can use a fixed timestamp.
func Deduplicate(existing []Atom, candidates []Atom, now time.Time) DeduplicationResult {
	// Build index: SupersessionKey → existing atom.
	index := make(map[SupersessionKey]Atom, len(existing))
	for _, a := range existing {
		k := supersessionKey(&a)
		index[k] = a
	}

	var result DeduplicationResult
	seen := make(map[SupersessionKey]bool) // dedup within candidates

	for i := range candidates {
		c := candidates[i]
		k := supersessionKey(&c)

		if seen[k] {
			// Duplicate within the candidate batch — skip.
			continue
		}
		seen[k] = true

		existing, exists := index[k]
		if !exists {
			// Genuinely new atom.
			result.Fresh = append(result.Fresh, c)
			continue
		}

		// Same subject+predicate.
		if strings.EqualFold(strings.TrimSpace(existing.Value), strings.TrimSpace(c.Value)) {
			if isDistinctDatedOccurrence(existing, c) {
				result.Fresh = append(result.Fresh, c)
				continue
			}
			// Identical value — reinforce (no action needed; existing stays active).
			continue
		}

		// Different value — supersession.
		// Update the new atom to link back to the old one.
		c.Supersedes = existing.ID

		// Mark the old atom as retired (valid_to = now).
		retiredAt := now
		retired := existing
		retired.ValidTo = &retiredAt

		result.Superseded = append(result.Superseded, SupersessionPair{
			Old: retired,
			New: c,
		})
	}

	return result
}

func isDistinctDatedOccurrence(existing, candidate Atom) bool {
	isEvent := candidate.Type == TypeEvent || candidate.Type == TypeStatusChange
	if !isEvent || existing.ValidFrom == nil || candidate.ValidFrom == nil {
		return false
	}
	return !eventOccurrenceDate(*existing.ValidFrom).Equal(eventOccurrenceDate(*candidate.ValidFrom))
}

func eventOccurrenceDate(value time.Time) time.Time {
	return time.Date(value.Year(), value.Month(), value.Day(), 0, 0, 0, 0, time.UTC)
}
