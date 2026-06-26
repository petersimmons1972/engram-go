#!/usr/bin/env bash
# lme-lever-matrix.sh — Run Phase 4 lever matrix for lme-s-pref30 × Qwen3-32B
# Waits for any existing Run A process, then runs B and C sequentially.

set -euo pipefail
MAIN=${ENGRAM_MAIN:-$HOME/projects/engram-go}
WORKTREE=${ENGRAM_WORKTREE:-$MAIN/.claude/worktrees/lme-preference-constraint}
LOG=$MAIN/results/lever-matrix.log

log() { echo "[$(date -u +%H:%M:%SZ)] $*" | tee -a "$LOG"; }

cd "$MAIN"
mkdir -p results

# ── Wait for any running Run A process ──────────────────────────────────────
# pgrep may return multiple PIDs (parent+child); take first
EXISTING=$(pgrep -f "longmemeval run.*lme-s-qwen3-32b-pe-20260626" | head -1 || true)
if [[ -n "$EXISTING" ]]; then
    log "Run A already in progress (PID $EXISTING) — waiting..."
    while kill -0 "$EXISTING" 2>/dev/null; do
        DONE=$(wc -l < results/lme-s-qwen3-32b-pe-20260626/checkpoint-run.jsonl 2>/dev/null || echo 0)
        log "  run A: $DONE/30 items"
        sleep 30
    done
    log "Run A generation process finished"
else
    DONE=$(wc -l < results/lme-s-qwen3-32b-pe-20260626/checkpoint-run.jsonl 2>/dev/null || echo 0)
    if [[ "$DONE" -lt 30 ]]; then
        log "Run A incomplete ($DONE/30), no process running — resuming..."
        ./bin/longmemeval run \
            --data results/lme-s-qwen3-32b-pe-20260626/lme-s-pref30.json \
            --out  results/lme-s-qwen3-32b-pe-20260626 \
            --llm-url http://oblivion.petersimmons.com:8000/v1 \
            --llm-model inference \
            --cleanup-policy never \
            --workers 4 \
            --preference-enumerate
        log "Run A generation done"
    else
        log "Run A generation already complete ($DONE/30)"
    fi
fi

# ── Score Run A ─────────────────────────────────────────────────────────────
if [[ ! -f results/lme-s-qwen3-32b-pe-20260626/score_report.json ]]; then
    log "Scoring Run A..."
    ./bin/longmemeval score-efficient \
        --data results/lme-s-qwen3-32b-pe-20260626/lme-s-pref30.json \
        --out  results/lme-s-qwen3-32b-pe-20260626 \
        --scorer-url http://192.168.0.138:30411/olla/openai/v1 \
        --scorer-model inference \
        --workers 4
    log "Run A scored"
else
    log "Run A already scored"
fi

cd "$WORKTREE"
bash bin/bench-register.sh \
    --result-dir "$MAIN/results/lme-s-qwen3-32b-pe-20260626" \
    --suite lme-s-pref30 \
    --model "Qwen/Qwen3-32B" \
    --model-family Qwen3 \
    --hardware oblivion/DGX-Spark-GB10 \
    --generator llm-direct \
    --notes "pe only lever — does enumerate help vs no-levers 50.0%?" \
    2>&1 | tee -a "$LOG"
cd "$MAIN"

# ── Run B: pe + ef ──────────────────────────────────────────────────────────
log "Starting Run B (pe+ef)..."
mkdir -p results/lme-s-qwen3-32b-pe-ef-20260626
cp results/lme-s-qwen3-32b-nothinking-20260626/lme-s-pref30.json results/lme-s-qwen3-32b-pe-ef-20260626/
cp results/lme-s-qwen3-32b-nothinking-20260626/checkpoint-ingest.jsonl results/lme-s-qwen3-32b-pe-ef-20260626/

./bin/longmemeval run \
    --data results/lme-s-qwen3-32b-pe-ef-20260626/lme-s-pref30.json \
    --out  results/lme-s-qwen3-32b-pe-ef-20260626 \
    --llm-url http://oblivion.petersimmons.com:8000/v1 \
    --llm-model inference \
    --cleanup-policy never \
    --workers 4 \
    --preference-enumerate \
    --enumerate-first
log "Run B generation done"

./bin/longmemeval score-efficient \
    --data results/lme-s-qwen3-32b-pe-ef-20260626/lme-s-pref30.json \
    --out  results/lme-s-qwen3-32b-pe-ef-20260626 \
    --scorer-url http://192.168.0.138:30411/olla/openai/v1 \
    --scorer-model inference \
    --workers 4
log "Run B scored"

cd "$WORKTREE"
bash bin/bench-register.sh \
    --result-dir "$MAIN/results/lme-s-qwen3-32b-pe-ef-20260626" \
    --suite lme-s-pref30 \
    --model "Qwen/Qwen3-32B" \
    --model-family Qwen3 \
    --hardware oblivion/DGX-Spark-GB10 \
    --generator llm-direct \
    --notes "pe+ef — enumerate-first reordering on top of pe" \
    2>&1 | tee -a "$LOG"
cd "$MAIN"

# ── Run C: context-topk 30 ──────────────────────────────────────────────────
log "Starting Run C (topk=30)..."
mkdir -p results/lme-s-qwen3-32b-topk30-20260626
cp results/lme-s-qwen3-32b-nothinking-20260626/lme-s-pref30.json results/lme-s-qwen3-32b-topk30-20260626/
cp results/lme-s-qwen3-32b-nothinking-20260626/checkpoint-ingest.jsonl results/lme-s-qwen3-32b-topk30-20260626/

./bin/longmemeval run \
    --data results/lme-s-qwen3-32b-topk30-20260626/lme-s-pref30.json \
    --out  results/lme-s-qwen3-32b-topk30-20260626 \
    --llm-url http://oblivion.petersimmons.com:8000/v1 \
    --llm-model inference \
    --cleanup-policy never \
    --workers 4 \
    --context-topk 30
log "Run C generation done"

./bin/longmemeval score-efficient \
    --data results/lme-s-qwen3-32b-topk30-20260626/lme-s-pref30.json \
    --out  results/lme-s-qwen3-32b-topk30-20260626 \
    --scorer-url http://192.168.0.138:30411/olla/openai/v1 \
    --scorer-model inference \
    --workers 4
log "Run C scored"

cd "$WORKTREE"
bash bin/bench-register.sh \
    --result-dir "$MAIN/results/lme-s-qwen3-32b-topk30-20260626" \
    --suite lme-s-pref30 \
    --model "Qwen/Qwen3-32B" \
    --model-family Qwen3 \
    --hardware oblivion/DGX-Spark-GB10 \
    --generator llm-direct \
    --notes "context-topk=30 — wider retrieval window vs default" \
    2>&1 | tee -a "$LOG"

# ── Final report ────────────────────────────────────────────────────────────
log "=== LEVER MATRIX COMPLETE ==="
python3 bin/bench-report.py --suite lme-s-pref30 2>&1 | tee -a "$LOG"

git add results/benchmark-registry.jsonl
git commit -m "$(printf 'data(lme): Phase 4 lever matrix results — Qwen3-32B x pref30\n\nRun A: --preference-enumerate\nRun B: --preference-enumerate --enumerate-first\nRun C: --context-topk 30\n\nCo-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>\nClaude-Session: https://claude.ai/code/session_01FDMQaqHn8mC9CzZpwBwM6S')" \
    2>&1 | tee -a "$LOG"
log "Registry committed. DONE."
