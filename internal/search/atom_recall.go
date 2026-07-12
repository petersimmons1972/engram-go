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
	eventWindowPadDays = 7
	eventWindowAtomCap = 30
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
	backend, ok := e.backend.(AtomBackend)
	if !ok {
		return "", fmt.Errorf("backend does not support atom queries")
	}
	return RecallEventWindowAtoms(ctx, backend, project, since, before)
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
	if len(atoms) == 0 {
		return ""
	}
	var b []byte
	b = append(b, "=== Extracted Preference Atoms ===\n"...)
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
	if since == nil || before == nil {
		return "", nil
	}

	paddedSince := dateOnlyUTC(*since).AddDate(0, 0, -eventWindowPadDays)
	paddedBefore := dateOnlyUTC(*before).AddDate(0, 0, eventWindowPadDays)
	atoms, err := backend.GetActiveAtomsFiltered(ctx, project, db.AtomQueryOpts{
		AtomType:        atom.TypeEvent,
		ValidFromSince:  &paddedSince,
		ValidFromBefore: &paddedBefore,
		OrderValidFrom:  true,
	})
	if err != nil {
		return "", fmt.Errorf("recalling event-window atoms: %w", err)
	}
	if len(atoms) == 0 {
		return "", nil
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

	omitted := len(atoms) - eventWindowAtomCap
	if omitted > 0 {
		atoms = atoms[:eventWindowAtomCap]
	}
	block := FormatAtomsAsContext(atoms)
	if omitted > 0 {
		block += fmt.Sprintf("(+%d more)\n", omitted)
	}
	return block, nil
}

func dateOnlyUTC(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
}
