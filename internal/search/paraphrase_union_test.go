// paraphrase_union_test.go — unit tests for the ParaphraseUnion retrieval path.
//
// Coverage targets:
//  - unionCandidates: dedup, order preservation, empty inputs.
//  - RuleBasedParaphraser.Paraphrase: rule correctness, n limit, no-dup guarantee,
//    empty/blank input, flag-off (ParaphraseUnion=false) identity guarantee.
//  - expandContractions, stripWHPrefix, stripAuxiliaryPrefix: rule correctness.
package search

import (
	"reflect"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// unionCandidates
// ---------------------------------------------------------------------------

func TestUnionCandidates_EmptyBoth(t *testing.T) {
	got := unionCandidates(nil, nil)
	if len(got) != 0 {
		t.Errorf("expected empty, got %v", got)
	}
}

func TestUnionCandidates_EmptySetB(t *testing.T) {
	a := []string{"id1", "id2"}
	got := unionCandidates(a, nil)
	if !reflect.DeepEqual(got, a) {
		t.Errorf("expected %v, got %v", a, got)
	}
}

func TestUnionCandidates_EmptySetA(t *testing.T) {
	b := []string{"id3", "id4"}
	got := unionCandidates(nil, b)
	if len(got) != 2 || got[0] != "id3" || got[1] != "id4" {
		t.Errorf("expected %v, got %v", b, got)
	}
}

func TestUnionCandidates_DisjointSets(t *testing.T) {
	a := []string{"a", "b"}
	b := []string{"c", "d"}
	got := unionCandidates(a, b)
	want := []string{"a", "b", "c", "d"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("expected %v, got %v", want, got)
	}
}

func TestUnionCandidates_DeduplicatesOverlap(t *testing.T) {
	// setA and setB share "b"; "b" must appear only once in output.
	a := []string{"a", "b", "c"}
	b := []string{"b", "d", "e"}
	got := unionCandidates(a, b)
	want := []string{"a", "b", "c", "d", "e"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("expected %v, got %v", want, got)
	}
}

func TestUnionCandidates_PreservesSetAOrder(t *testing.T) {
	// setA order must be preserved in the output prefix.
	a := []string{"z", "y", "x"}
	b := []string{"x", "w"}
	got := unionCandidates(a, b)
	if got[0] != "z" || got[1] != "y" || got[2] != "x" {
		t.Errorf("setA order not preserved: %v", got)
	}
	if len(got) != 4 || got[3] != "w" {
		t.Errorf("unexpected tail: %v", got)
	}
}

func TestUnionCandidates_AllDuplicates(t *testing.T) {
	a := []string{"a", "b"}
	b := []string{"a", "b"}
	got := unionCandidates(a, b)
	if !reflect.DeepEqual(got, a) {
		t.Errorf("expected %v (all dups), got %v", a, got)
	}
}

func TestUnionCandidates_SingleElement(t *testing.T) {
	got := unionCandidates([]string{"x"}, []string{"y"})
	if len(got) != 2 || got[0] != "x" || got[1] != "y" {
		t.Errorf("unexpected result: %v", got)
	}
}

// ---------------------------------------------------------------------------
// RuleBasedParaphraser.Paraphrase
// ---------------------------------------------------------------------------

func TestRuleBasedParaphraser_EmptyQuery(t *testing.T) {
	p := RuleBasedParaphraser{}
	got := p.Paraphrase("", 3)
	if len(got) != 0 {
		t.Errorf("expected nil for empty query, got %v", got)
	}
}

func TestRuleBasedParaphraser_BlankQuery(t *testing.T) {
	p := RuleBasedParaphraser{}
	got := p.Paraphrase("   ", 3)
	if len(got) != 0 {
		t.Errorf("expected nil for blank query, got %v", got)
	}
}

func TestRuleBasedParaphraser_ZeroN(t *testing.T) {
	p := RuleBasedParaphraser{}
	got := p.Paraphrase("what is the capital of France", 0)
	if len(got) != 0 {
		t.Errorf("expected nil for n=0, got %v", got)
	}
}

func TestRuleBasedParaphraser_NegativeN(t *testing.T) {
	p := RuleBasedParaphraser{}
	got := p.Paraphrase("who is Alice", -1)
	if len(got) != 0 {
		t.Errorf("expected nil for n<0, got %v", got)
	}
}

func TestRuleBasedParaphraser_LimitN(t *testing.T) {
	p := RuleBasedParaphraser{}
	// Ask for only 1 paraphrase — must get at most 1.
	got := p.Paraphrase("what is John's phone number", 1)
	if len(got) > 1 {
		t.Errorf("expected at most 1 paraphrase, got %d: %v", len(got), got)
	}
}

func TestRuleBasedParaphraser_NoDuplicates(t *testing.T) {
	p := RuleBasedParaphraser{}
	got := p.Paraphrase("who is Alice Johnson", 5)
	seen := make(map[string]int)
	for i, s := range got {
		seen[s]++
		if seen[s] > 1 {
			t.Errorf("duplicate paraphrase %q at index %d", s, i)
		}
	}
}

func TestRuleBasedParaphraser_OriginalNotIncluded(t *testing.T) {
	p := RuleBasedParaphraser{}
	query := "where does alice live"
	got := p.Paraphrase(query, 5)
	for _, s := range got {
		if s == query {
			t.Errorf("original query %q should not appear in paraphrases", query)
		}
	}
}

func TestRuleBasedParaphraser_WHPrefixStripped(t *testing.T) {
	p := RuleBasedParaphraser{}
	// "what is X" → bare form "X" must appear somewhere in the results.
	got := p.Paraphrase("what is alice's address", 5)
	found := false
	for _, s := range got {
		if s == "alice's address" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected bare form 'alice's address' in paraphrases; got: %v", got)
	}
}

func TestRuleBasedParaphraser_ContractionExpanded(t *testing.T) {
	p := RuleBasedParaphraser{}
	// "what's" → contraction expanded to "what is" before prefix stripping.
	got := p.Paraphrase("what's alice's job", 5)
	// At least one result should not start with "what's".
	anyExpanded := false
	for _, s := range got {
		if !strings.HasPrefix(s, "what's") {
			anyExpanded = true
			break
		}
	}
	if !anyExpanded {
		t.Errorf("expected at least one contraction-expanded form in %v", got)
	}
}

func TestRuleBasedParaphraser_TellMeAbout(t *testing.T) {
	p := RuleBasedParaphraser{}
	got := p.Paraphrase("tell me about bob's hobbies", 5)
	found := false
	for _, s := range got {
		if s == "bob's hobbies" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected bare 'bob's hobbies' in %v", got)
	}
}

func TestRuleBasedParaphraser_InformationAboutWrapper(t *testing.T) {
	p := RuleBasedParaphraser{}
	got := p.Paraphrase("alice johnson", 5)
	found := false
	for _, s := range got {
		if s == "information about alice johnson" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected 'information about alice johnson' in %v", got)
	}
}

func TestRuleBasedParaphraser_PlainQueryProducesVariants(t *testing.T) {
	p := RuleBasedParaphraser{}
	// A plain noun phrase with no wh-prefix should still produce >= 1 variants.
	got := p.Paraphrase("Bob Smith restaurant preference", 5)
	if len(got) == 0 {
		t.Error("expected at least one paraphrase for a plain noun phrase")
	}
}

func TestRuleBasedParaphraser_AllParaphrasesAreLowercase(t *testing.T) {
	p := RuleBasedParaphraser{}
	got := p.Paraphrase("What Is Alice Johnson's Address", 5)
	for _, s := range got {
		if s != strings.ToLower(s) {
			t.Errorf("paraphrase %q is not lowercase", s)
		}
	}
}

// ---------------------------------------------------------------------------
// Flag-off identity guarantee (zero-value test, no DB required)
// ---------------------------------------------------------------------------

// TestParaphraseUnionFlagOff verifies that RecallOpts.ParaphraseUnion defaults
// to false (zero value), ensuring callers that don't set the flag get identical
// baseline behavior with no extra recall passes.
func TestParaphraseUnionFlagOff_IsZeroValue(t *testing.T) {
	var opts RecallOpts
	if opts.ParaphraseUnion {
		t.Error("RecallOpts.ParaphraseUnion default must be false (flag-off = baseline)")
	}
}

// ---------------------------------------------------------------------------
// expandContractions
// ---------------------------------------------------------------------------

func TestExpandContractions_NoChange(t *testing.T) {
	got := expandContractions("alice lives in paris")
	want := "alice lives in paris"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestExpandContractions_SingleContraction(t *testing.T) {
	got := expandContractions("what's alice's address")
	want := "what is alice's address"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestExpandContractions_MultipleContractions(t *testing.T) {
	got := expandContractions("it's not what i'm thinking")
	want := "it is not what i am thinking"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestExpandContractions_CantExpansion(t *testing.T) {
	got := expandContractions("i can't stand opera")
	want := "i cannot stand opera"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// ---------------------------------------------------------------------------
// stripWHPrefix
// ---------------------------------------------------------------------------

func TestStripWHPrefix_WhatIs(t *testing.T) {
	got := stripWHPrefix("what is alice's phone")
	want := "alice's phone"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestStripWHPrefix_WhoIs(t *testing.T) {
	got := stripWHPrefix("who is bob smith")
	want := "bob smith"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestStripWHPrefix_WhereIs(t *testing.T) {
	got := stripWHPrefix("where is the library")
	want := "the library"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestStripWHPrefix_NoPrefix(t *testing.T) {
	got := stripWHPrefix("alice johnson address")
	want := "alice johnson address"
	if got != want {
		t.Errorf("got %q (no change expected), want %q", got, want)
	}
}

func TestStripWHPrefix_PrefixOnlyNoChange(t *testing.T) {
	// "where is " with trailing space but no subject → should return unchanged.
	got := stripWHPrefix("where is ")
	// trailing TrimSpace makes remainder empty → no strip
	if got != "where is " {
		t.Errorf("got %q, want 'where is ' (empty remainder, no strip)", got)
	}
}

func TestStripWHPrefix_TellMeAbout(t *testing.T) {
	got := stripWHPrefix("tell me about alice's pets")
	want := "alice's pets"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// ---------------------------------------------------------------------------
// stripAuxiliaryPrefix
// ---------------------------------------------------------------------------

func TestStripAuxiliaryPrefix_Did(t *testing.T) {
	got := stripAuxiliaryPrefix("did alice change jobs")
	want := "alice change jobs"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestStripAuxiliaryPrefix_Does(t *testing.T) {
	got := stripAuxiliaryPrefix("does bob live in new york")
	want := "bob live in new york"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestStripAuxiliaryPrefix_NoPrefix(t *testing.T) {
	got := stripAuxiliaryPrefix("alice new york")
	want := "alice new york"
	if got != want {
		t.Errorf("got %q (no change expected), want %q", got, want)
	}
}

func TestStripAuxiliaryPrefix_Is(t *testing.T) {
	got := stripAuxiliaryPrefix("is alice vegetarian")
	want := "alice vegetarian"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// ---------------------------------------------------------------------------
// Helpers (used only by tests in this file)
// ---------------------------------------------------------------------------

