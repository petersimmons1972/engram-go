package atom_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/petersimmons1972/engram/internal/atom"
)

// TestAtom_NewFields_BackCompat verifies that the new Polarity/Entity/Domain
// fields are optional — existing atoms without them remain valid.
func TestAtom_NewFields_BackCompat(t *testing.T) {
	a := atom.Atom{
		Type:       atom.TypePreference,
		Subject:    "the user",
		Predicate:  "prefers",
		Value:      "dark chocolate",
		Statement:  "The user prefers dark chocolate.",
		Scope:      atom.ScopeGlobal,
		Confidence: 0.9,
	}
	assert.True(t, a.IsValid(), "atom without Polarity/Entity/Domain must still be valid")
}

// TestAtom_NewFields_Populated verifies IsValid passes when all three new
// fields are populated.
func TestAtom_NewFields_Populated(t *testing.T) {
	a := atom.Atom{
		Type:       atom.TypePreference,
		Subject:    "the user",
		Predicate:  "prefers",
		Value:      "dark chocolate",
		Statement:  "The user prefers dark chocolate.",
		Scope:      atom.ScopeGlobal,
		Confidence: 0.9,
		Polarity:   "like",
		Entity:     "dark chocolate",
		Domain:     "food",
	}
	assert.True(t, a.IsValid(), "atom with Polarity/Entity/Domain must be valid")
}

// TestAtom_Polarity_Dislike verifies dislike polarity is accepted.
func TestAtom_Polarity_Dislike(t *testing.T) {
	a := atom.Atom{
		Type:       atom.TypePreference,
		Subject:    "the user",
		Predicate:  "dislikes",
		Value:      "cilantro",
		Statement:  "The user dislikes cilantro.",
		Scope:      atom.ScopeGlobal,
		Confidence: 0.9,
		Polarity:   "dislike",
		Entity:     "cilantro",
		Domain:     "food",
	}
	assert.True(t, a.IsValid())
	assert.Equal(t, "dislike", a.Polarity)
	assert.Equal(t, "cilantro", a.Entity)
	assert.Equal(t, "food", a.Domain)
}

// TestAtom_NewFields_ZeroValues confirms zero-value strings are acceptable.
func TestAtom_NewFields_ZeroValues(t *testing.T) {
	a := atom.Atom{
		Type:       atom.TypeFact,
		Subject:    "Alice",
		Predicate:  "works at",
		Value:      "Acme",
		Statement:  "Alice works at Acme.",
		Scope:      atom.ScopeGlobal,
		Confidence: 1.0,
		Polarity:   "",
		Entity:     "",
		Domain:     "",
	}
	assert.True(t, a.IsValid(), "fact atom with empty optional fields must be valid")
}
