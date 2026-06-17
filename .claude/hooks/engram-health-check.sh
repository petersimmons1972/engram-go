#!/usr/bin/env bash
. ~/.claude/hooks/lib/timing.sh 2>/dev/null || true
# PreToolUse hook: detect Engram MCP disconnections before executing Engram tools.
# Tracks consecutive failures and surfaces high-visibility warning to user.
# Issue: engram-go#408

set -euo pipefail

# Load centralized endpoint
# shellcheck source=engram-endpoint.conf
source "$HOME/.claude/hooks/engram-endpoint.conf" 2>/dev/null || ENGRAM_BASE_URL="http://127.0.0.1:8788"

STATE_FILE="$HOME/.claude/.engram-health-state"
WARN_THRESHOLD=2  # Number of consecutive failures before warning

# Short-circuit: if Engram is known-degraded, fast-skip
# Degraded state expires after 20 minutes.
DISCONNECT_STATE="$HOME/.claude/.engram-disconnect-state"
if [[ -f "$DISCONNECT_STATE" ]]; then
  AGE_DISCONNECT=$(( $(date +%s) - $(date -r "$DISCONNECT_STATE" +%s 2>/dev/null || echo 0) ))
  if [[ "$AGE_DISCONNECT" -lt 1200 ]]; then
    exit 0
  fi
  rm -f "$DISCONNECT_STATE"
fi

# Extract bearer token from mcp_servers.json (reuse pattern from engram-session-recall.sh)
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

# Perform health check: POST to /quick-recall with 2s timeout
HTTP_STATUS=$(curl -so /dev/null -w "%{http_code}" --max-time 2 \
  -H "Authorization: Bearer ${TOKEN}" \
  -H "Content-Type: application/json" \
  -X POST "${ENGRAM_BASE_URL}/quick-recall" \
  -d '{"query":"health-check","project":"global","limit":1}' 2>/dev/null || echo "000")

# Read previous state
PREV_FAILURES=0
LAST_FAILURE_TIME=""
if [[ -f "$STATE_FILE" ]]; then
  PREV_FAILURES=$(grep -oP '(?<=FAILURES=)\d+' "$STATE_FILE" 2>/dev/null || echo "0")
  LAST_FAILURE_TIME=$(grep -oP '(?<=LAST_FAILURE=).+' "$STATE_FILE" 2>/dev/null || echo "")
fi

# Check if this call succeeded (200-299 is success, 000 is timeout/unreachable)
if [[ "$HTTP_STATUS" =~ ^[2][0-9]{2}$ ]]; then
  # Success: reset counter and clear degraded marker
  FAILURES=0
  LAST_FAILURE_TIME=""
  rm -f "$DISCONNECT_STATE"
else
  # Failure: increment counter
  FAILURES=$((PREV_FAILURES + 1))
  LAST_FAILURE_TIME=$(date -u +'%H:%M:%S UTC')
fi

# Update state file atomically
STATE_DIR=$(dirname "$STATE_FILE")
mkdir -p "$STATE_DIR"
TMPFILE=$(mktemp "${STATE_DIR}/.engram-health-state.XXXXXX")
trap 'rm -f "${TMPFILE:-}"' EXIT
{
  echo "FAILURES=$FAILURES"
  echo "LAST_FAILURE=$LAST_FAILURE_TIME"
  echo "LAST_CHECK=$(date -u +'%Y-%m-%d %H:%M:%S UTC')"
} > "$TMPFILE"
mv "$TMPFILE" "$STATE_FILE"

# Emit warning if threshold exceeded — but DO NOT block the tool call.
# Exit 1 from a PreToolUse hook is interpreted by Claude Code as a tool denial,
# which made every engram MCP call fail when this probe was unreliable
# (e.g., when /quick-recall blocked behind a degraded embed pipeline).
# The hook's job is to warn, not gatekeep: warn via systemMessage and exit 0
# so the tool call proceeds. Engram itself reports degraded state via /health
# and the circuit breaker handles persistent embed failures.
if [[ "$FAILURES" -ge "$WARN_THRESHOLD" ]]; then
  printf '{"systemMessage":"Engram health probe failing (consecutive failures=%s, since %s). MCP call will proceed; recall may degrade to BM25+recency. Investigate: curl %s/health"}' "$FAILURES" "$LAST_FAILURE_TIME" "$ENGRAM_BASE_URL"
fi

exit 0
