#!/usr/bin/env python3
"""bench-report.py — render benchmark registry as a comparison table.

Usage:
  bin/bench-report.py [--suite SUITE] [--model SUBSTR] [--json]

Reads results/benchmark-registry.jsonl and prints a Markdown table sorted by
suite then strict_accuracy DESC.

Flags:
  --suite SUITE       Filter to a single suite name (e.g. lme-s-pref30)
  --model SUBSTR      Filter by substring match on model or model_family
  --family SUBSTR     Filter by substring match on model_family only
  --json              Raw JSON output (one object per line, filtered)
  --help              Show this message
"""
import sys
import json
import os
import argparse

def main():
    parser = argparse.ArgumentParser(description="LME benchmark report", add_help=True)
    parser.add_argument("--suite",  default="", help="Filter by suite name")
    parser.add_argument("--model",  default="", help="Filter by model or model_family substring")
    parser.add_argument("--family", default="", help="Filter by model_family substring")
    parser.add_argument("--json",   action="store_true", help="Raw JSON output")
    args = parser.parse_args()

    script_dir = os.path.dirname(os.path.abspath(__file__))
    registry   = os.path.join(script_dir, "..", "results", "benchmark-registry.jsonl")

    if not os.path.exists(registry):
        print(f"No registry found at {registry}", file=sys.stderr)
        sys.exit(1)

    records = []
    with open(registry) as f:
        for i, line in enumerate(f, 1):
            line = line.strip()
            if not line:
                continue
            try:
                records.append(json.loads(line))
            except json.JSONDecodeError as e:
                print(f"WARNING: line {i} invalid JSON: {e}", file=sys.stderr)

    # Filter
    if args.suite:
        records = [r for r in records if r.get("suite", "") == args.suite]
    if args.model:
        sub = args.model.lower()
        records = [r for r in records if sub in r.get("model", "").lower() or sub in r.get("model_family", "").lower()]
    if args.family:
        sub = args.family.lower()
        records = [r for r in records if sub in r.get("model_family", "").lower()]

    if not records:
        print("No matching records.")
        return

    # Sort: suite ASC, strict_accuracy DESC
    records.sort(key=lambda r: (r.get("suite",""), -r.get("strict_accuracy", 0)))

    if args.json:
        for r in records:
            print(json.dumps(r))
        return

    # Build config flags string
    def config_str(c):
        parts = []
        if c.get("preference_enumerate"): parts.append("pe")
        if c.get("enumerate_first"):      parts.append("ef")
        topk = c.get("context_topk", 0)
        if topk:                           parts.append(f"tk={topk}")
        if c.get("enable_thinking"):       parts.append("think")
        if c.get("inject_question_date"):  parts.append("qdate")
        if c.get("chrono_sort"):           parts.append("chrono")
        recall = c.get("recall_topk", 100)
        if recall != 100:                  parts.append(f"rk={recall}")
        return " ".join(parts) if parts else "—"

    # Markdown table
    cols = ["Suite", "Family", "Model", "Generator", "Config", "Strict%", "Lenient%", "Correct/Total", "Notes"]
    rows = []
    for r in records:
        rows.append([
            r.get("suite",""),
            r.get("model_family",""),
            # Shorten model name for readability
            r.get("model","").replace("Qwen/","").replace("nvidia/",""),
            r.get("generator",""),
            config_str(r.get("config",{})),
            f"{r.get('strict_accuracy',0):.1%}",
            f"{r.get('lenient_accuracy',0):.1%}",
            f"{r.get('correct',0)}/{r.get('total',0)}",
            r.get("notes","")[:50],
        ])

    # Column widths
    widths = [max(len(c), max((len(row[i]) for row in rows), default=0)) for i, c in enumerate(cols)]

    def fmt_row(cells):
        return "| " + " | ".join(c.ljust(w) for c, w in zip(cells, widths)) + " |"

    def sep_row():
        return "|-" + "-|-".join("-" * w for w in widths) + "-|"

    print(f"\n### LME Benchmark Results ({len(records)} records)\n")
    print(fmt_row(cols))
    print(sep_row())
    for row in rows:
        print(fmt_row(row))
    print()

if __name__ == "__main__":
    main()
