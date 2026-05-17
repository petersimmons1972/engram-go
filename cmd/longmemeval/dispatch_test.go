package main

import (
	"bytes"
	"io"
	"strings"
	"testing"
)

// TestDispatch_Help — #662: `longmemeval help` must exit 0 and print usage.
// Currently the binary prints `unknown subcommand "help"` to stderr and exits 1.
func TestDispatch_Help(t *testing.T) {
	var stdout, stderr bytes.Buffer
	exit := dispatch([]string{"longmemeval", "help"}, &stdout, &stderr)

	if exit != 0 {
		t.Errorf("`longmemeval help` exit = %d, want 0", exit)
	}
	out := stdout.String()
	if !strings.Contains(out, "Usage:") {
		t.Errorf("help output missing 'Usage:' header\n--- stdout ---\n%s\n--- stderr ---\n%s",
			out, stderr.String())
	}
	for _, sub := range []string{"ingest", "run", "score", "all"} {
		if !strings.Contains(out, sub) {
			t.Errorf("help output missing subcommand %q in listing", sub)
		}
	}
	if strings.Contains(stderr.String(), "unknown subcommand") {
		t.Errorf("help should NOT report itself as an unknown subcommand: %s", stderr.String())
	}
}

// TestDispatch_NoArgs — invoking with no subcommand should print usage to stderr
// and exit non-zero (matches existing behaviour for unknown subcommands).
func TestDispatch_NoArgs(t *testing.T) {
	var stdout, stderr bytes.Buffer
	exit := dispatch([]string{"longmemeval"}, &stdout, &stderr)

	if exit == 0 {
		t.Errorf("`longmemeval` with no subcommand must exit non-zero, got 0")
	}
	if !strings.Contains(stderr.String()+stdout.String(), "Usage") {
		t.Errorf("no-args invocation must print usage somewhere; stderr=%q stdout=%q",
			stderr.String(), stdout.String())
	}
}

// TestDispatch_UnknownSubcommand — preserves existing error-on-unknown behaviour.
func TestDispatch_UnknownSubcommand(t *testing.T) {
	var stdout, stderr bytes.Buffer
	exit := dispatch([]string{"longmemeval", "frobnicate"}, &stdout, &stderr)

	if exit == 0 {
		t.Error("unknown subcommand must exit non-zero")
	}
	if !strings.Contains(stderr.String(), "unknown subcommand") {
		t.Errorf("expected 'unknown subcommand' in stderr, got %q", stderr.String())
	}
}

// silenceWriter discards writes — used as a placeholder for tests that don't
// inspect output but need to satisfy a writer parameter.
type silenceWriter struct{}

func (silenceWriter) Write(b []byte) (int, error) { return len(b), nil }

var _ io.Writer = silenceWriter{}
