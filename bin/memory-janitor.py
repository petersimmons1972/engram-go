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
    entries = re.findall(r"^## \[\d{4}-\d{2}-\d{2}\]", content, re.MULTILINE)

    if not entries:
        print("🧹 Memory janitor: fallback.md clean — no pending entries")
        return 0

    print(f"🧹 Memory janitor: ⚠️  {len(entries)} pending entr{'y' if len(entries)==1 else 'ies'} in fallback.md — flush to Engram when available")
    return 0


if __name__ == "__main__":
    sys.exit(main())
