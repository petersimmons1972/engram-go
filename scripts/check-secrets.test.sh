#!/usr/bin/env bash
# Test suite for scripts/check-secrets.sh
# Usage: bash scripts/check-secrets.test.sh
#
# Strategy: each test creates a throwaway git repo, stages a file, invokes the
# guard, and asserts the exit code + message. No state in the host repo.

set -u

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
GUARD="${SCRIPT_DIR}/check-secrets.sh"

PASS=0
FAIL=0

assert_exit() {
    local desc="$1"; local expected="$2"; local actual="$3"
    if [ "$expected" -eq "$actual" ]; then
        echo "✓ PASS: $desc (exit=$actual)"
        PASS=$((PASS+1))
    else
        echo "✗ FAIL: $desc (expected exit=$expected, got=$actual)"
        FAIL=$((FAIL+1))
    fi
}

assert_contains() {
    local desc="$1"; local needle="$2"; local haystack="$3"
    if echo "$haystack" | grep -qF "$needle"; then
        echo "✓ PASS: $desc"
        PASS=$((PASS+1))
    else
        echo "✗ FAIL: $desc (did not find '$needle' in output)"
        echo "    output: $haystack"
        FAIL=$((FAIL+1))
    fi
}

make_repo() {
    local d; d=$(mktemp -d)
    git -C "$d" init -q -b main
    git -C "$d" config user.email t@t
    git -C "$d" config user.name t
    echo "$d"
}

# ─── Test 1: empty index passes ────────────────────────────────────────────────
R=$(make_repo)
out=$(cd "$R" && bash "$GUARD" 2>&1); rc=$?
assert_exit "empty index passes" 0 $rc
rm -rf "$R"

# ─── Test 2: staging .env is blocked ───────────────────────────────────────────
R=$(make_repo)
echo "POSTGRES_PASSWORD=abc123" > "$R/.env"
git -C "$R" add -f .env
out=$(cd "$R" && bash "$GUARD" 2>&1); rc=$?
assert_exit "staging .env is blocked" 1 $rc
assert_contains "blocks .env with explanatory message" ".env" "$out"
rm -rf "$R"

# ─── Test 3: .env.example is allowed ───────────────────────────────────────────
R=$(make_repo)
echo "POSTGRES_PASSWORD=change_me" > "$R/.env.example"
git -C "$R" add .env.example
out=$(cd "$R" && bash "$GUARD" 2>&1); rc=$?
assert_exit ".env.example is allowed" 0 $rc
rm -rf "$R"

# ─── Test 4: staging .env.bak.<ts> is blocked ──────────────────────────────────
R=$(make_repo)
echo "ENGRAM_API_KEY=stale" > "$R/.env.bak.1234"
git -C "$R" add -f .env.bak.1234
out=$(cd "$R" && bash "$GUARD" 2>&1); rc=$?
assert_exit "staging .env.bak.* is blocked" 1 $rc
rm -rf "$R"

# ─── Test 5: staging .env.machine-identity is blocked ──────────────────────────
R=$(make_repo)
echo "INFISICAL_CLIENT_SECRET=xyz" > "$R/.env.machine-identity"
git -C "$R" add -f .env.machine-identity
out=$(cd "$R" && bash "$GUARD" 2>&1); rc=$?
assert_exit "staging .env.machine-identity is blocked" 1 $rc
rm -rf "$R"

# ─── Test 6: clean non-secret content passes ───────────────────────────────────
R=$(make_repo)
echo "hello world" > "$R/README.md"
echo "package main" > "$R/main.go"
git -C "$R" add README.md main.go
out=$(cd "$R" && bash "$GUARD" 2>&1); rc=$?
assert_exit "clean files pass" 0 $rc
rm -rf "$R"

# ─── Test 7: 64-char hex KEY=VALUE in a regular file is blocked ────────────────
R=$(make_repo)
HEX=$(printf "%064x" 12345); echo "POSTGRES_PASSWORD=${HEX}" > "$R/config.yml"
git -C "$R" add config.yml
out=$(cd "$R" && bash "$GUARD" 2>&1); rc=$?
assert_exit "64-char hex KEY=VALUE in any file is blocked" 1 $rc
assert_contains "names the offending file" "config.yml" "$out"
rm -rf "$R"

# ─── Test 8: ANTHROPIC sk-ant- key is blocked ──────────────────────────────────
R=$(make_repo)
TOKEN="sk-""ant-api03-$(printf 'a%.0s' {1..60})"; echo "api_key = \"${TOKEN}\"" > "$R/settings.toml"
git -C "$R" add settings.toml
out=$(cd "$R" && bash "$GUARD" 2>&1); rc=$?
assert_exit "Anthropic sk-ant- token is blocked" 1 $rc
rm -rf "$R"

# ─── Test 9: --staged-only mode skips unstaged changes ─────────────────────────
R=$(make_repo)
echo "ok" > "$R/clean.txt"
git -C "$R" add clean.txt
HEX=$(printf "%064x" 12345); echo "POSTGRES_PASSWORD=${HEX}" > "$R/.env"
# .env exists in working tree but NOT staged — should still pass
out=$(cd "$R" && bash "$GUARD" 2>&1); rc=$?
assert_exit "unstaged .env in working tree does not block" 0 $rc
rm -rf "$R"

# ─── Test 10: ignored group-readable .env is blocked without content leak ─────
R=$(make_repo)
echo ".env" > "$R/.gitignore"
git -C "$R" add .gitignore
git -C "$R" commit -qm "ignore env"
echo "TOPSECRET_SHOULD_NOT_PRINT=abc123" > "$R/.env"
chmod 0640 "$R/.env"
out=$(cd "$R" && bash "$GUARD" 2>&1); rc=$?
assert_exit "ignored group-readable .env is blocked" 1 $rc
assert_contains "mode warning names env file" ".env" "$out"
if echo "$out" | grep -qF "TOPSECRET_SHOULD_NOT_PRINT"; then
    echo "✗ FAIL: ignored env permission warning leaked file contents"
    FAIL=$((FAIL+1))
else
    echo "✓ PASS: ignored env permission warning does not leak contents"
    PASS=$((PASS+1))
fi
rm -rf "$R"

# ─── Summary ───────────────────────────────────────────────────────────────────
echo
echo "─── ${PASS} passed, ${FAIL} failed ───"
[ "$FAIL" -eq 0 ]
