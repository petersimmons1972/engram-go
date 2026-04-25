#!/usr/bin/env bash
# Smoke test: hook writes one JSONL line per allowed tool call
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
HOOK="$SCRIPT_DIR/../../hooks/post-tool-use.sh"
FIXTURE_DIR="$SCRIPT_DIR/fixtures"
TMP_DIR=$(mktemp -d)
BUFFER="$TMP_DIR/instinct/buffer.jsonl"

export XDG_STATE_HOME="$TMP_DIR"
export INSTINCT_ENABLED=1
export INSTINCT_CONSOLIDATE_EVERY=9999  # prevent consolidator spawn
export CLAUDE_SESSION_ID="sess-test"
export CLAUDE_PROJECT_DIR="/tmp"

cleanup() { rm -rf "$TMP_DIR"; }
trap cleanup EXIT

pass=0
fail=0

check() {
    local label="$1" result="$2"
    if [[ "$result" == "pass" ]]; then
        echo "PASS: $label"
        ((pass++)) || true
    else
        echo "FAIL: $label — $result"
        ((fail++)) || true
    fi
}

# Test 1: Edit tool writes one buffer line
bash "$HOOK" < "$FIXTURE_DIR/hook_stdin_edit.json"
count=$(wc -l < "$BUFFER" 2>/dev/null || echo 0)
[[ "$count" -eq 1 ]] && check "Edit tool writes one line" "pass" || check "Edit tool writes one line" "expected 1 line, got $count"

# Test 2: Bash tool writes another line
bash "$HOOK" < "$FIXTURE_DIR/hook_stdin_bash.json"
count=$(wc -l < "$BUFFER" 2>/dev/null || echo 0)
[[ "$count" -eq 2 ]] && check "Bash tool appends second line" "pass" || check "Bash tool appends second line" "expected 2 lines, got $count"

# Test 3: INSTINCT_ENABLED=0 writes nothing
rm -f "$BUFFER"
INSTINCT_ENABLED=0 bash "$HOOK" < "$FIXTURE_DIR/hook_stdin_edit.json"
[[ ! -f "$BUFFER" ]] && check "Disabled hook writes nothing" "pass" || check "Disabled hook writes nothing" "buffer was created when disabled"

# Test 4: buffer lines are valid JSON with required fields
bash "$HOOK" < "$FIXTURE_DIR/hook_stdin_edit.json"
result=$(python3 -c "
import json
lines = open('$BUFFER').read().strip().splitlines()
required = {'timestamp','session_id','project_id','tool_name','tool_input_hash','tool_output_summary','exit_status','schema_version'}
for line in lines:
    d = json.loads(line)
    missing = required - set(d.keys())
    if missing:
        print('missing fields: ' + str(missing))
        exit(1)
print('ok')
" 2>&1)
[[ "$result" == "ok" ]] && check "Buffer lines have required fields" "pass" || check "Buffer lines have required fields" "$result"

# Test 5: Read tool (not in allowlist) writes nothing
rm -f "$BUFFER"
echo '{"tool_name":"Read","file_path":"/tmp/test.py","tool_response":"content"}' | bash "$HOOK"
[[ ! -f "$BUFFER" ]] && check "Read tool not captured (allowlist)" "pass" || check "Read tool not captured (allowlist)" "buffer written for disallowed tool"

echo ""
echo "Results: $pass passed, $fail failed"
[[ "$fail" -eq 0 ]] && exit 0 || exit 1
