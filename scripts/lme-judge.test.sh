#!/usr/bin/env bash
# Test suite for scripts/lme-judge.sh
# Usage: bash scripts/lme-judge.test.sh
#
# These checks validate invocation handling only. Actual scoring still happens in
# the Go binary and is exercised by go test in cmd/longmemeval.

set -u

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
SCRIPT="$SCRIPT_DIR/lme-judge.sh"

PASS=0
FAIL=0

assert_exit() {
  local desc="$1" expected="$2" actual="$3"
  if [ "$expected" -eq "$actual" ]; then
    echo "✓ PASS: $desc (exit=$actual)"
    PASS=$((PASS + 1))
  else
    echo "✗ FAIL: $desc (expected=$expected got=$actual)"
    FAIL=$((FAIL + 1))
  fi
}

assert_contains() {
  local desc="$1" needle="$2" haystack="$3"
  if echo "$haystack" | grep -qF "$needle"; then
    echo "✓ PASS: $desc"
    PASS=$((PASS + 1))
  else
    echo "✗ FAIL: $desc (missing '$needle')"
    FAIL=$((FAIL + 1))
  fi
}

out=$(bash "$SCRIPT" --help 2>&1); rc=$?
assert_exit "help exits cleanly" 0 "$rc"
assert_contains "help documents judges" " --judge <name>" "$out"

out=$(bash "$SCRIPT" 2>&1); rc=$?
assert_exit "missing arguments fail" 2 "$rc"
assert_contains "missing run arg explains usage" "--run is required" "$out"

tmp=$(mktemp -d)
run_dir="$tmp/run"
mkdir -p "$run_dir"
out=$(bash "$SCRIPT" --run "$run_dir" 2>&1); rc=$?
assert_exit "judge required without --bundle" 2 "$rc"
assert_contains "judge required message" "required when --bundle is not set" "$out"

rm -rf "$tmp"

echo
echo "─── ${PASS} passed, ${FAIL} failed ───"
[ "$FAIL" -eq 0 ]
