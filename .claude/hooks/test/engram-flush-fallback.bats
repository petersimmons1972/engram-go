#!/usr/bin/env bats
# Tests for engram-flush-fallback.sh — engram-go#397
# Tests for 4xx drop, 5xx retry, retry counter, timeout handling.
# All new tests should FAIL before the fix is applied.

HOOK="$HOME/.claude/hooks/engram-flush-fallback.sh"
MOCK_SCRIPT="$BATS_TEST_DIRNAME/mock-server.py"

fail() { echo "FAIL: $*" >&2; return 1; }

MOCK_PORT=19788
MOCK_PID_FILE="/tmp/mock-server-${MOCK_PORT}.pid"
BASE_URL="http://127.0.0.1:${MOCK_PORT}"

_configure_mock() {
    curl -sf -X POST "$BASE_URL/__configure" \
        -H "Content-Type: application/json" \
        -d "$1" > /dev/null
}

_reset_mock() {
    curl -sf -X POST "$BASE_URL/__reset" > /dev/null 2>&1 || true
}

_request_count() {
    # $1: endpoint path (leading slash stripped to avoid double-slash in URL)
    local ep="${1#/}"
    curl -sf "$BASE_URL/__requests/$ep" 2>/dev/null \
        | python3 -c "import json,sys; print(len(json.load(sys.stdin)))" 2>/dev/null || echo 0
}

setup_file() {
    MOCK_PORT=19788
    MOCK_PID_FILE="/tmp/mock-server-${MOCK_PORT}.pid"
    MOCK_PORT=$MOCK_PORT MOCK_PID_FILE=$MOCK_PID_FILE \
        python3 "$MOCK_SCRIPT" &
    sleep 0.3
    # Wait up to 3s for server to be ready
    for i in $(seq 1 30); do
        curl -sf "$BASE_URL/__reset" > /dev/null 2>&1 && break
        sleep 0.1
    done
}

teardown_file() {
    [[ -f "$MOCK_PID_FILE" ]] && kill "$(cat "$MOCK_PID_FILE")" 2>/dev/null || true
}

setup() {
    TMPDIR=$(mktemp -d)
    FALLBACK="$TMPDIR/fallback.md"
    _reset_mock
    # Default mock responses needed by every flush run
    _configure_mock '{"endpoint":"/health","mode":"200"}'
    _configure_mock '{"endpoint":"/setup-token","mode":"200"}'
}

teardown() {
    rm -rf "$TMPDIR"
}

_make_fallback_entry() {
    # $1: title suffix, $2: optional retry tag
    local today
    today=$(date +%Y-%m-%d)
    local retry_tag="${2:-}"
    cat > "$FALLBACK" <<EOF
# Fallback Memory Store

## Pending Entries

## [${today}] Test entry ${1}
**Project:** global
**Type:** context
**Tags:** [test]

Test memory content for entry ${1}.${retry_tag}
<!-- Add entries below -->
EOF
}

_run_flush() {
    PORT=19788 BASE="$BASE_URL" \
        FALLBACK="$FALLBACK" \
        bash -c "
            FALLBACK='$FALLBACK'
            PORT=19788
            BASE='$BASE_URL'
            source '$HOOK' 2>/dev/null
        " 2>&1 || true
    # Run the actual hook with env overrides
    env FALLBACK_OVERRIDE="$FALLBACK" \
        bash "$HOOK" 2>&1 || true
}

_flush_with_fallback() {
    # Run the flush hook with env var overrides for test isolation
    ENGRAM_TEST_FALLBACK="$FALLBACK" \
    ENGRAM_TEST_PORT="$MOCK_PORT" \
        bash "$HOOK" 2>&1
}

_has_pending_entries() {
    # Match actual entry headers only — template comment also contains **Project:** so
    # we use the date-bracketed entry header which only appears in real entries
    grep -qE '^\#\# \[[0-9]{4}-[0-9]{2}-[0-9]{2}\]' "$FALLBACK" 2>/dev/null
}

@test "#397: 4xx response drops entry permanently (not re-appended)" {
    _make_fallback_entry "4xx-test"
    _configure_mock '{"endpoint":"/quick-store","mode":"422"}'

    _flush_with_fallback

    # fallback.md must have no remaining entry headers
    ! _has_pending_entries \
        || fail "4xx entry was re-appended to fallback.md — should be dropped permanently"
}

@test "#397: 4xx sends exactly 1 request (no retry within flush)" {
    _make_fallback_entry "4xx-retry-check"
    _configure_mock '{"endpoint":"/quick-store","mode":"422"}'

    _flush_with_fallback

    local count
    count=$(_request_count "/quick-store")
    [[ "$count" -eq 1 ]] \
        || fail "Expected 1 request for 4xx, got $count — hook is retrying permanent failures"
}

@test "#397: 5xx response keeps entry for next session" {
    _make_fallback_entry "5xx-test"
    _configure_mock '{"endpoint":"/quick-store","mode":"503"}'

    _flush_with_fallback

    _has_pending_entries \
        || fail "5xx entry was dropped — should be kept in fallback.md for retry next session"
}

@test "#397: entry is dropped after 3 failed 5xx attempts (retry counter)" {
    # Pre-tag entry with retry:2 (one more attempt will hit max)
    _make_fallback_entry "retry-counter-test" $'\n<!-- retry:2 -->'
    _configure_mock '{"endpoint":"/quick-store","mode":"503"}'

    run _flush_with_fallback

    # After 3rd failure, entry should be gone
    if _has_pending_entries; then
        fail "Entry with retry:2 + 5xx should be dropped after max retries, but it remains in fallback.md"
    fi
    # Stderr should mention dropping
    [[ "$output" == *"retry"* || "$output" == *"drop"* ]] \
        || fail "Expected stderr mention of retry drop, got: $output"
}

@test "#397: network timeout re-appends entry with incremented retry counter" {
    _make_fallback_entry "timeout-test"
    _configure_mock '{"endpoint":"/quick-store","mode":"timeout"}'

    # Run with short timeout so test doesn't hang — the hook uses 5s internally
    timeout 10 bash -c "ENGRAM_TEST_FALLBACK='$FALLBACK' ENGRAM_TEST_PORT='$MOCK_PORT' bash '$HOOK'" 2>&1 || true

    # Entry should still be in fallback.md
    _has_pending_entries \
        || fail "Timeout entry was dropped — should be re-appended for next session"

    # Entry should now have a retry counter
    grep -q "retry:" "$FALLBACK" \
        || fail "Re-appended entry missing retry counter tag"
}

@test "#397: 2xx removes both entries cleanly (regression)" {
    _make_fallback_entry "success-1"
    # Append a second entry
    local today
    today=$(date +%Y-%m-%d)
    cat >> "$FALLBACK" <<EOF

## [${today}] Test entry success-2
**Project:** global
**Type:** context

Second test memory content.
EOF
    _configure_mock '{"endpoint":"/quick-store","mode":"200"}'

    run _flush_with_fallback

    ! _has_pending_entries \
        || fail "fallback.md still has entries after successful 200 flush"
    [[ "$output" == *"flushed"* ]] \
        || fail "Expected 'flushed' in stdout, got: $output"
}
