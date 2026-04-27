#!/usr/bin/env bash
# SessionStart hook: ensure Engram is running and MCP token is current.
# Self-heals: starts Engram if down, syncs token, never just warns and gives up.
# Uses atomic writes (write-then-rename) so Claude Code never reads a partial file.

set -euo pipefail

ENGRAM_DIR="$HOME/projects/engram-go"
PORT=8788

[[ -d "$ENGRAM_DIR" ]] || exit 0

# ── 1. Ensure server is up ───────────────────────────────────────────────────
if ! curl -sf --max-time 2 "http://127.0.0.1:${PORT}/health" > /dev/null 2>&1; then
  echo "⚠️  Engram: server not responding — starting it now..."
  (cd "$ENGRAM_DIR" && make up) 2>&1 | sed 's/^/   /'

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

# ── 2. Fetch token (retry once for cold-start lag) ──────────────────────────
TOKEN_JSON=$(curl -sf --max-time 5 "http://127.0.0.1:${PORT}/setup-token" 2>/dev/null || true)
if [[ -z "$TOKEN_JSON" ]]; then
  sleep 3
  TOKEN_JSON=$(curl -sf --max-time 5 "http://127.0.0.1:${PORT}/setup-token" 2>/dev/null || true)
fi
if [[ -z "$TOKEN_JSON" ]]; then
  echo "❌ Engram: /setup-token unreachable after retry — memory recall disabled this session"
  exit 0
fi

TOKEN=$(echo "$TOKEN_JSON" | python3 -c "import json,sys; print(json.load(sys.stdin).get('token',''))" 2>/dev/null || true)
ENDPOINT=$(echo "$TOKEN_JSON" | python3 -c "import json,sys; print(json.load(sys.stdin).get('endpoint',''))" 2>/dev/null || true)
if [[ -z "$TOKEN" || -z "$ENDPOINT" ]]; then
  echo "❌ Engram: /setup-token returned malformed response: ${TOKEN_JSON}"
  exit 0
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

# For .claude.json: only update if engram key already present
servers = cfg.get("mcpServers", cfg)  # .claude.json nests under mcpServers; mcp_servers.json uses mcpServers too
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

echo "✅ Engram: ready"
