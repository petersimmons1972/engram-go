// Package atom provides atom extraction, deduplication, and retrieval for the
// extraction-first memory layer (Milestone 1: preference atoms, issue #938).
//
// An "atom" is a minimal, typed belief extracted from a memory session. Each
// atom carries a subject, predicate, value, and a canonical NL statement that
// is embedded for vector recall. The bi-temporal contract (valid_from/valid_to)
// mirrors the memories table; supersession links chain atoms across updates.
package atom

import "time"

// Type constants for the atom_type column.
const (
	TypePreference   = "preference"
	TypeFact         = "fact"
	TypeEvent        = "event"
	TypeAttribute    = "attribute"
	TypeRelationship = "relationship"
)

// Scope prefixes for the scope column.
const (
	ScopeGlobal = "global"
	// Session-scoped: "session:<id>"
	// Entity-scoped:  "entity:<id>"
)

// Atom is a single extracted, typed belief or preference.
type Atom struct {
	ID     string `json:"id"`
	Project string `json:"project"`

	// Typed triple.
	Type      string `json:"atom_type"`  // preference|fact|event|attribute|relationship
	Subject   string `json:"subject"`
	Predicate string `json:"predicate"`
	Value     string `json:"value"`

	// Statement is the canonical NL sentence derived from the triple.
	// This is the text that gets embedded for vector recall.
	Statement string `json:"statement"`

	// Scope controls visibility: "global", "session:<id>", or "entity:<id>".
	Scope string `json:"scope"`

	// Bi-temporal columns (nil = unbounded).
	ValidFrom *time.Time `json:"valid_from,omitempty"`
	ValidTo   *time.Time `json:"valid_to,omitempty"`

	// Confidence in [0,1] range; defaults to 1.0 when not specified.
	Confidence float64 `json:"confidence"`

	// Provenance links back to the source memory.
	ProvenanceMemoryID string `json:"provenance_memory_id,omitempty"`
	ProvenanceSpan     string `json:"provenance_span,omitempty"` // e.g. "chars:120-180"

	// Supersedes is the ID of an earlier atom this one replaces (nullable).
	Supersedes string `json:"supersedes,omitempty"`

	CreatedAt time.Time `json:"created_at"`
}

// IsValid returns true when a is well-formed enough to persist.
// It does NOT validate database constraints — it catches obvious programmer
// errors (empty required fields, out-of-range confidence) before a DB round-trip.
func (a *Atom) IsValid() bool {
	if a.Type == "" || a.Subject == "" || a.Predicate == "" || a.Value == "" || a.Statement == "" {
		return false
	}
	switch a.Type {
	case TypePreference, TypeFact, TypeEvent, TypeAttribute, TypeRelationship:
	default:
		return false
	}
	if a.Confidence < 0 || a.Confidence > 1 {
		return false
	}
	return true
}
