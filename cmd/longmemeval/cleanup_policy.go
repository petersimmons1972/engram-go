package main

import "strings"

// CleanupPolicy controls whether Engram projects are deleted after the run stage.
type CleanupPolicy string

const (
	// CleanupPolicyAuto (default) deletes only projects created by this run
	// invocation — identified by the lme-{runID}-* name prefix. Externally
	// supplied or cache-reused projects are preserved.
	CleanupPolicyAuto CleanupPolicy = "auto"

	// CleanupPolicyAlways unconditionally deletes every project touched by
	// the run stage, matching the pre-v0 behavior.
	CleanupPolicyAlways CleanupPolicy = "always"

	// CleanupPolicyNever preserves all projects regardless of provenance.
	// Equivalent to the deprecated --no-cleanup flag.
	CleanupPolicyNever CleanupPolicy = "never"
)

// shouldCleanupProject returns true when the configured cleanup policy
// requires project to be deleted after the run stage.
//
// Policy semantics:
//   - auto:   delete iff project name matches lme-{cfg.RunID}-* (created by this run)
//   - always: always delete
//   - never:  never delete
func shouldCleanupProject(cfg *Config, project string) bool {
	switch cfg.CleanupPolicy {
	case CleanupPolicyAlways:
		return true
	case CleanupPolicyNever:
		return false
	default: // CleanupPolicyAuto
		prefix := "lme-" + cfg.RunID + "-"
		return strings.HasPrefix(project, prefix)
	}
}
