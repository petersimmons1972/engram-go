#!/usr/bin/env bats
# Tests for instinct-post-tool-use.sh — instinct#8
# All tests should FAIL before the flock fix is applied.

HOOK="$HOME/.claude/hooks/instinct-post-tool-use.sh"
FIXTURE="$BATS_TEST_DIRNAME/fixtures/post-tool-use-success.json"

fail() { echo "FAIL: $*" >&2; return 1; }

setup() {
    TMPDIR=$(mktemp -d)
    export XDG_STATE_HOME="$TMPDIR/state"
    mkdir -p "$TMPDIR/state/instinct"
    BUFFER_DIR="$TMPDIR/state/instinct"
    BUFFER_FILE="$BUFFER_DIR/buffer.jsonl"
    LOG_FILE="$BUFFER_DIR/run.log"
    # Disable consolidator spawn (no binary in test env)
    export INSTINCT_ENABLED=1
    # Point hook at our tmpdir via env override — hook uses XDG_STATE_HOME
}

teardown() {
    rm -rf "$TMPDIR"
}

_run_hook() {
    cat "$FIXTURE" | bash "$HOOK" 2>/dev/null
}

@test "instinct#8: concurrent buffer writes produce no partial JSON lines" {
    # Spawn 20 simultaneous hook invocations
    pids=()
    for i in $(seq 1 20); do
        _run_hook &
        pids+=($!)
    done
    for pid in "${pids[@]}"; do
        wait "$pid" || true
    done

    # Every line in buffer.jsonl must parse as valid JSON
    if [[ -f "$BUFFER_FILE" ]]; then
        while IFS= read -r line; do
            [[ -z "$line" ]] && continue
            echo "$line" | python3 -c "import json,sys; json.load(sys.stdin)" \
                || fail "Invalid JSON line in buffer.jsonl: $line"
        done < "$BUFFER_FILE"
    fi
}

@test "instinct#8: disown targets only the spawned background process" {
    # The hook should use 'disown $!' not bare 'disown'
    # Bare 'disown' disowns ALL jobs, which is a side effect bug
    grep -q 'disown \$!' "$HOOK" \
        || fail "Hook uses bare 'disown' instead of 'disown \$!' — fix: use disown \$!"
}

@test "instinct#8: buffer file write is protected by flock" {
    # The hook must use flock when appending to buffer.jsonl
    # Without flock, concurrent writes can interleave
    grep -q 'flock' "$HOOK" \
        || fail "Hook has no flock on buffer file write — concurrent appends can corrupt buffer.jsonl"
}

@test "instinct#8: log file write is protected by flock" {
    # Background consolidator log writes must also be serialized
    # Check that the LOG_FILE append path uses flock
    # This checks the warn-log path (echo ... >> LOG_FILE without consolidator)
    run grep -c 'flock' "$HOOK"
    # Should have at least 2 flock uses: one for buffer, one for log
    [[ "$output" -ge 2 ]] \
        || fail "Expected at least 2 flock uses in hook (buffer + log), found: $output"
}
