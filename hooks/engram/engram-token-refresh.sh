#!/usr/bin/env bash
. ~/.claude/hooks/lib/timing.sh 2>/dev/null || true
. ~/.claude/hooks/lib/timing-v2.sh 2>/dev/null || true
# SessionStart hook: ensure Engram is running and MCP token is current.
# Self-heals: starts Engram if down, syncs token, never just warns and gives up.
# Uses atomic writes (write-then-rename) so Claude Code never reads a partial file.

set -euo pipefail

ENGRAM_DIR="$HOME/projects/engram-go"
PORT="${ENGRAM_TEST_PORT:-8788}"

[[ -d "$ENGRAM_DIR" ]] || exit 0

# shellcheck source=lib/engram-state.sh
source "$HOME/.claude/hooks/lib/engram-state.sh" 2>/dev/null || true

# ── 1. Ensure server is up ───────────────────────────────────────────────────
if ! curl -sf --max-time 2 "http://127.0.0.1:${PORT}/health" > /dev/null 2>&1; then
  echo "⚠️  Engram: server not responding — starting it now..."
  (cd "$ENGRAM_DIR" && timeout 30 make up) 2>&1 | sed 's/^/   /'

  # Wait up to 15s for it to become healthy
  for i in $(seq 1 15); do
    sleep 1
    if curl -sf --max-time 1 "http://127.0.0.1:${PORT}/health" > /dev/null 2>&1; then
      echo "✅ Engram: started (took ${i}s)"
      break
    fi
    if [[ "$i" == "15" ]]; then
      echo "❌ Engram: failed to start after 15s — memory recall disabled this session"
      echo "   Debug: cd ~/projects/engram-go && make logs"
      exit 0
    fi
  done
fi

# ── 2. Read existing token from config (avoid /setup-token rate limit) ───────
# /setup-token is rate-limited to 3 calls per 5 minutes. Read the cached token
# from mcp_servers.json first; only call /setup-token if the cached token fails
# auth or is missing. (#375, #376)
EXISTING_TOKEN=$(python3 -c "
import json, os
try:
    with open(os.path.expanduser('~/.claude/mcp_servers.json')) as f:
        d = json.load(f)
    tok = d.get('mcpServers',{}).get('engram',{}).get('headers',{}).get('Authorization','')
    print(tok.removeprefix('Bearer ').strip())
except Exception:
    print('')
" 2>/dev/null || true)

_fetch_setup_token() {
  local json
  json=$(curl -sf --max-time 5 "http://127.0.0.1:${PORT}/setup-token" 2>/dev/null || true)
  if [[ -z "$json" ]]; then
    sleep 3
    json=$(curl -sf --max-time 5 "http://127.0.0.1:${PORT}/setup-token" 2>/dev/null || true)
  fi
  echo "$json"
}

TOKEN=""
ENDPOINT="http://127.0.0.1:${PORT}/sse"

if [[ -n "$EXISTING_TOKEN" ]]; then
  TOKEN="$EXISTING_TOKEN"
  # Phase 1 timing (#396): token resolved from cached MCP config.
  timing_mark_auth_resolved 2>/dev/null || true
fi

# If no cached token, fetch from server
if [[ -z "$TOKEN" ]]; then
  TOKEN_JSON=$(_fetch_setup_token)
  if [[ -z "$TOKEN_JSON" ]]; then
    echo "❌ Engram: /setup-token unreachable — memory recall disabled this session"
    exit 0
  fi
  TOKEN=$(echo "$TOKEN_JSON" | python3 -c "import json,sys; print(json.load(sys.stdin).get('token',''))" 2>/dev/null || true)
  ENDPOINT=$(echo "$TOKEN_JSON" | python3 -c "import json,sys; print(json.load(sys.stdin).get('endpoint',''))" 2>/dev/null || true)
  if [[ -z "$TOKEN" ]]; then
    echo "❌ Engram: /setup-token returned malformed response"
    exit 0
  fi
  # Phase 1 timing (#396): token freshly fetched from /setup-token.
  timing_mark_auth_resolved 2>/dev/null || true
fi

# ── 3. Write token to both MCP config files (atomic) ────────────────────────
_write_token() {
  python3 - "$1" <<PYEOF
import json, os, sys, tempfile
from urllib.parse import urlparse, urlencode, parse_qs, urlunparse

def merge_url(existing: str, new: str) -> str:
    ep = urlparse(existing)
    np = urlparse(new)
    merged = {**parse_qs(ep.query, keep_blank_values=True),
              **parse_qs(np.query, keep_blank_values=True)}
    q = urlencode({k: v[0] for k, v in merged.items()})
    return urlunparse((np.scheme, np.netloc, np.path, '', q, ''))

path = sys.argv[1]
if not os.path.exists(path):
    sys.exit(0)

with open(path) as f:
    cfg = json.load(f)

# For .claude.json: only update if engram key already present (#395)
is_claude_json = "mcpServers" in cfg
servers = cfg.setdefault("mcpServers", {})

if os.path.basename(path) == ".claude.json" and "engram" not in servers:
    sys.exit(0)

existing_url = servers.get("engram", {}).get("url", "$ENDPOINT")
servers["engram"] = {
    "type": "sse",
    "url": merge_url(existing_url, "$ENDPOINT"),
    "headers": {"Authorization": "Bearer $TOKEN"}
}

dir_ = os.path.dirname(path) or "."
fd, tmp = tempfile.mkstemp(dir=dir_, prefix=".engram_token_tmp")
try:
    with os.fdopen(fd, "w") as f:
        json.dump(cfg, f, indent=2)
        f.write("\n")
    os.replace(tmp, path)
except Exception:
    os.unlink(tmp)
    raise
PYEOF
}

_write_token "$HOME/.claude/mcp_servers.json"
_write_token "$HOME/.claude.json"

# ── 4. Validate auth — /health is not enough, /quick-recall requires Bearer ──
# Tests the actual authenticated REST endpoint rather than the SSE/MCP protocol.
# Uses /quick-recall because it's a simple POST that requires Bearer auth and
# doesn't require an SSE handshake. Returns 401 on bad token, 200 on good.
# (#375)
_test_auth() {
  local tok="$1"
  local http_status
  http_status=$(curl -so /dev/null -w "%{http_code}" --max-time 5 \
    -H "Authorization: Bearer ${tok}" \
    -H "Content-Type: application/json" \
    -X POST "http://127.0.0.1:${PORT}/quick-recall" \
    -d '{"query":"auth-check","project":"global","limit":1}' 2>/dev/null || echo "000")
  # 401 = bad token; 000 = unreachable; anything else = auth OK (500 = recall
  # failed internally, but token was accepted)
  [[ "$http_status" != "401" && "$http_status" != "000" ]]
}

# Phase 1 timing (#396): about to send auth probe to Engram.
timing_mark_request_sent 2>/dev/null || true
if _test_auth "$TOKEN"; then
  # Phase 1 timing (#396): auth probe completed successfully.
  timing_mark_response_received 2>/dev/null || true
  # ── 5. Detect container restart since last token write ───────────────────────
  # If the container restarted after the last time mcp_servers.json was written,
  # the Claude Code MCP connection is using a token from before the restart.
  # The token itself is stable (same ENGRAM_API_KEY), but the SSE session was
  # dropped. Prompt the user to run /mcp to reconnect. (#376)
  CONTAINER_STARTED=$(docker inspect --format '{{.State.StartedAt}}' engram-go-app 2>/dev/null || true)
  CONFIG_MTIME=$(python3 -c "import os; print(os.path.getmtime(os.path.expanduser('~/.claude/mcp_servers.json')))" 2>/dev/null || true)

  if [[ -n "$CONTAINER_STARTED" && -n "$CONFIG_MTIME" ]]; then
    CONTAINER_TS=$(python3 -c "
from datetime import datetime, timezone
import sys
try:
    s = '${CONTAINER_STARTED}'.replace('Z','+00:00')
    print(datetime.fromisoformat(s).timestamp())
except Exception:
    print(0)
" 2>/dev/null || echo "0")

    NEEDS_MCP=$(python3 -c "print('yes' if ${CONTAINER_TS} > ${CONFIG_MTIME} else 'no')" 2>/dev/null || echo "no")
    if [[ "$NEEDS_MCP" == "yes" ]]; then
      # Container restarted since last config write — SSE session is stale.
      # Output systemMessage so Claude surfaces the /mcp step immediately.
      printf '{"systemMessage":"⚠️  Engram container restarted since last session.\\nRun /mcp in Claude Code to reconnect memory — this is the only step needed."}'
      exit 0
    fi
  fi

  # Record session start and check for degraded state (#404)
  _now=$(date -u +%Y-%m-%dT%H:%M:%SZ)
  update_state "last_session_start" "\"${_now}\"" 2>/dev/null || true
  increment_state "sessions_since_last_flush" 2>/dev/null || true

  # Emit degraded-state systemMessage if thresholds exceeded
  _degraded=$(python3 - "$STATE_FILE" 2>/dev/null <<'PYEOF'
import json, sys
try:
    s = json.load(open(sys.argv[1]))
    msgs = []
    fc = int(s.get("fallback_entry_count") or 0)
    sf = int(s.get("sessions_since_last_flush") or 0)
    af = int(s.get("consecutive_auth_failures") or 0)
    if fc > 10:
        msgs.append(f"{fc} entries queued in fallback.md")
    if sf > 2:
        msgs.append(f"no successful flush in {sf} sessions")
    if af > 0:
        msgs.append(f"{af} consecutive auth failure(s)")
    if msgs:
        detail = " | ".join(msgs)
        print(f'{{"systemMessage":"⚠️  Engram memory is degraded: {detail}.\\nRun: engram-flush --force  or restart Engram to recover."}}')
except Exception:
    pass
PYEOF
  )
  if [[ -n "$_degraded" ]]; then
    printf '%s' "$_degraded"
  else
    echo "✅ Engram: MCP authenticated and ready"
  fi
else
  # Phase 1 timing (#396): auth probe completed (failure path).
  timing_mark_response_received 2>/dev/null || true
  # Recovery path (#615, #616): cached token failed — try fallback key sources.
  # Priority: ~/.config/engram/api_key (Infisical backup) > .env (docker default).
  # /starter injects the real key from Infisical at runtime; .env may diverge.
  ENV_KEY=""
  if [[ -f "$HOME/.config/engram/api_key" ]]; then
    ENV_KEY=$(cat "$HOME/.config/engram/api_key" | tr -d '[:space:]')
  fi
  if [[ -z "$ENV_KEY" ]]; then
    ENV_KEY=$(grep '^ENGRAM_API_KEY=' "$ENGRAM_DIR/.env" 2>/dev/null | tail -1 | cut -d= -f2- | tr -d '[:space:]')
  fi
  ENV_RECOVERED=false
  if [[ -n "$ENV_KEY" ]]; then
    ENV_STATUS=$(curl -so /dev/null -w "%{http_code}" --max-time 5 \
      -H "Authorization: Bearer ${ENV_KEY}" \
      -H "Content-Type: application/json" \
      -X POST "http://127.0.0.1:${PORT}/quick-recall" \
      -d '{"query":"auth-check","project":"global","limit":1}' 2>/dev/null || echo "000")
    if [[ "$ENV_STATUS" != "401" && "$ENV_STATUS" != "000" ]]; then
      TOKEN="$ENV_KEY"
      _write_token "$HOME/.claude/mcp_servers.json"
      _write_token "$HOME/.claude.json"
      ENV_RECOVERED=true
    fi
  fi

  if [[ "$ENV_RECOVERED" == "true" ]]; then
    echo "✅ Engram: recovered from .env key — token updated in MCP config"
    update_state "last_session_start" "\"$(date -u +%Y-%m-%dT%H:%M:%SZ)\"" 2>/dev/null || true
    update_state "consecutive_auth_failures" "0" 2>/dev/null || true
  else
    echo "❌ Engram: MCP auth failed — token written but Claude Code MCP connection is stale"
    echo ""
    echo "   To reconnect, run these two steps:"
    echo "   1.  cd ~/projects/engram-go && make restart && make setup"
    echo "   2.  Type /mcp in Claude Code to reconnect"
    echo ""
    echo "   Memory recall is DISABLED this session until reconnected."
    # Exit 0 — don't block the session, but make the error impossible to miss
  fi
fi
