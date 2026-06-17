#!/usr/bin/env bash
# ORPHANED — not registered in any settings.json (verified 2026-06-17, Article 041 audit).
# Superseded by `~/bin/engram hook UserPromptSubmit` (registered in settings.json).
# SECURITY NOTE: line 112 passes ENV_KEY as sys.argv[2] — exposes API key in
# /proc/<pid>/cmdline. If re-enabling, change to env var injection first. (FM-98)
. ~/.claude/hooks/lib/timing.sh 2>/dev/null || true
# UserPromptSubmit hook: fast per-message Engram auth check.
# If auth is broken: auto-runs engram-setup to refresh the token,
# then outputs a systemMessage telling Claude to surface the /mcp step.
# If auth is healthy: silent, exits 0 in under 200ms.
# Never blocks the session — auth check has a hard 3s timeout. (#376)

set -euo pipefail

# Load centralized endpoint
# shellcheck source=engram-endpoint.conf
source "$HOME/.claude/hooks/engram-endpoint.conf" 2>/dev/null || ENGRAM_BASE_URL="http://127.0.0.1:8788"

ENGRAM_DIR="$HOME/projects/engram-go"
MCP_CONFIG="$HOME/.claude/mcp_servers.json"

# shellcheck source=lib/engram-state.sh
source "$HOME/.claude/hooks/lib/engram-state.sh" 2>/dev/null || true

# Skip if engram-go project not installed
[[ -d "$ENGRAM_DIR" ]] || exit 0
[[ -f "$MCP_CONFIG" ]] || exit 0

# Short-circuit: if Engram is known-degraded/disconnected, skip the network call
# Degraded state expires after 20 minutes — auto-heals when engram recovers.
DISCONNECT_STATE="$HOME/.claude/.engram-disconnect-state"
if [[ -f "$DISCONNECT_STATE" ]]; then
  AGE_DISCONNECT=$(( $(date +%s) - $(date -r "$DISCONNECT_STATE" +%s 2>/dev/null || echo 0) ))
  if [[ "$AGE_DISCONNECT" -lt 1200 ]]; then
    # Still within 20-minute degraded window — fast-skip
    exit 0
  fi
  # Expired — remove stale marker and proceed with live check
  rm -f "$DISCONNECT_STATE"
fi

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

# File-based auth cache — 600s TTL to avoid per-message latency (#400)
CACHE="$HOME/.claude/.engram-auth-ok"
CACHE_TTL=600

if [[ -f "$CACHE" ]]; then
  age=$(( $(date +%s) - $(date -r "$CACHE" +%s 2>/dev/null || echo 0) ))
  [[ "$age" -lt "$CACHE_TTL" ]] && exit 0
fi

# Fast auth probe — 2s hard limit
# 200 or 500 = auth OK (500 = recall backend error, but token was accepted)
# 401 or 000 = auth broken
HTTP_STATUS=$(curl -so /dev/null -w "%{http_code}" --max-time 2 \
  -H "Authorization: Bearer ${TOKEN}" \
  -H "Content-Type: application/json" \
  -X POST "${ENGRAM_BASE_URL}/quick-recall" \
  -d '{"query":"auth-check","project":"global","limit":1}' 2>/dev/null || echo "000")

if [[ "$HTTP_STATUS" == "401" || "$HTTP_STATUS" == "000" ]]; then
  # Invalidate stale cache on auth failure; track in state (#404)
  rm -f "$CACHE"
  increment_state "consecutive_auth_failures" 2>/dev/null || true

  # Recovery path 1: engram-setup (works when /setup-token doesn't require auth).
  # Since #540 /setup-token requires Bearer auth, this will fail if we have no
  # valid token — but try it first in case it's been fixed or the server is older.
  REFRESHED=false
  if [[ -d "$ENGRAM_DIR" ]]; then
    if command -v engram-setup &>/dev/null; then
      engram-setup >/dev/null 2>&1 && REFRESHED=true || true
    elif [[ -x "$ENGRAM_DIR/engram-setup" ]]; then
      "$ENGRAM_DIR/engram-setup" >/dev/null 2>&1 && REFRESHED=true || true
    else
      (cd "$ENGRAM_DIR" && timeout 30 go run ./cmd/engram-setup >/dev/null 2>&1) && REFRESHED=true || true
    fi
  fi

  # Recovery path 2 (#614, #616): probe fallback key sources in priority order:
  #   1. ~/.config/engram/api_key  — backup of the Infisical key (most reliable)
  #   2. ENGRAM_API_KEY in .env    — docker-level default (may diverge from Infisical)
  # /starter injects the real key from Infisical at runtime; .env is NOT authoritative.
  if [[ "$REFRESHED" == "false" ]]; then
    FALLBACK_KEY=""
    # Try Infisical backup first
    if [[ -f "$HOME/.config/engram/api_key" ]]; then
      FALLBACK_KEY=$(cat "$HOME/.config/engram/api_key" | tr -d '[:space:]')
    fi
    # Fall back to .env if config backup didn't work or is absent
    if [[ -z "$FALLBACK_KEY" && -f "$ENGRAM_DIR/.env" ]]; then
      FALLBACK_KEY=$(grep '^ENGRAM_API_KEY=' "$ENGRAM_DIR/.env" 2>/dev/null | tail -1 | cut -d= -f2- | tr -d '[:space:]')
    fi
    ENV_KEY="$FALLBACK_KEY"
    if [[ -n "$ENV_KEY" ]]; then
      ENV_STATUS=$(curl -so /dev/null -w "%{http_code}" --max-time 2 \
        -H "Authorization: Bearer ${ENV_KEY}" \
        -H "Content-Type: application/json" \
        -X POST "${ENGRAM_BASE_URL}/quick-recall" \
        -d '{"query":"auth-check","project":"global","limit":1}' 2>/dev/null || echo "000")
      if [[ "$ENV_STATUS" != "401" && "$ENV_STATUS" != "000" ]]; then
        # Fallback key is valid — write it atomically to mcp_servers.json (#614)
        ENGRAM_KEY_INJECT="$ENV_KEY" python3 - "$MCP_CONFIG" <<'PYEOF' 2>/dev/null && REFRESHED=true || true
import json, os, sys, tempfile
path = sys.argv[1]
key = os.environ.pop('ENGRAM_KEY_INJECT', '')
if not os.path.exists(path):
    sys.exit(1)
with open(path) as f:
    cfg = json.load(f)
cfg.setdefault("mcpServers", {}).setdefault("engram", {})["headers"] = {"Authorization": "Bearer " + key}
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
      fi
    fi
  fi

  if [[ "$REFRESHED" == "true" ]]; then
    printf '{"systemMessage":"⚠️  Engram auth token was stale — recovered from .env.\nRun /mcp in Claude Code to reconnect memory."}'
  else
    printf '{"systemMessage":"❌ Engram auth failed and auto-recovery failed.\nRun: cd ~/projects/engram-go && make restart && make setup\nThen run /mcp in Claude Code."}'
  fi
  exit 0  # must exit here — fall-through would overwrite failure state updates
fi

# Auth OK: update cache, reset failure counter, clear any stale degraded marker, silent exit (#404)
touch "$CACHE"
rm -f "$DISCONNECT_STATE"
update_state "consecutive_auth_failures" "0" 2>/dev/null || true
update_state "last_auth_ok_at" "\"$(date -u +%Y-%m-%dT%H:%M:%SZ)\"" 2>/dev/null || true
exit 0
