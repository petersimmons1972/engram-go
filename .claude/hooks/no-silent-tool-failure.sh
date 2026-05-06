#!/usr/bin/env bash
# Stop hook: forbid the assistant from ending a turn after a tool error
# without emitting any explanatory text.
#
# Reads the last few messages from the transcript JSON on stdin.
# If the final assistant message ends with a tool_use_error/denial and no
# subsequent text block, returns {"decision":"block","reason":"..."} which
# forces Claude to emit a response before yielding the turn.
#
# Why: 2026-05-05 — silent tool denials caused the assistant to appear
# hung for hours. User had to manually interrupt to see anything.
#
# Fix: 2026-05-06 — SC2259: heredoc was overriding the pipe, so Python
# never received $raw. Now uses a temp file to pass the payload.

set -euo pipefail

raw=$(cat)

_tmpfile=$(mktemp)
printf '%s' "$raw" > "$_tmpfile"

verdict=$(python3 - "$_tmpfile" <<'PY' 2>/dev/null || echo "ok"
import json, sys

try:
    with open(sys.argv[1]) as f:
        payload = json.loads(f.read())
except Exception:
    print("ok"); sys.exit(0)

# transcript may be a list under different keys depending on CC version
msgs = payload.get("messages") or payload.get("transcript") or []
if not isinstance(msgs, list) or not msgs:
    print("ok"); sys.exit(0)

# Walk backwards to find the final assistant message
last_assistant = None
for m in reversed(msgs):
    if m.get("role") == "assistant":
        last_assistant = m
        break
if not last_assistant:
    print("ok"); sys.exit(0)

content = last_assistant.get("content")
if isinstance(content, str):
    # plain-text final message — fine
    print("ok"); sys.exit(0)
if not isinstance(content, list):
    print("ok"); sys.exit(0)

# Walk content blocks. Find any tool_result with is_error true,
# OR a text block whose text contains the standard denial wording.
saw_error = False
saw_text_after_error = False
DENIAL_MARKERS = (
    "user doesn't want to proceed with this tool use",
    "tool use was rejected",
    "Permission",
    '"is_error":true',
    "error executing tool",
)

for block in content:
    if not isinstance(block, dict):
        continue
    btype = block.get("type", "")
    if btype == "tool_result":
        if block.get("is_error"):
            saw_error = True
            saw_text_after_error = False
            continue
        # also catch denial-message tool results without is_error flag
        body = block.get("content")
        body_str = json.dumps(body) if not isinstance(body, str) else body
        if any(m in body_str for m in DENIAL_MARKERS):
            saw_error = True
            saw_text_after_error = False
            continue
    elif btype == "text":
        text = block.get("text", "").strip()
        if saw_error and text:
            saw_text_after_error = True

if saw_error and not saw_text_after_error:
    print("block")
else:
    print("ok")
PY
)

rm -f "$_tmpfile"

if [[ "$verdict" == "block" ]]; then
    cat <<'JSON'
{"decision":"block","reason":"You ended your turn after a tool returned an error or denial without emitting any text to the user. This makes the assistant appear hung. Always emit at least one sentence acknowledging the failure and stating what you'll do next BEFORE yielding the turn. Respond now."}
JSON
    exit 0
fi

exit 0
