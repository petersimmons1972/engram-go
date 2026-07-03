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
  if echo "$haystack" | grep -qF -- "$needle"; then
    echo "✓ PASS: $desc"
    PASS=$((PASS + 1))
  else
    echo "✗ FAIL: $desc (missing '$needle')"
    FAIL=$((FAIL + 1))
  fi
}

assert_not_contains() {
  local desc="$1" needle="$2" haystack="$3"
  if echo "$haystack" | grep -qF -- "$needle"; then
    echo "✗ FAIL: $desc (found unexpected '$needle')"
    FAIL=$((FAIL + 1))
  else
    echo "✓ PASS: $desc"
    PASS=$((PASS + 1))
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

tmp=$(mktemp -d)
run_dir="$tmp/run"
fake_bin="$tmp/longmemeval"
args_log="$tmp/args.log"
mkdir -p "$run_dir"
cat >"$fake_bin" <<EOF
#!/usr/bin/env bash
set -eu
printf '%s\n' "\$@" >"$args_log"
out_dir=""
while [ "\$#" -gt 0 ]; do
  if [ "\$1" = "--out" ]; then
    out_dir="\$2"
    shift 2
    continue
  fi
  shift
done
mkdir -p "\$out_dir"
cat >"\$out_dir/score_report.json" <<'JSON'
{"overall":{"correct":1,"partially_correct":0,"total":1}}
JSON
EOF
chmod +x "$fake_bin"

out=$(BIN="$fake_bin" SCORER_MAX_TOKENS=4096 bash "$SCRIPT" --run "$run_dir" --judge qwen3 --thinking off 2>&1); rc=$?
assert_exit "qwen3 wrapper invocation succeeds with fake scorer" 0 "$rc"
args=$(cat "$args_log")
assert_contains "qwen3 invokes score-efficient" "score-efficient" "$args"
assert_contains "qwen3 still forwards scorer-thinking override" "--scorer-thinking=false" "$args"
assert_not_contains "qwen3 locked path omits scorer-max-tokens" "--scorer-max-tokens" "$args"
assert_not_contains "qwen3 locked path omits preserve-correct" "--preserve-correct" "$args"

out=$(BIN="$fake_bin" OPENAI_API_KEY=test-key SCORER_MAX_TOKENS=4096 bash "$SCRIPT" --run "$run_dir" --judge gpt4o --thinking off 2>&1); rc=$?
assert_exit "gpt4o wrapper invocation succeeds with fake scorer" 0 "$rc"
args=$(cat "$args_log")
assert_contains "gpt4o path still forwards scorer-max-tokens" "--scorer-max-tokens" "$args"
assert_contains "gpt4o path still forwards preserve-correct" "--preserve-correct" "$args"

rm -rf "$tmp"

echo
echo "─── ${PASS} passed, ${FAIL} failed ───"
[ "$FAIL" -eq 0 ]
