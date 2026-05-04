#!/usr/bin/env bats
# Tests for hook pipeline observability — engram-go#404
# Verifies ~/.claude/.engram-hook-state.json is written and read correctly.
# All tests should FAIL before lib/engram-state.sh and hook wiring are in place.

LIB="$HOME/.claude/hooks/lib/engram-state.sh"
# Isolated state file — keeps tests from racing with live session hooks
# Exported so that when tests source $LIB, STATE_FILE resolves to this path
export ENGRAM_TEST_STATE_FILE="/tmp/engram-test-state-$$.json"
STATE_FILE="$ENGRAM_TEST_STATE_FILE"
MOCK_SCRIPT="$BATS_TEST_DIRNAME/mock-server.py"

fail() { echo "FAIL: $*" >&2; return 1; }
MOCK_PORT=19789  # different port from flush tests
MOCK_PID_FILE="/tmp/mock-server-${MOCK_PORT}.pid"
BASE_URL="http://127.0.0.1:${MOCK_PORT}"

_configure_mock() {
    curl -sf -X POST "$BASE_URL/__configure" \
        -H "Content-Type: application/json" \
        -d "$1" > /dev/null 2>&1 || true
}

_reset_mock() {
    curl -sf -X POST "$BASE_URL/__reset" > /dev/null 2>&1 || true
}

_read_state() {
    python3 -c "
import json, sys
try:
    d = json.load(open('$STATE_FILE'))
    print(d.get('$1', ''))
except: print('')
" 2>/dev/null || echo ""
}

_write_state_direct() {
    # $1: JSON blob to write directly to state file (for test setup)
    echo "$1" > "$STATE_FILE"
}

setup_file() {
    MOCK_PORT=$MOCK_PORT MOCK_PID_FILE=$MOCK_PID_FILE \
        python3 "$MOCK_SCRIPT" &
    sleep 0.3
    for i in $(seq 1 30); do
        curl -sf "$BASE_URL/__reset" > /dev/null 2>&1 && break
        sleep 0.1
    done
}

teardown_file() {
    [[ -f "$MOCK_PID_FILE" ]] && kill "$(cat "$MOCK_PID_FILE")" 2>/dev/null || true
}

setup() {
    _reset_mock
    # Back up and clear state file
    [[ -f "$STATE_FILE" ]] && cp "$STATE_FILE" "${STATE_FILE}.bak" || true
    rm -f "$STATE_FILE"
    # Clear auth cache so hooks don't exit early on cache hit
    rm -f "$HOME/.claude/.engram-auth-ok"
}

teardown() {
    # Restore state file
    rm -f "$STATE_FILE"
    [[ -f "${STATE_FILE}.bak" ]] && mv "${STATE_FILE}.bak" "$STATE_FILE" || true
}

_hook() {
    # $1: hook filename — run with env var overrides so no sed patching needed
    local hook="$HOME/.claude/hooks/$1"
    ENGRAM_TEST_PORT="$MOCK_PORT" \
    ENGRAM_TEST_STATE_FILE="$STATE_FILE" \
        bash "$hook" 2>&1
}

@test "#404: lib/engram-state.sh exists" {
    [[ -f "$LIB" ]] \
        || fail "lib/engram-state.sh does not exist — create ~/.claude/hooks/lib/engram-state.sh"
}

@test "#404: update_state creates state file with expected keys" {
    [[ -f "$LIB" ]] || skip "lib/engram-state.sh not yet created"
    source "$LIB"
    update_state "last_session_start" "\"2026-05-01T00:00:00Z\""

    [[ -f "$STATE_FILE" ]] \
        || fail "update_state did not create $STATE_FILE"

    local val
    val=$(_read_state "last_session_start")
    [[ "$val" == "2026-05-01T00:00:00Z" ]] \
        || fail "Expected last_session_start=2026-05-01T00:00:00Z, got: $val"
}

@test "#404: update_state is atomic (concurrent writes don't corrupt)" {
    [[ -f "$LIB" ]] || skip "lib/engram-state.sh not yet created"
    source "$LIB"

    # Spawn 10 concurrent updates
    pids=()
    for i in $(seq 1 10); do
        update_state "fallback_entry_count" "$i" &
        pids+=($!)
    done
    for pid in "${pids[@]}"; do wait "$pid" || true; done

    # State file must be valid JSON after concurrent writes
    python3 -c "import json; json.load(open('$STATE_FILE'))" \
        || fail "State file is invalid JSON after concurrent writes"
}

@test "#404: auth failure increments consecutive_auth_failures" {
    [[ -f "$LIB" ]] || skip "lib/engram-state.sh not yet created"
    _write_state_direct '{"consecutive_auth_failures": 0, "last_session_start": null, "last_recall_results": 0, "last_flush_at": null, "fallback_entry_count": 0, "last_auth_ok_at": null, "sessions_since_last_flush": 0}'
    _configure_mock '{"endpoint":"/quick-recall","mode":"401"}'

    _hook "engram-auth-check.sh" || true

    local val
    val=$(_read_state "consecutive_auth_failures")
    [[ "$val" -gt 0 ]] \
        || fail "consecutive_auth_failures not incremented after 401. Got: $val"
}

@test "#404: auth success resets consecutive_auth_failures to 0" {
    [[ -f "$LIB" ]] || skip "lib/engram-state.sh not yet created"
    _write_state_direct '{"consecutive_auth_failures": 2, "last_session_start": null, "last_recall_results": 0, "last_flush_at": null, "fallback_entry_count": 0, "last_auth_ok_at": null, "sessions_since_last_flush": 0}'
    _configure_mock '{"endpoint":"/quick-recall","mode":"200"}'

    _hook "engram-auth-check.sh" || true

    local val
    val=$(_read_state "consecutive_auth_failures")
    [[ "$val" -eq 0 ]] \
        || fail "consecutive_auth_failures not reset after successful auth. Got: $val"
}

@test "#404: degraded state injects systemMessage when thresholds exceeded" {
    [[ -f "$LIB" ]] || skip "lib/engram-state.sh not yet created"
    _write_state_direct '{"consecutive_auth_failures": 0, "last_session_start": null, "last_recall_results": 0, "last_flush_at": null, "fallback_entry_count": 15, "last_auth_ok_at": null, "sessions_since_last_flush": 3}'
    _configure_mock '{"endpoint":"/health","mode":"200"}'
    _configure_mock '{"endpoint":"/setup-token","mode":"200"}'
    _configure_mock '{"endpoint":"/quick-recall","mode":"200"}'

    run _hook "engram-token-refresh.sh"

    [[ "$output" == *"systemMessage"* ]] \
        || fail "Expected systemMessage in output for degraded state, got: $output"
    [[ "$output" == *"degraded"* || "$output" == *"fallback"* || "$output" == *"queued"* ]] \
        || fail "systemMessage should mention degraded state/fallback count. Got: $output"
}

@test "#404: flush updates last_flush_at and resets sessions_since_last_flush" {
    [[ -f "$LIB" ]] || skip "lib/engram-state.sh not yet created"
    _write_state_direct '{"consecutive_auth_failures": 0, "last_session_start": null, "last_recall_results": 0, "last_flush_at": null, "fallback_entry_count": 1, "last_auth_ok_at": null, "sessions_since_last_flush": 2}'
    _configure_mock '{"endpoint":"/health","mode":"200"}'
    _configure_mock '{"endpoint":"/setup-token","mode":"200"}'
    _configure_mock '{"endpoint":"/quick-store","mode":"200"}'

    # Create a minimal fallback.md with one entry
    local today
    today=$(date +%Y-%m-%d)
    local fallback="$HOME/.claude/projects/-home-psimmons/memory/fallback.md"
    local backup="${fallback}.bak"
    [[ -f "$fallback" ]] && cp "$fallback" "$backup" || true
    cat > "$fallback" <<EOF
# Fallback Memory Store
## Pending Entries
## [${today}] Test observability entry
**Project:** global
**Type:** context

Observability test content.
<!-- Add entries below -->
EOF

    _hook "engram-flush-fallback.sh" || true

    # Restore
    rm -f "$fallback"
    [[ -f "$backup" ]] && mv "$backup" "$fallback" || true

    local flush_at
    flush_at=$(_read_state "last_flush_at")
    [[ -n "$flush_at" ]] \
        || fail "last_flush_at not written after successful flush"

    local sessions
    sessions=$(_read_state "sessions_since_last_flush")
    [[ "$sessions" -eq 0 ]] \
        || fail "sessions_since_last_flush not reset to 0. Got: $sessions"
}

@test "#404: session-end emits summary line with recall and fallback counts" {
    [[ -f "$LIB" ]] || skip "lib/engram-state.sh not yet created"
    _write_state_direct '{"consecutive_auth_failures": 0, "last_session_start": "2026-05-01T00:00:00Z", "last_recall_results": 4, "last_flush_at": "2026-05-01T00:00:00Z", "fallback_entry_count": 2, "last_auth_ok_at": "2026-05-01T00:00:00Z", "sessions_since_last_flush": 0}'
    _configure_mock '{"endpoint":"/health","mode":"200"}'
    _configure_mock '{"endpoint":"/setup-token","mode":"200"}'

    run _hook "engram-session-end.sh"

    [[ "$output" == *"session closed"* ]] \
        || fail "Expected 'session closed' in session-end output, got: $output"
    [[ "$output" == *"4"* ]] \
        || fail "Expected recall count (4) in session-end output, got: $output"
    [[ "$output" == *"2"* ]] \
        || fail "Expected fallback count (2) in session-end output, got: $output"
}
