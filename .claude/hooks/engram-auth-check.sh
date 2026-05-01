#!/usr/bin/env bash
# UserPromptSubmit hook: fast per-message Engram auth check.
# If auth is broken: auto-runs engram-setup to refresh the token,
# then outputs a systemMessage telling Claude to surface the /mcp step.
# If auth is healthy: silent, exits 0 in under 200ms.
# Never blocks the session — auth check has a hard 3s timeout. (#376)

set -euo pipefail

PORT=8788
ENGRAM_DIR="$HOME/projects/engram-go"
MCP_CONFIG="$HOME/.claude/mcp_servers.json"

# Skip if engram-go project not installed
[[ -d "$ENGRAM_DIR" ]] || exit 0
[[ -f "$MCP_CONFIG" ]] || exit 0

# Read token from config (#395)
TOKEN=$(python3 -c "
import json, os, sys
try:
    with open(os.path.expanduser('~/.claude/mcp_servers.json')) as f:
        d = json.load(f)
    tok = d.get('mcpServers',{}).get('engram',{}).get('headers',{}).get('Authorization','')
    print(tok.removeprefix('Bearer ').strip())
except Exception:
    print('')
" 2>/dev/null || true)

[[ -z "$TOKEN" ]] && exit 0

# Fast auth probe — 3s hard limit
# 200 or 500 = auth OK (500 = recall backend error, but token was accepted)
# 401 or 000 = auth broken
HTTP_STATUS=$(curl -so /dev/null -w "%{http_code}" --max-time 3 \
  -H "Authorization: Bearer ${TOKEN}" \
  -H "Content-Type: application/json" \
  -X POST "http://127.0.0.1:${PORT}/quick-recall" \
  -d '{"query":"auth-check","project":"global","limit":1}' 2>/dev/null || echo "000")

if [[ "$HTTP_STATUS" == "401" || "$HTTP_STATUS" == "000" ]]; then
  # Auto-remediate: refresh the token by re-running setup.
  # Uses pre-built binary if available (fast), falls back to go run.
  if [[ -d "$ENGRAM_DIR" ]]; then
    if command -v engram-setup &>/dev/null; then
      engram-setup >/dev/null 2>&1 || true
    elif [[ -x "$ENGRAM_DIR/engram-setup" ]]; then
      "$ENGRAM_DIR/engram-setup" >/dev/null 2>&1 || true
    else
      (cd "$ENGRAM_DIR" && timeout 30 go run ./cmd/engram-setup >/dev/null 2>&1) || true
    fi
  fi

  # Output systemMessage so Claude surfaces this to the user immediately
  printf '{"systemMessage":"⚠️  Engram auth was stale — token refreshed automatically.\\nRun /mcp in Claude Code to reconnect memory. Without this step, memory tools will fail."}'
  # Exit 0 — don't block the session; Claude will display the systemMessage
fi

# Auth OK: silent exit
exit 0
