#!/usr/bin/env bash
# timing-v2.sh — per-stage hook timing library for engram hooks (Phase 1, #396).
#
# Source this file at the TOP of any engram hook script:
#   . "$HOME/.claude/hooks/lib/timing-v2.sh" 2>/dev/null || true
#
# Then call these functions at each major checkpoint:
#   timing_mark_auth_resolved
#   timing_mark_mcp_connected
#   timing_mark_request_sent
#   timing_mark_response_received
#
# On EXIT the hook emits one TSV row to HOOK_TIMING_V2_LOG
# (default: ~/.claude/hook-timings-v2.tsv).
#
# TSV schema (tab-separated, matches Go timing.go):
#   iso_ts  hook_name  exec_start_ms  auth_resolved_ms  mcp_connected_ms
#   request_sent_ms  response_received_ms  exit_ms  exit_code  pid
#
# All *_ms fields are millisecond offsets from exec_start_ms (== 0 for that
# column). exec_start_ms is the absolute epoch-ms of process start.
# Unreached stages emit an empty field, not zero, so consumers can
# distinguish "not reached" from "reached at time 0".
#
# Safe: all code is wrapped in { } 2>/dev/null so a buggy bash version or
# missing `date` never propagates an error into the sourcing hook.
#
# Reference: docs/design/396-phase1-measurement.md

{
  # nanosecond-precision monotonic start time
  _tv2_t0_ns=$(date +%s%N 2>/dev/null) || _tv2_t0_ns=
  _tv2_t0_ms=$(( ${_tv2_t0_ns:-0} / 1000000 ))

  _tv2_hook=$(basename "${BASH_SOURCE[1]:-${BASH_SOURCE[0]:-$0}}" 2>/dev/null) || _tv2_hook=unknown
  _tv2_log="${HOOK_TIMING_V2_LOG:-$HOME/.claude/hook-timings-v2.tsv}"

  # Stage timestamps (ns); empty = not reached
  _tv2_auth_ns=
  _tv2_mcp_ns=
  _tv2_req_ns=
  _tv2_resp_ns=
} 2>/dev/null

# ── Stage mark helpers ─────────────────────────────────────────────────────────

timing_mark_auth_resolved()     { _tv2_auth_ns=$(date +%s%N 2>/dev/null) || true; }
timing_mark_mcp_connected()     { _tv2_mcp_ns=$(date +%s%N 2>/dev/null)  || true; }
timing_mark_request_sent()      { _tv2_req_ns=$(date +%s%N 2>/dev/null)   || true; }
timing_mark_response_received() { _tv2_resp_ns=$(date +%s%N 2>/dev/null)  || true; }

# ── Internal helpers ───────────────────────────────────────────────────────────

# _tv2_ms_offset <ns_var_value>
# Prints the millisecond offset from _tv2_t0_ns, or empty if arg is blank.
_tv2_ms_offset() {
  local ns="$1"
  if [[ -z "$ns" || -z "$_tv2_t0_ns" ]]; then
    printf ''
    return
  fi
  printf '%s' "$(( (ns - _tv2_t0_ns) / 1000000 ))"
}

# _tv2_write_header <path>
# Writes the TSV header to <path>. Called only when the file does not yet exist.
_tv2_write_header() {
  printf '%s\n' \
    "iso_ts	hook_name	exec_start_ms	auth_resolved_ms	mcp_connected_ms	request_sent_ms	response_received_ms	exit_ms	exit_code	pid" \
    >> "$1" 2>/dev/null || true
}

# ── EXIT trap ─────────────────────────────────────────────────────────────────

_tv2_on_exit() {
  local _exit_code=$?
  {
    local _t_exit_ns
    _t_exit_ns=$(date +%s%N 2>/dev/null) || _t_exit_ns=

    local _iso_ts
    _iso_ts=$(date -u +%Y-%m-%dT%H:%M:%SZ 2>/dev/null) || _iso_ts=unknown

    local _auth_ms _mcp_ms _req_ms _resp_ms _exit_ms
    _auth_ms=$(_tv2_ms_offset "${_tv2_auth_ns:-}")
    _mcp_ms=$(_tv2_ms_offset "${_tv2_mcp_ns:-}")
    _req_ms=$(_tv2_ms_offset "${_tv2_req_ns:-}")
    _resp_ms=$(_tv2_ms_offset "${_tv2_resp_ns:-}")
    _exit_ms=$(_tv2_ms_offset "${_t_exit_ns:-}")

    # Create file + header if needed
    if [[ ! -f "$_tv2_log" ]]; then
      _tv2_write_header "$_tv2_log"
    fi

    printf '%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n' \
      "$_iso_ts" \
      "$_tv2_hook" \
      "${_tv2_t0_ms:-}" \
      "$_auth_ms" \
      "$_mcp_ms" \
      "$_req_ms" \
      "$_resp_ms" \
      "$_exit_ms" \
      "$_exit_code" \
      "$$" \
      >> "$_tv2_log" 2>/dev/null || true
  } 2>/dev/null
  return "$_exit_code"
}

trap _tv2_on_exit EXIT 2>/dev/null || true
