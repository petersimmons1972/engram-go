#!/usr/bin/env bash
. ~/.claude/hooks/lib/timing.sh 2>/dev/null || true
# UserPromptSubmit hook: task-specific Engram recall on the first user message.
# Reads the .prompt field from the event JSON on stdin, calls /quick-recall
# with that text, and emits a systemMessage so Claude gets task-relevant context
# before generating its first response.
#
# FAIL-OPEN CONTRACT: this hook MUST exit 0 under all error conditions.
# A hanging or crashing UserPromptSubmit hook blocks every Claude Code turn.
# The timeout 1 wrapper enforces this at the OS level.

# Read stdin (UserPromptSubmit event JSON) before anything else
_STDIN=$(cat 2>/dev/null || true)

PORT="${ENGRAM_TEST_PORT:-8788}"
export _STDIN PORT
export MCP_CONFIG="$HOME/.claude/mcp_servers.json"

# Entire recall logic runs inside a 1-second hard timeout.
# Any Python exception, curl error, or timeout → outer `|| exit 0` catches it.
# NOTE: closing ) must come AFTER the PYEOF terminator, not on the <<'PYEOF' line.
_result=$(timeout 1 python3 - 2>/dev/null <<'PYEOF'
import json, sys, os, subprocess, urllib.request, urllib.error

stdin      = os.environ.get('_STDIN', '')
port       = os.environ.get('PORT', '8788')
mcp_config = os.environ.get('MCP_CONFIG', '')

# ── parse prompt ──────────────────────────────────────────────────────────────
try:
    prompt = json.loads(stdin).get('prompt', '') if stdin else ''
except Exception:
    sys.exit(0)

if len(prompt) < 20:
    sys.exit(0)

# ── read token ────────────────────────────────────────────────────────────────
try:
    cfg_path = mcp_config or os.path.expanduser('~/.claude/mcp_servers.json')
    with open(cfg_path) as f:
        cfg = json.load(f)
    token = (cfg.get('mcpServers', {})
                .get('engram', {})
                .get('headers', {})
                .get('Authorization', ''))
    token = token.removeprefix('Bearer ').strip()
except Exception:
    sys.exit(0)

if not token:
    sys.exit(0)

# ── infer project from git ────────────────────────────────────────────────────
try:
    repo = subprocess.check_output(
        ['git', 'rev-parse', '--show-toplevel'],
        stderr=subprocess.DEVNULL, timeout=0.2
    ).decode().strip()
    name = os.path.basename(repo)
except Exception:
    name = ''

project = next(
    (v for k, v in [('clearwatch', 'clearwatch'), ('engram', 'engram'),
                    ('homelab', 'homelab'), ('instinct', 'instinct')]
     if name.startswith(k)),
    'global'
)

# ── POST /quick-recall — 0.7s budget (0.3s margin inside the 1s wrapper) ─────
url  = f'http://127.0.0.1:{port}/quick-recall'
body = json.dumps({'query': prompt[:500], 'project': project, 'limit': 5}).encode()
req  = urllib.request.Request(
    url, data=body,
    headers={'Authorization': f'Bearer {token}', 'Content-Type': 'application/json'}
)
try:
    with urllib.request.urlopen(req, timeout=0.7) as resp:
        results = json.load(resp).get('results', [])
except Exception:
    sys.exit(0)

if not results:
    sys.exit(0)

# ── format as systemMessage JSON ──────────────────────────────────────────────
lines = ['## Engram Task Recall', '']
for i, r in enumerate(results, 1):
    summary = r.get('summary') or r.get('content', '')[:120]
    tags    = ', '.join(r.get('tags', [])[:3])
    lines.append(f'**{i}.** {summary}')
    if tags:
        lines.append(f'   *{tags}*')
    lines.append('')

print(json.dumps({'systemMessage': '\n'.join(lines).rstrip()}))
PYEOF
) || exit 0

[[ -n "$_result" ]] && printf '%s' "$_result"
exit 0
