#!/usr/bin/env bash
# PostToolUse hook: capture tool calls to instinct buffer
set -euo pipefail

# Kill switch
[[ "${INSTINCT_ENABLED:-1}" == "0" ]] && exit 0

BUFFER_DIR="${XDG_STATE_HOME:-$HOME/.local/state}/instinct"
BUFFER_FILE="$BUFFER_DIR/buffer.jsonl"
LOG_FILE="$BUFFER_DIR/run.log"

mkdir -p "$BUFFER_DIR"
chmod 700 "$BUFFER_DIR"
touch "$BUFFER_FILE"
chmod 600 "$BUFFER_FILE"

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

import re
_REDACT = [
    # Most specific first — full var names before generic keyword match
    re.compile(r"(?i)(INFISICAL_CLIENT_(?:ID|SECRET)|ENGRAM_API_KEY|POSTGRES_PASSWORD)\s*=\s*\S+"),
    re.compile(r"(?i)postgres://[^@\s]+@\S+"),
    # Shell KEY=VALUE and HTTP header KEY: VALUE forms
    re.compile(r"(?i)\b(password|passwd|secret|api[_-]?key|token|bearer|auth\w*)\s*[=:]\s*\S+"),
    # JSON "key": "value" form
    re.compile(r'(?i)"(password|passwd|secret|api[_-]?key|token|bearer|auth\w*)"\s*:\s*"[^"]*"'),
    # JWTs: three base64url segments separated by dots
    re.compile(r"[A-Za-z0-9\-_]{20,}\.[A-Za-z0-9\-_]{20,}\.[A-Za-z0-9\-_]{20,}"),
    # Standard base64 and URL-safe base64 (GitHub PATs, Anthropic keys, etc.)
    re.compile(r"[A-Za-z0-9+/\-_]{40,}={0,2}"),
    # Uppercase or lowercase hex blobs ≥48 chars
    re.compile(r"[0-9a-fA-F]{48,}"),
]
def _redact(text):
    for pat in _REDACT:
        text = pat.sub("[REDACTED]", text)
    return text

# Redact before truncating — a secret straddling char 200 would evade patterns if truncated first
tool_output_summary = _redact(str(resp).replace("\n", " "))[:200]

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

# Append to buffer
echo "$parsed" >> "$BUFFER_FILE"

# Trigger consolidator every N events
count=$(wc -l < "$BUFFER_FILE" 2>/dev/null || echo 0)
threshold="${INSTINCT_CONSOLIDATE_EVERY:-20}"
if (( count % threshold == 0 )); then
    if command -v instinct &>/dev/null; then
        instinct >> "$LOG_FILE" 2>&1 &
        disown
    else
        echo "$(date -Iseconds) instinct binary not found on PATH — run hooks/install.sh" >> "$LOG_FILE"
    fi
fi

exit 0
