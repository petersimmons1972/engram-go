#!/usr/bin/env bash
# PreCompact hook: store a session snapshot to engram-go before context compression.
# Uses /quick-store (sessionless REST endpoint) — no SSE handshake required.
set -euo pipefail

# Load centralized endpoint
# shellcheck source=engram-endpoint.conf
source "$HOME/.claude/hooks/engram-endpoint.conf" 2>/dev/null || ENGRAM_BASE_URL="http://127.0.0.1:8788"

BASE="$ENGRAM_BASE_URL"
PAYLOAD_FILE=$(mktemp "${TMPDIR:-/tmp}/engram-precompact.XXXXXX.json")
trap 'rm -f "$PAYLOAD_FILE"' EXIT
cat > "$PAYLOAD_FILE" || true

# Short-circuit: if Engram is known-degraded, skip — never block compaction
DISCONNECT_STATE="$HOME/.claude/.engram-disconnect-state"
if [[ -f "$DISCONNECT_STATE" ]]; then
  AGE_DISCONNECT=$(( $(date +%s) - $(date -r "$DISCONNECT_STATE" +%s 2>/dev/null || echo 0) ))
  if [[ "$AGE_DISCONNECT" -lt 1200 ]]; then
    exit 0
  fi
  rm -f "$DISCONNECT_STATE"
fi

# Bail if engram is down — never block compaction
if ! curl -sf --max-time 2 "${BASE}/health" > /dev/null 2>&1; then
    exit 0
fi

# Fetch bearer token (unauthenticated endpoint)
TOKEN=$(curl -sf --max-time 3 "${BASE}/setup-token" 2>/dev/null \
    | python3 -c "import json,sys; print(json.load(sys.stdin).get('token',''))" 2>/dev/null || true)
[[ -z "$TOKEN" ]] && exit 0

# Read the compaction payload from stdin, extract a summary of recent assistant turns
SUMMARY=$(python3 - "$PAYLOAD_FILE" <<'PYEOF'
import json, sys, datetime

payload_path = sys.argv[1]
try:
    with open(payload_path, "r", encoding="utf-8") as f:
        raw = f.read()
except Exception:
    raw = ""
try:
    data = json.loads(raw)
except Exception:
    print("[pre-compact snapshot] (parse error) Captured: " + datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ"))
    sys.exit(0)

messages = data.get("messages") or data.get("conversation") or []
if not messages and data:
    sys.stderr.write(f"[pre-compact] WARN: no messages key. Payload keys: {list(data.keys())}\n")
recent = []
for m in reversed(messages):
    role = m.get("role", "")
    if role != "assistant":
        continue
    content = m.get("content", "")
    if isinstance(content, list):
        parts = [c.get("text", "") for c in content if isinstance(c, dict) and c.get("type") == "text"]
        content = " ".join(parts)
    content = content.strip()
    if content:
        recent.append(content[:300])
    if len(recent) >= 3:
        break

summary_parts = ["[pre-compact snapshot]"]
if recent:
    summary_parts.append("Recent work: " + " | ".join(reversed(recent)))
summary_parts.append("Captured: " + datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ"))
print(" ".join(summary_parts)[:1200])
PYEOF
)

# Store to engram via /quick-store (no session required)
PAYLOAD=$(python3 -c "
import json, sys
print(json.dumps({
    'content': sys.argv[1],
    'project': 'global',
    'tags': ['pre-compact', 'session-snapshot'],
    'importance': 1
}))" "$SUMMARY")

curl -sf --max-time 5 \
    -X POST \
    -H "Authorization: Bearer ${TOKEN}" \
    -H "Content-Type: application/json" \
    -d "$PAYLOAD" \
    "${BASE}/quick-store" > /dev/null 2>&1 || true

exit 0
