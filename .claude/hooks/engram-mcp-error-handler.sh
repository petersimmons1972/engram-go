#!/usr/bin/env bash
# PostToolUse hook: catch Engram MCP connection failures and self-heal.
# Fires on any mcp__engram__* tool call. Silent on success.
# On error: writes tool input to fallback.md and injects a systemMessage.
# This makes Engram MCP errors 100% self-healing — no Claude intervention needed.

set -euo pipefail

FALLBACK="$HOME/.claude/projects/-home-psimmons/memory/fallback.md"

# Read stdin — tool call JSON (same format as instinct-post-tool-use.sh)
raw_input=$(cat)

# Check if the tool response contains an MCP error
is_error=$(printf '%s' "$raw_input" | python3 -c '
import json, sys

raw = sys.stdin.read()
try:
    d = json.loads(raw)
except Exception:
    sys.exit(1)

resp = d.get("tool_response") or ""
if isinstance(resp, dict):
    resp = str(resp)

error_patterns = ["MCP error", "Connection closed", "connection closed", "ECONNRESET", "-32000", "-32603"]
if any(p in resp for p in error_patterns):
    print("yes")
else:
    print("no")
' 2>/dev/null || echo "no")

[[ "$is_error" != "yes" ]] && exit 0

# Extract useful context from tool_input to write to fallback.md
fallback_entry=$(printf '%s' "$raw_input" | python3 -c '
import json, sys, datetime

raw = sys.stdin.read()
try:
    d = json.loads(raw)
except Exception:
    sys.exit(1)

tool_name = d.get("tool_name", "unknown")
inp = d.get("tool_input") or {}
if isinstance(inp, str):
    try:
        inp = json.loads(inp)
    except Exception:
        pass

today = datetime.date.today().isoformat()
content = inp.get("content", "") if isinstance(inp, dict) else str(inp)
project = inp.get("project", "unknown") if isinstance(inp, dict) else "unknown"
memory_type = inp.get("memory_type", "context") if isinstance(inp, dict) else "context"
tags = inp.get("tags", []) if isinstance(inp, dict) else []

# Only write if there is meaningful content to preserve
if not content:
    print("")
    sys.exit(0)

print(f"""## [{today}] Auto-captured from failed {tool_name}
**Project:** {project}
**Type:** {memory_type}
**Tags:** {tags}

{content}""")
' 2>/dev/null || true)

# Write to fallback.md if we have content to preserve
if [[ -n "$fallback_entry" ]]; then
    # flock + atomic write to avoid race with engram-flush-fallback.sh (#394)
    python3 - "$FALLBACK" "$fallback_entry" <<'PYEOF'
import sys, os, tempfile, fcntl, re

path = sys.argv[1]
entry = sys.argv[2]
lock_path = path + ".lock"

with open(lock_path, "w") as lf:
    fcntl.flock(lf, fcntl.LOCK_EX)

    try:
        with open(path) as f:
            content = f.read()
    except FileNotFoundError:
        content = ""

    # Count existing entries — refuse to append if backlog is too large (#401)
    entry_count = len(re.findall(r'^## \[\d{4}-\d{2}-\d{2}\]', content, re.MULTILINE))
    if entry_count >= 50:
        sys.stderr.write(f'[engram-error-handler] fallback.md full ({entry_count} entries) — dropping new entry\n')
        sys.exit(0)

    marker = "<!-- Add entries below"
    if marker in content:
        content = content.replace(marker, entry + "\n\n" + marker, 1)
    else:
        content = content.rstrip() + "\n\n" + entry + "\n"

    dir_ = os.path.dirname(path) or "."
    fd, tmp = tempfile.mkstemp(dir=dir_, prefix=".fallback_tmp")
    try:
        with os.fdopen(fd, "w") as f:
            f.write(content)
        os.replace(tmp, path)
    except Exception:
        os.unlink(tmp)
        raise
PYEOF
fi

# Inject systemMessage so Claude knows what happened and does NOT retry
printf '{"systemMessage":"⚠️  Engram MCP error auto-handled by hook.\nThe failed tool input was written to fallback.md and will be flushed to Engram at next session start.\nDo NOT retry the Engram call — continue without it."}'

exit 0
