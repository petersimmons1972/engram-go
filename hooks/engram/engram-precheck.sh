#!/usr/bin/env bash
. ~/.claude/hooks/lib/timing.sh 2>/dev/null || true
. ~/.claude/hooks/lib/timing-v2.sh 2>/dev/null || true
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

# Phase 1 timing (#396): about to send health probe to Engram.
timing_mark_request_sent 2>/dev/null || true

# Fast path: healthy → done in <100ms
if curl -sf --max-time 1 "$HEALTH_URL" >/dev/null 2>&1; then
    timing_mark_response_received 2>/dev/null || true
    exit 0
fi

# Health check failed — still mark response received before background restart.
timing_mark_response_received 2>/dev/null || true

# Engram not responding. Kick off a background restart and return immediately.
# Never poll here — polling blocks the MCP call and makes Claude appear hung.
if [[ -d "$ENGRAM_DIR" ]]; then
    (cd "$ENGRAM_DIR" && docker compose up -d engram-go >/dev/null 2>&1) &
    disown $! 2>/dev/null || true
fi

printf '{"systemMessage":"⚠️  Engram health check failed — background restart initiated. This MCP call may fail; if so, the result will be captured to fallback.md. Check: docker logs engram-go-app"}'
exit 0
