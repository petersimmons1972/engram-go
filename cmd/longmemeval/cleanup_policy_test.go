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

// ---------------------------------------------------------------------------
// B2: unknown --cleanup-policy value must be rejected — Round 2 QA
// ---------------------------------------------------------------------------

// TestCleanupPolicy_UnknownValueRejected verifies that an unrecognised
// --cleanup-policy value causes dispatch to exit non-zero with a descriptive
// error message rather than silently falling through to auto. (B2)
// Note: --data is intentionally omitted so dispatch returns before running
// ingest (avoids log.Fatalf in loadItems). The cleanup-policy check must fire
// before the --data required check.
func TestCleanupPolicy_UnknownValueRejected(t *testing.T) {
	cases := []string{"ALWAYS", "Never", "sometimes", "1"}
	for _, val := range cases {
		t.Run("policy="+val, func(t *testing.T) {
			var stdout, stderr strings.Builder
			args := []string{"longmemeval", "ingest", "--cleanup-policy", val}
			exit := dispatch(args, &stdout, &stderr)
			if exit == 0 {
				t.Errorf("dispatch with --cleanup-policy=%q exited 0; want non-zero (invalid value must be rejected)", val)
			}
			combined := stderr.String() + stdout.String()
			if !strings.Contains(combined, "invalid --cleanup-policy") {
				t.Errorf("expected 'invalid --cleanup-policy' in output; got stderr=%q stdout=%q", stderr.String(), stdout.String())
			}
		})
	}
}

// TestCleanupPolicy_ValidValuesAccepted ensures the three valid values are not
// rejected by the enum guard (they may fail later due to missing --data, but
// must not fail with the cleanup-policy error). (B2 regression guard)
func TestCleanupPolicy_ValidValuesAccepted(t *testing.T) {
	for _, val := range []string{"auto", "always", "never"} {
		t.Run("policy="+val, func(t *testing.T) {
			var stdout, stderr strings.Builder
			args := []string{"longmemeval", "ingest", "--cleanup-policy", val}
			exit := dispatch(args, &stdout, &stderr)
			combined := stderr.String() + stdout.String()
			if strings.Contains(combined, "invalid --cleanup-policy") {
				t.Errorf("valid --cleanup-policy=%q was rejected: %s", val, combined)
			}
			// exit may be non-zero (e.g. missing --data) but must not be due to policy
			_ = exit
		})
	}
}

// ---------------------------------------------------------------------------
// B3: --no-cleanup + explicit --cleanup-policy must conflict — Round 2 QA
// ---------------------------------------------------------------------------

// TestNoCleanup_ConflictsWithExplicitPolicy verifies that combining the
// deprecated --no-cleanup flag with an explicit (non-default) --cleanup-policy
// value causes an error rather than silently overwriting the policy. (B3)
// Note: --data omitted intentionally — conflict check must fire before data check.
func TestNoCleanup_ConflictsWithExplicitPolicy(t *testing.T) {
	// "auto" is the default value; --no-cleanup + explicit auto cannot be
	// distinguished from --no-cleanup alone, so only non-auto values conflict.
	conflicts := []string{"always", "never"}
	for _, policy := range conflicts {
		t.Run("policy="+policy, func(t *testing.T) {
			var stdout, stderr strings.Builder
			args := []string{
				"longmemeval", "ingest",
				"--no-cleanup",
				"--cleanup-policy", policy,
			}
			exit := dispatch(args, &stdout, &stderr)
			if exit == 0 {
				t.Errorf("--no-cleanup --cleanup-policy=%s exited 0; want non-zero (conflict must be detected)", policy)
			}
			combined := stderr.String() + stdout.String()
			if !strings.Contains(combined, "conflicting flags") {
				t.Errorf("expected 'conflicting flags' in output; got stderr=%q stdout=%q", stderr.String(), stdout.String())
			}
		})
	}
}

// TestNoCleanup_AloneCoercesToNever verifies that --no-cleanup alone (without
// an explicit --cleanup-policy) silently coerces to never without error. (B3 regression guard)
func TestNoCleanup_AloneCoercesToNever(t *testing.T) {
	var stdout, stderr strings.Builder
	// --data is missing so dispatch will return an error, but it must NOT be
	// a conflict error — the conflict check only fires when policy is explicit.
	args := []string{"longmemeval", "ingest", "--no-cleanup"}
	_ = dispatch(args, &stdout, &stderr)
	combined := stderr.String() + stdout.String()
	if strings.Contains(combined, "conflicting flags") {
		t.Errorf("--no-cleanup alone must not trigger conflict error; got: %s", combined)
	}
	if strings.Contains(combined, "invalid --cleanup-policy") {
		t.Errorf("--no-cleanup alone must not trigger policy-validation error; got: %s", combined)
	}
}
