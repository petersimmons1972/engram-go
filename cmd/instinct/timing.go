// Package main — per-stage wall-clock instrumentation for Phase 1 of #396.
//
// Phase 1 is measurement-only: always-on, zero-overhead-on-happy-path,
// appends a single TSV row to ~/.claude/hook-timings-v2.tsv when the binary
// exits. No daemon, no session cache, no new flags.
//
// TSV schema (tab-separated, header row on first line):
//
//	iso_ts          hook_name            exec_start_ms auth_resolved_ms
//	mcp_connected_ms request_sent_ms response_received_ms exit_ms
//	exit_code pid
//
// All *_ms fields are millisecond offsets from exec_start_ms (== 0 for that
// column). exec_start_ms is the absolute epoch-ms of program start, so rows
// from different sources (bash hooks + this binary) can be joined on that.
//
// See docs/design/396-phase1-measurement.md for the full schema spec.
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// HookTimingV2File is the path to the per-stage timing log.
// Exported so tests can override via the HOOK_TIMING_V2_LOG env variable.
const HookTimingV2File = ".claude/hook-timings-v2.tsv"

// HookTimingV2Header is the TSV header line written once when the file is created.
const HookTimingV2Header = "iso_ts\thook_name\texec_start_ms\tauth_resolved_ms\tmcp_connected_ms\trequest_sent_ms\tresponse_received_ms\texit_ms\texit_code\tpid"

// stageTimes captures the absolute time.Time for each instrumentation point.
// Zero value means "not reached yet" — emitted as empty string in the TSV row.
type stageTimes struct {
	execStart        time.Time
	authResolved     time.Time
	mcpConnected     time.Time
	requestSent      time.Time
	responseReceived time.Time
	exitTime         time.Time
}

// newStageTimes initialises a stageTimes with execStart set to now.
func newStageTimes() *stageTimes {
	return &stageTimes{execStart: time.Now()}
}

// msOffset returns the millisecond delta from s.execStart to t,
// or an empty string if t is the zero value (stage not reached).
func (s *stageTimes) msOffset(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return strconv.FormatInt(t.Sub(s.execStart).Milliseconds(), 10)
}

// toTSVRow formats one row in the hook-timings-v2.tsv schema.
// hookName is "instinct" (the binary name); exitCode is the process exit code.
func (s *stageTimes) toTSVRow(hookName string, exitCode int) string {
	isoTs := s.execStart.UTC().Format(time.RFC3339)
	execStartMs := strconv.FormatInt(s.execStart.UnixMilli(), 10)
	pid := strconv.Itoa(os.Getpid())

	fields := []string{
		isoTs,
		hookName,
		execStartMs,
		s.msOffset(s.authResolved),
		s.msOffset(s.mcpConnected),
		s.msOffset(s.requestSent),
		s.msOffset(s.responseReceived),
		s.msOffset(s.exitTime),
		strconv.Itoa(exitCode),
		pid,
	}
	return strings.Join(fields, "\t")
}

// timingLogPath returns the path for the per-stage TSV log.
// Override via HOOK_TIMING_V2_LOG env variable (for tests).
func timingLogPath() string {
	if v := os.Getenv("HOOK_TIMING_V2_LOG"); v != "" {
		return v
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, HookTimingV2File)
}

// appendTimingRow appends one TSV row to the per-stage timing log.
// Creates the file with header if it does not exist. Failures are silently
// swallowed — timing instrumentation must never affect hook exit codes.
func appendTimingRow(row string) {
	path := timingLogPath()
	if path == "" {
		return
	}

	// Check whether file exists and needs a header.
	needsHeader := false
	if _, err := os.Stat(path); os.IsNotExist(err) {
		needsHeader = true
	}

	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	defer func() { _ = f.Close() }()

	if needsHeader {
		_, _ = fmt.Fprintln(f, HookTimingV2Header)
	}
	_, _ = fmt.Fprintln(f, row)
}
