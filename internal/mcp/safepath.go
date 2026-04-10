package mcp

import (
	"fmt"
	"path/filepath"
	"strings"
)

// SafePath resolves target against baseDir and verifies the result stays
// within baseDir. Returns the cleaned absolute path on success.
//
// If baseDir is empty the check is skipped and the cleaned path is returned
// as-is — callers MUST ensure baseDir is populated from trusted config before
// exposing file-operation tools.
func SafePath(baseDir, target string) (string, error) {
	if baseDir == "" {
		// No base dir configured: accept the path but warn callers that this
		// path is unconstrained. The tool handlers gate on DataDir being set.
		return filepath.Clean(target), nil
	}

	absBase, err := filepath.Abs(baseDir)
	if err != nil {
		return "", fmt.Errorf("resolving base dir: %w", err)
	}

	absTarget, err := filepath.Abs(target)
	if err != nil {
		return "", fmt.Errorf("resolving target path: %w", err)
	}

	// Ensure absTarget is inside absBase (with trailing separator to prevent
	// /data-evil from matching /data prefix).
	prefix := absBase + string(filepath.Separator)
	if absTarget != absBase && !strings.HasPrefix(absTarget, prefix) {
		return "", fmt.Errorf("path %q escapes allowed directory %q", target, baseDir)
	}

	return absTarget, nil
}
