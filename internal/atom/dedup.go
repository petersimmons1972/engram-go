package atom

import (
	"log/slog"
	"sort"
	"strings"
	"time"
)

const (
	// confidenceSupersessionTolerance permits a candidate to supersede an atom
	// up to 0.2 more confident than itself. Lower-confidence contradictions coexist.
	confidenceSupersessionTolerance = 0.2
	// confidenceComparisonEpsilon keeps values computed at the confidence gate
	// from falling on the wrong side because of floating-point rounding.
	confidenceComparisonEpsilon = 1e-9
)

// SupersessionKey is the dedup key for an atom: (project, type, subject, predicate).
// Two atoms with the same key represent competing beliefs about the same property.
type SupersessionKey struct {
	Project   string
	Type      string
	Subject   string
	Predicate string
}

type batchDeduplicationKey struct {
	SupersessionKey
	Value     string
	EventDate time.Time
}

// supersessionKey returns the dedup key for a.
func supersessionKey(a *Atom) SupersessionKey {
	return SupersessionKey{
		Project:   a.Project,
		Type:      a.Type,
		Subject:   strings.ToLower(strings.TrimSpace(a.Subject)),
		Predicate: strings.ToLower(strings.TrimSpace(a.Predicate)),
	}
}

func candidateBatchKey(a *Atom) batchDeduplicationKey {
	key := batchDeduplicationKey{
		SupersessionKey: supersessionKey(a),
		Value:           strings.ToLower(strings.TrimSpace(a.Value)),
	}
	if a.Type == TypeEvent && a.ValidFrom != nil {
		key.EventDate = eventOccurrenceDate(*a.ValidFrom)
	}
	return key
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
//   - If no existing atom has the same (type, subject, predicate), it is "fresh".
//   - If an existing atom matches on (type, subject, predicate) AND the value is
//     identical (case-insensitive), the candidate is dropped as a duplicate.
//   - If an existing non-event atom matches on (type, subject, predicate) but the value
//     differs, a supersession pair is created: the old atom is retired (caller
//     sets valid_to to the candidate assertion time) and the new atom links
//     supersedes = old.ID. Events never supersede.
//
// now is the assertion-time fallback for callers that supply legacy atoms
// without ObservedAt; normal ingestion always supplies ObservedAt.
func Deduplicate(existing []Atom, candidates []Atom, now time.Time) DeduplicationResult {
	// Build index: SupersessionKey → existing atom.
	index := make(map[SupersessionKey]Atom, len(existing))
	for _, a := range existing {
		k := supersessionKey(&a)
		current, exists := index[k]
		if !exists || atomIsLater(a, current, now) {
			index[k] = a
		}
	}

	result := DeduplicationResult{
		Fresh:      []Atom{},
		Superseded: []SupersessionPair{},
	}
	seen := make(map[batchDeduplicationKey]bool) // dedup exact candidates within the batch
	unique := make([]Atom, 0, len(candidates))

	for i := range candidates {
		c := candidates[i]
		batchKey := candidateBatchKey(&c)

		if seen[batchKey] {
			// Duplicate within the candidate batch — skip.
			slog.Debug(
				"atom deduplication: dropped duplicate candidate within batch",
				"key", supersessionKey(&c),
				"value", c.Value,
			)
			continue
		}
		seen[batchKey] = true
		unique = append(unique, c)
	}

	// Stable sorting gives status changes assertion-time order while preserving
	// extractor order for ties. Other atom types retain their original order.
	sort.SliceStable(unique, func(i, j int) bool {
		left := unique[i]
		right := unique[j]
		leftStatus := left.Type == TypeStatusChange
		rightStatus := right.Type == TypeStatusChange
		if leftStatus != rightStatus {
			return !leftStatus
		}
		if !leftStatus {
			return false
		}
		leftKey := supersessionKey(&left)
		rightKey := supersessionKey(&right)
		if leftKey != rightKey {
			return supersessionKeyLess(leftKey, rightKey)
		}
		return assertionTime(left, now).Before(assertionTime(right, now))
	})

	for i := range unique {
		c := unique[i]
		k := supersessionKey(&c)

		current, exists := index[k]
		if !exists {
			result.Fresh = append(result.Fresh, c)
			if c.Type != TypeEvent {
				index[k] = c
			}
			continue
		}
		if strings.EqualFold(strings.TrimSpace(current.Value), strings.TrimSpace(c.Value)) {
			if isDistinctDatedOccurrence(current, c) {
				result.Fresh = append(result.Fresh, c)
				continue
			}
			// Identical value — reinforce (no action needed; existing stays active).
			continue
		}

		if c.Type == TypeEvent {
			result.Fresh = append(result.Fresh, c)
			continue
		}
		if c.Confidence+confidenceSupersessionTolerance+confidenceComparisonEpsilon < current.Confidence {
			slog.Info(
				"atom supersession skipped: candidate below confidence gate",
				"existing_id", current.ID,
				"existing_value", current.Value,
				"candidate_id", c.ID,
				"candidate_value", c.Value,
			)
			result.Fresh = append(result.Fresh, c)
			continue
		}
		candidateAssertion := assertionTime(c, now)
		if candidateAssertion.Before(validityStart(current, now)) {
			slog.Info(
				"atom supersession skipped: candidate predates active atom",
				"existing_id", current.ID,
				"candidate_id", c.ID,
			)
			result.Fresh = append(result.Fresh, c)
			continue
		}

		c.Supersedes = current.ID

		retiredAt := candidateAssertion
		retired := current
		retired.ValidTo = &retiredAt

		result.Superseded = append(result.Superseded, SupersessionPair{
			Old: retired,
			New: c,
		})
		index[k] = c
	}

	return result
}

func validityStart(a Atom, fallback time.Time) time.Time {
	if a.ValidFrom != nil {
		return *a.ValidFrom
	}
	return assertionTime(a, fallback)
}

func assertionTime(a Atom, fallback time.Time) time.Time {
	if a.ObservedAt != nil {
		return *a.ObservedAt
	}
	return fallback
}

func atomIsLater(candidate, current Atom, fallback time.Time) bool {
	candidateAssertion := assertionTime(candidate, fallback)
	currentAssertion := assertionTime(current, fallback)
	if !candidateAssertion.Equal(currentAssertion) {
		return candidateAssertion.After(currentAssertion)
	}
	return candidate.CreatedAt.After(current.CreatedAt)
}

func supersessionKeyLess(left, right SupersessionKey) bool {
	if left.Project != right.Project {
		return left.Project < right.Project
	}
	if left.Type != right.Type {
		return left.Type < right.Type
	}
	if left.Subject != right.Subject {
		return left.Subject < right.Subject
	}
	return left.Predicate < right.Predicate
}

func isDistinctDatedOccurrence(existing, candidate Atom) bool {
	if candidate.Type != TypeEvent || existing.ValidFrom == nil || candidate.ValidFrom == nil {
		return false
	}
	return !eventOccurrenceDate(*existing.ValidFrom).Equal(eventOccurrenceDate(*candidate.ValidFrom))
}

func eventOccurrenceDate(value time.Time) time.Time {
	return time.Date(value.Year(), value.Month(), value.Day(), 0, 0, 0, 0, time.UTC)
}
