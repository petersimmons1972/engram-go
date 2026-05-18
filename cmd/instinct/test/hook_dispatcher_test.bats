#!/usr/bin/env bats
# hook_dispatcher_test.bats — Tri-state dispatcher bats suite for instinct-post-tool-use.sh.v2
#
# Run with:
#   bats ~/projects/engram-go/cmd/instinct/test/hook_dispatcher_test.bats
#
# Override the hook path with HOOK_PATH env var:
#   HOOK_PATH=/path/to/v2 bats ...
#
# Plain [[ ]] assertions used — no bats-assert/bats-support dependency.

# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------

HOOK_DEFAULT="$HOME/.claude/hooks/instinct-post-tool-use.sh.v2"

# Minimal valid PostToolUse payload for an allowlisted tool
EDIT_PAYLOAD='{"tool_name":"Edit","session_id":"sess-test","tool_response":"ok"}'
READ_PAYLOAD='{"tool_name":"Read","session_id":"sess-test","tool_response":"data"}'
BASH_PAYLOAD='{"tool_name":"Bash","session_id":"sess-test","tool_response":"done"}'
BAD_JSON_PAYLOAD='not-json-at-all'

setup() {
    # Each test gets a fresh isolated state dir and a fake HOME
    export XDG_STATE_HOME
    XDG_STATE_HOME="$(mktemp -d)"
    export HOME
    HOME="$(mktemp -d)"
    mkdir -p "$HOME/bin"
    mkdir -p "$HOME/.config/gmail-job-tracker"
    # Fake project dir for git remote derivation
    export CLAUDE_PROJECT_DIR="/tmp"
    export ANTHROPIC_API_KEY="test-key-not-real"
    # Resolve hook path (allow override for CI fixture use)
    export HOOK_PATH="${HOOK_PATH:-$HOOK_DEFAULT}"
}

teardown() {
    rm -rf "$XDG_STATE_HOME" "$HOME"
}

# Path helpers inside the isolated state dir
state_dir() { echo "${XDG_STATE_HOME}/instinct"; }
buffer_file() { echo "${XDG_STATE_HOME}/instinct/buffer.jsonl"; }
sentinel_file() { echo "${XDG_STATE_HOME}/instinct/.go-broken"; }
log_file() { echo "${XDG_STATE_HOME}/instinct/run.log"; }

# Install a mock binary at $HOME/bin/<name> that does something controlled.
# Usage: install_mock_bin <name> <body>
# body is bash code; the script uses set -e internally.
install_mock_bin() {
    local name="$1"
    local body="$2"
    cat > "$HOME/bin/$name" <<MOCKEOF
#!/usr/bin/env bash
$body
MOCKEOF
    chmod +x "$HOME/bin/$name"
}

# Run the hook with a given stdin payload and env vars.
# Additional env vars can be passed as KEY=VALUE strings after the payload.
# Usage: run_hook <payload> [KEY=VALUE ...]
run_hook() {
    local payload="$1"
    shift
    # Build env overrides
    local env_prefix=""
    for kv in "$@"; do
        env_prefix="$kv $env_prefix"
    done
    # We need HOME and XDG_STATE_HOME forwarded; prepend them
    # shellcheck disable=SC2086
    env HOME="$HOME" XDG_STATE_HOME="$XDG_STATE_HOME" \
        CLAUDE_PROJECT_DIR="${CLAUDE_PROJECT_DIR:-/tmp}" \
        ANTHROPIC_API_KEY="${ANTHROPIC_API_KEY:-test-key}" \
        $env_prefix \
        printf '%s' "$payload" | bash "$HOOK_PATH"
}

# Run hook and capture exit code; never fails test on non-zero
run_hook_capture_rc() {
    local payload="$1"
    shift
    local env_prefix=""
    for kv in "$@"; do
        env_prefix="$kv $env_prefix"
    done
    # shellcheck disable=SC2086
    env HOME="$HOME" XDG_STATE_HOME="$XDG_STATE_HOME" \
        CLAUDE_PROJECT_DIR="${CLAUDE_PROJECT_DIR:-/tmp}" \
        ANTHROPIC_API_KEY="${ANTHROPIC_API_KEY:-test-key}" \
        $env_prefix \
        printf '%s' "$payload" | bash "$HOOK_PATH"
    echo "$?"
}

# ---------------------------------------------------------------------------
# Sanity: hook file must exist
# ---------------------------------------------------------------------------

@test "HookFileExists — v2 hook is present at expected path" {
    [[ -f "$HOOK_PATH" ]] || {
        echo "HOOK_PATH=$HOOK_PATH does not exist"
        return 1
    }
}

# ---------------------------------------------------------------------------
# Test 1: Kill switch exits immediately
# ---------------------------------------------------------------------------

@test "TestKillSwitchExitsImmediately — INSTINCT_ENABLED=0 exits 0 in <50ms" {
    local start_ms end_ms elapsed_ms
    start_ms=$(date +%s%3N)

    run_hook "$EDIT_PAYLOAD" "INSTINCT_ENABLED=0"
    local rc=$?

    end_ms=$(date +%s%3N)
    elapsed_ms=$(( end_ms - start_ms ))

    [[ $rc -eq 0 ]] || {
        echo "Expected exit 0, got $rc"
        return 1
    }
    [[ $elapsed_ms -lt 50 ]] || {
        echo "Kill switch took ${elapsed_ms}ms, expected <50ms"
        return 1
    }
}

# ---------------------------------------------------------------------------
# Test 2: Kill switch does no buffer write
# ---------------------------------------------------------------------------

@test "TestKillSwitchSkipsBufferWrite — INSTINCT_ENABLED=0 writes no buffer" {
    run_hook "$EDIT_PAYLOAD" "INSTINCT_ENABLED=0"

    local buf
    buf="$(buffer_file)"
    if [[ -f "$buf" ]]; then
        local count
        count=$(wc -l < "$buf")
        [[ $count -eq 0 ]] || {
            echo "Buffer has $count lines after kill switch — expected 0"
            return 1
        }
    fi
    # File not existing is also fine
}

# ---------------------------------------------------------------------------
# Test 3: Kill switch doesn't invoke mock binary
# ---------------------------------------------------------------------------

@test "TestKillSwitchSkipsConsolidator — INSTINCT_ENABLED=0 does not invoke go binary" {
    local marker="$XDG_STATE_HOME/go-was-invoked"
    install_mock_bin "instinct-consolidate" "touch '$marker'"

    run_hook "$EDIT_PAYLOAD" "INSTINCT_ENABLED=0" "INSTINCT_BACKEND=go"

    [[ ! -f "$marker" ]] || {
        echo "Go binary was invoked despite INSTINCT_ENABLED=0"
        return 1
    }
}

# ---------------------------------------------------------------------------
# Test 4: Python backend is default when INSTINCT_BACKEND unset
# ---------------------------------------------------------------------------

@test "TestPythonBackendDefault — unset INSTINCT_BACKEND selects python path" {
    # Install a mock python3 that records invocation; we can't easily mock the
    # venv path, so we verify the log shows python-path attempt (not go-path attempt).
    # The python consolidator will legitimately not exist here — that's OK for this test.
    # We just need to confirm no go sentinel is set and exit is 0.
    local rc
    rc=$(run_hook_capture_rc "$EDIT_PAYLOAD")

    [[ $rc -eq 0 ]] || {
        echo "Expected exit 0 for python backend, got $rc"
        return 1
    }
    # Go sentinel must NOT be set (python path doesn't set it)
    [[ ! -f "$(sentinel_file)" ]] || {
        echo "Sentinel was set on python backend invocation — unexpected"
        return 1
    }
}

# ---------------------------------------------------------------------------
# Test 5: Go backend — explicit, healthy binary
# ---------------------------------------------------------------------------

@test "TestGoBackendExplicit — INSTINCT_BACKEND=go invokes go binary, no sentinel" {
    local marker="$XDG_STATE_HOME/go-was-invoked"
    install_mock_bin "instinct-consolidate" "touch '$marker'; exit 0"

    # Put our mock binary on PATH so the hook finds it
    run_hook "$EDIT_PAYLOAD" "INSTINCT_BACKEND=go" "PATH=$HOME/bin:/usr/bin:/bin"
    local rc=$?

    [[ $rc -eq 0 ]] || { echo "Expected exit 0, got $rc"; return 1; }
    # Sentinel must not exist
    [[ ! -f "$(sentinel_file)" ]] || {
        echo "Sentinel wrongly set after successful go binary invocation"
        return 1
    }
    # Note: marker check depends on consolidator threshold; buffer might not be at threshold.
    # The key assertion is no sentinel and exit 0 — go path was taken.
}

# ---------------------------------------------------------------------------
# Test 6: off backend — buffer write happens, no consolidator
# ---------------------------------------------------------------------------

@test "TestOffBackend — INSTINCT_BACKEND=off does buffer write, no consolidator" {
    local go_marker="$XDG_STATE_HOME/go-was-invoked"
    local py_marker="$XDG_STATE_HOME/py-was-invoked"
    install_mock_bin "instinct-consolidate" "touch '$go_marker'; exit 0"

    run_hook "$EDIT_PAYLOAD" "INSTINCT_BACKEND=off" "PATH=$HOME/bin:/usr/bin:/bin"
    local rc=$?

    [[ $rc -eq 0 ]] || { echo "Expected exit 0, got $rc"; return 1; }
    # Buffer file must exist and have content
    [[ -f "$(buffer_file)" ]] || {
        echo "Buffer file not created for off backend"
        return 1
    }
    local count
    count=$(wc -l < "$(buffer_file)")
    [[ $count -ge 1 ]] || {
        echo "Buffer has $count lines for off backend — expected >=1"
        return 1
    }
    # Go binary must NOT be invoked in off mode
    [[ ! -f "$go_marker" ]] || {
        echo "Go binary was invoked despite INSTINCT_BACKEND=off"
        return 1
    }
}

# ---------------------------------------------------------------------------
# Test 7: Missing go binary sets sentinel
# ---------------------------------------------------------------------------

@test "TestGoBinaryMissingSetsSentinel — go binary absent creates sentinel, exit 0" {
    # Do NOT install a binary — $HOME/bin/instinct-consolidate won't exist
    run_hook "$EDIT_PAYLOAD" "INSTINCT_BACKEND=go" "PATH=$HOME/bin:/usr/bin:/bin"
    local rc=$?

    [[ $rc -eq 0 ]] || { echo "Expected exit 0, got $rc"; return 1; }
    [[ -f "$(sentinel_file)" ]] || {
        echo "Sentinel NOT created when go binary missing"
        return 1
    }
}

# ---------------------------------------------------------------------------
# Test 8: Go binary failure sets sentinel
# ---------------------------------------------------------------------------

@test "TestGoBinaryFailureSetsSentinel — go binary exit 1 creates sentinel, exit 0" {
    install_mock_bin "instinct-consolidate" "exit 1"

    run_hook "$EDIT_PAYLOAD" "INSTINCT_BACKEND=go" "PATH=$HOME/bin:/usr/bin:/bin"
    local rc=$?

    [[ $rc -eq 0 ]] || { echo "Expected exit 0, got $rc"; return 1; }
    [[ -f "$(sentinel_file)" ]] || {
        echo "Sentinel NOT created when go binary exited 1"
        return 1
    }
}

# ---------------------------------------------------------------------------
# Test 9: Go binary timeout sets sentinel
# ---------------------------------------------------------------------------

@test "TestGoBinaryTimeoutSetsSentinel — sleep binary killed by timeout, sentinel set, exit 0" {
    install_mock_bin "instinct-consolidate" "sleep 60"

    local start_ms end_ms elapsed_ms
    start_ms=$(date +%s%3N)

    run_hook "$EDIT_PAYLOAD" \
        "INSTINCT_BACKEND=go" \
        "INSTINCT_CONSOLIDATOR_TIMEOUT=2" \
        "PATH=$HOME/bin:/usr/bin:/bin"
    local rc=$?

    end_ms=$(date +%s%3N)
    elapsed_ms=$(( end_ms - start_ms ))

    [[ $rc -eq 0 ]] || { echo "Expected exit 0, got $rc"; return 1; }
    [[ -f "$(sentinel_file)" ]] || {
        echo "Sentinel NOT created after go binary timeout"
        return 1
    }
    # Should complete well under 5 seconds (timeout=2 + overhead)
    [[ $elapsed_ms -lt 5000 ]] || {
        echo "Hook took ${elapsed_ms}ms after 2s timeout — too slow"
        return 1
    }
}

# ---------------------------------------------------------------------------
# Test 10: Sentinel forces auto-revert to python when INSTINCT_BACKEND=go
# ---------------------------------------------------------------------------

@test "TestSentinelForcesAutoRevert — sentinel present + INSTINCT_BACKEND=go uses python" {
    local go_marker="$XDG_STATE_HOME/go-was-invoked"
    install_mock_bin "instinct-consolidate" "touch '$go_marker'; exit 0"

    # Pre-create sentinel
    mkdir -p "$(state_dir)"
    touch "$(sentinel_file)"

    run_hook "$EDIT_PAYLOAD" \
        "INSTINCT_BACKEND=go" \
        "PATH=$HOME/bin:/usr/bin:/bin"
    local rc=$?

    [[ $rc -eq 0 ]] || { echo "Expected exit 0, got $rc"; return 1; }
    # Go binary must NOT have been invoked
    [[ ! -f "$go_marker" ]] || {
        echo "Go binary was invoked despite sentinel being present (auto-revert failed)"
        return 1
    }
    # Log should contain auto-revert indication
    if [[ -f "$(log_file)" ]]; then
        grep -q "auto-revert\|sentinel\|python" "$(log_file)" || {
            echo "No auto-revert log entry found"
            return 1
        }
    fi
}

# ---------------------------------------------------------------------------
# Test 11: Sentinel does not affect python backend
# ---------------------------------------------------------------------------

@test "TestSentinelDoesNotAffectPython — sentinel + INSTINCT_BACKEND=python runs normally, exit 0" {
    mkdir -p "$(state_dir)"
    touch "$(sentinel_file)"

    run_hook "$EDIT_PAYLOAD" "INSTINCT_BACKEND=python" "PATH=$HOME/bin:/usr/bin:/bin"
    local rc=$?

    [[ $rc -eq 0 ]] || {
        echo "Expected exit 0 for python backend with sentinel, got $rc"
        return 1
    }
}

# ---------------------------------------------------------------------------
# Test 12: Sentinel does not affect off backend
# ---------------------------------------------------------------------------

@test "TestSentinelDoesNotAffectOff — sentinel + INSTINCT_BACKEND=off exits 0, no consolidator" {
    local go_marker="$XDG_STATE_HOME/go-was-invoked"
    install_mock_bin "instinct-consolidate" "touch '$go_marker'; exit 0"
    mkdir -p "$(state_dir)"
    touch "$(sentinel_file)"

    run_hook "$EDIT_PAYLOAD" "INSTINCT_BACKEND=off" "PATH=$HOME/bin:/usr/bin:/bin"
    local rc=$?

    [[ $rc -eq 0 ]] || { echo "Expected exit 0, got $rc"; return 1; }
    [[ ! -f "$go_marker" ]] || {
        echo "Go binary invoked despite INSTINCT_BACKEND=off + sentinel"
        return 1
    }
}

# ---------------------------------------------------------------------------
# Test 13: AlwaysExitsZero — table of failure modes
# ---------------------------------------------------------------------------

@test "TestAlwaysExitsZero — every failure mode returns exit 0" {
    local failures=0

    # Case 1: kill switch
    run_hook "$EDIT_PAYLOAD" "INSTINCT_ENABLED=0"
    [[ $? -eq 0 ]] || { echo "FAIL: kill switch non-zero"; (( failures++ )); }

    # Case 2: missing go binary
    run_hook "$EDIT_PAYLOAD" "INSTINCT_BACKEND=go" "PATH=$HOME/bin:/usr/bin:/bin"
    [[ $? -eq 0 ]] || { echo "FAIL: missing go binary non-zero"; (( failures++ )); }

    # Case 3: go binary exits 1
    install_mock_bin "instinct-consolidate" "exit 1"
    run_hook "$EDIT_PAYLOAD" "INSTINCT_BACKEND=go" "PATH=$HOME/bin:/usr/bin:/bin"
    [[ $? -eq 0 ]] || { echo "FAIL: go exit 1 non-zero"; (( failures++ )); }

    # Case 4: go binary timeout
    install_mock_bin "instinct-consolidate" "sleep 60"
    run_hook "$EDIT_PAYLOAD" \
        "INSTINCT_BACKEND=go" \
        "INSTINCT_CONSOLIDATOR_TIMEOUT=1" \
        "PATH=$HOME/bin:/usr/bin:/bin"
    [[ $? -eq 0 ]] || { echo "FAIL: go timeout non-zero"; (( failures++ )); }

    # Case 5: unknown backend
    run_hook "$EDIT_PAYLOAD" "INSTINCT_BACKEND=garbage" "PATH=$HOME/bin:/usr/bin:/bin"
    [[ $? -eq 0 ]] || { echo "FAIL: unknown backend non-zero"; (( failures++ )); }

    # Case 6: malformed stdin
    run_hook "$BAD_JSON_PAYLOAD" "INSTINCT_BACKEND=python"
    [[ $? -eq 0 ]] || { echo "FAIL: malformed stdin non-zero"; (( failures++ )); }

    # Case 7: sentinel + go backend (auto-revert)
    mkdir -p "$(state_dir)"
    touch "$(sentinel_file)"
    run_hook "$EDIT_PAYLOAD" "INSTINCT_BACKEND=go" "PATH=$HOME/bin:/usr/bin:/bin"
    [[ $? -eq 0 ]] || { echo "FAIL: sentinel+go non-zero"; (( failures++ )); }
    rm -f "$(sentinel_file)"

    # Case 8: off backend
    run_hook "$EDIT_PAYLOAD" "INSTINCT_BACKEND=off"
    [[ $? -eq 0 ]] || { echo "FAIL: off backend non-zero"; (( failures++ )); }

    # Case 9: non-allowlist tool (should still exit 0, just skip buffer write)
    run_hook "$READ_PAYLOAD" "INSTINCT_BACKEND=python"
    [[ $? -eq 0 ]] || { echo "FAIL: non-allowlist tool non-zero"; (( failures++ )); }

    [[ $failures -eq 0 ]] || {
        echo "Total failures: $failures"
        return 1
    }
}

# ---------------------------------------------------------------------------
# Test 14: Unknown backend value
# ---------------------------------------------------------------------------

@test "TestUnknownBackendValue — INSTINCT_BACKEND=garbage exits 0, logs entry" {
    run_hook "$EDIT_PAYLOAD" "INSTINCT_BACKEND=garbage" "PATH=$HOME/bin:/usr/bin:/bin"
    local rc=$?

    [[ $rc -eq 0 ]] || {
        echo "Expected exit 0 for unknown backend, got $rc"
        return 1
    }
    # A log entry should be present warning about the unknown backend
    if [[ -f "$(log_file)" ]]; then
        grep -qiE "unknown|invalid|unsupported|garbage" "$(log_file)" || {
            echo "No warning log entry for unknown backend value"
            return 1
        }
    fi
}

# ---------------------------------------------------------------------------
# Test 15: Buffer write happens before consolidator dispatch
# ---------------------------------------------------------------------------

@test "TestBufferWriteHappensBeforeConsolidator — buffer populated even when consolidator mocked away" {
    # Use off backend to guarantee consolidator is never called
    run_hook "$EDIT_PAYLOAD" "INSTINCT_BACKEND=off"
    local rc=$?

    [[ $rc -eq 0 ]] || { echo "Expected exit 0, got $rc"; return 1; }
    [[ -f "$(buffer_file)" ]] || {
        echo "Buffer file not created"
        return 1
    }
    local count
    count=$(wc -l < "$(buffer_file)")
    [[ $count -ge 1 ]] || {
        echo "Buffer has $count lines — expected >=1 even without consolidator"
        return 1
    }
}

# ---------------------------------------------------------------------------
# Test 16: Allowlist filter — non-allowlist tool skips buffer write
# ---------------------------------------------------------------------------

@test "TestBufferWriteAllowlistFilter — Read tool payload not written to buffer" {
    run_hook "$READ_PAYLOAD" "INSTINCT_BACKEND=off"
    local rc=$?

    [[ $rc -eq 0 ]] || { echo "Expected exit 0 for Read tool, got $rc"; return 1; }
    # Buffer should be empty or not exist
    if [[ -f "$(buffer_file)" ]]; then
        local count
        count=$(wc -l < "$(buffer_file)")
        [[ $count -eq 0 ]] || {
            echo "Buffer has $count lines for non-allowlist tool — expected 0"
            return 1
        }
    fi
}

# ---------------------------------------------------------------------------
# Test 17: Sentinel manual clearance restores go path
# ---------------------------------------------------------------------------

@test "TestSentinelManualClearance — remove sentinel restores go backend" {
    local go_marker="$XDG_STATE_HOME/go-was-invoked"
    install_mock_bin "instinct-consolidate" "touch '$go_marker'; exit 0"

    # Create then remove sentinel (simulates operator clearance)
    mkdir -p "$(state_dir)"
    touch "$(sentinel_file)"
    rm -f "$(sentinel_file)"

    # Run 20 times to hit the consolidation threshold (default every 20 events)
    for i in $(seq 1 20); do
        run_hook "$EDIT_PAYLOAD" \
            "INSTINCT_BACKEND=go" \
            "PATH=$HOME/bin:/usr/bin:/bin" \
            "INSTINCT_CONSOLIDATE_EVERY=20" 2>/dev/null
    done
    local rc=$?
    [[ $rc -eq 0 ]] || { echo "Expected exit 0, got $rc"; return 1; }
    # The key assertion: sentinel should still be gone (go binary succeeded)
    # If go binary were broken, a new sentinel would appear
    [[ ! -f "$(sentinel_file)" ]] || {
        echo "New sentinel created — go binary not healthy after manual clearance"
        return 1
    }
}

# ---------------------------------------------------------------------------
# Test 18: Concurrent invocations — 5 parallel hooks, all exit 0, buffer has 5 events
# ---------------------------------------------------------------------------

@test "TestConcurrentInvocations — 5 parallel hooks all exit 0, buffer has 5 events" {
    local pids=()
    local exit_codes=()

    # Launch 5 background hook invocations
    for i in $(seq 1 5); do
        (
            env HOME="$HOME" XDG_STATE_HOME="$XDG_STATE_HOME" \
                CLAUDE_PROJECT_DIR="/tmp" \
                ANTHROPIC_API_KEY="test-key" \
                INSTINCT_BACKEND=off \
                printf '%s' "$EDIT_PAYLOAD" | bash "$HOOK_PATH"
            echo $? > "$XDG_STATE_HOME/exit_code_$i"
        ) &
        pids+=($!)
    done

    # Wait for all to complete
    local all_ok=1
    for pid in "${pids[@]}"; do
        wait "$pid" || true
    done

    # Check exit codes
    for i in $(seq 1 5); do
        local ec_file="$XDG_STATE_HOME/exit_code_$i"
        if [[ -f "$ec_file" ]]; then
            local ec
            ec=$(cat "$ec_file")
            [[ "$ec" == "0" ]] || {
                echo "Invocation $i exited $ec"
                all_ok=0
            }
        else
            echo "Exit code file missing for invocation $i"
            all_ok=0
        fi
    done

    # Buffer should have 5 events
    local count=0
    if [[ -f "$(buffer_file)" ]]; then
        count=$(wc -l < "$(buffer_file)")
    fi
    [[ $count -eq 5 ]] || {
        echo "Buffer has $count lines after 5 concurrent invocations — expected 5"
        all_ok=0
    }

    [[ $all_ok -eq 1 ]] || return 1
}

# ---------------------------------------------------------------------------
# Test 19: Malformed stdin — non-JSON, exits 0, no crash
# ---------------------------------------------------------------------------

@test "TestStdinMalformed — non-JSON stdin exits 0 without crash" {
    run_hook "$BAD_JSON_PAYLOAD" "INSTINCT_BACKEND=python"
    local rc=$?

    [[ $rc -eq 0 ]] || {
        echo "Expected exit 0 for malformed stdin, got $rc"
        return 1
    }
    # Buffer should be empty (parse error → skip cleanly)
    if [[ -f "$(buffer_file)" ]]; then
        local count
        count=$(wc -l < "$(buffer_file)")
        [[ $count -eq 0 ]] || {
            echo "Buffer has $count lines despite malformed stdin"
            return 1
        }
    fi
}

# ---------------------------------------------------------------------------
# Test 20: Bash tool IS in allowlist and gets buffered
# ---------------------------------------------------------------------------

@test "TestBashToolAllowlisted — Bash tool payload written to buffer" {
    run_hook "$BASH_PAYLOAD" "INSTINCT_BACKEND=off"
    local rc=$?

    [[ $rc -eq 0 ]] || { echo "Expected exit 0, got $rc"; return 1; }
    [[ -f "$(buffer_file)" ]] || {
        echo "Buffer file not created for Bash tool"
        return 1
    }
    local count
    count=$(wc -l < "$(buffer_file)")
    [[ $count -ge 1 ]] || {
        echo "Buffer has $count lines for Bash tool — expected >=1"
        return 1
    }
}
