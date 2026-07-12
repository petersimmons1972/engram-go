package search

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/petersimmons1972/engram/internal/atom"
	"github.com/petersimmons1972/engram/internal/db"
)

const (
	// eventWindowPadDays follows duramind results/layerc-2026-07/MISS-TAXONOMY-133.md.
	// Keep this package-level so a future option can tune it without changing retrieval semantics.
	eventWindowPadDays  = 7
	eventWindowAtomCap  = 30
	eventWindowFetchCap = 40
)

// AtomRecallOpts controls atom-level retrieval.
type AtomRecallOpts struct {
	// AtomType filters results to this type (e.g. "preference"). Empty = all types.
	AtomType string
	// TopK limits the number of atoms returned. 0 = no limit.
	TopK int
}

// AtomBackend is the narrow interface the atom recall path needs from the
// database layer. Satisfied by *db.PostgresBackend; declared here to avoid
// importing the db package from within search (which would create an import
// cycle in tests).
type AtomBackend interface {
	GetActiveAtoms(ctx context.Context, project string, atomType string) ([]atom.Atom, error)
	GetActiveAtomsFiltered(ctx context.Context, project string, opts db.AtomQueryOpts) ([]atom.Atom, error)
}

// RecallEventWindowContext resolves event atoms through the engine's backend
// without changing any memory recall pass or writing atom state.
func (e *SearchEngine) RecallEventWindowContext(
	ctx context.Context,
	project string,
	since, before *time.Time,
) (string, error) {
	result, err := e.RecallEventWindow(ctx, project, since, before, false)
	return result.Context, err
}

// RecallEventWindow returns the prompt-ready B3 block and the exact fetched
// atoms so generation-time consumers can derive other views without a second
// database query.
func (e *SearchEngine) RecallEventWindow(
	ctx context.Context,
	project string,
	since, before *time.Time,
	includeSuperseded bool,
) (EventWindowResult, error) {
	backend, ok := e.backend.(AtomBackend)
	if !ok {
		return EventWindowResult{}, fmt.Errorf("backend does not support atom queries")
	}
	return recallEventWindow(ctx, backend, project, since, before, includeSuperseded)
}

// RecallAtoms returns active atoms for the project, filtered by opts.AtomType
// and ranked by confidence (descending). When an embedder is available in the
// future, statement-level cosine similarity can augment ranking (TODO tier-2).
//
// This is the Milestone 1 recall path: structured pre-filter (atom_type) →
// confidence-ranked statement list. The atom-mode context assembler in
// cmd/longmemeval/run.go uses this function when --atom-mode is set.
func RecallAtoms(ctx context.Context, backend AtomBackend, project string, opts AtomRecallOpts) ([]atom.Atom, error) {
	atoms, err := backend.GetActiveAtoms(ctx, project, opts.AtomType)
	if err != nil {
		return nil, err
	}

	// Rank by confidence descending so the most reliable atoms surface first.
	sort.Slice(atoms, func(i, j int) bool {
		return atoms[i].Confidence > atoms[j].Confidence
	})

	if opts.TopK > 0 && len(atoms) > opts.TopK {
		atoms = atoms[:opts.TopK]
	}
	return atoms, nil
}

// FormatAtomsAsContext formats a slice of atoms into a labeled string suitable
// for injection into a generation prompt (--atom-mode). Each atom is rendered
// as a single line: "[TYPE] Statement (confidence: N)".
func FormatAtomsAsContext(atoms []atom.Atom) string {
	return formatAtomsAsContext(atoms, "=== Extracted Preference Atoms ===")
}

func formatAtomsAsContext(atoms []atom.Atom, header string) string {
	if len(atoms) == 0 {
		return ""
	}
	var b []byte
	b = append(b, header...)
	b = append(b, '\n')
	for _, a := range atoms {
		b = append(b, '[')
		b = append(b, a.Type...)
		b = append(b, "] "...)
		b = append(b, a.Statement...)
		b = append(b, '\n')
	}
	return string(b)
}

// RecallPreferenceAtoms returns the latest active preference atoms as a prompt-ready preamble.
// Cold-start safe: when no atoms exist it returns "", nil.
func RecallPreferenceAtoms(ctx context.Context, backend AtomBackend, project, query string, asOf *time.Time) (string, error) {
	_ = query
	atoms, err := backend.GetActiveAtomsFiltered(ctx, project, db.AtomQueryOpts{
		AtomType:   atom.TypePreference,
		AsOf:       asOf,
		LatestOnly: true,
	})
	if err != nil {
		return "", err
	}
	if len(atoms) == 0 {
		return "", nil
	}
	sort.SliceStable(atoms, func(i, j int) bool {
		switch {
		case atoms[i].ObservedAt == nil && atoms[j].ObservedAt == nil:
			return atoms[i].Confidence > atoms[j].Confidence
		case atoms[i].ObservedAt == nil:
			return false
		case atoms[j].ObservedAt == nil:
			return true
		case atoms[i].ObservedAt.Equal(*atoms[j].ObservedAt):
			return atoms[i].Confidence > atoms[j].Confidence
		default:
			return atoms[i].ObservedAt.After(*atoms[j].ObservedAt)
		}
	})
	return FormatAtomsAsContext(atoms), nil
}

// RecallEventWindowAtoms returns an additive context block containing event atoms
// whose date-only valid_from falls within the parsed temporal window plus padding.
// Missing bounds and empty corpora are quiet no-ops.
func RecallEventWindowAtoms(
	ctx context.Context,
	backend AtomBackend,
	project string,
	since, before *time.Time,
) (string, error) {
	result, err := RecallEventWindow(ctx, backend, project, since, before)
	return result.Context, err
}

// EventWindowResult contains both representations derived from one event atom
// query. Atoms preserve their structured temporal and supersession fields.
type EventWindowResult struct {
	Context string
	Atoms   []atom.Atom
}

// RecallEventWindow performs the B3 event-window query once and returns both
// the existing context block and the fetched atoms used to build it.
func RecallEventWindow(
	ctx context.Context,
	backend AtomBackend,
	project string,
	since, before *time.Time,
) (EventWindowResult, error) {
	return recallEventWindow(ctx, backend, project, since, before, false)
}

// RecallEventWindowIncludingSuperseded is the B4 history view. It changes only
// the active-row predicate while sharing the B3 query, bounds, ordering, and
// context formatting.
func RecallEventWindowIncludingSuperseded(
	ctx context.Context,
	backend AtomBackend,
	project string,
	since, before *time.Time,
) (EventWindowResult, error) {
	return recallEventWindow(ctx, backend, project, since, before, true)
}

func recallEventWindow(
	ctx context.Context,
	backend AtomBackend,
	project string,
	since, before *time.Time,
	includeSuperseded bool,
) (EventWindowResult, error) {
	if since == nil || before == nil {
		return EventWindowResult{}, nil
	}

	paddedSince := dateOnlyUTC(*since).AddDate(0, 0, -eventWindowPadDays)
	paddedBefore := dateOnlyUTC(*before).AddDate(0, 0, eventWindowPadDays)
	fetchCap := eventWindowAtomCap
	if includeSuperseded {
		fetchCap = eventWindowFetchCap
	}
	atoms, err := backend.GetActiveAtomsFiltered(ctx, project, db.AtomQueryOpts{
		AtomType:          atom.TypeEvent,
		ValidFromSince:    &paddedSince,
		ValidFromBefore:   &paddedBefore,
		IncludeSuperseded: includeSuperseded,
		OrderValidFrom:    true,
		Limit:             fetchCap + 1,
	})
	if err != nil {
		return EventWindowResult{}, fmt.Errorf("recalling event-window atoms: %w", err)
	}
	if len(atoms) == 0 {
		return EventWindowResult{}, nil
	}

	sort.SliceStable(atoms, func(i, j int) bool {
		if atoms[i].ValidFrom == nil {
			return false
		}
		if atoms[j].ValidFrom == nil {
			return true
		}
		return atoms[i].ValidFrom.Before(*atoms[j].ValidFrom)
	})

	ledgerAtoms := atoms
	contextAtoms := atoms
	omitted := len(contextAtoms) - eventWindowAtomCap
	if omitted > 0 {
		contextAtoms = contextAtoms[:eventWindowAtomCap]
	}
	block := formatAtomsAsContext(contextAtoms, "=== Dated Events (window) ===")
	if omitted > 0 {
		block += "(+more events truncated)\n"
	}
	return EventWindowResult{Context: block, Atoms: ledgerAtoms}, nil
}

func dateOnlyUTC(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
}
