#!/usr/bin/env bash
# ORPHANED — not registered in any settings.json (verified 2026-06-17, Article 041 audit).
# Superseded by engram-health-check.sh which provides the same function with state
# tracking, failure counting, and the shared disconnect-state short-circuit.
# Do NOT re-register without first removing or adapting engram-health-check.sh to avoid
# double-probing on every mcp__engram__* PreToolUse event.
# Kept on disk for reference; safe to delete once confirmed unused.
. ~/.claude/hooks/lib/timing.sh 2>/dev/null || true
. ~/.claude/hooks/lib/timing-v2.sh 2>/dev/null || true
# PreToolUse hook: fast connectivity check before any mcp__engram__* call.
# If Engram responds healthy → silent exit 0 (<100ms).
# If Engram is down → log a warning, emit a systemMessage, exit 0 immediately.
# Never polls. Never blocks. Never attempts Docker restarts (engram-go is in k8s).
#
# engram-go#408: polling loop removed — polling held the PreToolUse hook for
# up to 20s, making Claude appear hung and prompting accidental user interrupts.

set -euo pipefail

# Load centralized endpoint
# shellcheck source=engram-endpoint.conf
source "$HOME/.claude/hooks/engram-endpoint.conf" 2>/dev/null || ENGRAM_BASE_URL="http://127.0.0.1:8788"

HEALTH_URL="${ENGRAM_BASE_URL}/health"

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

# Fast path: healthy → done in <100ms
if curl -sf --max-time 1 "$HEALTH_URL" >/dev/null 2>&1; then
    exit 0
fi

# Engram not responding. Log it and return immediately.
# engram-go runs in Kubernetes — do NOT attempt docker compose restart here.
# If the pod is crashlooping, use: kubectl rollout restart deployment/engram-go -n engram
echo "[engram-precheck] Health check failed for ${HEALTH_URL}" >&2

printf '{"systemMessage":"⚠️  Engram health check failed. This MCP call may fail; result may be captured to fallback.md. To investigate: kubectl get pods -n engram"}'
exit 0
