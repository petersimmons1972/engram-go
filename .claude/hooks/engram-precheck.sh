#!/usr/bin/env bash
# PreToolUse hook: fast connectivity check before any mcp__engram__* call.
# If Engram responds healthy → silent exit 0 (<100ms).
# If Engram is down → kick off a background restart, emit a systemMessage,
#   exit 0 immediately. Never polls. Never blocks.
#
# engram-go#408: polling loop removed — polling held the PreToolUse hook for
# up to 20s, making Claude appear hung and prompting accidental user interrupts.

set -euo pipefail

PORT="${ENGRAM_TEST_PORT:-8788}"
ENGRAM_DIR="${ENGRAM_TEST_DIR:-$HOME/projects/engram-go}"
HEALTH_URL="http://localhost:${PORT}/health"

# Fast path: healthy → done in <100ms
if curl -sf --max-time 1 "$HEALTH_URL" >/dev/null 2>&1; then
    exit 0
fi

# Engram not responding. Kick off a background restart and return immediately.
# Never poll here — polling blocks the MCP call and makes Claude appear hung.
if [[ -d "$ENGRAM_DIR" ]]; then
    (cd "$ENGRAM_DIR" && docker compose up -d engram-go >/dev/null 2>&1) &
    disown $! 2>/dev/null || true
fi

printf '{"systemMessage":"⚠️  Engram health check failed — background restart initiated. This MCP call may fail; if so, the result will be captured to fallback.md. Check: docker logs engram-go-app"}'
exit 0
