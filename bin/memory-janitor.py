#!/usr/bin/env python3
"""
Memory janitor — checks if fallback.md has pending Engram entries.
Engram is the primary store. fallback.md is a staging area only.
"""

import re
import sys
from pathlib import Path

FALLBACK = Path.home() / ".claude/projects/-home-psimmons/memory/fallback.md"


def main():
    if not FALLBACK.exists():
        print("🧹 Memory janitor: fallback.md not found — OK")
        return 0

    content = FALLBACK.read_text()
    entries_dated = re.findall(r"^## \[\d{4}-\d{2}-\d{2}\]", content, re.MULTILINE)
    entries_pending = re.findall(r"^PENDING_ENGRAM_ENTRY", content, re.MULTILINE)
    total = len(entries_dated) + len(entries_pending)

    if total == 0:
        print("🧹 Memory janitor: fallback.md clean — no pending entries")
        return 0

    print(f"🧹 Memory janitor: ⚠️  {total} pending entr{'y' if total==1 else 'ies'} in fallback.md — flush to Engram when available")
    return 0


if __name__ == "__main__":
    sys.exit(main())
