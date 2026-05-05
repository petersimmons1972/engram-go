package mcp

import (
	"fmt"
	"path/filepath"
	"strings"
)

// SafePath resolves target against baseDir and verifies the result stays
// within baseDir. Returns the cleaned absolute path on success.
//
// baseDir must be non-empty; file-operation tools require a configured data
// directory to be safe. Symlinks in both baseDir and target are resolved via
// filepath.EvalSymlinks before the containment check (prevents symlink-escape
// attacks). The ".." traversal check runs against the non-symlink-resolved
// absolute paths so it works even for paths that do not yet exist on disk.
func SafePath(baseDir, target string) (string, error) {
	if baseDir == "" {
		return "", fmt.Errorf("safe path: base directory is not configured")
	}

	absBase, err := filepath.Abs(baseDir)
	if err != nil {
		return "", fmt.Errorf("resolving base dir: %w", err)
	}
	// Resolve symlinks in the base directory itself.
	realBase, err := filepath.EvalSymlinks(absBase)
	if err != nil {
		return "", fmt.Errorf("resolving real base dir: %w", err)
	}

	// Resolve target against baseDir if it's a relative path; otherwise use as-is.
	// This ensures that relative paths are always resolved within baseDir, not
	// against the process's current working directory.
	var absTarget string
	if filepath.IsAbs(target) {
		var err error
		absTarget, err = filepath.Abs(target)
		if err != nil {
			return "", fmt.Errorf("resolving target path: %w", err)
		}
	} else {
		// Relative path: join with baseDir and clean
		absTarget = filepath.Join(absBase, target)
	}

	// First check (traversal): absTarget must sit inside absBase. This blocks
	// ".." attacks even for paths that don't exist on disk yet.
	absPrefix := absBase + string(filepath.Separator)
	if absTarget != absBase && !strings.HasPrefix(absTarget, absPrefix) {
		return "", fmt.Errorf("path %q escapes allowed directory %q", target, baseDir)
	}

	realPrefix := realBase + string(filepath.Separator)

	// Second check (symlink escape): resolve symlinks in target and verify
	// the resolved path is still inside realBase.
	realTarget, err := filepath.EvalSymlinks(absTarget)
	if err == nil {
		// Target exists — ensure its real path stays within realBase.
		if realTarget != realBase && !strings.HasPrefix(realTarget, realPrefix) {
			return "", fmt.Errorf("path %q escapes allowed directory %q", target, baseDir)
		}
		return realTarget, nil
	}

	// Target doesn't exist yet (e.g. a new file to be written). Try to resolve
	// the parent directory to catch symlinks in the parent.
	parentReal, err2 := filepath.EvalSymlinks(filepath.Dir(absTarget))
	if err2 == nil {
		candidate := filepath.Join(parentReal, filepath.Base(absTarget))
		if candidate != realBase && !strings.HasPrefix(candidate, realPrefix) {
			return "", fmt.Errorf("path %q escapes allowed directory %q", target, baseDir)
		}
		return candidate, nil
	}

	// Parent directory doesn't exist either — return the cleaned absolute path.
	// The traversal check above already verified containment without symlink
	// resolution, which is sufficient for paths that do not yet exist.
	return absTarget, nil
}
