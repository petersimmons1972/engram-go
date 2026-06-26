package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/petersimmons1972/engram/internal/atom"
	"github.com/petersimmons1972/engram/internal/search"
)

// ── fetchAtomContextBlock unit tests ─────────────────────────────────────────

// stubAtomFetcher implements atomFetcher for unit testing fetchAtomContextBlock.
type stubAtomFetcher struct {
	atoms []atom.Atom
	err   error
}

func (s *stubAtomFetcher) FetchAtoms(_ context.Context, _ string, _ string, _ int) ([]atom.Atom, error) {
	return s.atoms, s.err
}

// TestFetchAtomContextBlock_ReturnsBlockWhenAtomsPresent verifies that
// fetchAtomContextBlock returns a non-empty prompt block when the fetcher
// returns atoms.
func TestFetchAtomContextBlock_ReturnsBlockWhenAtomsPresent(t *testing.T) {
	stub := &stubAtomFetcher{
		atoms: []atom.Atom{
			{
				ID: "a1", Type: atom.TypePreference,
				Subject: "the user", Predicate: "prefers", Value: "dark chocolate",
				Statement: "The user prefers dark chocolate.", Scope: atom.ScopeGlobal, Confidence: 0.9,
			},
		},
	}
	block := fetchAtomContextBlock(context.Background(), stub, "proj", "q1", "")
	if block == "" {
		t.Error("expected non-empty block when atoms are available")
	}
	if !strings.Contains(block, "The user prefers dark chocolate.") {
		t.Error("block should contain atom statement")
	}
}

// TestFetchAtomContextBlock_ReturnsEmptyOnNoAtoms verifies empty block when
// the fetcher returns no atoms and no cache dir is set.
func TestFetchAtomContextBlock_ReturnsEmptyOnNoAtoms(t *testing.T) {
	stub := &stubAtomFetcher{}
	block := fetchAtomContextBlock(context.Background(), stub, "proj", "q1", "")
	if block != "" {
		t.Errorf("expected empty block with no atoms, got %q", block)
	}
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
	assertTHelper(t, strings.Contains(block, "[preference]"), "atom block should contain [preference] label")
	assertTHelper(t, strings.Contains(block, "The user prefers dark chocolate."), "atom block should contain the statement")
	assertTHelper(t, strings.Contains(block, "=== Extracted Preference Atoms ==="), "atom block should have header")
}

// TestAtomMode_EmptyAtoms_NoBlockAdded verifies that when no atoms are found,
// no atom context block is prepended to the prompt.
func TestAtomMode_EmptyAtoms_NoBlockAdded(t *testing.T) {
	block := search.FormatAtomsAsContext(nil)
	assertTHelper(t, block == "", "empty atoms should produce empty block")
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

// assertTHelper is a minimal boolean assertion helper (avoids importing testify
// into tests that are purely structural checks on source files).
func assertTHelper(t *testing.T, ok bool, msg string) {
	t.Helper()
	if !ok {
		t.Error(msg)
	}
}

// ── atom-cache JSON round-trip (#1181) ────────────────────────────────────────

// TestAtomCache_RoundTrip_PreferenceEntityFields verifies that Polarity/Entity/Domain
// survive a write→read cycle through the --atom-cache-dir JSON files used by
// atom-build and atom-mode. The eval path reads atoms from cache without touching
// the DB, so the fields MUST round-trip correctly.
func TestAtomCache_RoundTrip_PreferenceEntityFields(t *testing.T) {
	dir := t.TempDir()

	original := []atom.Atom{
		{
			ID:        "a1",
			Type:      atom.TypePreference,
			Subject:   "the user",
			Predicate: "prefers",
			Value:     "dark chocolate",
			Statement: "The user prefers dark chocolate over milk chocolate.",
			Scope:     atom.ScopeGlobal,
			Confidence: 0.9,
			Polarity:  "like",
			Entity:    "dark chocolate",
			Domain:    "food",
		},
		{
			ID:        "a2",
			Type:      atom.TypePreference,
			Subject:   "the user",
			Predicate: "dislikes",
			Value:     "cilantro",
			Statement: "The user dislikes cilantro.",
			Scope:     atom.ScopeGlobal,
			Confidence: 0.88,
			Polarity:  "dislike",
			Entity:    "cilantro",
			Domain:    "food",
		},
		{
			// Legacy atom without entity fields — must still round-trip cleanly.
			ID:        "a3",
			Type:      atom.TypePreference,
			Subject:   "the user",
			Predicate: "prefers",
			Value:     "tea",
			Statement: "The user prefers tea.",
			Scope:     atom.ScopeGlobal,
			Confidence: 0.85,
		},
	}

	// Write via writeAtomCacheFile (same function used by atom-build).
	if err := writeAtomCacheFile(dir, "test-project", original); err != nil {
		t.Fatalf("writeAtomCacheFile: %v", err)
	}

	// Verify the file was created.
	cachePath := filepath.Join(dir, "test-project.json")
	if _, err := os.Stat(cachePath); err != nil {
		t.Fatalf("cache file not created: %v", err)
	}

	// Read back via readAtomCache (same function used by atom-mode fallback).
	got, err := readAtomCache(dir, "test-project", atom.TypePreference, 0)
	if err != nil {
		t.Fatalf("readAtomCache: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("expected 3 atoms, got %d", len(got))
	}

	// Verify like-polarity atom round-trips.
	if got[0].Polarity != "like" {
		t.Errorf("atom[0].Polarity: want %q, got %q", "like", got[0].Polarity)
	}
	if got[0].Entity != "dark chocolate" {
		t.Errorf("atom[0].Entity: want %q, got %q", "dark chocolate", got[0].Entity)
	}
	if got[0].Domain != "food" {
		t.Errorf("atom[0].Domain: want %q, got %q", "food", got[0].Domain)
	}

	// Verify dislike-polarity atom round-trips.
	if got[1].Polarity != "dislike" {
		t.Errorf("atom[1].Polarity: want %q, got %q", "dislike", got[1].Polarity)
	}
	if got[1].Entity != "cilantro" {
		t.Errorf("atom[1].Entity: want %q, got %q", "cilantro", got[1].Entity)
	}

	// Verify legacy atom (no entity fields) round-trips cleanly.
	if got[2].Polarity != "" {
		t.Errorf("atom[2].Polarity: want empty, got %q", got[2].Polarity)
	}
	if got[2].Entity != "" {
		t.Errorf("atom[2].Entity: want empty, got %q", got[2].Entity)
	}

	// Verify FormatAtomsAsContext surfaces the entity names correctly.
	formatted := search.FormatAtomsAsContext(got)
	if !strings.Contains(formatted, "[LIKES: dark chocolate]") {
		t.Error("formatted output must contain [LIKES: dark chocolate]")
	}
	if !strings.Contains(formatted, "[AVOIDS: cilantro]") {
		t.Error("formatted output must contain [AVOIDS: cilantro]")
	}
}
