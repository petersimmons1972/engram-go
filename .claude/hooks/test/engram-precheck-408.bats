#!/usr/bin/env bats
# Tests for engram-precheck.sh — engram-go#408
#
# Fixes verified:
#   1. Hook must return in < 3s even when Engram is unreachable (no 20s poll loop)
#   4. Hook must never emit "action":"block" regardless of state

HOOK="$HOME/.claude/hooks/engram-precheck.sh"

# ── helpers ──────────────────────────────────────────────────────────────────

_elapsed() {
    local start end
    start=$(date +%s)
    "$@"
    end=$(date +%s)
    echo $(( end - start ))
}

# ── tests ─────────────────────────────────────────────────────────────────────

@test "fast-path: exits 0 silently when /health returns 200" {
    # Engram is running on 8788 per session-start hook confirmation
    run "$HOOK"
    [ "$status" -eq 0 ]
    [ -z "$output" ]
}

@test "unreachable: completes in under 3s when /health is unreachable" {
    # Port 19799 is unbound — curl will fail immediately (ECONNREFUSED)
    # Current code enters a 20s polling loop; fixed code must exit fast.
    local start end elapsed
    start=$(date +%s)
    ENGRAM_TEST_PORT=19799 run "$HOOK"
    end=$(date +%s)
    elapsed=$(( end - start ))
    [ "$status" -eq 0 ]
    [ "$elapsed" -lt 3 ] || {
        echo "Hook took ${elapsed}s — polling loop not removed" >&2
        return 1
    }
}

@test "unreachable: emits systemMessage JSON when /health is unreachable" {
    ENGRAM_TEST_PORT=19799 run "$HOOK"
    [ "$status" -eq 0 ]
    python3 -c "
import json, sys
try:
    d = json.loads('$output')
except Exception as e:
    print(f'output is not valid JSON: {e}', file=sys.stderr)
    sys.exit(1)
assert 'systemMessage' in d, f'no systemMessage key in {d}'
" || {
        echo "output was: $output" >&2
        return 1
    }
}

@test "never emits action:block regardless of reachability" {
    ENGRAM_TEST_PORT=19799 run "$HOOK"
    [ "$status" -eq 0 ]
    [[ "$output" != *'"action":"block"'* ]] || {
        echo "output contained action:block: $output" >&2
        return 1
    }
}

@test "never emits action:block when ENGRAM_DIR is missing" {
    # This tests the previously-existing code path that emitted action:block
    # when the project directory was not found. It must now emit systemMessage only.
    ENGRAM_TEST_PORT=19799 ENGRAM_TEST_DIR=/nonexistent/path run "$HOOK"
    [ "$status" -eq 0 ]
    [[ "$output" != *'"action":"block"'* ]] || {
        echo "output contained action:block for missing dir: $output" >&2
        return 1
    }
}
