package main

import (
	"context"
	"errors"
	"runtime/debug"
	"testing"
)

// TestResolveHarnessSHA_Override verifies that an explicit --harness-sha
// value always wins, without even attempting git or build-info lookups.
func TestResolveHarnessSHA_Override(t *testing.T) {
	calledGit := false
	calledBuildInfo := false
	runGit := func(context.Context) (string, error) {
		calledGit = true
		return "deadbeef", nil
	}
	readBuildInfo := func() (*debug.BuildInfo, bool) {
		calledBuildInfo = true
		return nil, false
	}

	got := resolveHarnessSHA(context.Background(), "override-sha", runGit, readBuildInfo)

	if got != "override-sha" {
		t.Fatalf("resolveHarnessSHA() = %q, want %q", got, "override-sha")
	}
	if calledGit {
		t.Error("resolveHarnessSHA() called runGit despite an explicit override being provided")
	}
	if calledBuildInfo {
		t.Error("resolveHarnessSHA() called readBuildInfo despite an explicit override being provided")
	}
}

// TestResolveHarnessSHA_GitSucceeds is the happy path: no override, git
// resolves successfully — the git SHA must be used and build info must not
// be consulted.
func TestResolveHarnessSHA_GitSucceeds(t *testing.T) {
	calledBuildInfo := false
	runGit := func(context.Context) (string, error) {
		return "abc1234", nil
	}
	readBuildInfo := func() (*debug.BuildInfo, bool) {
		calledBuildInfo = true
		return nil, false
	}

	got := resolveHarnessSHA(context.Background(), "", runGit, readBuildInfo)

	if got != "abc1234" {
		t.Fatalf("resolveHarnessSHA() = %q, want %q", got, "abc1234")
	}
	if calledBuildInfo {
		t.Error("resolveHarnessSHA() called readBuildInfo despite git succeeding")
	}
}

// TestResolveHarnessSHA_GitFailsFallsBackToBuildInfo covers the issue's core
// scenario: no git checkout available (shallow clone, extracted tarball,
// detached HEAD without refs). The function must not abort the run — it
// must fall back to the VCS revision recorded in the binary's build info.
func TestResolveHarnessSHA_GitFailsFallsBackToBuildInfo(t *testing.T) {
	runGit := func(context.Context) (string, error) {
		return "", errors.New("fatal: not a git repository")
	}
	readBuildInfo := func() (*debug.BuildInfo, bool) {
		return &debug.BuildInfo{
			Settings: []debug.BuildSetting{
				{Key: "vcs", Value: "git"},
				{Key: "vcs.revision", Value: "0123456789abcdef0123456789abcdef01234567"},
				{Key: "vcs.modified", Value: "false"},
			},
		}, true
	}

	got := resolveHarnessSHA(context.Background(), "", runGit, readBuildInfo)

	want := "0123456789ab" // truncated to 12 chars
	if got != want {
		t.Fatalf("resolveHarnessSHA() = %q, want %q", got, want)
	}
}

// TestResolveHarnessSHA_GitFailsShortRevisionNotTruncated verifies short
// vcs.revision values (fewer than 12 chars) are returned as-is rather than
// panicking on a slice-out-of-range.
func TestResolveHarnessSHA_GitFailsShortRevisionNotTruncated(t *testing.T) {
	runGit := func(context.Context) (string, error) {
		return "", errors.New("no git binary")
	}
	readBuildInfo := func() (*debug.BuildInfo, bool) {
		return &debug.BuildInfo{
			Settings: []debug.BuildSetting{
				{Key: "vcs.revision", Value: "abc123"},
			},
		}, true
	}

	got := resolveHarnessSHA(context.Background(), "", runGit, readBuildInfo)

	if got != "abc123" {
		t.Fatalf("resolveHarnessSHA() = %q, want %q", got, "abc123")
	}
}

// TestResolveHarnessSHA_AllSourcesFail is the boundary/degradation case: no
// override, no git, no usable build info. resolveHarnessSHA must degrade
// gracefully to a sentinel value rather than the caller aborting the whole
// eval run (the original bug: gitShortSHA's error propagated up and killed
// the run outside a git checkout).
func TestResolveHarnessSHA_AllSourcesFail(t *testing.T) {
	tests := []struct {
		name          string
		runGit        func(context.Context) (string, error)
		readBuildInfo func() (*debug.BuildInfo, bool)
	}{
		{
			name: "git fails, no build info available",
			runGit: func(context.Context) (string, error) {
				return "", errors.New("not a git repository")
			},
			readBuildInfo: func() (*debug.BuildInfo, bool) {
				return nil, false
			},
		},
		{
			name: "git fails, build info present but no vcs.revision setting",
			runGit: func(context.Context) (string, error) {
				return "", errors.New("not a git repository")
			},
			readBuildInfo: func() (*debug.BuildInfo, bool) {
				return &debug.BuildInfo{
					Settings: []debug.BuildSetting{
						{Key: "GOOS", Value: "linux"},
					},
				}, true
			},
		},
		{
			name: "git succeeds but returns empty output, build info has empty revision",
			runGit: func(context.Context) (string, error) {
				return "   ", nil
			},
			readBuildInfo: func() (*debug.BuildInfo, bool) {
				return &debug.BuildInfo{
					Settings: []debug.BuildSetting{
						{Key: "vcs.revision", Value: ""},
					},
				}, true
			},
		},
		{
			name:   "readBuildInfo is nil",
			runGit: func(context.Context) (string, error) { return "", errors.New("no git") },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resolveHarnessSHA(context.Background(), "", tt.runGit, tt.readBuildInfo)
			if got != harnessSHAUnknown {
				t.Fatalf("resolveHarnessSHA() = %q, want %q", got, harnessSHAUnknown)
			}
		})
	}
}

// TestGitShortSHA_RealGit is a light integration check that the real
// gitShortSHA implementation (used as resolveHarnessSHA's runGit argument in
// production) still returns a non-empty short SHA when actually run inside
// this repo's git checkout.
func TestGitShortSHA_RealGit(t *testing.T) {
	sha, err := gitShortSHA(context.Background())
	if err != nil {
		t.Skipf("gitShortSHA() error (expected outside a git checkout): %v", err)
	}
	if sha == "" {
		t.Error("gitShortSHA() returned an empty string with no error")
	}
}
