package main

import (
	"strings"
	"testing"

	"github.com/petersimmons1972/engram/internal/longmemeval"
)

// TestRunLogFormat_ErrorIncludesCause verifies that when runOne returns an
// error entry, the log message format string produced by runWorker would
// include the error cause — not just hypothesis_len=0.
//
// We test the format string directly since runWorker logs via log.Printf and
// the real worker requires a live server. The assertion is on the format
// string constant in the source; this test documents the contract so that
// any future change to the log line that drops the error field will fail CI.
func TestRunLogFormat_ErrorIncludesCause(t *testing.T) {
	// Build a synthetic error entry as runOne would return.
	entry := longmemeval.RunEntry{
		QuestionID: "q-001",
		Status:     "error",
		Error:      "recall: connection refused",
	}

	// The format string used in runWorker must contain %s for the error field.
	// We verify this by formatting the log line ourselves.
	msg := runEntryLogLine(entry)

	if !strings.Contains(msg, "status=error") {
		t.Errorf("log line missing status=error: %q", msg)
	}
	if !strings.Contains(msg, "recall: connection refused") {
		t.Errorf("log line missing error cause: %q", msg)
	}
}

// TestRunLogFormat_SuccessNoError verifies that successful entries do not
// spuriously include an error field in the log line.
func TestRunLogFormat_SuccessNoError(t *testing.T) {
	entry := longmemeval.RunEntry{
		QuestionID: "q-002",
		Hypothesis: "The answer is 42.",
		Status:     "done",
	}
	msg := runEntryLogLine(entry)
	if !strings.Contains(msg, "status=done") {
		t.Errorf("log line missing status=done: %q", msg)
	}
	if !strings.Contains(msg, "hypothesis_len=17") {
		t.Errorf("log line missing hypothesis_len: %q", msg)
	}
	// No error field should appear on success.
	if strings.Contains(msg, "error=") {
		t.Errorf("log line should not contain error= on success: %q", msg)
	}
}
