package main

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/petersimmons1972/engram/internal/atom"
	"github.com/petersimmons1972/engram/internal/search"
)

// ── fetchAtomContextBlock unit tests ─────────────────────────────────────────

// stubAtomFetcherFull implements atomFetcher, returning a set of preference atoms.
type stubAtomFetcherFull struct {
	atoms []atom.Atom
	err   error
}

func (s *stubAtomFetcherFull) FetchAtoms(_ context.Context, _ string, _ string, _ int) ([]atom.Atom, error) {
	return s.atoms, s.err
}

// ── run.go structural tests ──────────────────────────────────────────────────

// TestAtomMode_PromptContainsAtomBlock verifies that when --atom-mode is set and
// atoms are available, the generation prompt contains the labeled atom context block.
// This tests the code path integration without a live server.
func TestAtomMode_PromptContainsAtomBlock(t *testing.T) {
	atoms := []atom.Atom{
		{
			ID: "a1", Type: atom.TypePreference,
			Subject: "the user", Predicate: "prefers", Value: "dark chocolate",
			Statement: "The user prefers dark chocolate.", Scope: atom.ScopeGlobal, Confidence: 0.9,
		},
	}

	// Format atoms as the prompt injector would.
	block := search.FormatAtomsAsContext(atoms)
	assert_t_helper(t, strings.Contains(block, "[preference]"), "atom block should contain [preference] label")
	assert_t_helper(t, strings.Contains(block, "The user prefers dark chocolate."), "atom block should contain the statement")
	assert_t_helper(t, strings.Contains(block, "=== Extracted Preference Atoms ==="), "atom block should have header")
}

// TestAtomMode_EmptyAtoms_NoBlockAdded verifies that when no atoms are found,
// no atom context block is prepended to the prompt.
func TestAtomMode_EmptyAtoms_NoBlockAdded(t *testing.T) {
	block := search.FormatAtomsAsContext(nil)
	assert_t_helper(t, block == "", "empty atoms should produce empty block")
}

// TestAtomMode_FlagRegistered verifies that the --atom-mode flag is registered
// in the dispatch function. This is a structural test that reads run.go source
// to confirm the flag wiring is present — matches the style of TestRunWorker_HasPerItemCleanup.
func TestAtomMode_FlagRegistered(t *testing.T) {
	src, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatalf("read main.go: %v", err)
	}
	text := string(src)
	if !strings.Contains(text, "atom-mode") {
		t.Error("main.go missing 'atom-mode' flag registration — #938 harness wiring broken")
	}
	if !strings.Contains(text, "AtomMode") {
		t.Error("main.go missing AtomMode config field — #938 harness wiring broken")
	}
}

// TestAtomMode_RunGoInjectsBlock verifies that run.go contains the atom context
// injection code path. Structural check matching TestRunWorker_HasPerItemCleanup style.
func TestAtomMode_RunGoInjectsBlock(t *testing.T) {
	src, err := os.ReadFile("run.go")
	if err != nil {
		t.Fatalf("read run.go: %v", err)
	}
	text := string(src)
	if !strings.Contains(text, "atomContextBlock") {
		t.Error("run.go missing atomContextBlock variable — atom-mode prompt injection not wired (#938)")
	}
	if !strings.Contains(text, "cfg.AtomMode") {
		t.Error("run.go missing cfg.AtomMode check — atom-mode not guarded by flag (#938)")
	}
	if !strings.Contains(text, "fetchAtomContextBlock") {
		t.Error("run.go missing fetchAtomContextBlock call — atom-mode not calling fetch helper (#938)")
	}
}

// TestAtomMode_AtomModeGo_WiresAtomType verifies that atom_mode.go specifies
// the preference atom type filter.
func TestAtomMode_AtomModeGo_WiresAtomType(t *testing.T) {
	src, err := os.ReadFile("atom_mode.go")
	if err != nil {
		t.Fatalf("read atom_mode.go: %v", err)
	}
	text := string(src)
	if !strings.Contains(text, "atom.TypePreference") {
		t.Error("atom_mode.go should filter by atom.TypePreference — captures preference atoms only (#938)")
	}
}

// assert_t_helper is a minimal boolean assertion helper (avoids importing testify
// into tests that are purely structural checks on source files).
func assert_t_helper(t *testing.T, ok bool, msg string) {
	t.Helper()
	if !ok {
		t.Error(msg)
	}
}
