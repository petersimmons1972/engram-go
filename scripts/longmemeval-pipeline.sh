#!/usr/bin/env bash
set -euo pipefail

# Maintained LongMemEval pipeline wrapper.
# This script replaces ad-hoc result-local wrappers in ignored results trees.

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
DEFAULT_REPO="$(cd "${SCRIPT_DIR}/.." && pwd)"

REPO=${REPO:-$DEFAULT_REPO}
BIN=${BIN:-$REPO/longmemeval}
DATA=${DATA:-$REPO/testdata/longmemeval/longmemeval_m_cleaned.json}
OUT=${OUT:-$REPO/results/longmemeval}
RUN_ID=${RUN_ID:-}
WORKERS=${WORKERS:-1}
RECALL_TOPK=${RECALL_TOPK:-100}
CONTEXT_TOPK=${CONTEXT_TOPK:-15}
GEN_URL=${GEN_URL:-}
GEN_MODEL=${GEN_MODEL:-}
SCORER_URL=${SCORER_URL:-}
SCORER_MODEL=${SCORER_MODEL:-}
ROUTE_DISCOVER=${ROUTE_DISCOVER:-0}

log() { echo "$(date '+%Y/%m/%d %H:%M:%S') $*"; }

if [[ ! -x "$BIN" ]]; then
  log "ERROR: binary not found or not executable: $BIN"
  exit 1
fi

mkdir -p "$OUT"

if [[ "$ROUTE_DISCOVER" == "1" ]]; then
  route_path="$OUT/route-discover.json"
  go run "$REPO/cmd/longmemeval" route-discover --purpose generation > "$route_path"
  GEN_URL=$(jq -r '.llm_url' "$route_path")
  GEN_MODEL=$(jq -r '.llm_model' "$route_path")
  log "route-discover captured at $route_path"
fi

if [[ -z "$GEN_URL" || -z "$GEN_MODEL" ]]; then
  log "ERROR: GEN_URL and GEN_MODEL are required (or set ROUTE_DISCOVER=1)"
  exit 1
fi

if [[ -z "$SCORER_URL" ]]; then
  SCORER_URL="$GEN_URL"
fi
if [[ -z "$SCORER_MODEL" ]]; then
  SCORER_MODEL="$GEN_MODEL"
fi

run_flags=(
  --data "$DATA"
  --out "$OUT"
  --workers "$WORKERS"
  --recall-topk "$RECALL_TOPK"
  --context-topk "$CONTEXT_TOPK"
  --llm-url "$GEN_URL"
  --llm-model "$GEN_MODEL"
  --cleanup-policy never
)
if [[ -n "$RUN_ID" ]]; then
  run_flags+=(--run-id "$RUN_ID")
fi

ingest_flags=(--data "$DATA" --out "$OUT" --workers "$WORKERS" --cleanup-policy never)
if [[ -n "$RUN_ID" ]]; then
  ingest_flags+=(--run-id "$RUN_ID")
fi

score_flags=(
  --data "$DATA"
  --out "$OUT"
  --scorer-url "$SCORER_URL"
  --scorer-model "$SCORER_MODEL"
  --workers "$WORKERS"
  --preserve-correct
)

log "=== ingest ==="
"$BIN" ingest "${ingest_flags[@]}"

log "=== run ==="
"$BIN" run "${run_flags[@]}"

log "=== score-efficient ==="
"$BIN" score-efficient "${score_flags[@]}"

log "complete: $OUT"
