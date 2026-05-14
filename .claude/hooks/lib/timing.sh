#!/usr/bin/env bash
# Source from a hook to record per-invocation wall time.
# Safe: failures are silently swallowed and never affect the host hook's exit code.
# Output: $HOOK_TIMING_LOG (default ~/.claude/hook-timings.tsv), append-only.
# Columns: iso_ts \t hook_name \t duration_ms \t exit_code \t pid

{
  _hook_timing_t0=$(date +%s%N 2>/dev/null) || _hook_timing_t0=
  _hook_timing_name=$(basename "${BASH_SOURCE[1]:-$0}" 2>/dev/null) || _hook_timing_name=unknown
  _hook_timing_log_file="${HOOK_TIMING_LOG:-$HOME/.claude/hook-timings.tsv}"
} 2>/dev/null

_hook_timing_log_exit() {
  local exit_code=$?
  {
    local t1
    t1=$(date +%s%N 2>/dev/null)
    local ms=-1
    if [[ -n "$_hook_timing_t0" && -n "$t1" ]]; then
      ms=$(( (t1 - _hook_timing_t0) / 1000000 ))
    fi
    local ts
    ts=$(date -u +%Y-%m-%dT%H:%M:%SZ 2>/dev/null)
    printf '%s\t%s\t%s\t%s\t%s\n' "${ts:-unknown}" "$_hook_timing_name" "$ms" "$exit_code" "$$" >> "$_hook_timing_log_file"
  } 2>/dev/null
  return "$exit_code"
}

trap _hook_timing_log_exit EXIT 2>/dev/null || true
