#!/usr/bin/env bash
# verify-recall-fetch.sh — diagnose recall→fetch orphans (engram-go#634 fix#1)
#
# Calls /quick-recall for a query, then calls memory_fetch (via /mcp POST) for
# each returned handle ID, and reports how many handles could not be fetched.
# A non-zero exit code means orphaned handles were detected.
#
# Usage:
#   ./scripts/verify-recall-fetch.sh [OPTIONS]
#
# Options:
#   -q, --query QUERY      Search query (default: "test")
#   -p, --project PROJECT  Engram project (default: "global")
#   -n, --limit N          Max results from recall (default: 10)
#   -u, --url URL          Engram server base URL (default: http://localhost:8788)
#   -k, --api-key KEY      API key (default: $ENGRAM_API_KEY)
#   -h, --help             Show this help and exit
#
# Environment variables (all overridable by flags):
#   ENGRAM_API_KEY         Bearer token for authentication
#   ENGRAM_URL             Engram server base URL

set -euo pipefail

# ── defaults ──────────────────────────────────────────────────────────────────
QUERY="test"
PROJECT="global"
LIMIT=10
BASE_URL="${ENGRAM_URL:-http://localhost:8788}"
API_KEY="${ENGRAM_API_KEY:-}"

# ── argument parsing ──────────────────────────────────────────────────────────
usage() {
    grep '^#' "$0" | grep -v '#!/' | sed 's/^# \{0,1\}//'
    exit 0
}

while [[ $# -gt 0 ]]; do
    case "$1" in
        -h|--help)    usage ;;
        -q|--query)   QUERY="$2";   shift 2 ;;
        -p|--project) PROJECT="$2"; shift 2 ;;
        -n|--limit)   LIMIT="$2";   shift 2 ;;
        -u|--url)     BASE_URL="$2"; shift 2 ;;
        -k|--api-key) API_KEY="$2"; shift 2 ;;
        *) echo "Unknown option: $1" >&2; echo "Run with --help for usage." >&2; exit 1 ;;
    esac
done

# ── auth header ───────────────────────────────────────────────────────────────
AUTH_HEADER=""
if [[ -n "$API_KEY" ]]; then
    AUTH_HEADER="Authorization: Bearer $API_KEY"
fi

curl_auth() {
    if [[ -n "$AUTH_HEADER" ]]; then
        curl -s -H "$AUTH_HEADER" "$@"
    else
        curl -s "$@"
    fi
}

# ── step 1: memory_recall via MCP (returns handles, same path as the orphan bug) ──
# The orphan scenario (#634) manifests when memory_recall returns handle IDs that
# memory_fetch cannot resolve. /quick-recall bypasses the handle path and returns
# stored objects directly — it would not reproduce the bug. We must call the MCP
# memory_recall tool to get handle IDs from the same index that surfaces orphans.
echo "==> memory_recall (MCP): query='$QUERY' project='$PROJECT' limit=$LIMIT" >&2

RECALL_PAYLOAD=$(printf \
    '{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"memory_recall","arguments":{"project":"%s","query":"%s","limit":%d,"detail":"handle"}}}' \
    "$(printf '%s' "$PROJECT" | sed 's/"/\\"/g')" \
    "$(printf '%s' "$QUERY" | sed 's/"/\\"/g')" \
    "$LIMIT")

RECALL_RESP=$(curl_auth -X POST \
    -H "Content-Type: application/json" \
    -H "Accept: application/json, text/event-stream" \
    -d "$RECALL_PAYLOAD" \
    --max-time 30 \
    "${BASE_URL}/mcp" 2>/dev/null)

if [[ -z "$RECALL_RESP" ]]; then
    echo "ERROR: empty response from /mcp memory_recall" >&2
    exit 2
fi

# Extract handle IDs from the MCP response.
# memory_recall returns {"handles":[{"id":"...","summary":"...","is_handle":true},...]}
# nested inside the MCP tool result content[0].text JSON.
IDS=$(printf '%s' "$RECALL_RESP" | python3 -c '
import sys, json

raw = sys.stdin.read()
ids = []
for line in raw.splitlines():
    line = line.strip()
    if line.startswith("data:"):
        line = line[5:].strip()
    if not line:
        continue
    try:
        obj = json.loads(line)
    except json.JSONDecodeError:
        continue
    result = obj.get("result", {})
    for c in result.get("content", []):
        if not isinstance(c, dict) or "text" not in c:
            continue
        try:
            inner = json.loads(c["text"])
        except Exception:
            continue
        for h in inner.get("handles", []):
            hid = h.get("id", "")
            if hid:
                ids.append(hid)
        # also accept flat results list (non-handle mode fallback)
        for r in inner.get("results", []):
            hid = r.get("id", "")
            if hid:
                ids.append(hid)
print("\n".join(ids))
' 2>/dev/null)

TOTAL_HANDLES=0
FETCH_SUCCESS=0
FETCH_404=0
FETCH_OTHER_ERROR=0

if [[ -z "$IDS" ]]; then
    echo "==> recall returned 0 results — nothing to verify" >&2
else
    while IFS= read -r id; do
        [[ -z "$id" ]] && continue
        TOTAL_HANDLES=$((TOTAL_HANDLES + 1))

        # memory_fetch via the MCP /message endpoint.
        # Use the JSON-RPC call_tool format that the /message SSE endpoint accepts.
        # We POST a tools/call request and read the first response line.
        FETCH_PAYLOAD=$(printf \
            '{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"memory_fetch","arguments":{"project":"%s","id":"%s","detail":"summary"}}}' \
            "$(printf '%s' "$PROJECT" | sed 's/"/\\"/g')" \
            "$(printf '%s' "$id" | sed 's/"/\\"/g')")

        # /mcp is the Streamable HTTP endpoint; POST returns newline-delimited JSON events.
        FETCH_RESP=$(curl_auth -X POST \
            -H "Content-Type: application/json" \
            -H "Accept: application/json, text/event-stream" \
            -d "$FETCH_PAYLOAD" \
            --max-time 10 \
            "${BASE_URL}/mcp" 2>/dev/null)

        # Detect "not found" vs. success vs. other error.
        if printf '%s' "$FETCH_RESP" | python3 -c '
import sys, json

raw = sys.stdin.read()
# SSE lines start with "data: "; plain JSON otherwise.
for line in raw.splitlines():
    line = line.strip()
    if line.startswith("data:"):
        line = line[5:].strip()
    if not line:
        continue
    try:
        obj = json.loads(line)
    except json.JSONDecodeError:
        continue
    # Look for a result or error at top level or nested.
    result = obj.get("result", obj)
    content = result.get("content", [])
    for c in content:
        if isinstance(c, dict) and "text" in c:
            try:
                inner = json.loads(c["text"])
                if "not found" in str(inner).lower():
                    sys.exit(1)
            except Exception:
                pass
    if result.get("isError"):
        text_items = [c.get("text","") for c in content if isinstance(c,dict)]
        if any("not found" in t.lower() for t in text_items):
            sys.exit(1)
        sys.exit(2)
    sys.exit(0)
sys.exit(2)
' 2>/dev/null; then
            FETCH_SUCCESS=$((FETCH_SUCCESS + 1))
        elif [[ $? -eq 1 ]]; then
            FETCH_404=$((FETCH_404 + 1))
            echo "  ORPHAN: $id (not found via memory_fetch)" >&2
        else
            FETCH_OTHER_ERROR=$((FETCH_OTHER_ERROR + 1))
            echo "  ERROR:  $id (unexpected error from memory_fetch)" >&2
        fi
    done <<< "$IDS"
fi

# ── report ────────────────────────────────────────────────────────────────────
echo ""
echo "verify-recall-fetch results"
echo "---------------------------"
echo "total_handles:     $TOTAL_HANDLES"
echo "fetch_success:     $FETCH_SUCCESS"
echo "fetch_404:         $FETCH_404"
echo "fetch_other_error: $FETCH_OTHER_ERROR"

if [[ "$FETCH_404" -gt 0 ]]; then
    echo ""
    echo "FAIL: $FETCH_404 orphaned handle(s) detected — index references IDs with no store row." >&2
    exit 1
fi

if [[ "$FETCH_OTHER_ERROR" -gt 0 ]]; then
    echo ""
    echo "WARN: $FETCH_OTHER_ERROR handle(s) returned unexpected errors (not 404)." >&2
    exit 1
fi

echo ""
echo "OK: all $TOTAL_HANDLES handles resolved successfully."
exit 0
