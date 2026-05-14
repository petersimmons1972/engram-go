#!/usr/bin/env bash
. ~/.claude/hooks/lib/timing.sh 2>/dev/null || true
# Stop hook: probe Engram MCP after every Claude turn for proactive disconnection detection.
# On failure, emits a systemMessage visible in the next turn so the user never discovers
# a broken connection manually. Must exit 0 always — never blocks Claude Code from stopping.
# Issue: engram-go#614 (resilience track)

DISCONNECT_STATE="$HOME/.claude/.engram-disconnect-state"
MCP_URL="http://127.0.0.1:8788/mcp"

# Extract bearer token from mcp_servers.json (same one-liner as engram-health-check.sh)
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

# No token means engram isn't configured — nothing to probe
[[ -z "$TOKEN" ]] && exit 0

PAYLOAD='{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"memory_status_ping","arguments":{}}}'

# Attempt probe using xh (preferred per CLAUDE.md), fall back to curl
PROBE_OK=0
if command -v xh >/dev/null 2>&1; then
    if xh --timeout=5 POST "$MCP_URL" \
        "Authorization:Bearer ${TOKEN}" \
        "Content-Type:application/json" \
        --raw "$PAYLOAD" >/dev/null 2>&1; then
        PROBE_OK=1
    fi
else
    if curl -s --max-time 5 \
        -H "Authorization: Bearer ${TOKEN}" \
        -H "Content-Type: application/json" \
        -X POST "$MCP_URL" \
        -d "$PAYLOAD" >/dev/null 2>&1; then
        PROBE_OK=1
    fi
fi

if [[ "$PROBE_OK" -eq 1 ]]; then
    # Server is reachable
    if [[ -f "$DISCONNECT_STATE" ]]; then
        # Was previously disconnected — emit recovery message and clean up state
        rm -f "$DISCONNECT_STATE"
        printf '{"type":"system","content":"✅ Engram MCP reconnected — memory tools restored."}'
    fi
    # Healthy and was healthy: no output
else
    # Server is unreachable
    if [[ ! -f "$DISCONNECT_STATE" ]]; then
        # First failure — write state file and emit disconnection message
        mkdir -p "$(dirname "$DISCONNECT_STATE")"
        date -u +'%Y-%m-%dT%H:%M:%SZ' > "$DISCONNECT_STATE"
        printf '{"type":"system","content":"⚠️  Engram MCP disconnected — memory tools unavailable. Check `docker compose ps` in ~/projects/engram-go. No MCP reset needed — tools will recover automatically when server restarts."}'
    fi
    # Already flagged: stay quiet to avoid repeating the warning every turn
fi

exit 0
