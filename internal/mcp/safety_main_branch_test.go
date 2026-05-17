package mcp

// Regression tests for #709: classifyAction's main-branch detection uses
// a word-boundary regex (mainBranchRe) instead of brittle substring match.

import "testing"

func TestMainBranchRe_PositiveCases(t *testing.T) {
	for _, s := range []string{
		"push to main",
		"merge into master",
		"deploy main branch",
		"force push to origin/main",
		"git checkout main",
		"PUSH TO MAIN", // case-insensitive
	} {
		if !mainBranchRe.MatchString(s) {
			t.Errorf("expected match for %q", s)
		}
	}
}

func TestMainBranchRe_NegativeCases(t *testing.T) {
	for _, s := range []string{
		"deploy to mainframe",     // "main" inside "mainframe" — must not match
		"connect domain/maintest", // "main" inside path segment — must not match
		"remains",                 // "main" as substring of unrelated word
		"masterclass",             // "master" inside "masterclass"
		"feature/main-cluster",    // "main" as path-segment prefix
	} {
		if mainBranchRe.MatchString(s) {
			t.Errorf("expected NO match for %q", s)
		}
	}
}
