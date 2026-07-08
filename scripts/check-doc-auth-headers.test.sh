#!/usr/bin/env bash
# TDD tests for scripts/check-doc-auth-headers.sh (issue #1340)

set -u

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
CHECKER="${SCRIPT_DIR}/check-doc-auth-headers.sh"

PASS=0
FAIL=0

pass() { echo "PASS: $1"; PASS=$((PASS+1)); }
fail() { echo "FAIL: $1"; FAIL=$((FAIL+1)); }

assert_exit() {
  local desc="$1" expected="$2" actual="$3"
  if [[ "$expected" -eq "$actual" ]]; then
    pass "$desc (exit=$actual)"
  else
    fail "$desc (expected exit=$expected, got=$actual)"
  fi
}

make_fixture_dir() {
  mktemp -d
}

run_checker() {
  local target="$1"
  (cd "$SCRIPT_DIR/.." && bash "$CHECKER" "$target")
}

# Test 1: valid curl header with quoted ${VAR} placeholder passes
R="$(make_fixture_dir)"
cat >"$R/valid.md" <<'EOF'
```bash
curl -H "Authorization: Bearer ${ENGRAM_API_KEY}" http://localhost:8788/metrics
```
EOF
out="$(run_checker "$R" 2>&1)"; rc=$?
assert_exit "valid curl header passes" 0 "$rc"
rm -rf "$R"

# Test 2: valid xh header with quoted ${VAR} placeholder passes
R="$(make_fixture_dir)"
cat >"$R/valid-xh.md" <<'EOF'
```bash
xh POST http://localhost:8788/mcp \
  "Authorization: Bearer ${ENGRAM_API_KEY}" \
  Content-Type:application/json
```
EOF
out="$(run_checker "$R" 2>&1)"; rc=$?
assert_exit "valid xh header passes" 0 "$rc"
rm -rf "$R"

# Test 3: redacted bearer token is rejected
R="$(make_fixture_dir)"
cat >"$R/redacted.md" <<'EOF'
```bash
curl -H "Authorization: Bearer ***" http://localhost:8788/metrics
```
EOF
out="$(run_checker "$R" 2>&1)"; rc=$?
assert_exit "redacted bearer token is rejected" 1 "$rc"
if echo "$out" | grep -q "redacted bearer token"; then
  pass "redacted token failure explains the problem"
else
  fail "redacted token failure should explain the problem"
fi
rm -rf "$R"

# Test 4: missing closing quote is rejected
R="$(make_fixture_dir)"
cat >"$R/unbalanced.md" <<'EOF'
```bash
curl \
  --header "Authorization: Bearer ${ENGRAM_API_KEY} \
  http://localhost:8788/setup-token
```
EOF
out="$(run_checker "$R" 2>&1)"; rc=$?
assert_exit "unterminated auth header is rejected" 1 "$rc"
if echo "$out" | grep -q "malformed Authorization header snippet"; then
  pass "unterminated header failure explains the problem"
else
  fail "unterminated header failure should explain the problem"
fi
rm -rf "$R"

# Test 5: bare $VAR without braces is rejected to keep snippets uniform
R="$(make_fixture_dir)"
cat >"$R/bare-var.md" <<'EOF'
```bash
curl -H "Authorization: Bearer $ENGRAM_API_KEY" http://localhost:8788/metrics
```
EOF
out="$(run_checker "$R" 2>&1)"; rc=$?
assert_exit "bare shell variable is rejected" 1 "$rc"
if echo "$out" | grep -q 'use "Authorization: Bearer ${VAR}"'; then
  pass "bare variable failure explains the preferred placeholder"
else
  fail "bare variable failure should explain the preferred placeholder"
fi
rm -rf "$R"

echo
echo "Results: $PASS passed, $FAIL failed"
[[ "$FAIL" -eq 0 ]]
