#!/usr/bin/env bash
# checkin-lint_smoke_test.sh — TDD smoke tests for the check-in lint infra.
# Tests defined in issue #1095 / plan codex-1095-checkin-lint-infra.
# Run from repo root: bash bin/checkin-lint_smoke_test.sh
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
PASS=0
FAIL=0
RED='\033[0;31m'; GRN='\033[0;32m'; BOLD='\033[1m'; RST='\033[0m'

pass() { echo -e "${GRN}PASS${RST}  $1"; ((PASS++)) || true; }
fail() { echo -e "${RED}FAIL${RST}  $1"; ((FAIL++)) || true; }

echo ""
echo -e "${BOLD}checkin-lint smoke tests${RST}"
echo "────────────────────────────────────────"

# ── test_entrypoint_exists_and_runs ──────────────────────────────────────────
# Verify bin/checkin-lint.sh exists, is executable, and exits without
# "No such file or directory" when invoked.
echo ""
echo "test_entrypoint_exists_and_runs"
if [[ ! -f "${REPO_ROOT}/bin/checkin-lint.sh" ]]; then
  fail "bin/checkin-lint.sh does not exist"
elif [[ ! -x "${REPO_ROOT}/bin/checkin-lint.sh" ]]; then
  fail "bin/checkin-lint.sh exists but is not executable"
else
  # Run it; we only care that it doesn't produce "No such file or directory"
  output="$(cd "${REPO_ROOT}" && bash bin/checkin-lint.sh 2>&1 || true)"
  if echo "$output" | grep -q 'No such file or directory'; then
    fail "checkin-lint.sh ran but emitted 'No such file or directory'"
    echo "  output: $output"
  else
    pass "bin/checkin-lint.sh exists, is executable, and runs without missing-file errors"
  fi
fi

# ── test_baseline_has_no_worktree_paths ──────────────────────────────────────
# The committed baseline must contain ZERO paths matching \.worktrees/ or
# \.claude/worktrees/ — the hard gate from the #1093 incident report.
echo ""
echo "test_baseline_has_no_worktree_paths"
BASELINE="${REPO_ROOT}/bin/checkin-lint.baseline"
if [[ ! -f "${BASELINE}" ]]; then
  fail "bin/checkin-lint.baseline does not exist"
else
  bad_count=$(grep -cE '\.worktrees/|\.claude/worktrees/' "${BASELINE}" 2>/dev/null || true)
  if [[ "${bad_count}" -gt 0 ]]; then
    fail "baseline contains ${bad_count} ephemeral worktree path(s) — hard gate violation"
    grep -E '\.worktrees/|\.claude/worktrees/' "${BASELINE}" | head -5 | sed 's/^/  /'
  else
    pass "bin/checkin-lint.baseline contains zero worktree paths (grep exit 1 = clean)"
  fi
fi

# ── test_clean_tree_passes ────────────────────────────────────────────────────
# Running the linter on the current tree (which should match what a clean
# checkout of this PR looks like) must exit 0 — no net-new findings beyond the
# regenerated baseline.
echo ""
echo "test_clean_tree_passes"
lint_output="$(cd "${REPO_ROOT}" && bash bin/checkin-lint.sh 2>&1)" || lint_exit=$?
lint_exit="${lint_exit:-0}"
if [[ "${lint_exit}" -eq 0 ]]; then
  pass "linter exits 0 on current tree — 0 net-new findings"
else
  # Extract any non-baselined findings for the failure message
  net_new=$(echo "$lint_output" | sed 's/\x1b\[[0-9;]*m//g' | grep '^FINDING' || true)
  fail "linter exited ${lint_exit} — net-new findings detected"
  echo "$net_new" | head -10 | sed 's/^/  /'
fi

# ── Summary ───────────────────────────────────────────────────────────────────
echo ""
echo "────────────────────────────────────────"
total=$((PASS + FAIL))
if [[ "${FAIL}" -eq 0 ]]; then
  echo -e "${GRN}${BOLD}All ${total} smoke tests passed.${RST}"
  exit 0
else
  echo -e "${RED}${BOLD}${FAIL}/${total} smoke test(s) FAILED.${RST}"
  exit 1
fi
