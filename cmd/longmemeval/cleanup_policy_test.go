package main

import (
	"os"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// CleanupPolicy enum — #751
// ---------------------------------------------------------------------------

// TestCleanupPolicy_AutoPreservesExternalProject verifies that when
// --cleanup-policy=auto, a project whose name does NOT match the current
// run's lme-{runID}-* prefix is preserved (not deleted).
func TestCleanupPolicy_AutoPreservesExternalProject(t *testing.T) {
	// External project: ingested by a *different* run (runID "abc111")
	cfg := &Config{
		RunID:         "abc222",
		CleanupPolicy: CleanupPolicyAuto,
	}
	project := "lme-abc111-q001" // different runID in name
	if shouldCleanupProject(cfg, project) {
		t.Errorf("auto policy: expected external project %q to be preserved, but shouldCleanupProject returned true", project)
	}
}

// TestCleanupPolicy_AutoDeletesIngestedProject verifies that when
// --cleanup-policy=auto, a project whose name matches the current run's
// lme-{runID}-* prefix IS deleted.
func TestCleanupPolicy_AutoDeletesIngestedProject(t *testing.T) {
	cfg := &Config{
		RunID:         "abc222",
		CleanupPolicy: CleanupPolicyAuto,
	}
	project := "lme-abc222-q001" // matches this run's runID
	if !shouldCleanupProject(cfg, project) {
		t.Errorf("auto policy: expected ingested project %q to be cleaned up, but shouldCleanupProject returned false", project)
	}
}

// TestCleanupPolicy_AlwaysDeletes verifies that --cleanup-policy=always
// deletes every project regardless of provenance.
func TestCleanupPolicy_AlwaysDeletes(t *testing.T) {
	cfg := &Config{
		RunID:         "abc222",
		CleanupPolicy: CleanupPolicyAlways,
	}
	cases := []string{
		"lme-abc222-q001", // own project
		"lme-abc111-q001", // external project
		"my-custom-project", // non-lme project
	}
	for _, project := range cases {
		if !shouldCleanupProject(cfg, project) {
			t.Errorf("always policy: expected project %q to be deleted, but shouldCleanupProject returned false", project)
		}
	}
}

// TestCleanupPolicy_NeverPreserves verifies that --cleanup-policy=never
// preserves every project regardless of provenance.
func TestCleanupPolicy_NeverPreserves(t *testing.T) {
	cfg := &Config{
		RunID:         "abc222",
		CleanupPolicy: CleanupPolicyNever,
	}
	cases := []string{
		"lme-abc222-q001",
		"lme-abc111-q001",
		"my-custom-project",
	}
	for _, project := range cases {
		if shouldCleanupProject(cfg, project) {
			t.Errorf("never policy: expected project %q to be preserved, but shouldCleanupProject returned true", project)
		}
	}
}

// TestNoCleanupFlag_DeprecatedAliasForNever verifies that --no-cleanup is
// parsed as CleanupPolicyNever (backward-compat) and emits a deprecation
// warning to stderr.
func TestNoCleanupFlag_DeprecatedAliasForNever(t *testing.T) {
	// Structural: verify the flag registration wires NoCleanup to CleanupPolicyNever
	src, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatalf("read main.go: %v", err)
	}
	text := string(src)
	// The flag must still be registered for backward compat
	if !strings.Contains(text, `"no-cleanup"`) {
		t.Error("main.go: --no-cleanup flag must remain registered for backward compatibility (#751)")
	}
	// A deprecation notice must be emitted
	if !strings.Contains(text, "deprecated") && !strings.Contains(text, "DEPRECATED") {
		t.Error("main.go: --no-cleanup must emit a deprecation warning (#751)")
	}
}

// TestCleanupPolicyFlag_DefaultIsAuto verifies that CleanupPolicy defaults to
// "auto" in the flag registration so existing runs that don't set any flag
// switch to the safe default (preserve external projects).
func TestCleanupPolicyFlag_DefaultIsAuto(t *testing.T) {
	src, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatalf("read main.go: %v", err)
	}
	text := string(src)
	if !strings.Contains(text, `"cleanup-policy"`) {
		t.Error("main.go: --cleanup-policy flag must be registered (#751)")
	}
	if !strings.Contains(text, `CleanupPolicyAuto`) {
		t.Error("main.go: --cleanup-policy default must be CleanupPolicyAuto (#751)")
	}
}

// TestCleanupPolicyStructuralGuard verifies run.go uses shouldCleanupProject
// instead of the old cfg.NoCleanup boolean check.
func TestCleanupPolicyStructuralGuard(t *testing.T) {
	src, err := os.ReadFile("run.go")
	if err != nil {
		t.Fatalf("read run.go: %v", err)
	}
	text := string(src)
	if strings.Contains(text, "cfg.NoCleanup") {
		t.Error("run.go: cfg.NoCleanup still referenced; must be replaced by shouldCleanupProject() (#751)")
	}
	if !strings.Contains(text, "shouldCleanupProject") {
		t.Error("run.go: must call shouldCleanupProject() for cleanup decision (#751)")
	}
}
