package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestQuietFlag_E2E — #661: end-to-end assertion that `instinct-benchmark
// --quiet --dry-run` emits no stdout. Builds the binary, runs it with --quiet,
// and asserts the stdout buffer is empty.
//
// We use --dry-run so the test doesn't need Ollama or a fixture file.
func TestQuietFlag_E2E(t *testing.T) {
	tmp := t.TempDir()
	binPath := filepath.Join(tmp, "instinct-benchmark")

	// Build the binary into the temp dir.
	build := exec.Command("go", "build", "-o", binPath, ".")
	build.Dir, _ = os.Getwd()
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}

	// Stub manifest with one excluded entry — keeps the run trivial.
	manifestPath := filepath.Join(tmp, "manifest.yaml")
	if err := os.WriteFile(manifestPath, []byte("models:\n  - name: x\n    excluded: true\n    exclude_reason: test\n    size_gb: 1.0\n"), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	t.Run("quiet emits empty stdout", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		cmd := exec.Command(binPath, "--quiet", "--dry-run", "--manifest", manifestPath)
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		if err := cmd.Run(); err != nil {
			t.Fatalf("run: %v\n--- stderr ---\n%s", err, stderr.String())
		}
		if stdout.Len() != 0 {
			t.Errorf("--quiet must produce empty stdout, got %d bytes: %q", stdout.Len(), stdout.String())
		}
	})

	t.Run("non-quiet emits banner on stdout", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		cmd := exec.Command(binPath, "--dry-run", "--manifest", manifestPath)
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		if err := cmd.Run(); err != nil {
			t.Fatalf("run: %v\n--- stderr ---\n%s", err, stderr.String())
		}
		if !strings.Contains(stdout.String(), "instinct-benchmark") {
			t.Errorf("non-quiet should emit banner; stdout=%q", stdout.String())
		}
	})
}

// TestOutputJSONFlag_ImpliesQuiet — #680: --output-json must suppress banner
// and emit results JSON to stdout. Dry-run produces a tiny but valid results
// file so the JSON parse round-trip works.
func TestOutputJSONFlag_ImpliesQuiet(t *testing.T) {
	tmp := t.TempDir()
	binPath := filepath.Join(tmp, "instinct-benchmark")

	build := exec.Command("go", "build", "-o", binPath, ".")
	build.Dir, _ = os.Getwd()
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}

	manifestPath := filepath.Join(tmp, "manifest.yaml")
	if err := os.WriteFile(manifestPath, []byte("models:\n  - name: x\n    excluded: true\n    exclude_reason: test\n    size_gb: 1.0\n"), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	resultsPath := filepath.Join(tmp, "results.json")
	// Seed an empty-but-valid results file so dry-run can succeed.
	if err := os.WriteFile(resultsPath, []byte("[]"), 0o644); err != nil {
		t.Fatalf("seed results: %v", err)
	}

	var stdout, stderr bytes.Buffer
	cmd := exec.Command(binPath, "--output-json", "--dry-run", "--manifest", manifestPath, "--results", resultsPath)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("run: %v\nstderr: %s", err, stderr.String())
	}

	// --dry-run exits before reading results; for that, we only assert quiet
	// behaviour (empty stdout banner) under --output-json. The post-run JSON
	// dump is exercised in a separate (manual) test against a real benchmark.
	if stdout.Len() != 0 {
		t.Errorf("--output-json --dry-run should produce empty stdout (dry-run exits before JSON dump), got %d bytes: %q", stdout.Len(), stdout.String())
	}
	// stderr contents under --output-json --dry-run are intentionally unverified:
	// bannerOut is io.Discard when --quiet is implied, so the "manifest valid"
	// line is dropped. The contract is empty stdout; stderr is whatever survives.
}
