#!/usr/bin/env bash
set -euo pipefail

# Maintained judging harness for LongMemEval score reports.
# - Supports bundled judge presets (qwen3 and gpt4o)
# - Preserves CORRECT rows by default on unlocked judges (resume-friendly)
# - qwen3 is lock-backed; lock-owned scorer knobs must not be overridden here
# - Emits strict and lenient percentages with optional comparison deltas

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO=${REPO:-$(cd "$SCRIPT_DIR/.." && pwd)}
BIN=${BIN:-$REPO/longmemeval}
DATA=${DATA:-$REPO/testdata/longmemeval/longmemeval_m_cleaned.json}
SCORER_LOCK=${SCORER_LOCK:-$REPO/docs/lme-campaign/scorer-lock.json}
ITEM_SET=${ITEM_SET:-lme-s-500q}
SYSTEM=${SYSTEM:-engram-go}
WORKERS=${WORKERS:-4}
COMPARE=
RUN_DIR=
JUDGE=
THINKING=on
GOLD_VERSION=
BUNDLE=0
SHOW_HELP=0
SCORER_MAX_TOKENS=${SCORER_MAX_TOKENS:-2048}
GPT4O_MODEL=${GPT4O_MODEL:-gpt-4o-2024-11-20}

usage() {
  cat <<'EOF'
Usage: lme-judge.sh --run <results-dir> --judge <qwen3|gpt4o> [--thinking off]
       lme-judge.sh --run <results-dir> --bundle

Options:
  --run <dir>      LongMemEval output directory containing run checkpoints
  --judge <name>   Judge preset: qwen3 | gpt4o
  --gold-version <tag>  Frozen gold snapshot/version tag (required)
  --thinking <on|off>  Scorer chain-of-thought flag (default: on)
  --compare <dir>   Also print delta against baseline score_report.json
  --bundle          Run both qwen3 (default thinking on) and gpt4o, writing suffix
  --help            Show this help
EOF
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --run)
      RUN_DIR="$2"
      shift 2
      ;;
    --judge)
      JUDGE="$2"
      shift 2
      ;;
    --gold-version)
      GOLD_VERSION="$2"
      shift 2
      ;;
    --thinking)
      THINKING="$2"
      shift 2
      ;;
    --compare)
      COMPARE="$2"
      shift 2
      ;;
    --bundle)
      BUNDLE=1
      shift 1
      ;;
    --help|-h)
      SHOW_HELP=1
      shift 1
      break
      ;;
    *)
      echo "ERROR: unknown argument: $1" >&2
      usage >&2
      exit 2
      ;;
  esac
done

if [[ "$SHOW_HELP" == "1" ]]; then
  usage
  help_exit=0
  exit "$help_exit"
fi

if [[ -z "$RUN_DIR" ]]; then
  echo "ERROR: --run is required" >&2
  usage >&2
  exit 2
fi
if [[ -z "$GOLD_VERSION" ]]; then
  echo "ERROR: --gold-version is required" >&2
  usage >&2
  exit 2
fi
if [[ "$BUNDLE" == "1" ]]; then
  :
elif [[ -z "$JUDGE" ]]; then
  echo "ERROR: --judge is required when --bundle is not set" >&2
  usage >&2
  exit 2
fi
if [[ "$THINKING" != "on" && "$THINKING" != "off" ]]; then
  echo "ERROR: --thinking must be on or off" >&2
  exit 2
fi

if [[ ! -x "$BIN" ]]; then
  echo "ERROR: binary not found or not executable: $BIN" >&2
  exit 1
fi

run_judge() {
  local judge=$1
  local out_dir=$2
  local scorer_url=""
  local scorer_model=""
  local scorer_api_key=""
  local scorer_thinking="--scorer-thinking=true"
  local -a score_args=()

  case "$judge" in
    qwen3)
      if [[ ! -f "$SCORER_LOCK" ]]; then
        echo "ERROR: qwen3 scorer lock not found: $SCORER_LOCK" >&2
        exit 1
      fi
      ;;
    gpt4o)
      scorer_url="https://api.openai.com/v1"
      scorer_model="$GPT4O_MODEL"
      scorer_api_key="${LME_SCORER_API_KEY:-${OPENAI_API_KEY:-}}"
      if [[ -z "$scorer_api_key" ]]; then
        echo "ERROR: --judge=gpt4o requires LME_SCORER_API_KEY or OPENAI_API_KEY" >&2
        exit 1
      fi
      if [[ "$THINKING" != "on" ]]; then
        scorer_thinking="--scorer-thinking=false"
      fi
      ;;
    *)
      echo "ERROR: unknown judge '$judge'" >&2
      exit 1
  esac

  mkdir -p "$out_dir"
  score_args=(
    score-efficient
    --data "$DATA"
    --out "$out_dir"
    --gold-version "$GOLD_VERSION"
    --item-set "$ITEM_SET"
    --system "$SYSTEM"
    --workers "$WORKERS"
  )
  if [[ "$judge" == "qwen3" ]]; then
    score_args+=(
      --scorer-lock "$SCORER_LOCK"
    )
  else
    score_args+=(
      --scorer-url "$scorer_url"
      --scorer-model "$scorer_model"
      --scorer-api-key "$scorer_api_key"
      "$scorer_thinking"
      --scorer-max-tokens "$SCORER_MAX_TOKENS"
      --preserve-correct
    )
  fi
  echo "==> score-efficient: $judge -> $out_dir"
  "$BIN" "${score_args[@]}"
}

read_report() {
  local report_path="$1"
  python3 - "$report_path" <<'PY'
import json
import sys

path = sys.argv[1]
with open(path, "r", encoding="utf-8") as fp:
    report = json.load(fp)

def _overall(report):
    return report.get("overall", {}).get("correct", 0), \
           report.get("overall", {}).get("partially_correct", 0), \
           report.get("overall", {}).get("total", 0)

def ratio(a, b):
    if b == 0:
        return 0.0
    return a / b * 100.0

correct, partial, total = _overall(report)
overall_strict = ratio(correct, total)
overall_lenient = ratio(correct + partial, total)

print("Overall strict: %.2f%% (%d/%d)" % (overall_strict, correct, total))
print("Overall lenient: %.2f%% (%d/%d)" % (overall_lenient, correct + partial, total))

comparison = report.get("baseline_comparison", {})
status = comparison.get("status")
nearest = comparison.get("nearest_baseline")
observed = comparison.get("observed_strict_pct")
if status:
    print("Baseline comparison: %s (nearest=%s, strict=%.2f%%)" %
          (status, nearest, observed or 0.0))

by_type = report.get("by_type", {})
if by_type:
    print()
    print("By question_type:")
    for qtype in sorted(by_type.keys()):
        row = by_type[qtype]
        c = row.get("correct", 0)
        p = row.get("partially_correct", 0)
        t = row.get("total", 0)
        print("  %-28s strict %.2f%% (%d/%d) lenient %.2f%% (%d/%d)" %
              (qtype, ratio(c, t), c, t, ratio(c + p, t), c + p, t))

error_items = report.get("error_items", [])
if error_items:
    print()
    print("Error items:")
    for row in error_items:
        print("  %-28s %s" % (row.get("question_id", "?"), row.get("error", "")))
PY
}

print_summary() {
  local report_path="$1"
  local label="$2"
  echo "==> $label"
  read_report "$report_path"
  echo
}

if [[ "$BUNDLE" == "1" ]]; then
  qwen_out="$RUN_DIR/qwen3"
  gpt_out="$RUN_DIR/gpt4o"
  run_judge qwen3 "$qwen_out"
  run_judge gpt4o "$gpt_out"
  print_summary "$qwen_out/score_report.json" "qwen3 (fast)"
  print_summary "$gpt_out/score_report.json" "gpt4o"
  if [[ -n "$COMPARE" ]]; then
    print_summary "$COMPARE/score_report.json" "compare baseline"
  fi
else
  run_judge "$JUDGE" "$RUN_DIR"
  print_summary "$RUN_DIR/score_report.json" "$JUDGE report"

  if [[ -n "$COMPARE" ]]; then
    compare_path="$COMPARE/score_report.json"
    if [[ ! -f "$compare_path" ]]; then
      echo "ERROR: baseline report missing: $compare_path" >&2
      exit 1
    fi

    python3 - "$RUN_DIR/score_report.json" "$compare_path" <<'PY'
import json
import sys

new_report_path, old_report_path = sys.argv[1], sys.argv[2]
with open(new_report_path, "r", encoding="utf-8") as fp:
    new = json.load(fp)
with open(old_report_path, "r", encoding="utf-8") as fp:
    old = json.load(fp)

def ratio(row, key):
    total = row.get("total", 0)
    if total == 0:
        return 0.0
    return row.get(key, 0) / total * 100.0

def strict(r): return ratio(r, "correct")
def lenient(r): return ratio(r, "partially_correct") + ratio(r, "correct")

new_overall = new.get("overall", {})
old_overall = old.get("overall", {})
new_strict = strict(new_overall)
old_strict = strict(old_overall)
new_lenient = lenient(new_overall)
old_lenient = lenient(old_overall)

print("Delta vs baseline:")
print("  strict:  %+0.2f%% (%+0.2f vs %+0.2f)" % (new_strict - old_strict, new_strict, old_strict))
print("  lenient: %+0.2f%% (%+0.2f vs %+0.2f)" % (new_lenient - old_lenient, new_lenient, old_lenient))
PY
  fi
fi
