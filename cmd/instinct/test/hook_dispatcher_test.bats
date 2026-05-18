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

# HOOK_DEFAULT captured at parse time (real HOME, before setup overrides it)
HOOK_DEFAULT="$HOME/.claude/hooks/instinct-post-tool-use.sh.v2"

# Minimal valid PostToolUse payload for an allowlisted tool
EDIT_PAYLOAD='{"tool_name":"Edit","session_id":"sess-test","tool_response":"ok"}'
READ_PAYLOAD='{"tool_name":"Read","session_id":"sess-test","tool_response":"data"}'
BASH_PAYLOAD='{"tool_name":"Bash","session_id":"sess-test","tool_response":"done"}'
BAD_JSON_PAYLOAD='not-json-at-all'

setup() {
    # Each test gets a fresh isolated state dir and a fake HOME.
    # HOOK_PATH is resolved once and exported so setup() HOME override doesn't break it.
    export HOOK_PATH="${HOOK_PATH:-$HOOK_DEFAULT}"
    export XDG_STATE_HOME
    XDG_STATE_HOME="$(mktemp -d)"
    export HOME
    HOME="$(mktemp -d)"
    mkdir -p "$HOME/bin"
    mkdir -p "$HOME/.config/gmail-job-tracker"
    export CLAUDE_PROJECT_DIR="/tmp"
    export ANTHROPIC_API_KEY="test-key-not-real"
}

teardown() {
    rm -rf "$XDG_STATE_HOME" "$HOME"
}

# Path helpers inside the isolated state dir
state_dir()    { echo "${XDG_STATE_HOME}/instinct"; }
buffer_file()  { echo "${XDG_STATE_HOME}/instinct/buffer.jsonl"; }
sentinel_file(){ echo "${XDG_STATE_HOME}/instinct/.go-broken"; }
log_file()     { echo "${XDG_STATE_HOME}/instinct/run.log"; }

# Install a mock binary at $HOME/bin/<name> that does something controlled.
# Usage: install_mock_bin <name> <body>
install_mock_bin() {
    local name="$1"
    local body="$2"
    # shellcheck disable=SC2016
    printf '#!/usr/bin/env bash\n%s\n' "$body" > "$HOME/bin/$name"
    chmod +x "$HOME/bin/$name"
}

# Run the hook with a given stdin payload and optional extra env KEY=VALUE pairs.
# CRITICAL: env vars must be applied to the 'bash' side of the pipe, not printf side.
# Usage: run_hook <payload> [KEY=VALUE ...]
run_hook() {
    local payload="$1"
    shift
    # Build an env array for the bash invocation
    local -a extra_env=(
        HOME="$HOME"
        XDG_STATE_HOME="$XDG_STATE_HOME"
        CLAUDE_PROJECT_DIR="${CLAUDE_PROJECT_DIR:-/tmp}"
        ANTHROPIC_API_KEY="${ANTHROPIC_API_KEY:-test-key}"
    )
    for kv in "$@"; do
        extra_env+=("$kv")
    done
    printf '%s' "$payload" | env "${extra_env[@]}" bash "$HOOK_PATH"
}

# ---------------------------------------------------------------------------
# Sanity: hook file must exist before any other test runs
# ---------------------------------------------------------------------------

@test "HookFileExists — v2 hook is present at expected path" {
    [[ -f "$HOOK_PATH" ]] || {
        echo "HOOK_PATH=$HOOK_PATH does not exist — check that Phase 1 Track D wrote the file"
        return 1
    }
}

# ---------------------------------------------------------------------------
# Test 1: Kill switch exits immediately and in <100ms
# ---------------------------------------------------------------------------

@test "TestKillSwitchExitsImmediately — INSTINCT_ENABLED=0 exits 0 in <100ms" {
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
    # 100ms budget: accounts for process startup and env setup overhead
    [[ $elapsed_ms -lt 100 ]] || {
        echo "Kill switch took ${elapsed_ms}ms — expected <100ms (50ms spec + overhead)"
        return 1
    }
}

# ---------------------------------------------------------------------------
# Test 2: Kill switch writes NO buffer
# ---------------------------------------------------------------------------

@test "TestKillSwitchSkipsBufferWrite — INSTINCT_ENABLED=0 writes no buffer" {
    run_hook "$EDIT_PAYLOAD" "INSTINCT_ENABLED=0"

    if [[ -f "$(buffer_file)" ]]; then
        local count
        count=$(wc -l < "$(buffer_file)")
        [[ $count -eq 0 ]] || {
            echo "Buffer has $count lines after kill switch — expected 0"
            return 1
        }
    fi
    # Buffer file not existing is also correct
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

@test "TestPythonBackendDefault — unset INSTINCT_BACKEND selects python path, exit 0" {
    # Python venv won't exist in isolated HOME — that's fine, hook warns and moves on.
    # Key assertion: exit 0, no go sentinel.
    run_hook "$EDIT_PAYLOAD"
    local rc=$?

    [[ $rc -eq 0 ]] || {
        echo "Expected exit 0 for python backend (default), got $rc"
        return 1
    }
    [[ ! -f "$(sentinel_file)" ]] || {
        echo "Sentinel was set on default (python) backend invocation — unexpected"
        return 1
    }
}

# ---------------------------------------------------------------------------
# Test 5: Go backend — explicit, healthy binary, no sentinel
# ---------------------------------------------------------------------------

@test "TestGoBackendExplicit — INSTINCT_BACKEND=go with healthy binary exits 0, no sentinel" {
    local marker="$XDG_STATE_HOME/go-was-invoked"
    install_mock_bin "instinct-consolidate" "touch '$marker'; exit 0"

    # Force consolidation on every event so the go dispatch block is always reached
    run_hook "$EDIT_PAYLOAD" \
        "INSTINCT_BACKEND=go" \
        "INSTINCT_CONSOLIDATE_EVERY=1" \
        "PATH=$HOME/bin:/usr/bin:/bin"
    local rc=$?

    [[ $rc -eq 0 ]] || { echo "Expected exit 0, got $rc"; return 1; }
    # Sentinel must not exist after successful go invocation
    [[ ! -f "$(sentinel_file)" ]] || {
        echo "Sentinel wrongly set after successful go binary invocation"
        return 1
    }
    # Go binary marker should exist (consolidation was triggered)
    [[ -f "$marker" ]] || {
        echo "Go binary marker not found — go path may not have been taken"
        return 1
    }
}

# ---------------------------------------------------------------------------
# Test 6: off backend — buffer write happens, no consolidator
# ---------------------------------------------------------------------------

@test "TestOffBackend — INSTINCT_BACKEND=off does buffer write, no consolidator" {
    local go_marker="$XDG_STATE_HOME/go-was-invoked"
    install_mock_bin "instinct-consolidate" "touch '$go_marker'; exit 0"

    run_hook "$EDIT_PAYLOAD" \
        "INSTINCT_BACKEND=off" \
        "INSTINCT_CONSOLIDATE_EVERY=1" \
        "PATH=$HOME/bin:/usr/bin:/bin"
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
    run_hook "$EDIT_PAYLOAD" \
        "INSTINCT_BACKEND=go" \
        "INSTINCT_CONSOLIDATE_EVERY=1" \
        "PATH=$HOME/bin:/usr/bin:/bin"
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

    run_hook "$EDIT_PAYLOAD" \
        "INSTINCT_BACKEND=go" \
        "INSTINCT_CONSOLIDATE_EVERY=1" \
        "PATH=$HOME/bin:/usr/bin:/bin"
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
        "INSTINCT_CONSOLIDATE_EVERY=1" \
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
    # Should complete well under 6 seconds (timeout=2 + overhead)
    [[ $elapsed_ms -lt 6000 ]] || {
        echo "Hook took ${elapsed_ms}ms after 2s timeout — too slow"
        return 1
    }
}

# ---------------------------------------------------------------------------
# Test 10: Sentinel forces auto-revert to python when INSTINCT_BACKEND=go
# ---------------------------------------------------------------------------

@test "TestSentinelForcesAutoRevert — sentinel present + INSTINCT_BACKEND=go uses python, go NOT invoked" {
    local go_marker="$XDG_STATE_HOME/go-was-invoked"
    install_mock_bin "instinct-consolidate" "touch '$go_marker'; exit 0"

    # Pre-create sentinel AND state dir (hook won't create state dir before kill-switch check)
    mkdir -p "$(state_dir)"
    touch "$(sentinel_file)"

    run_hook "$EDIT_PAYLOAD" \
        "INSTINCT_BACKEND=go" \
        "INSTINCT_CONSOLIDATE_EVERY=1" \
        "PATH=$HOME/bin:/usr/bin:/bin"
    local rc=$?

    [[ $rc -eq 0 ]] || { echo "Expected exit 0, got $rc"; return 1; }
    # Go binary must NOT have been invoked (auto-revert forced python)
    [[ ! -f "$go_marker" ]] || {
        echo "Go binary was invoked despite sentinel (auto-revert failed)"
        return 1
    }
    # Log should contain auto-revert indication
    if [[ -f "$(log_file)" ]]; then
        grep -qE "auto-revert|sentinel|python" "$(log_file)" || {
            echo "No auto-revert log entry found in $(log_file)"
            return 1
        }
    fi
}

# ---------------------------------------------------------------------------
# Test 11: Sentinel does not affect python backend
# ---------------------------------------------------------------------------

@test "TestSentinelDoesNotAffectPython — sentinel + INSTINCT_BACKEND=python, exit 0" {
    mkdir -p "$(state_dir)"
    touch "$(sentinel_file)"

    run_hook "$EDIT_PAYLOAD" \
        "INSTINCT_BACKEND=python" \
        "PATH=$HOME/bin:/usr/bin:/bin"
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

    run_hook "$EDIT_PAYLOAD" \
        "INSTINCT_BACKEND=off" \
        "INSTINCT_CONSOLIDATE_EVERY=1" \
        "PATH=$HOME/bin:/usr/bin:/bin"
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
    run_hook "$EDIT_PAYLOAD" "INSTINCT_ENABLED=0" || { echo "FAIL: kill switch non-zero"; (( failures++ )); true; }

    # Case 2: missing go binary (EVERY=1 to trigger dispatch block)
    run_hook "$EDIT_PAYLOAD" \
        "INSTINCT_BACKEND=go" \
        "INSTINCT_CONSOLIDATE_EVERY=1" \
        "PATH=$HOME/bin:/usr/bin:/bin" || { echo "FAIL: missing go binary non-zero"; (( failures++ )); true; }

    # Case 3: go binary exits 1
    install_mock_bin "instinct-consolidate" "exit 1"
    run_hook "$EDIT_PAYLOAD" \
        "INSTINCT_BACKEND=go" \
        "INSTINCT_CONSOLIDATE_EVERY=1" \
        "PATH=$HOME/bin:/usr/bin:/bin" || { echo "FAIL: go exit 1 non-zero"; (( failures++ )); true; }

    # Case 4: go binary timeout
    install_mock_bin "instinct-consolidate" "sleep 60"
    run_hook "$EDIT_PAYLOAD" \
        "INSTINCT_BACKEND=go" \
        "INSTINCT_CONSOLIDATE_EVERY=1" \
        "INSTINCT_CONSOLIDATOR_TIMEOUT=1" \
        "PATH=$HOME/bin:/usr/bin:/bin" || { echo "FAIL: go timeout non-zero"; (( failures++ )); true; }

    # Case 5: unknown backend
    run_hook "$EDIT_PAYLOAD" \
        "INSTINCT_BACKEND=garbage" \
        "INSTINCT_CONSOLIDATE_EVERY=1" \
        "PATH=$HOME/bin:/usr/bin:/bin" || { echo "FAIL: unknown backend non-zero"; (( failures++ )); true; }

    # Case 6: malformed stdin
    run_hook "$BAD_JSON_PAYLOAD" "INSTINCT_BACKEND=python" || { echo "FAIL: malformed stdin non-zero"; (( failures++ )); true; }

    # Case 7: sentinel + go backend (auto-revert)
    mkdir -p "$(state_dir)"
    touch "$(sentinel_file)"
    run_hook "$EDIT_PAYLOAD" \
        "INSTINCT_BACKEND=go" \
        "INSTINCT_CONSOLIDATE_EVERY=1" \
        "PATH=$HOME/bin:/usr/bin:/bin" || { echo "FAIL: sentinel+go non-zero"; (( failures++ )); true; }
    rm -f "$(sentinel_file)"

    # Case 8: off backend
    run_hook "$EDIT_PAYLOAD" "INSTINCT_BACKEND=off" || { echo "FAIL: off backend non-zero"; (( failures++ )); true; }

    # Case 9: non-allowlist tool (exits 0, skips buffer)
    run_hook "$READ_PAYLOAD" "INSTINCT_BACKEND=python" || { echo "FAIL: non-allowlist tool non-zero"; (( failures++ )); true; }

    [[ $failures -eq 0 ]] || {
        echo "Total failure cases: $failures"
        return 1
    }
}

# ---------------------------------------------------------------------------
# Test 14: Unknown backend value
# ---------------------------------------------------------------------------

@test "TestUnknownBackendValue — INSTINCT_BACKEND=garbage exits 0, logs entry" {
    run_hook "$EDIT_PAYLOAD" \
        "INSTINCT_BACKEND=garbage" \
        "INSTINCT_CONSOLIDATE_EVERY=1" \
        "PATH=$HOME/bin:/usr/bin:/bin"
    local rc=$?

    [[ $rc -eq 0 ]] || {
        echo "Expected exit 0 for unknown backend, got $rc"
        return 1
    }
    # Log should warn about unknown backend
    if [[ -f "$(log_file)" ]]; then
        grep -qiE "unknown|invalid|unsupported|garbage" "$(log_file)" || {
            echo "No warning log entry for unknown backend — expected grep match in $(log_file)"
            return 1
        }
    fi
}

# ---------------------------------------------------------------------------
# Test 15: Buffer write happens before consolidator dispatch
# ---------------------------------------------------------------------------

@test "TestBufferWriteHappensBeforeConsolidator — buffer populated even with off backend" {
    # off backend guarantees no consolidator runs; buffer must still be written
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
            echo "Buffer has $count lines for non-allowlist tool (Read) — expected 0"
            return 1
        }
    fi
}

# ---------------------------------------------------------------------------
# Test 17: Sentinel manual clearance restores go path
# ---------------------------------------------------------------------------

@test "TestSentinelManualClearance — rm sentinel lets go binary succeed without new sentinel" {
    local go_marker="$XDG_STATE_HOME/go-was-invoked"
    install_mock_bin "instinct-consolidate" "touch '$go_marker'; exit 0"

    # Create then manually remove sentinel (simulates operator clearance)
    mkdir -p "$(state_dir)"
    touch "$(sentinel_file)"
    rm -f "$(sentinel_file)"

    # CONSOLIDATE_EVERY=1 so go dispatch runs on first event
    run_hook "$EDIT_PAYLOAD" \
        "INSTINCT_BACKEND=go" \
        "INSTINCT_CONSOLIDATE_EVERY=1" \
        "PATH=$HOME/bin:/usr/bin:/bin"
    local rc=$?

    [[ $rc -eq 0 ]] || { echo "Expected exit 0, got $rc"; return 1; }
    # Go binary should have been called and succeeded → no new sentinel
    [[ ! -f "$(sentinel_file)" ]] || {
        echo "New sentinel appeared after manual clearance — go binary not healthy"
        return 1
    }
    # Go marker confirms the go path was actually taken
    [[ -f "$go_marker" ]] || {
        echo "Go binary marker missing — go path may not have run"
        return 1
    }
}

# ---------------------------------------------------------------------------
# Test 18: Concurrent invocations — 5 parallel hooks, all exit 0, buffer has 5 events
# ---------------------------------------------------------------------------

@test "TestConcurrentInvocations — 5 parallel invocations all exit 0, buffer has 5 events" {
    local pids=()

    # Launch 5 background hook invocations with off backend (no consolidator to slow them)
    for i in $(seq 1 5); do
        (
            printf '%s' "$EDIT_PAYLOAD" | \
                env HOME="$HOME" XDG_STATE_HOME="$XDG_STATE_HOME" \
                    CLAUDE_PROJECT_DIR="/tmp" \
                    ANTHROPIC_API_KEY="test-key" \
                    INSTINCT_BACKEND=off \
                    bash "$HOOK_PATH"
            echo $? > "$XDG_STATE_HOME/exit_code_$i"
        ) &
        pids+=($!)
    done

    # Wait for all background jobs
    for pid in "${pids[@]}"; do
        wait "$pid" || true
    done

    # Verify all exited 0
    local all_ok=1
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

    # Buffer must contain exactly 5 events (flock ensures no torn writes)
    local count=0
    if [[ -f "$(buffer_file)" ]]; then
        count=$(wc -l < "$(buffer_file)")
    fi
    [[ $count -eq 5 ]] || {
        echo "Buffer has $count events after 5 concurrent invocations — expected 5"
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
    # Buffer should be empty (parse error → skip cleanly via || exit 0)
    if [[ -f "$(buffer_file)" ]]; then
        local count
        count=$(wc -l < "$(buffer_file)")
        [[ $count -eq 0 ]] || {
            echo "Buffer has $count lines despite malformed stdin — expected 0"
            return 1
        }
    fi
}

# ---------------------------------------------------------------------------
# Test 20: Bash tool is in allowlist — buffer write confirmed
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
