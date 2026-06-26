#!/usr/bin/env bash
# bench-register.sh — append a completed LME result to results/benchmark-registry.jsonl
#
# Registry schema (one JSON object per line):
#   ts               ISO8601 UTC timestamp of registration
#   run_id           hex run ID from score_report.json
#   suite            lme-s-pref30 | lme-s-user30 | lme-s-multi30 | lme-s-temporal30 | full-500
#   model            HuggingFace model ID or API model name
#   model_family     Qwen3 | Qwen2.5 | Nemotron | Sonnet | Haiku | Opus
#   hardware         oblivion/DGX-Spark-GB10 | leviathan/7900XT | olla/cluster
#   generator        llm-direct | sonnet | haiku | opus
#   config           object: preference_enumerate, enumerate_first, context_topk, enable_thinking, recall_topk
#   strict_accuracy  float 0-1
#   lenient_accuracy float 0-1
#   correct          int (strict credited_correct)
#   total            int
#   result_dir       relative path to result directory
#   git_sha          git SHA at time of run (from RUN_STATUS.json)
#   notes            free-form annotation
#
# Usage:
#   bin/bench-register.sh \
#     --result-dir  results/lme-s-qwen3-32b-nothinking-20260626 \
#     --suite       lme-s-pref30 \
#     --model       "Qwen/Qwen3-32B" \
#     --model-family Qwen3 \
#     --hardware    oblivion/DGX-Spark-GB10 \
#     --generator   llm-direct \
#     --notes       "gate PASS 50.0% no levers BF16"

set -euo pipefail

RESULT_DIR=""
SUITE=""
MODEL=""
MODEL_FAMILY=""
HARDWARE=""
GENERATOR="llm-direct"
NOTES=""
BY_TYPE=""  # optional: use by_type[TYPE] instead of overall (for full-500 runs with type breakdown)

usage() {
    echo "Usage: $0 --result-dir DIR --suite SUITE --model MODEL --model-family FAMILY --hardware HW [--generator GEN] [--by-type QTYPE] [--notes TEXT]"
    echo "  --by-type QTYPE   Use score from by_type[QTYPE] instead of overall (e.g. single-session-preference)"
    exit 1
}

while [[ $# -gt 0 ]]; do
    case "$1" in
        --result-dir)   RESULT_DIR="$2"; shift 2 ;;
        --suite)        SUITE="$2"; shift 2 ;;
        --model)        MODEL="$2"; shift 2 ;;
        --model-family) MODEL_FAMILY="$2"; shift 2 ;;
        --hardware)     HARDWARE="$2"; shift 2 ;;
        --generator)    GENERATOR="$2"; shift 2 ;;
        --by-type)      BY_TYPE="$2"; shift 2 ;;
        --notes)        NOTES="$2"; shift 2 ;;
        --help|-h)      usage ;;
        *) echo "Unknown flag: $1"; usage ;;
    esac
done

[[ -z "$RESULT_DIR" ]] && { echo "ERROR: --result-dir required"; usage; }
[[ -z "$SUITE"      ]] && { echo "ERROR: --suite required"; usage; }
[[ -z "$MODEL"      ]] && { echo "ERROR: --model required"; usage; }
[[ -z "$MODEL_FAMILY" ]] && { echo "ERROR: --model-family required"; usage; }
[[ -z "$HARDWARE"   ]] && { echo "ERROR: --hardware required"; usage; }

SCORE_FILE="$RESULT_DIR/score_report.json"
STATUS_FILE="$RESULT_DIR/RUN_STATUS.json"

[[ -f "$SCORE_FILE"  ]] || { echo "ERROR: $SCORE_FILE not found"; exit 1; }

# Resolve registry path relative to this script
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
REGISTRY="$REPO_ROOT/results/benchmark-registry.jsonl"

python3 - "$RESULT_DIR" "$SUITE" "$MODEL" "$MODEL_FAMILY" "$HARDWARE" "$GENERATOR" "$NOTES" "$SCORE_FILE" "${STATUS_FILE:-}" "$REGISTRY" "${BY_TYPE:-}" <<'PYEOF'
import sys, json, datetime, os

result_dir, suite, model, model_family, hardware, generator, notes, score_file, status_file, registry, by_type = sys.argv[1:12]

score = json.load(open(score_file))
status = {}
if status_file and os.path.exists(status_file):
    status = json.load(open(status_file))

# Use by_type breakdown if requested (e.g. registering ss-pref slice of a full-500 run)
if by_type:
    bt = score.get("by_type", {})
    if by_type not in bt:
        print(f"ERROR: by_type '{by_type}' not in score_report. Available: {list(bt.keys())}", file=sys.stderr)
        sys.exit(1)
    o = bt[by_type]
else:
    o = score["overall"]

# Parse config flags from command_line list in RUN_STATUS
cmd = status.get("command_line", [])

def has_flag(f):
    return f in cmd

def flag_val(f, default=0):
    try:
        return int(cmd[cmd.index(f) + 1])
    except (ValueError, IndexError):
        return default

config = {
    "preference_enumerate": has_flag("--preference-enumerate"),
    "enumerate_first":      has_flag("--enumerate-first"),
    "context_topk":         flag_val("--context-topk", default=0),
    "enable_thinking":      has_flag("--enable-thinking"),
    "recall_topk":          flag_val("--recall-topk", default=100),
    "inject_question_date": has_flag("--inject-question-date"),
    "chrono_sort":          has_flag("--chrono-sort"),
}

record = {
    "ts":               datetime.datetime.utcnow().strftime("%Y-%m-%dT%H:%M:%SZ"),
    "run_id":           score.get("run_id", ""),
    "suite":            suite,
    "model":            model,
    "model_family":     model_family,
    "hardware":         hardware,
    "generator":        generator,
    "config":           config,
    "strict_accuracy":  round(o["strict"]["accuracy"], 6),
    "lenient_accuracy": round(o["lenient"]["accuracy"], 6),
    "correct":          o["strict"]["credited_correct"],
    "total":            o["total"],
    "result_dir":       result_dir,
    "git_sha":          status.get("git_sha", ""),
    "notes":            notes,
}

os.makedirs(os.path.dirname(registry), exist_ok=True)
with open(registry, "a") as f:
    f.write(json.dumps(record) + "\n")

print(f"✓ Registered: {suite} | {model_family} | strict={record['strict_accuracy']:.1%} ({record['correct']}/{record['total']}) | {notes[:60]}")
PYEOF
