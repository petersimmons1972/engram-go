#!/usr/bin/env bash
# SessionStart hook: inject recent Engram memories into MEMORY.md so Claude
# starts every session with relevant context already loaded. (#378)
#
# Uses POST /quick-recall (REST, no SSE handshake required) to keep this fast.
# Appends results under "## Engram Session Recall" in MEMORY.md — that section
# is already included in Claude's context at session start.
#
# Fails silently if Engram is unreachable — never blocks the session.

set -euo pipefail

PORT=8788
MEMORY_FILE="$HOME/.claude/projects/-home-psimmons/memory/MEMORY.md"
MAX_RESULTS=5

# Read token from mcp_servers.json
TOKEN=$(python3 -c "
import json, os
try:
    with open(os.path.expanduser('~/.claude/mcp_servers.json')) as f:
        d = json.load(f)
    tok = d.get('mcpServers',{}).get('engram',{}).get('headers',{}).get('Authorization','')
    print(tok.removeprefix('Bearer ').strip())
except Exception:
    print('')
" 2>/dev/null || true)

[[ -z "$TOKEN" ]] && exit 0

# Quick auth smoke-test — same endpoint used by all other hooks (#393)
HTTP_STATUS=$(curl -so /dev/null -w "%{http_code}" --max-time 3 \
  -H "Authorization: Bearer ${TOKEN}" \
  -H "Content-Type: application/json" \
  -X POST "http://127.0.0.1:${PORT}/quick-recall" \
  -d '{"query":"auth-check","project":"global","limit":1}' 2>/dev/null || echo "000")
[[ "$HTTP_STATUS" == "401" || "$HTTP_STATUS" == "000" ]] && exit 0

# Call /quick-recall for global project context
RECALL_JSON=$(curl -sf --max-time 8 \
  -H "Authorization: Bearer ${TOKEN}" \
  -H "Content-Type: application/json" \
  -X POST "http://127.0.0.1:${PORT}/quick-recall" \
  -d "{\"query\":\"current project status recent work decisions\",\"project\":\"global\",\"limit\":${MAX_RESULTS}}" \
  2>/dev/null || true)

[[ -z "$RECALL_JSON" ]] && exit 0

# Write RECALL_JSON to a temp file to avoid shell injection into Python (#393)
_recall_tmp=$(mktemp)
printf '%s' "$RECALL_JSON" > "$_recall_tmp"
RECALL_SECTION=$(python3 - "$_recall_tmp" <<'PYEOF'
import json, sys

try:
    with open(sys.argv[1]) as f:
        data = json.load(f)
    results = data.get('results', [])
except Exception:
    sys.exit(0)

if not results:
    sys.exit(0)

lines = ["", "## Engram Session Recall", ""]
for i, r in enumerate(results, 1):
    summary = r.get('summary') or r.get('content', '')[:120]
    tags = ', '.join(r.get('tags', [])[:4])
    score = r.get('score', 0)
    lines.append(f"**{i}.** {summary}")
    if tags:
        lines.append(f"   *tags: {tags} | score: {score:.2f}*")
    lines.append("")

print('\n'.join(lines))
PYEOF
)
rm -f "$_recall_tmp"

[[ -z "$RECALL_SECTION" ]] && exit 0

# Strip any previous recall section and append fresh one
if [[ -f "$MEMORY_FILE" ]]; then
  python3 - "$MEMORY_FILE" "$RECALL_SECTION" <<'PYEOF'
import sys, os, tempfile

path = sys.argv[1]
new_section = sys.argv[2]

with open(path) as f:
    content = f.read()

# Remove previous recall section if present
if "## Engram Session Recall" in content:
    content = content[:content.index("## Engram Session Recall")].rstrip()

content = content.rstrip() + "\n" + new_section + "\n"

# Atomic write — never leaves MEMORY.md truncated on interrupt (#393)
dir_ = os.path.dirname(path) or "."
fd, tmp = tempfile.mkstemp(dir=dir_, prefix=".memory_recall_tmp")
try:
    with os.fdopen(fd, "w") as f:
        f.write(content)
    os.replace(tmp, path)
except Exception:
    os.unlink(tmp)
    raise

print("✅ Engram: session recall injected into MEMORY.md")
PYEOF
else
  echo "⚠️  Engram session recall: MEMORY.md not found at ${MEMORY_FILE}"
fi
