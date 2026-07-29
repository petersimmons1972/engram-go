#!/usr/bin/env bash
# Shared content-key and allowance-consumption helpers for checkin-lint.
# Source this file; do not execute it directly.

checkin_lint_baseline_key() {
  local rule="$1" file="$2" content="$3"
  local hash_output content_hash

  hash_output="$(printf '%s' "$content" | sha1sum)"
  content_hash="${hash_output%% *}"
  printf '%s::%s::%s\n' "$rule" "$file" "$content_hash"
}

# Atomically claim one of the allowed occurrences for key.
# Returns 0 when claimed, 1 when all allowances are used, and 2 on I/O or
# locking failure. flock keeps the count-and-append transaction safe across
# finding() calls running in separate processes.
checkin_lint_claim_baseline() {
  local used_file="$1" key="$2" allowed="$3"
  local lock_fd used result grep_rc=0

  [[ "$allowed" -gt 0 ]] || return 1
  if [[ -z "$used_file" ]]; then
    echo "checkin-lint: baseline usage file is not configured" >&2
    return 2
  fi
  if ! exec {lock_fd}>"${used_file}.lock"; then
    echo "checkin-lint: failed to open baseline usage lock" >&2
    return 2
  fi
  if ! flock -x "$lock_fd"; then
    echo "checkin-lint: failed to lock baseline usage state" >&2
    exec {lock_fd}>&-
    return 2
  fi
  used="$(grep -Fxc -- "$key" "$used_file")" || grep_rc=$?
  if [[ "$grep_rc" -gt 1 ]]; then
    echo "checkin-lint: failed to read baseline usage state" >&2
    exec {lock_fd}>&-
    return 2
  fi

  if [[ "$used" -lt "$allowed" ]]; then
    if ! printf '%s\n' "$key" >> "$used_file"; then
      echo "checkin-lint: failed to record baseline usage" >&2
      exec {lock_fd}>&-
      return 2
    fi
    result=0
  else
    result=1
  fi

  exec {lock_fd}>&-
  return "$result"
}

export -f checkin_lint_baseline_key checkin_lint_claim_baseline
