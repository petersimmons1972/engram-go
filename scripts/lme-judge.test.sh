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
assert_contains "help documents gold version" " --gold-version <tag>" "$out"
assert_contains "help warns against scorer secrets on argv" "do not pass secrets on argv" "$out"

out=$(bash "$SCRIPT" 2>&1); rc=$?
assert_exit "missing arguments fail" 2 "$rc"
assert_contains "missing run arg explains usage" "--run is required" "$out"

tmp=$(mktemp -d)
run_dir="$tmp/run"
mkdir -p "$run_dir"
out=$(bash "$SCRIPT" --run "$run_dir" --gold-version gold-v1 2>&1); rc=$?
assert_exit "judge required without --bundle" 2 "$rc"
assert_contains "judge required message" "required when --bundle is not set" "$out"

tmp=$(mktemp -d)
run_dir="$tmp/run"
fake_bin="$tmp/longmemeval"
args_log="$tmp/args.log"
env_log="$tmp/env.log"
ready_file="$tmp/ready"
pid_file="$tmp/pid"
lock_path="$tmp/scorer-lock.json"
mkdir -p "$run_dir"
cat >"$fake_bin" <<EOF
#!/usr/bin/env bash
set -eu
printf '%s\n' "\$@" >"$args_log"
printf '%s' "\${LME_SCORER_API_KEY:-}" >"$env_log"
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
{"overall":{"correct":1,"partially_correct":0,"total":1},"baseline_comparison":{"status":"near","nearest_baseline":"honest_plateau","observed_strict_pct":70.0}}
JSON
if [ -n "\${FAKE_BIN_READY_FILE:-}" ]; then
  printf '%s' "\$\$" >"$pid_file"
  : >"\$FAKE_BIN_READY_FILE"
fi
if [ "\${FAKE_BIN_SLEEP_SECS:-0}" != "0" ]; then
  sleep "\$FAKE_BIN_SLEEP_SECS"
fi
EOF
chmod +x "$fake_bin"
cat >"$lock_path" <<'EOF'
{"version":"tier1-qwen3-2026-06-22","scorer_url":"http://locked/v1","scorer_model":"inference","scorer_thinking":false,"scorer_max_tokens":2048}
EOF

out=$(BIN="$fake_bin" SCORER_LOCK="$lock_path" bash "$SCRIPT" --run "$run_dir" --judge qwen3 --gold-version gold-v1 --thinking off 2>&1); rc=$?
assert_exit "qwen3 wrapper invocation succeeds with fake scorer" 0 "$rc"
args=$(cat "$args_log")
assert_contains "qwen3 invokes score-efficient" "score-efficient" "$args"
assert_contains "qwen3 forwards scorer-lock" "--scorer-lock" "$args"
assert_contains "qwen3 forwards gold version" "--gold-version" "$args"
assert_contains "qwen3 forwards item set" "--item-set" "$args"
assert_not_contains "qwen3 lock path omits scorer-url" "--scorer-url" "$args"
assert_not_contains "qwen3 lock path omits scorer-model" "--scorer-model" "$args"
assert_not_contains "qwen3 lock path omits scorer-thinking" "--scorer-thinking" "$args"
assert_not_contains "qwen3 lock path omits scorer-max-tokens" "--scorer-max-tokens" "$args"
assert_not_contains "qwen3 lock path omits preserve-correct" "--preserve-correct" "$args"

out=$(BIN="$fake_bin" OPENAI_API_KEY=test-key bash "$SCRIPT" --run "$run_dir" --judge gpt4o --gold-version gold-v1 --thinking off 2>&1); rc=$?
assert_exit "gpt4o wrapper invocation succeeds with fake scorer" 0 "$rc"
args=$(cat "$args_log")
assert_contains "gpt4o path still forwards scorer-url" "--scorer-url" "$args"
assert_contains "gpt4o path still forwards scorer-model" "--scorer-model" "$args"
assert_not_contains "gpt4o path omits scorer-api-key flag" "--scorer-api-key" "$args"
assert_not_contains "gpt4o path omits scorer-api-key value" "test-key" "$args"
assert_contains "gpt4o path still forwards scorer-thinking" "--scorer-thinking=false" "$args"
assert_contains "gpt4o path still forwards scorer-max-tokens" "--scorer-max-tokens" "$args"
assert_contains "gpt4o path still forwards preserve-correct" "--preserve-correct" "$args"
env_value=$(cat "$env_log")
assert_contains "gpt4o path exports scorer key via environment" "test-key" "$env_value"

rm -f "$args_log" "$env_log" "$ready_file" "$pid_file"
BIN="$fake_bin" OPENAI_API_KEY=redacted-test-key FAKE_BIN_READY_FILE="$ready_file" FAKE_BIN_SLEEP_SECS=5 \
  bash "$SCRIPT" --run "$run_dir" --judge gpt4o --gold-version gold-v1 >"$tmp/bg.out" 2>&1 &
judge_pid=$!
for _ in $(seq 1 50); do
  if [ -f "$ready_file" ]; then
    break
  fi
  sleep 0.1
done
if [ ! -f "$ready_file" ]; then
  echo "✗ FAIL: active gpt4o judge reached ready state"
  FAIL=$((FAIL + 1))
  wait "$judge_pid" || true
else
  fake_pid=$(cat "$pid_file")
  if [ ! -r "/proc/$judge_pid/cmdline" ] || [ ! -r "/proc/$fake_pid/cmdline" ]; then
    echo "✗ FAIL: active judge cmdline was not readable under /proc"
    FAIL=$((FAIL + 1))
  else
    judge_cmdline=$(tr '\0' ' ' </proc/"$judge_pid"/cmdline)
    fake_cmdline=$(tr '\0' ' ' </proc/"$fake_pid"/cmdline)
    if [[ "$judge_cmdline" == *"redacted-test-key"* ]] || [[ "$fake_cmdline" == *"redacted-test-key"* ]]; then
      echo "✗ FAIL: active judge cmdline leaked scorer key"
      FAIL=$((FAIL + 1))
    else
      echo "✓ PASS: active judge cmdline does not contain scorer key"
      PASS=$((PASS + 1))
    fi
  fi
  wait "$judge_pid"; rc=$?
  assert_exit "active gpt4o judge run completes cleanly" 0 "$rc"
fi

rm -rf "$tmp" "$run_dir"

echo
echo "─── ${PASS} passed, ${FAIL} failed ───"
[ "$FAIL" -eq 0 ]
