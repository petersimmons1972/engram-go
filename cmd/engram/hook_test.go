package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHookSocketPath(t *testing.T) {
	p := hookSocketPath()
	if !strings.HasSuffix(p, filepath.Join(".claude", ".engram-hook.sock")) {
		t.Fatalf("unexpected socket path: %s", p)
	}
}

func TestClaudeProjectSlug(t *testing.T) {
	cases := map[string]string{
		"/home/psimmons":  "-home-psimmons",
		"/home/alice":     "-home-alice",
		"/Users/bob":      "-Users-bob",
		"/home/psimmons/": "-home-psimmons", // trailing slash cleaned
	}
	for in, want := range cases {
		if got := claudeProjectSlug(in); got != want {
			t.Errorf("claudeProjectSlug(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestMemoryDir(t *testing.T) {
	// Computed from home path — no hardcoded slug.
	got := memoryDir("/home/alice")
	want := filepath.Join("/home/alice", ".claude", "projects", "-home-alice", "memory")
	if got != want {
		t.Fatalf("memoryDir(/home/alice) = %q, want %q", got, want)
	}

	// ENGRAM_MEMORY_DIR overrides the computed path.
	t.Setenv("ENGRAM_MEMORY_DIR", "/tmp/custom-mem")
	if got := memoryDir("/home/alice"); got != "/tmp/custom-mem" {
		t.Fatalf("ENGRAM_MEMORY_DIR override not honored: %q", got)
	}
}

func TestInferEngramProject(t *testing.T) {
	// In this repo (engram-go), the inferred project should be "engram".
	if got := inferEngramProject(); got != "engram" && got != "" {
		// "" is acceptable if git is unavailable in the sandbox; "engram" is the
		// expected mapping for the engram-go checkout.
		t.Fatalf("unexpected project mapping: %q", got)
	}
}

func TestRotateHookLog(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "daemon.log")

	// Small file → no rotation.
	os.WriteFile(path, []byte("small"), 0o644)
	rotateHookLog(path)
	if _, err := os.Stat(path + ".1"); err == nil {
		t.Fatal("small log should not rotate")
	}

	// Oversized file → rotates to .1.
	big := make([]byte, (5<<20)+1)
	os.WriteFile(path, big, 0o644)
	rotateHookLog(path)
	if _, err := os.Stat(path + ".1"); err != nil {
		t.Fatalf("oversized log should rotate to .1: %v", err)
	}
}
