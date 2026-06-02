package search

import (
	"context"
	"sort"

	"github.com/petersimmons1972/engram/internal/atom"
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
