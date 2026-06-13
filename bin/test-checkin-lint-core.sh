#!/usr/bin/env bash
# test-checkin-lint-core.sh — regression tests for checkin-lint-core.sh
#
# Usage: bash ~/bin/test-checkin-lint-core.sh
# Exit:  0 = all pass  1 = failures

set -euo pipefail

PASS=0; FAIL=0
CORE="$(cd "$(dirname "$0")" && pwd)/checkin-lint-core.sh"
FIXTURES="$(cd "$(dirname "$0")" && pwd)/test-fixtures/checkin-lint-core"
TMPDIR_BASE="$(mktemp -d)"
trap 'rm -rf "$TMPDIR_BASE"' EXIT

GREEN='\033[0;32m'; RED='\033[0;31m'; BOLD='\033[1m'; RST='\033[0m'

pass() { echo -e "${GREEN}PASS${RST}: $*"; ((PASS++)) || true; }
fail() { echo -e "${RED}FAIL${RST}: $*"; ((FAIL++)) || true; }

# run_check <test-name> <fixture-file> <check-fn> <expected-findings>
run_check() {
  local name="$1" fixture="$2" check_fn="$3" expected="$4"
  local tmpdir="${TMPDIR_BASE}/${name}"
  mkdir -p "$tmpdir"
  [[ -n "$fixture" ]] && cp "$FIXTURES/$fixture" "$tmpdir/"
  local actual
  actual=$(cd "$tmpdir" && \
    git init -q 2>/dev/null && \
    git remote add origin "https://github.com/petersimmons1972/test.git" 2>/dev/null || true
    FINDINGS=0
    EXPECTED_REMOTE="petersimmons1972/test"
    CHECKIN_K8S=0
    export EXPECTED_REMOTE CHECKIN_K8S
    source "$CORE" 2>/dev/null
    "$check_fn" >/dev/null 2>&1 || true
    echo "$FINDINGS"
  )
  if [[ "$actual" -eq "$expected" ]]; then
    pass "[$name] findings=$actual (expected $expected)"
  else
    fail "[$name] findings=$actual (expected $expected)"
  fi
}

# ── C.home-literal ────────────────────────────────────────────────────────────
run_check "home-literal-bad"  "bad-home-literal.yaml"  "_check_home_literal"  1
run_check "home-literal-good" "good-home-literal.yaml" "_check_home_literal"  0

# ── C.version-pinned-path ─────────────────────────────────────────────────────
run_check "version-pin-bad"  "bad-version-pin.sh"  "_check_version_pinned_path"  1

# ── D.exit-zero-wrapper ───────────────────────────────────────────────────────
run_check "exit-zero-bad"  "bad-exit-zero.sh"  "_check_exit_zero_wrapper"  1

# ── G.latest-image ────────────────────────────────────────────────────────────
# Must set CHECKIN_K8S=1 for K8s checks
run_k8s_check() {
  local name="$1" fixture="$2" check_fn="$3" expected="$4"
  local tmpdir="${TMPDIR_BASE}/${name}"
  mkdir -p "$tmpdir"
  [[ -n "$fixture" ]] && cp "$FIXTURES/$fixture" "$tmpdir/"
  local actual
  actual=$(cd "$tmpdir" && \
    git init -q 2>/dev/null && \
    git remote add origin "https://github.com/petersimmons1972/test.git" 2>/dev/null || true
    FINDINGS=0
    EXPECTED_REMOTE="petersimmons1972/test"
    CHECKIN_K8S=1
    export EXPECTED_REMOTE CHECKIN_K8S
    source "$CORE" 2>/dev/null
    "$check_fn" >/dev/null 2>&1 || true
    echo "$FINDINGS"
  )
  if [[ "$actual" -eq "$expected" ]]; then
    pass "[$name] findings=$actual (expected $expected)"
  else
    fail "[$name] findings=$actual (expected $expected)"
  fi
}

run_k8s_check "latest-image-bad"       "bad-latest-image.yaml"       "_check_latest_image"  1
run_k8s_check "hardcoded-ip-bad"       "bad-networkpolicy-ip.yaml"   "_check_hardcoded_ip"  1

# ── _do_baseline_audit ────────────────────────────────────────────────────────
test_audit_baseline_stale() {
  local tmp_baseline tmp_keys
  tmp_baseline="$(mktemp)"; tmp_keys="$(mktemp)"
  # Put a stale key in the baseline (no matching current finding)
  echo "C.home-literal::./nonexistent.yaml::99" > "$tmp_baseline"
  # tmp_keys is empty — no current findings match
  local out
  out="$(CHECKIN_LINT_BASELINE="$tmp_baseline" _ALL_FINDING_KEYS_FILE="$tmp_keys" \
         bash -c "source \"$CORE\" 2>/dev/null; _do_baseline_audit" 2>&1)"
  rm -f "$tmp_baseline" "$tmp_keys"
  if echo "$out" | grep -q 'STALE'; then
    pass "[audit-baseline-stale] stale entry correctly flagged"
  else
    fail "[audit-baseline-stale] expected STALE in output; got: $out"
  fi
}

test_audit_baseline_active() {
  local tmp_baseline tmp_keys
  tmp_baseline="$(mktemp)"; tmp_keys="$(mktemp)"
  # Same key in both baseline and keys_file — active, not stale
  echo "C.home-literal::./foo.yaml::5" > "$tmp_baseline"
  echo "C.home-literal::./foo.yaml::5" > "$tmp_keys"
  local out
  out="$(CHECKIN_LINT_BASELINE="$tmp_baseline" _ALL_FINDING_KEYS_FILE="$tmp_keys" \
         bash -c "source \"$CORE\" 2>/dev/null; _do_baseline_audit" 2>&1)"
  rm -f "$tmp_baseline" "$tmp_keys"
  if echo "$out" | grep -q 'STALE'; then
    fail "[audit-baseline-active] active entry incorrectly flagged as stale; got: $out"
  else
    pass "[audit-baseline-active] active entry correctly not flagged"
  fi
}

test_audit_baseline_stale
test_audit_baseline_active

# ── Summary ───────────────────────────────────────────────────────────────────
echo ""
if [[ $FAIL -eq 0 ]]; then
  echo -e "${GREEN}${BOLD}✓ All ${PASS} tests passed${RST}"
  exit 0
else
  echo -e "${RED}${BOLD}✗ ${FAIL} test(s) failed (${PASS} passed)${RST}"
  exit 1
fi
