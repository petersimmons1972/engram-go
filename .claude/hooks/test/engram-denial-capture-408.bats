#!/usr/bin/env bats
# Tests for engram-denial-capture.sh — engram-go#408
#
# Fix verified:
#   2. Hook must emit systemMessage to stdout when a synthesized denial is captured

HOOK="$HOME/.claude/hooks/engram-denial-capture.sh"

setup() {
    TMPDIR=$(mktemp -d)
    export ENGRAM_TEST_DENIAL_LOG="$TMPDIR/denial-log.md"
    PAYLOAD_FILE="$TMPDIR/payload.json"
}

teardown() {
    rm -rf "$TMPDIR"
}

_write_payload() {
    # $1: tool_response string — write to file to avoid quoting issues
    python3 -c "
import json, sys
print(json.dumps({
    'tool_name': 'mcp__engram__memory_status',
    'tool_use_id': 'toolu_test_123',
    'session_id': 'test-session',
    'tool_input': {'project': 'global'},
    'tool_response': sys.argv[1]
}))" "$1" > "$PAYLOAD_FILE"
}

@test "emits systemMessage to stdout when tool_response contains denial marker" {
    _write_payload "The user doesn't want to proceed with this tool use"
    run bash -c "cat '$PAYLOAD_FILE' | ENGRAM_TEST_DENIAL_LOG='$ENGRAM_TEST_DENIAL_LOG' '$HOOK'"
    [ "$status" -eq 0 ]
    python3 -c "
import json, sys
raw = open('$TMPDIR/hook_out.txt').read() if False else '''$(cat "$TMPDIR/hook_out.txt" 2>/dev/null || true)'''
" 2>/dev/null || true
    echo "$output" | python3 -c "
import json, sys
raw = sys.stdin.read().strip()
if not raw:
    print('No output from hook', file=sys.stderr); sys.exit(1)
try:
    d = json.loads(raw)
except Exception as e:
    print(f'Not valid JSON: {e!r}  raw={raw!r}', file=sys.stderr); sys.exit(1)
assert 'systemMessage' in d, f'no systemMessage in {d}'
" || {
        echo "raw output was: [$output]" >&2
        return 1
    }
}

@test "emits systemMessage when tool_response contains 'tool use was rejected'" {
    _write_payload "tool use was rejected"
    run bash -c "cat '$PAYLOAD_FILE' | ENGRAM_TEST_DENIAL_LOG='$ENGRAM_TEST_DENIAL_LOG' '$HOOK'"
    [ "$status" -eq 0 ]
    [[ "$output" == *"systemMessage"* ]] || {
        echo "expected systemMessage in output, got: [$output]" >&2
        return 1
    }
}

@test "silent (no stdout) when tool_response is a normal success" {
    _write_payload '{"status":"ok","postgres":"ok","ollama":"ok"}'
    run bash -c "cat '$PAYLOAD_FILE' | ENGRAM_TEST_DENIAL_LOG='$ENGRAM_TEST_DENIAL_LOG' '$HOOK'"
    [ "$status" -eq 0 ]
    [ -z "$output" ] || {
        echo "expected no output for success response, got: [$output]" >&2
        return 1
    }
}

@test "logs denial record to ENGRAM_TEST_DENIAL_LOG file" {
    _write_payload "The user doesn't want to proceed with this tool use"
    run bash -c "cat '$PAYLOAD_FILE' | ENGRAM_TEST_DENIAL_LOG='$ENGRAM_TEST_DENIAL_LOG' '$HOOK'"
    [ "$status" -eq 0 ]
    [ -f "$ENGRAM_TEST_DENIAL_LOG" ] || {
        echo "expected denial-log at $ENGRAM_TEST_DENIAL_LOG — not created" >&2
        return 1
    }
    grep -q "tool_synthesized_denial" "$ENGRAM_TEST_DENIAL_LOG" || {
        echo "denial-log does not contain expected record:" >&2
        cat "$ENGRAM_TEST_DENIAL_LOG" >&2
        return 1
    }
}
