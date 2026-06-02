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
