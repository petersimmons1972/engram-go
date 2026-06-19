#!/bin/bash
# Post-turn checks: surface debug artifacts and changed files

# Drain stdin first: CC pipes the session transcript JSON here on Stop events.
# Without this, the kernel sends SIGPIPE to CC's write side if transcript >64KB,
# causing CC to log a spurious hook error. (FM-89)
cat > /dev/null

CHANGED=$(git diff --name-only 2>/dev/null)
[ -z "$CHANGED" ] && exit 0

# Show what changed this turn
echo "── changed ──────────────────────────────"
git diff --stat 2>/dev/null

# Debug artifacts in Python — only if a .py file changed
if echo "$CHANGED" | grep -q '\.py$'; then
    PY_HITS=$(git diff -- '*.py' 2>/dev/null | grep '^+' | grep -E '(print\(|pdb\.set_trace|breakpoint\(\)|import pdb)' | grep -v '^+++')
    if [ -n "$PY_HITS" ]; then
        echo "── ⚠️  debug artifacts (Python) ─────────"
        echo "$PY_HITS"
    fi
fi

# Console.log in JS/TS — only if a .js/.ts/.jsx/.tsx file changed
if echo "$CHANGED" | grep -qE '\.(js|ts|jsx|tsx)$'; then
    JS_HITS=$(git diff -- '*.ts' '*.tsx' '*.js' '*.jsx' 2>/dev/null | grep '^+' | grep 'console\.log' | grep -v '^+++')
    if [ -n "$JS_HITS" ]; then
        echo "── ⚠️  console.log (JS/TS) ──────────────"
        echo "$JS_HITS"
    fi
fi

exit 0
