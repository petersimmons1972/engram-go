#!/usr/bin/env bash
# PreToolUse hook: self-repair Engram before any mcp__engram__* call.
# If Engram responds healthy → silent exit 0.
# If Engram is down → attempt docker compose restart, wait up to 20s, then:
#   - If recovered → emit systemMessage and exit 0 (proceed with tool call)
#   - If still down → emit systemMessage and exit 0 (let PostToolUse handle fallback)

set -euo pipefail

PORT="${ENGRAM_TEST_PORT:-8788}"
ENGRAM_DIR="$HOME/projects/engram-go"
HEALTH_URL="http://localhost:${PORT}/health"

# Fast path: healthy → done in <100ms
if curl -sf --max-time 1 "$HEALTH_URL" >/dev/null 2>&1; then
    exit 0
fi

# Engram is not responding. Attempt container restart.
echo "[engram-precheck] Engram not responding — attempting self-repair..." >&2

if [[ ! -d "$ENGRAM_DIR" ]]; then
    printf '{"systemMessage":"⚠️  Engram unreachable and engram-go project not found at %s. Continuing without Engram — store to fallback.md.", "action":"block"}' "$ENGRAM_DIR"
    exit 0
fi

# Try docker compose restart (non-blocking in background; we poll below)
(cd "$ENGRAM_DIR" && docker compose up -d engram-go 2>&1) &
COMPOSE_PID=$!

# Poll for recovery — up to 20s
RECOVERED=false
for i in $(seq 1 20); do
    sleep 1
    if curl -sf --max-time 1 "$HEALTH_URL" >/dev/null 2>&1; then
        RECOVERED=true
        break
    fi
done

# Wait for compose to finish (it may still be running)
wait "$COMPOSE_PID" 2>/dev/null || true

if $RECOVERED; then
    printf '{"systemMessage":"⚠️  Engram was down and has been automatically restarted (recovered in ~%ds). The MCP SSE connection may be stale — if tool calls still fail, run /mcp to reconnect. Proceeding with this tool call."}' "$i"
    exit 0
else
    printf '{"systemMessage":"⚠️  Engram is down and did not recover within 20s. Check: docker logs engram-go-app. Continuing without Engram — critical writes will go to fallback.md."}'
    exit 0
fi
