package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestStageTimes_ToTSVRow verifies that toTSVRow emits the correct column
// count, correct hook name, and correct millisecond offsets from execStart.
func TestStageTimes_ToTSVRow(t *testing.T) {
	base := time.Date(2026, 5, 20, 12, 0, 0, 0, time.UTC)
	s := &stageTimes{
		execStart:        base,
		authResolved:     base.Add(50 * time.Millisecond),
		mcpConnected:     base.Add(120 * time.Millisecond),
		requestSent:      base.Add(130 * time.Millisecond),
		responseReceived: base.Add(310 * time.Millisecond),
		exitTime:         base.Add(315 * time.Millisecond),
	}

	row := s.toTSVRow("instinct", 0)
	fields := strings.Split(row, "\t")

	// Schema: iso_ts hook_name exec_start_ms auth_resolved_ms mcp_connected_ms
	//         request_sent_ms response_received_ms exit_ms exit_code pid
	const wantColumns = 10
	if len(fields) != wantColumns {
		t.Fatalf("toTSVRow: got %d columns, want %d\nrow: %q", len(fields), wantColumns, row)
	}

	t.Run("hook_name", func(t *testing.T) {
		if fields[1] != "instinct" {
			t.Errorf("hook_name = %q, want %q", fields[1], "instinct")
		}
	})

	t.Run("auth_resolved_ms", func(t *testing.T) {
		if fields[3] != "50" {
			t.Errorf("auth_resolved_ms = %q, want \"50\"", fields[3])
		}
	})

	t.Run("mcp_connected_ms", func(t *testing.T) {
		if fields[4] != "120" {
			t.Errorf("mcp_connected_ms = %q, want \"120\"", fields[4])
		}
	})

	t.Run("request_sent_ms", func(t *testing.T) {
		if fields[5] != "130" {
			t.Errorf("request_sent_ms = %q, want \"130\"", fields[5])
		}
	})

	t.Run("response_received_ms", func(t *testing.T) {
		if fields[6] != "310" {
			t.Errorf("response_received_ms = %q, want \"310\"", fields[6])
		}
	})

	t.Run("exit_ms", func(t *testing.T) {
		if fields[7] != "315" {
			t.Errorf("exit_ms = %q, want \"315\"", fields[7])
		}
	})

	t.Run("exit_code", func(t *testing.T) {
		if fields[8] != "0" {
			t.Errorf("exit_code = %q, want \"0\"", fields[8])
		}
	})
}

// TestStageTimes_ToTSVRow_PartialStages verifies that unreached stages
// (zero time.Time) emit empty strings rather than garbage offsets.
func TestStageTimes_ToTSVRow_PartialStages(t *testing.T) {
	base := time.Date(2026, 5, 20, 12, 0, 0, 0, time.UTC)
	s := &stageTimes{
		execStart:    base,
		authResolved: base.Add(40 * time.Millisecond),
		// mcpConnected, requestSent, responseReceived, exitTime all zero
	}

	row := s.toTSVRow("instinct", 1)
	fields := strings.Split(row, "\t")

	if len(fields) != 10 {
		t.Fatalf("partial row: got %d columns, want 10\nrow: %q", len(fields), row)
	}

	// auth_resolved should be present
	if fields[3] != "40" {
		t.Errorf("auth_resolved_ms = %q, want \"40\"", fields[3])
	}

	// mcp_connected through exit should be empty
	for i, col := range []int{4, 5, 6, 7} {
		if fields[col] != "" {
			t.Errorf("unreached stage col[%d] = %q, want empty (stage index %d)", col, fields[col], i)
		}
	}

	// exit_code should be 1
	if fields[8] != "1" {
		t.Errorf("exit_code = %q, want \"1\"", fields[8])
	}
}

// TestAppendTimingRow_CreatesFileWithHeader verifies that appendTimingRow
// creates the file with the correct header on first write, then appends
// subsequent rows without re-writing the header.
func TestAppendTimingRow_CreatesFileWithHeader(t *testing.T) {
	tmp := t.TempDir()
	logPath := filepath.Join(tmp, "hook-timings-v2.tsv")
	t.Setenv("HOOK_TIMING_V2_LOG", logPath)

	base := time.Date(2026, 5, 20, 12, 0, 0, 0, time.UTC)
	s := &stageTimes{
		execStart: base,
		exitTime:  base.Add(200 * time.Millisecond),
	}

	// First write — file must not exist yet.
	appendTimingRow(s.toTSVRow("instinct", 0))

	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("ReadFile after first write: %v", err)
	}
	lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("after first write: got %d lines, want 2 (header + row)\ncontent: %q", len(lines), string(data))
	}
	if lines[0] != HookTimingV2Header {
		t.Errorf("line[0] = %q, want header %q", lines[0], HookTimingV2Header)
	}

	// Second write — no additional header.
	s2 := &stageTimes{
		execStart: base.Add(time.Minute),
		exitTime:  base.Add(time.Minute + 100*time.Millisecond),
	}
	appendTimingRow(s2.toTSVRow("instinct", 0))

	data2, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("ReadFile after second write: %v", err)
	}
	lines2 := strings.Split(strings.TrimRight(string(data2), "\n"), "\n")
	if len(lines2) != 3 {
		t.Fatalf("after second write: got %d lines, want 3 (header + 2 rows)\ncontent: %q", len(lines2), string(data2))
	}
	// Confirm header still appears exactly once.
	headerCount := 0
	for _, l := range lines2 {
		if l == HookTimingV2Header {
			headerCount++
		}
	}
	if headerCount != 1 {
		t.Errorf("header appeared %d times, want exactly 1", headerCount)
	}
}

// TestTimingLogPath_EnvOverride verifies that HOOK_TIMING_V2_LOG overrides
// the default path, enabling test isolation.
func TestTimingLogPath_EnvOverride(t *testing.T) {
	want := "/tmp/test-hook-timings.tsv"
	t.Setenv("HOOK_TIMING_V2_LOG", want)
	got := timingLogPath()
	if got != want {
		t.Errorf("timingLogPath() = %q, want %q", got, want)
	}
}
