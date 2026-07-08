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

# --- Multi-line log: the newest (last) timestamp must be the one reported,
# not the first bracketed match in the file (#1362 nice-to-have #1). ---
older_ts="2020-01-01T00:00:00Z"
printf 'instinct: ok [%s]\ninstinct: ok [%s]\n' "$older_ts" "$now_ts" >"$tmp/state/instinct/run.log"

out=$(PATH="$tmp/bin:$PATH" XDG_STATE_HOME="$tmp/state" INSTINCT_MAX_GAP=7200 bash "$SCRIPT" 2>&1)
rc=$?
assert_exit "healthcheck picks newest of multiple log lines" 0 "$rc"
assert_contains "reports the newest timestamp, not the oldest" "Last: $now_ts" "$out"
if echo "$out" | grep -qF -- "Last: $older_ts"; then
  echo "FAIL: healthcheck reported the older timestamp instead of the newest"
  FAIL=$((FAIL + 1))
else
  echo "PASS: healthcheck did not report the older timestamp"
  PASS=$((PASS + 1))
fi

# --- Missing log file: must WARN and exit non-zero, with no date/arithmetic
# garbage (e.g. "date: invalid date" or unbound-variable errors) leaking into
# output (#1362 blocker #1). ---
rm -f "$tmp/state/instinct/run.log"
out=$(PATH="$tmp/bin:$PATH" XDG_STATE_HOME="$tmp/state" INSTINCT_MAX_GAP=7200 bash "$SCRIPT" 2>&1)
rc=$?
assert_exit "missing log file exits non-zero" 1 "$rc"
assert_contains "missing log file reports a clear WARN" "WARN: instinct run.log not found" "$out"
if echo "$out" | grep -qiE "invalid date|unbound variable|No such file or directory"; then
  echo "FAIL: missing-log path leaked raw command error output"
  FAIL=$((FAIL + 1))
else
  echo "PASS: missing-log path produced no raw command error output"
  PASS=$((PASS + 1))
fi

# --- Log exists but has no recognizable timestamp: must WARN and exit
# non-zero without falling through to gap arithmetic on an empty last_ts. ---
mkdir -p "$tmp/state/instinct"
printf 'instinct: started, no timestamp yet\n' >"$tmp/state/instinct/run.log"
out=$(PATH="$tmp/bin:$PATH" XDG_STATE_HOME="$tmp/state" INSTINCT_MAX_GAP=7200 bash "$SCRIPT" 2>&1)
rc=$?
assert_exit "log with no timestamp exits non-zero" 1 "$rc"
assert_contains "log with no timestamp reports a clear WARN" "WARN: no timestamped instinct runs found" "$out"
if echo "$out" | grep -qiE "invalid date|unbound variable"; then
  echo "FAIL: empty-last_ts path leaked date/arithmetic garbage"
  FAIL=$((FAIL + 1))
else
  echo "PASS: empty-last_ts path produced no date/arithmetic garbage"
  PASS=$((PASS + 1))
fi

# --- Log exists but is unreadable (permission denied): the sed extraction
# must not leak raw stderr, and the script must still fall through to the
# empty-last_ts WARN path cleanly (#1362 blocker #1). ---
if [ "$(id -u)" -ne 0 ]; then
  printf 'instinct: ok [%s]\n' "$now_ts" >"$tmp/state/instinct/run.log"
  chmod 000 "$tmp/state/instinct/run.log"
  out=$(PATH="$tmp/bin:$PATH" XDG_STATE_HOME="$tmp/state" INSTINCT_MAX_GAP=7200 bash "$SCRIPT" 2>&1)
  rc=$?
  chmod 644 "$tmp/state/instinct/run.log"
  assert_exit "unreadable log exits non-zero" 1 "$rc"
  if echo "$out" | grep -qiE "Permission denied|invalid date|unbound variable"; then
    echo "FAIL: unreadable-log path leaked raw sed/date error output"
    FAIL=$((FAIL + 1))
  else
    echo "PASS: unreadable-log path produced no raw sed/date error output"
    PASS=$((PASS + 1))
  fi
else
  echo "SKIP: unreadable-log test (running as root, chmod 000 has no effect)"
fi

# --- Stale run: gap exceeds INSTINCT_MAX_GAP, must FAIL / exit non-zero
# (#1362 nice-to-have #3). ---
old_ts="2020-01-01T00:00:00Z"
printf 'instinct: ok [%s]\n' "$old_ts" >"$tmp/state/instinct/run.log"
out=$(PATH="$tmp/bin:$PATH" XDG_STATE_HOME="$tmp/state" INSTINCT_MAX_GAP=60 bash "$SCRIPT" 2>&1)
rc=$?
assert_exit "stale run exits non-zero" 1 "$rc"
assert_contains "stale run reports WARN" "WARN: instinct last ran" "$out"

# --- Success path must not depend on the last command's exit status: an
# explicit `exit 0` must terminate the script (#1362 blocker #3). ---
printf 'instinct: ok [%s]\n' "$now_ts" >"$tmp/state/instinct/run.log"
if grep -qE '^exit 0$' "$SCRIPT"; then
  echo "PASS: success path has an explicit exit 0"
  PASS=$((PASS + 1))
else
  echo "FAIL: success path is missing an explicit exit 0"
  FAIL=$((FAIL + 1))
fi

echo
echo "--- ${PASS} passed, ${FAIL} failed ---"
[ "$FAIL" -eq 0 ]
