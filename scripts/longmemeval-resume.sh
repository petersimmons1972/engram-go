#!/usr/bin/env bash
set -euo pipefail

# Maintained resume wrapper for run/score stages.
# Optional RUN_PID waits for a running "longmemeval run" process before scoring.

REPO=${REPO:-/home/psimmons/projects/engram-go}
BIN=${BIN:-$REPO/longmemeval}
DATA=${DATA:-$REPO/testdata/longmemeval/longmemeval_m_cleaned.json}
OUT=${OUT:-$REPO/results/longmemeval}
LLM_URL=${LLM_URL:-}
LLM_MODEL=${LLM_MODEL:-}
WORKERS=${WORKERS:-1}
RUN_INGEST=${RUN_INGEST:-0}
SKIP_SCORE=${SKIP_SCORE:-0}
RUN_PID=${RUN_PID:-}
ROUTE_DISCOVER=${ROUTE_DISCOVER:-0}

if [[ "$ROUTE_DISCOVER" == "1" ]]; then
  route_path="$OUT/route-discover.json"
  mkdir -p "$OUT"
  go run "$REPO/cmd/longmemeval" route-discover --purpose generation > "$route_path"
  LLM_URL=$(jq -r '.llm_url' "$route_path")
  LLM_MODEL=$(jq -r '.llm_model' "$route_path")
  echo "route-discover: $route_path"
fi

if [[ -z "$LLM_URL" || -z "$LLM_MODEL" ]]; then
  echo "ERROR: LLM_URL and LLM_MODEL are required (or set ROUTE_DISCOVER=1)"
  exit 1
fi

mkdir -p "$OUT"

if [[ "$RUN_INGEST" == "1" && ! -s "$OUT/checkpoint-ingest.jsonl" ]]; then
  "$BIN" ingest --data "$DATA" --out "$OUT" --workers "$WORKERS" --cleanup-policy never 2>&1 | tee "$OUT/ingest.log"
fi

"$BIN" run \
  --data "$DATA" \
  --out "$OUT" \
  --llm-url "$LLM_URL" \
  --llm-model "$LLM_MODEL" \
  --workers "$WORKERS" \
  --cleanup-policy never \
  2>&1 | tee -a "$OUT/run.log"

if [[ "$SKIP_SCORE" == "1" ]]; then
  exit 0
fi

while [[ -n "$RUN_PID" ]] && kill -0 "$RUN_PID" 2>/dev/null; do
  sleep 30
done

"$BIN" score \
  --data "$DATA" \
  --out "$OUT" \
  --llm-url "$LLM_URL" \
  --llm-model "$LLM_MODEL" \
  --workers "$WORKERS" \
  2>&1 | tee "$OUT/score.log"
