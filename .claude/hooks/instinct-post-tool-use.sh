#!/usr/bin/env bash
# instinct-post-tool-use.sh.v2 — Tri-state dispatcher with auto-revert sentinel.
#
# Architecture:
#   INSTINCT_BACKEND=python|go|off  (default: python; Phase 3c flips to go)
#   INSTINCT_ENABLED=0              — hard kill switch, <50ms exit 0, no work done
#
# Auto-revert sentinel (Risk #1 mitigation):
#   If the go binary fails (rc!=0, timeout, or binary missing), sentinel file
#   ${BUFFER_DIR}/.go-broken is created. On subsequent invocations with
#   INSTINCT_BACKEND=go, if sentinel exists the hook FORCES python instead.
#   Sentinel is NOT auto-cleared — manual operator clearance is required:
#     rm ~/.local/state/instinct/.go-broken
#   This is intentional: the operator must confirm the issue is investigated
#   before re-enabling the go path.
#
# Safety contract:
#   - ALWAYS exit 0 to Claude Code. Every code path. Every failure mode.
#   - set -u ONLY. NOT -e, NOT pipefail. Explicit || true where needed.
#     (set -e would propagate failures past the safety wrappers)
#   - Buffer write block (lines 19-100 of live hook) preserved verbatim.
#
# Backend paths:
#   python: ~/projects/instinct/consolidator/.venv/bin/python -m instinct.run
#   go:     ~/bin/instinct-consolidate  (built by Track B Makefile)
#   off:    skip consolidator entirely; only buffer write happens
#
# Phase history:
#   Phase 1: created as .v2 sibling (this file); NOT the live hook
#   Phase 3a: installed as the live hook (Phase 3a step)
#   Phase 3c: default flipped from python to go

set -u  # NOT -e, NOT pipefail

# ---------------------------------------------------------------------------
# Kill switch — hard exit, <50ms, no work done
# ---------------------------------------------------------------------------
[[ "${INSTINCT_ENABLED:-1}" == "0" ]] && exit 0

# ---------------------------------------------------------------------------
# Configuration
# ---------------------------------------------------------------------------
BUFFER_DIR="${XDG_STATE_HOME:-$HOME/.local/state}/instinct"
BUFFER_FILE="$BUFFER_DIR/buffer.jsonl"
LOG_FILE="$BUFFER_DIR/run.log"
MAX_BUFFER_BYTES="${INSTINCT_MAX_BUFFER_BYTES:-1048576}"
MAX_BUFFER_EVENTS="${INSTINCT_MAX_BUFFER_EVENTS:-2000}"

# Tri-state backend selection. Phase 3c edit: change 'python' to 'go'.
BACKEND="${INSTINCT_BACKEND:-python}"
CONSOLIDATOR_TIMEOUT="${INSTINCT_CONSOLIDATOR_TIMEOUT:-30}"

# Sentinel file: presence forces auto-revert from go to python
SENTINEL="$BUFFER_DIR/.go-broken"

# ---------------------------------------------------------------------------
# Auto-revert: if sentinel exists and backend is go, force python
# ---------------------------------------------------------------------------
if [[ -f "$SENTINEL" && "$BACKEND" == "go" ]]; then
    BACKEND="python"
    # Log the auto-revert; use subshell with flock to avoid log corruption
    (
        if flock -w 0.1 -x 9; then
            echo "$(date -u +%Y-%m-%dT%H:%M:%SZ) instinct: auto-revert — sentinel present, forcing python backend" >> "$LOG_FILE"
        fi
    ) 9>"$BUFFER_DIR/.run.lock" || true
fi

# ---------------------------------------------------------------------------
# === BUFFER WRITE BLOCK (verbatim from live hook lines 19-100) ===
# Do NOT modify this block without diffing against the live hook.
# ---------------------------------------------------------------------------
mkdir -p "$BUFFER_DIR"

# Read stdin — contains the tool call JSON
raw_input=$(cat)

# Extract fields with python3 (available system-wide)
# Pipe raw_input via printf to avoid heredoc stdin conflict
parsed=$(printf '%s' "$raw_input" | python3 -c '
import json, sys, os, hashlib, datetime

raw = sys.stdin.read()
d = json.loads(raw)

tool_name = d.get("tool_name") or d.get("tool") or os.environ.get("CLAUDE_TOOL_NAME", "unknown")

# Allowlist: only capture meaningful write operations
ALLOWED = {"Edit", "Write", "Bash", "Task", "Agent"}
MCP_WRITE_KEYWORDS = ("store", "ingest", "write", "create", "update", "correct", "forget", "connect", "adopt")
is_mcp_write = tool_name.startswith("mcp__") and any(kw in tool_name for kw in MCP_WRITE_KEYWORDS)
if not any(tool_name.startswith(a) for a in ALLOWED) and not is_mcp_write:
    sys.exit(1)  # signal: skip this event

session_id = d.get("session_id") or os.environ.get("CLAUDE_SESSION_ID", "unknown")

# project_id: hash of git remote, fallback to project dir basename
project_dir = os.environ.get("CLAUDE_PROJECT_DIR", os.getcwd())
try:
    import subprocess
    remote = subprocess.check_output(
        ["git", "remote", "get-url", "origin"],
        cwd=project_dir, stderr=subprocess.DEVNULL
    ).decode().strip()
    project_id = hashlib.sha256(remote.encode()).hexdigest()[:12]
except Exception:
    project_id = os.path.basename(project_dir.rstrip("/"))

# Hash the full raw input for privacy
tool_input_hash = hashlib.sha256(raw.encode()).hexdigest()[:12]

# Output summary: pull from tool_response, truncate to 200 chars
resp = d.get("tool_response") or ""
if isinstance(resp, dict):
    resp = resp.get("text") or resp.get("content") or str(resp)
tool_output_summary = str(resp)[:200].replace("\n", " ")

event = {
    "timestamp": datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ"),
    "session_id": session_id,
    "project_id": project_id,
    "tool_name": tool_name,
    "tool_input_hash": tool_input_hash,
    "tool_output_summary": tool_output_summary,
    "exit_status": 0,
    "schema_version": 1,
}
print(json.dumps(event))
') || exit 0  # non-allowed tool or parse error: exit cleanly

# Append to buffer — flock serializes concurrent PostToolUse events (instinct#8)
(
    if flock -w 0.1 -x 9; then
        echo "$parsed" >> "$BUFFER_FILE"
        python3 - "$BUFFER_FILE" "$MAX_BUFFER_BYTES" "$MAX_BUFFER_EVENTS" <<'PYEOF'
import os, sys
path, max_bytes, max_events = sys.argv[1], int(sys.argv[2]), int(sys.argv[3])
try:
    with open(path, "r", encoding="utf-8") as f:
        lines = [line for line in f.read().splitlines() if line.strip()]
except FileNotFoundError:
    sys.exit(0)
if max_events > 0 and len(lines) > max_events:
    lines = lines[-max_events:]
while max_bytes > 0 and len(("\n".join(lines) + "\n").encode()) > max_bytes and lines:
    lines = lines[1:]
tmp = path + ".tmp"
with open(tmp, "w", encoding="utf-8") as f:
    if lines:
        f.write("\n".join(lines) + "\n")
os.replace(tmp, path)
PYEOF
    fi
) 9>"$BUFFER_DIR/.buffer.lock"
# === END BUFFER WRITE BLOCK ===

# ---------------------------------------------------------------------------
# Consolidator dispatch — tri-state
# ---------------------------------------------------------------------------
count=$(wc -l < "$BUFFER_FILE" 2>/dev/null || echo 0)
threshold="${INSTINCT_CONSOLIDATE_EVERY:-20}"

if (( count % threshold == 0 )); then
    case "$BACKEND" in

        off)
            # Off backend: skip consolidator entirely. Buffer write already done above.
            : ;;

        python)
            _consolidator="$HOME/projects/instinct/consolidator/.venv/bin/python"
            _consolidator_module="$HOME/projects/instinct/consolidator"
            if [[ -x "$_consolidator" ]]; then
                # Resolve ANTHROPIC_API_KEY: env var takes precedence, then known key file
                _api_key="${ANTHROPIC_API_KEY:-}"
                if [[ -z "$_api_key" ]]; then
                    _key_file="$HOME/.config/gmail-job-tracker/anthropic_api_key"
                    [[ -r "$_key_file" ]] && _api_key=$(tr -d '\n' < "$_key_file") || true
                fi
                (
                    flock -n 9 || exit 0
                    PYTHONPATH="$_consolidator_module" \
                        ANTHROPIC_API_KEY="$_api_key" \
                        "$_consolidator" -m instinct.run >> "$LOG_FILE" 2>&1
                ) 9>"$BUFFER_DIR/.run.lock" &
                disown $! || true
            else
                (
                    if flock -w 0.1 -x 9; then
                        echo "$(date -u +%Y-%m-%dT%H:%M:%SZ) instinct: WARN consolidator not found at $_consolidator — run: cd ~/projects/instinct/consolidator && uv sync" >> "$LOG_FILE"
                    fi
                ) 9>"$BUFFER_DIR/.run.lock" || true
            fi
            ;;

        go)
            _go_bin="$HOME/bin/instinct-consolidate"
            if [[ ! -x "$_go_bin" ]]; then
                touch "$SENTINEL" || true
                (
                    if flock -w 0.1 -x 9; then
                        echo "$(date -u +%Y-%m-%dT%H:%M:%SZ) instinct: go binary missing at $_go_bin — sentinel set; manual clearance: rm $SENTINEL" >> "$LOG_FILE"
                    fi
                ) 9>"$BUFFER_DIR/.run.lock" || true
            else
                # Resolve ANTHROPIC_API_KEY: env var takes precedence, then known key file
                _api_key="${ANTHROPIC_API_KEY:-}"
                if [[ -z "$_api_key" ]]; then
                    _key_file="$HOME/.config/gmail-job-tracker/anthropic_api_key"
                    [[ -r "$_key_file" ]] && _api_key=$(tr -d '\n' < "$_key_file") || true
                fi
                _rc=0
                # Option A: bounded wait for the dispatch lock. The previous
                # idiom `flock -n 9 || exit 0` inside this subshell swallowed
                # lock-busy as exit 0, which the outer `_rc=$?` then read as
                # success — masking the sentinel mechanism for a legitimate
                # failure mode (concurrent invocation already running). Using
                # `flock -w` waits briefly; if the lock cannot be acquired in
                # time, flock exits 1, the subshell propagates that, and the
                # outer code path correctly trips the sentinel. We do NOT use
                # background-and-disown here because the synchronous exit
                # code is the whole point of the sentinel mechanism for go.
                (
                    flock -w 0.5 -x 9 || exit 1
                    ANTHROPIC_API_KEY="$_api_key" \
                        timeout "$CONSOLIDATOR_TIMEOUT" "$_go_bin" >> "$LOG_FILE" 2>&1
                ) 9>"$BUFFER_DIR/.run.lock"
                _rc=$?
                # timeout exits 124 on kill; flock-busy exits 1; any non-zero
                # means failure → sentinel.
                if [[ $_rc -ne 0 ]]; then
                    touch "$SENTINEL" || true
                    (
                        if flock -w 0.1 -x 9; then
                            echo "$(date -u +%Y-%m-%dT%H:%M:%SZ) instinct: go binary failed rc=$_rc — sentinel set; manual clearance: rm $SENTINEL" >> "$LOG_FILE"
                        fi
                    ) 9>"$BUFFER_DIR/.run.lock" || true
                fi
            fi
            ;;

        *)
            # Unknown backend: log and skip. ALWAYS exit 0.
            (
                if flock -w 0.1 -x 9; then
                    echo "$(date -u +%Y-%m-%dT%H:%M:%SZ) instinct: unknown INSTINCT_BACKEND='$BACKEND' — skipping consolidator; valid: python|go|off" >> "$LOG_FILE"
                fi
            ) 9>"$BUFFER_DIR/.run.lock" || true
            ;;
    esac
fi

exit 0
