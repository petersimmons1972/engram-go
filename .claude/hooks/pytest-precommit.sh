#!/usr/bin/env bash
# PreToolUse hook: run pytest before git commit
input=$(cat)
cmd=$(echo "$input" | python3 -c "import json,sys; d=json.load(sys.stdin); print(d.get('command',''))" 2>/dev/null)
if echo "$cmd" | grep -q "git commit"; then
  python3 -m pytest tests/ -x -q --tb=short 2>&1 | tail -5
fi
