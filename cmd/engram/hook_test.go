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

func TestHookBaseURL(t *testing.T) {
	// Default: localhost on the configured port.
	t.Setenv("ENGRAM_TEST_PORT", "")
	t.Setenv("ENGRAM_URL", "")
	t.Setenv("ENGRAM_BASE_URL", "")
	if got := hookBaseURL(); !strings.HasPrefix(got, "http://127.0.0.1:") {
		t.Fatalf("default base URL should be loopback, got %q", got)
	}

	// ENGRAM_BASE_URL is honoured (trailing slash trimmed).
	t.Setenv("ENGRAM_BASE_URL", "https://engram.petersimmons.com/")
	if got := hookBaseURL(); got != "https://engram.petersimmons.com" {
		t.Fatalf("ENGRAM_BASE_URL not honoured, got %q", got)
	}

	// ENGRAM_URL takes precedence over ENGRAM_BASE_URL.
	t.Setenv("ENGRAM_URL", "https://primary.example.com")
	if got := hookBaseURL(); got != "https://primary.example.com" {
		t.Fatalf("ENGRAM_URL should win over ENGRAM_BASE_URL, got %q", got)
	}

	// ENGRAM_TEST_PORT overrides everything (test harness escape hatch).
	t.Setenv("ENGRAM_TEST_PORT", "9999")
	if got := hookBaseURL(); got != "http://127.0.0.1:9999" {
		t.Fatalf("ENGRAM_TEST_PORT should take top precedence, got %q", got)
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
