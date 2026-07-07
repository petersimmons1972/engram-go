#!/usr/bin/env bash
# Test suite for scripts/verify-recall-fetch.sh
# Usage: bash scripts/verify-recall-fetch.test.sh

set -u

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
SCRIPT="$SCRIPT_DIR/verify-recall-fetch.sh"

PASS=0
FAIL=0

assert_exit() {
  local desc="$1" expected="$2" actual="$3"
  if [ "$expected" -eq "$actual" ]; then
    echo "PASS: $desc (exit=$actual)"
    PASS=$((PASS + 1))
  else
    echo "FAIL: $desc (expected=$expected got=$actual)"
    FAIL=$((FAIL + 1))
  fi
}

assert_contains() {
  local desc="$1" needle="$2" haystack="$3"
  if echo "$haystack" | grep -qF -- "$needle"; then
    echo "PASS: $desc"
    PASS=$((PASS + 1))
  else
    echo "FAIL: $desc (missing '$needle')"
    FAIL=$((FAIL + 1))
  fi
}

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT
mkdir -p "$tmp/bin"

cat >"$tmp/bin/xh" <<'FAKE_XH'
#!/usr/bin/env bash
echo "fake xh must not be invoked" >&2
exit 99
FAKE_XH
chmod +x "$tmp/bin/xh"

cat >"$tmp/bin/curl" <<'FAKE_CURL'
#!/usr/bin/env bash
printf '%s\n' '{"jsonrpc":"2.0","id":1,"result":{"content":[{"type":"text","text":"{\"handles\":[]}"}]}}'
FAKE_CURL
chmod +x "$tmp/bin/curl"

out=$(PATH="$tmp/bin:$PATH" bash "$SCRIPT" --url http://127.0.0.1:8788 --query test 2>&1)
rc=$?
assert_exit "uses curl-compatible client even when xh is present" 0 "$rc"
assert_contains "empty recall succeeds" "OK: all 0 handles resolved successfully." "$out"

echo
echo "--- ${PASS} passed, ${FAIL} failed ---"
[ "$FAIL" -eq 0 ]
