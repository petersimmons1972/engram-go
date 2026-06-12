#!/usr/bin/env python3
"""
Render lessons-learned.jsonl → lessons-learned.md (human-readable view).
JSONL is canonical; the .md is a generated view only.

Usage:
    python3 ~/bin/render-lessons-learned.py
    # or to append a new lesson record:
    python3 ~/bin/render-lessons-learned.py --append '{"ts":"2026-06-11T...","trigger":"user_correction","title":"...","lesson":"..."}'
"""

import json
import sys
from pathlib import Path

JSONL_PATH = Path.home() / ".claude/projects/-home-psimmons/memory/lessons-learned.jsonl"
MD_PATH    = Path.home() / ".claude/projects/-home-psimmons/memory/lessons-learned.md"


def append_record(record_json: str) -> None:
    """Append a single JSON record to the JSONL file."""
    record = json.loads(record_json)
    with open(JSONL_PATH, "a") as f:
        f.write(json.dumps(record) + "\n")
    print(f"Appended record: {record.get('title', '(no title)')}")


def render() -> None:
    """Regenerate the markdown view from JSONL."""
    if not JSONL_PATH.exists():
        print(f"WARNING: {JSONL_PATH} not found")
        sys.exit(1)

    records = []
    with open(JSONL_PATH) as f:
        for line in f:
            line = line.strip()
            if line:
                records.append(json.loads(line))

    lines = [
        "# Lessons Learned",
        "",
        "Per CLAUDE.md: append a dated entry whenever the user corrects course or reframes a problem.",
        "**Source of truth: `lessons-learned.jsonl`** — this file is auto-generated.",
        "Regenerate with: `python3 ~/bin/render-lessons-learned.py`",
        "Append new entry: `python3 ~/bin/render-lessons-learned.py --append '{...}'`",
        "",
    ]

    for r in records:
        ts = r.get("ts", "")
        date_str = ts[:10] if ts else "unknown"
        title = r.get("title", "(untitled)")
        lesson = r.get("lesson", "").strip()

        lines.append(f"## {date_str} — {title}")
        lines.append("")
        if lesson:
            lines.append(lesson)
        lines.append("")

    MD_PATH.write_text("\n".join(lines))
    print(f"Rendered {len(records)} records -> {MD_PATH}")
    print(f"Size: {MD_PATH.stat().st_size} bytes, {len(records)} entries")


def main() -> None:
    args = sys.argv[1:]
    if args and args[0] == "--append" and len(args) >= 2:
        append_record(args[1])
        render()
    elif not args:
        render()
    else:
        print(__doc__)
        sys.exit(1)


if __name__ == "__main__":
    main()
