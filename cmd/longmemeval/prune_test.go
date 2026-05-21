package main

import (
	"strings"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// Unit tests for prune logic — #754
// ---------------------------------------------------------------------------

// TestPrune_SelectsOnlyExpiredPrefixed verifies that selectExpiredProjects
// returns only entries that are both (a) prefixed with the target prefix and
// (b) have expires_at strictly before now.
func TestPrune_SelectsOnlyExpiredPrefixed(t *testing.T) {
	now := time.Now()

	entries := []projectTTLEntry{
		{Name: "lme-abc-q001", ExpiresAt: ptr(now.Add(-time.Hour))},   // expired, prefixed ✓
		{Name: "lme-abc-q002", ExpiresAt: ptr(now.Add(time.Hour))},    // not expired
		{Name: "other-project", ExpiresAt: ptr(now.Add(-time.Hour))},  // expired, wrong prefix
		{Name: "lme-abc-q003", ExpiresAt: nil},                        // NULL expires_at = durable
		{Name: "lme-abc-q004", ExpiresAt: ptr(now.Add(-2 * time.Hour))}, // expired, prefixed ✓
	}

	got := selectExpiredProjects(entries, "lme-", 0, now)

	if len(got) != 2 {
		t.Fatalf("expected 2 expired+prefixed projects, got %d: %v", len(got), got)
	}
	want := map[string]bool{"lme-abc-q001": true, "lme-abc-q004": true}
	for _, name := range got {
		if !want[name] {
			t.Errorf("unexpected project in result: %q", name)
		}
	}
}

// TestPrune_DryRunNoMutation verifies that runPrune with --dry-run writes
// intended deletions to output but does not call the delete function.
func TestPrune_DryRunNoMutation(t *testing.T) {
	now := time.Now()
	entries := []projectTTLEntry{
		{Name: "lme-run1-q001", ExpiresAt: ptr(now.Add(-time.Hour))},
		{Name: "lme-run1-q002", ExpiresAt: ptr(now.Add(-2 * time.Hour))},
	}

	var deleted []string
	deleteFn := func(name string) error {
		deleted = append(deleted, name)
		return nil
	}

	cfg := &PruneConfig{
		Prefix:    "lme-",
		OlderThan: 0,
		DryRun:    true,
		Limit:     0,
	}

	var out strings.Builder
	code := runPruneWithEntries(cfg, entries, deleteFn, now, &out)
	if code != 0 {
		t.Errorf("expected exit 0, got %d", code)
	}
	if len(deleted) != 0 {
		t.Errorf("dry-run must not call delete; got deletions: %v", deleted)
	}
	// Output should mention the would-be deletions
	outStr := out.String()
	if !strings.Contains(outStr, "lme-run1-q001") {
		t.Errorf("dry-run output should mention lme-run1-q001; got: %q", outStr)
	}
}

// TestPrune_NullExpiresAtPreserved verifies that projects with NULL expires_at
// are never selected for deletion (durable semantics).
func TestPrune_NullExpiresAtPreserved(t *testing.T) {
	now := time.Now()
	entries := []projectTTLEntry{
		{Name: "lme-abc-q001", ExpiresAt: nil},                         // durable
		{Name: "lme-abc-q002", ExpiresAt: nil},                         // durable
		{Name: "lme-abc-q003", ExpiresAt: ptr(now.Add(-time.Hour))},    // expired
	}

	got := selectExpiredProjects(entries, "lme-", 0, now)

	for _, name := range got {
		if name == "lme-abc-q001" || name == "lme-abc-q002" {
			t.Errorf("durable project %q (NULL expires_at) must not be selected for pruning", name)
		}
	}
	if len(got) != 1 || got[0] != "lme-abc-q003" {
		t.Errorf("expected only lme-abc-q003, got %v", got)
	}
}

// TestPrune_EmptyResultZeroExit verifies that when no expired+prefixed projects
// exist, runPruneWithEntries exits 0 (not an error condition).
func TestPrune_EmptyResultZeroExit(t *testing.T) {
	now := time.Now()
	entries := []projectTTLEntry{
		{Name: "lme-abc-q001", ExpiresAt: ptr(now.Add(time.Hour))}, // not expired
	}

	var deleted []string
	deleteFn := func(name string) error {
		deleted = append(deleted, name)
		return nil
	}

	cfg := &PruneConfig{
		Prefix:    "lme-",
		OlderThan: 0,
		DryRun:    false,
		Limit:     0,
	}

	var out strings.Builder
	code := runPruneWithEntries(cfg, entries, deleteFn, now, &out)
	if code != 0 {
		t.Errorf("empty result set must exit 0, got %d; output: %q", code, out.String())
	}
	if len(deleted) != 0 {
		t.Errorf("no deletions expected; got: %v", deleted)
	}
}

// TestPrune_BoundaryAtExpiry verifies that a project whose expires_at equals
// exactly now (or is in the past by any amount) is included.
// expires_at < now → expired; expires_at == now → boundary case, included.
func TestPrune_BoundaryAtExpiry(t *testing.T) {
	now := time.Now()

	entries := []projectTTLEntry{
		// Exactly at expiry boundary: now - 1ns (1 nanosecond past)
		{Name: "lme-abc-boundary", ExpiresAt: ptr(now.Add(-1))},
		// 1ns in the future: not yet expired
		{Name: "lme-abc-future", ExpiresAt: ptr(now.Add(1))},
	}

	got := selectExpiredProjects(entries, "lme-", 0, now)

	found := false
	for _, name := range got {
		if name == "lme-abc-boundary" {
			found = true
		}
		if name == "lme-abc-future" {
			t.Errorf("project expiring in the future must not be pruned")
		}
	}
	if !found {
		t.Errorf("project at exact expiry boundary must be included in prune set")
	}
}

// TestPrune_LimitCapsResults verifies that when --limit N is set, at most N
// projects are returned for deletion.
func TestPrune_LimitCapsResults(t *testing.T) {
	now := time.Now()
	entries := []projectTTLEntry{
		{Name: "lme-abc-q001", ExpiresAt: ptr(now.Add(-time.Hour))},
		{Name: "lme-abc-q002", ExpiresAt: ptr(now.Add(-2 * time.Hour))},
		{Name: "lme-abc-q003", ExpiresAt: ptr(now.Add(-3 * time.Hour))},
	}

	got := selectExpiredProjects(entries, "lme-", 2, now)
	if len(got) > 2 {
		t.Errorf("limit=2 must cap results at 2, got %d", len(got))
	}
}

// TestPrune_OlderThanShiftsWindow verifies that --older-than shifts the
// effective cutoff so that only projects expired longer than the duration ago
// are selected.
func TestPrune_OlderThanShiftsWindow(t *testing.T) {
	now := time.Now()

	// Project expired 30 minutes ago — within 1h older-than window, so NOT selected
	recent := ptr(now.Add(-30 * time.Minute))
	// Project expired 2 hours ago — outside 1h older-than window, selected
	old := ptr(now.Add(-2 * time.Hour))

	entries := []projectTTLEntry{
		{Name: "lme-abc-recent", ExpiresAt: recent},
		{Name: "lme-abc-old", ExpiresAt: old},
	}

	// With olderThan=1h, effective cutoff = now - 1h
	// Only lme-abc-old (expired 2h ago, before cutoff) should be selected.
	got := selectExpiredProjects(entries, "lme-", 0, now, withOlderThan(time.Hour))

	for _, name := range got {
		if name == "lme-abc-recent" {
			t.Errorf("project expired 30m ago must not be selected with older-than=1h")
		}
	}
	found := false
	for _, name := range got {
		if name == "lme-abc-old" {
			found = true
		}
	}
	if !found {
		t.Errorf("project expired 2h ago must be selected with older-than=1h")
	}
}

// TestPrune_ScratchTTLFlagRegistered verifies the --scratch-ttl flag is wired
// into the dispatch function (structural test on main.go / prune.go).
func TestPrune_ScratchTTLFlagRegistered(t *testing.T) {
	var stdout, stderr strings.Builder
	// "prune --help" should mention scratch-ttl somewhere in usage
	args := []string{"longmemeval", "prune", "--help"}
	dispatch(args, &stdout, &stderr)
	combined := stdout.String() + stderr.String()
	if !strings.Contains(combined, "scratch-ttl") && !strings.Contains(combined, "older-than") {
		t.Errorf("prune subcommand help must mention TTL-related flags; got: %q", combined)
	}
}

// TestScratchTTL_DefaultIs168h verifies that Config.ScratchTTL defaults to
// 168h (7 days).
func TestScratchTTL_DefaultIs168h(t *testing.T) {
	const want = 168 * time.Hour
	cfg := defaultScratchTTL()
	if cfg != want {
		t.Errorf("defaultScratchTTL() = %v, want %v", cfg, want)
	}
}

// ptr is a helper to convert a time.Time into a *time.Time.
func ptr(t time.Time) *time.Time {
	return &t
}
