#!/usr/bin/env bash
# SessionStart hook: verify Engram API token is valid against k8s deployment.
# Replaced Docker/SSE self-heal logic on 2026-06-01 — Engram runs in k8s
# (namespace engram), not as a local Docker container. Transport is now
# Streamable HTTP (/mcp), not SSE. Each call is independent; no persistent
# connection to restart.
#
# Behavior:
#   - GET /health with 3s timeout
#   - 200 => token is valid, log OK
#   - 401 => token needs refresh, emit systemMessage
#   - unreachable => emit systemMessage with debug hint

set -euo pipefail

# shellcheck source=engram-endpoint.conf
source "$HOME/.claude/hooks/engram-endpoint.conf" 2>/dev/null || ENGRAM_BASE_URL="https://engram.petersimmons.com"

API_KEY_FILE="$HOME/.config/engram/api_key"
TOKEN=""
if [[ -f "$API_KEY_FILE" ]]; then
    TOKEN=$(tr -d '[:space:]' < "$API_KEY_FILE")
fi

HTTP_STATUS=$(curl -so /dev/null -w "%{http_code}" --max-time 3 \
    -H "Authorization: Bearer ${TOKEN}" \
    "${ENGRAM_BASE_URL}/health" 2>/dev/null || echo "000")

case "$HTTP_STATUS" in
  200)
    # Silent on healthy auth — no output needed
    ;;
  401)
    printf '{"systemMessage":"Engram token needs refresh.\nRun: kubectl -n engram rollout restart deployment/engram-go\nThen update ~/.config/engram/api_key with the new token from Infisical."}'
    ;;
  000)
    printf '{"systemMessage":"Engram is unreachable at %s/health (timeout 3s).\nCheck: kubectl -n engram get pods"}' "${ENGRAM_BASE_URL}"
    ;;
  *)
    printf '{"sessionMessage":"Engram: unexpected health status %s — memory recall may be degraded"}\n' "$HTTP_STATUS"
    ;;
esac

exit 0
