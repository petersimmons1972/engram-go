#!/usr/bin/env bash
. ~/.claude/hooks/lib/timing.sh 2>/dev/null || true
# Stop hook: record a session-end marker in Engram via /quick-store.
# Keeps engram_episodes_ended_clean_total accurate vs reaper count.
set -euo pipefail

# Load centralized endpoint
# shellcheck source=engram-endpoint.conf
source "$HOME/.claude/hooks/engram-endpoint.conf" 2>/dev/null || ENGRAM_BASE_URL="http://127.0.0.1:8788"

BASE="$ENGRAM_BASE_URL"

# shellcheck source=lib/engram-state.sh
source "$HOME/.claude/hooks/lib/engram-state.sh" 2>/dev/null || true

# Short-circuit: if Engram is known-degraded, fast-skip
DISCONNECT_STATE="$HOME/.claude/.engram-disconnect-state"
if [[ -f "$DISCONNECT_STATE" ]]; then
  AGE_DISCONNECT=$(( $(date +%s) - $(date -r "$DISCONNECT_STATE" +%s 2>/dev/null || echo 0) ))
  if [[ "$AGE_DISCONNECT" -lt 1200 ]]; then
    exit 0
  fi
  rm -f "$DISCONNECT_STATE"
fi

# Never block session end
if ! curl -sf --max-time 2 "${BASE}/health" > /dev/null 2>&1; then
    exit 0
fi

TOKEN=$(python3 -c "
import json, os
try:
    with open(os.path.expanduser('~/.claude/mcp_servers.json')) as f:
        d = json.load(f)
    tok = d.get('mcpServers',{}).get('engram',{}).get('headers',{}).get('Authorization','')
    print(tok.removeprefix('Bearer ').strip())
except Exception:
    print('')
" 2>/dev/null || echo "")
[[ -z "$TOKEN" ]] && exit 0

TIMESTAMP=$(date -u +"%Y-%m-%dT%H:%M:%SZ")
PAYLOAD=$(python3 -c "
import json, sys
print(json.dumps({
    'content': f'[session-end] Claude Code session ended cleanly at {sys.argv[1]}',
    'project': 'global',
    'tags': ['session-end', 'lifecycle'],
    'importance': 1,
}))" "$TIMESTAMP")

curl -sf --max-time 5 \
    -X POST \
    -H "Authorization: Bearer ${TOKEN}" \
    -H "Content-Type: application/json" \
    -d "$PAYLOAD" \
    "${BASE}/quick-store" > /dev/null 2>&1 || true

# Emit session summary from state (#404)
_recall=$(read_state "last_recall_results" 2>/dev/null || echo "0")
_fallback=$(read_state "fallback_entry_count" 2>/dev/null || echo "0")
_auth_failures=$(read_state "consecutive_auth_failures" 2>/dev/null || echo "0")
_auth_status="ok"
[[ "${_auth_failures:-0}" -gt 0 ]] && _auth_status="${_auth_failures} failure(s)"
echo "[engram] session closed — recall: ${_recall:-0} results | fallback: ${_fallback:-0} queued | auth: ${_auth_status}"

exit 0
