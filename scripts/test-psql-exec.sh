#!/usr/bin/env bash
# test-psql-exec.sh — TDD tests for psql-exec.sh (issue #646)
#
# Tests:
#   1. psql-exec.sh exists and is executable
#   2. psql-exec.sh uses the safe `docker cp` + `psql -f` pattern
#   3. psql-exec.sh does NOT use bare heredoc piping (stdin without -i)

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
TARGET="$SCRIPT_DIR/psql-exec.sh"

PASS=0
FAIL=0

pass() { echo "PASS: $1"; PASS=$((PASS+1)); }
fail() { echo "FAIL: $1"; FAIL=$((FAIL+1)); }

# Test 1: script exists and is executable
if [[ -x "$TARGET" ]]; then
  pass "psql-exec.sh exists and is executable"
else
  fail "psql-exec.sh does not exist or is not executable (expected: $TARGET)"
fi

# Test 2: uses docker cp (safe pattern)
if grep -q "docker cp" "$TARGET" 2>/dev/null; then
  pass "psql-exec.sh contains 'docker cp'"
else
  fail "psql-exec.sh is missing 'docker cp'"
fi

# Test 3: uses psql -f (safe pattern)
if grep -q "psql -" "$TARGET" 2>/dev/null && grep -q "\-f" "$TARGET" 2>/dev/null; then
  pass "psql-exec.sh contains 'psql -f' pattern"
else
  fail "psql-exec.sh does not use 'psql -f'"
fi

# Test 4: does NOT pipe stdin without -i (heredoc footgun)
# A bare `docker exec <container> psql` with a heredoc but no -i is the bad pattern.
# We check that if the script calls docker exec with psql, it either uses -f or -i.
if grep -E "docker exec [^|]*psql" "$TARGET" 2>/dev/null | grep -qv "\-f\|\-i"; then
  fail "psql-exec.sh has a bare 'docker exec ... psql' without -f or -i (heredoc footgun)"
else
  pass "psql-exec.sh does not contain bare heredoc-piping pattern"
fi

echo ""
echo "Results: $PASS passed, $FAIL failed"
if [[ $FAIL -gt 0 ]]; then
  exit 1
fi
