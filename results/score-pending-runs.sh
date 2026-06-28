#!/usr/bin/env bash
# Score pending P8E2 + KU runs when complete. Run manually or via cron.
set -euo pipefail

REPO="${REPO:-$HOME/projects/engram-go}"
JUDGE=$REPO/scripts/lme-judge.sh

P8E2=$REPO/results/lme-s-qwen3-32b-multi30-P8E2-recall-repair-20260628
KU_BASE=$REPO/results/lme-s-ku78-qwen3-32b-baseline-20260628
KU_REC=$REPO/results/lme-s-ku78-qwen3-32b-ku-recency-20260628

count_done() { wc -l < "${1}/checkpoint-run.jsonl" 2>/dev/null || echo 0; }

show_score() {
  python3 -c "import json; d=json.load(open('$1/score_report.json')); o=d.get('overall',{}); s=o.get('strict',{}); print(f'  strict={s.get(\"accuracy\",0)*100:.1f}% ({s.get(\"credited_correct\",0)}/{s.get(\"total\",0)})')"
}

echo "=== $(date) ==="

echo ""
echo "P8E2 (30 items, recall-repair):"
n=$(count_done "$P8E2"); echo "  checkpoint entries: $n/30"
if [[ "$n" -ge 30 ]] && [[ ! -f "$P8E2/score_report.json" ]]; then
  echo "  Scoring P8E2..."
  REPO=$REPO bash "$JUDGE" --run "$P8E2" --judge qwen3 2>&1 | tee "$P8E2/score.log"
elif [[ -f "$P8E2/score_report.json" ]]; then show_score "$P8E2"; fi

echo ""
echo "KU baseline (78 items, no flags):"
n=$(count_done "$KU_BASE"); echo "  checkpoint entries: $n/78"
if [[ "$n" -ge 78 ]] && [[ ! -f "$KU_BASE/score_report.json" ]]; then
  echo "  Scoring KU baseline..."
  REPO=$REPO bash "$JUDGE" --run "$KU_BASE" --judge qwen3 2>&1 | tee "$KU_BASE/score.log"
elif [[ -f "$KU_BASE/score_report.json" ]]; then show_score "$KU_BASE"; fi

echo ""
echo "KU recency (78 items, --ku-recency-prompt):"
n=$(count_done "$KU_REC"); echo "  checkpoint entries: $n/78"
if [[ "$n" -ge 78 ]] && [[ ! -f "$KU_REC/score_report.json" ]]; then
  echo "  Scoring KU recency..."
  REPO=$REPO bash "$JUDGE" --run "$KU_REC" --judge qwen3 2>&1 | tee "$KU_REC/score.log"
elif [[ -f "$KU_REC/score_report.json" ]]; then show_score "$KU_REC"; fi
