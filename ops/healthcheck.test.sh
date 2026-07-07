#!/usr/bin/env bash
# Test suite for ops/healthcheck.sh
# Usage: bash ops/healthcheck.test.sh

set -u

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
SCRIPT="$SCRIPT_DIR/healthcheck.sh"

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
mkdir -p "$tmp/bin" "$tmp/state/instinct"

cat >"$tmp/bin/grep" <<'FAKE_GREP'
#!/usr/bin/env bash
for arg in "$@"; do
  case "$arg" in
    *P*) echo "fake grep: -P is unavailable" >&2; exit 2 ;;
  esac
done
exec /usr/bin/grep "$@"
FAKE_GREP
chmod +x "$tmp/bin/grep"

now_ts="$(date -u +"%Y-%m-%dT%H:%M:%SZ")"
printf 'instinct: ok [%s]\n' "$now_ts" >"$tmp/state/instinct/run.log"

out=$(PATH="$tmp/bin:$PATH" XDG_STATE_HOME="$tmp/state" INSTINCT_MAX_GAP=7200 bash "$SCRIPT" 2>&1)
rc=$?
assert_exit "healthcheck works without grep -P" 0 "$rc"
assert_contains "reports healthy status" "OK: instinct last ran" "$out"
assert_contains "reports extracted timestamp" "Last: $now_ts" "$out"

echo
echo "--- ${PASS} passed, ${FAIL} failed ---"
[ "$FAIL" -eq 0 ]
