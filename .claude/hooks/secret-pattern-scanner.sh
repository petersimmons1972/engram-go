#!/usr/bin/env bash
# PreToolUse:Edit|Write — block writes/edits containing API keys or token patterns.
#
# Why: CLAUDE.md security: "NEVER put a secret in a command line, code, CI log,
# or GitHub issue/PR." This hook catches the case where a secret retrieved from
# Infisical is about to be written to disk — the only point we can intercept it.
#
# Design:
#   - Layer 1 (shell): skip known-safe paths (docs, tests, fixtures, ~/.claude/)
#     before Python runs — no scan cost on safe paths
#   - Layer 2 (Python): anchored patterns with minimum-length requirements to
#     reduce false positives; fail-open on any Python error
#   - Block message includes Infisical guidance so the author knows where to go
#
# False-positive mitigations:
#   - .md/.txt/.rst excluded (docs, failure-mode writeups, onboarding)
#   - test*/fixture*/example*/testdata* paths excluded
#   - ~/.claude/ excluded (config files, plans, memory)
#   - Patterns require minimum length (not just prefix match)
#   - password= pattern omitted entirely (too many test-config false positives)
#
# Fail-open guarantee: all Python failures → no stdout → hook exits 0.
# No set -e; unconditional exit 0 at end.

set -uo pipefail

input=$(cat)

filepath=$(python3 -c "
import sys, json
try:
    d = json.load(sys.stdin)
    print(d.get('tool_input', {}).get('file_path', ''))
except Exception:
    print('')
" <<< "$input" 2>/dev/null || true)

content=$(python3 -c "
import sys, json
try:
    d = json.load(sys.stdin)
    ti = d.get('tool_input', {})
    # Write tool uses 'content'; Edit tool uses 'new_string'. Prefer content, fall back.
    print(ti.get('content') or ti.get('new_string', ''))
except Exception:
    print('')
" <<< "$input" 2>/dev/null || true)

[ -z "$content" ] && exit 0

# Layer 1: skip known-safe paths before any scanning
case "$filepath" in
    # Documentation and prose
    *.md|*.txt|*.rst|*.org) exit 0 ;;
    # Test fixtures, examples, testdata — tightened to avoid matching 'attestation',
    # 'contest', 'contextual' etc. that contain 'test' as a substring (FM-91)
    */test/*|*/tests/*|test_*|*/test_*|*/fixture*|*/testdata/*|*/example/*|*/examples/*|*/mock/*|*/mocks/*|*/stub/*|*/stubs/*) exit 0 ;;
    # Claude config, memory, plans — never block writes here
    "$HOME/.claude/"*|"$HOME/.claude") exit 0 ;;
    # .env.example files are explicitly safe (they document, not store)
    *.env.example|*.env.sample|*.env.template) exit 0 ;;
esac

# Layer 2: anchored pattern scan.
# Content is passed via env var (_SECRET_SCANNER_CONTENT) to avoid two issues:
# (a) ARG_MAX: sys.argv is capped at ~3.2MB on Linux; large files cause execve E2BIG.
#     (FM-92)
# (b) pipe+heredoc conflict: `printf '%s' "$content" | python3 - <<'PYEOF'` feeds the
#     heredoc as the script source *and* as stdin simultaneously; bash resolves the
#     conflict by letting the heredoc win, leaving sys.stdin empty. Env var avoids this.
#     (FM-93)
export _SECRET_SCANNER_CONTENT="$content"
python3 - <<'PYEOF' 2>/dev/null || true
import os, re, json

content = os.environ.get('_SECRET_SCANNER_CONTENT', '')
if not content:
    raise SystemExit(0)

# Also search a whitespace-collapsed version to catch secrets split across
# line endings (e.g. a YAML block scalar that wraps mid-key). (FM-92)
content_compact = re.sub(r'\s+', '', content)

# Patterns are intentionally strict (anchored prefix + minimum length).
# Do not add loose patterns (e.g. bare 'token=') — false positive rate is too high.
patterns = [
    # Anthropic API key
    (r'\bsk-ant-[A-Za-z0-9_-]{20,}', 'Anthropic API key'),
    # GitHub personal access token (classic format)
    (r'\bghp_[A-Za-z0-9]{36}\b', 'GitHub personal access token'),
    # GitHub fine-grained PAT
    (r'\bgithub_pat_[A-Za-z0-9_]{40,}\b', 'GitHub fine-grained PAT'),
    # AWS access key
    (r'\bAKIA[0-9A-Z]{16}\b', 'AWS access key ID'),
    # Generic Bearer token (≥20 chars after "Bearer ")
    (r'\bBearer [A-Za-z0-9._/+=-]{20,}', 'Bearer token'),
    # OpenAI API key
    (r'\bsk-[A-Za-z0-9]{48}\b', 'OpenAI API key'),
]

hits = []
for pat, label in patterns:
    if re.search(pat, content) or re.search(pat, content_compact):
        hits.append(label)

if hits:
    reason = (
        'Possible secret(s) detected in file content: '
        + ', '.join(hits)
        + '. Secrets must live in Infisical (infisical.petersimmons.com). '
        + 'Reference the key path in code, never the value. '
        + '(secret-pattern-scanner.sh)'
    )
    print(json.dumps({'decision': 'block', 'reason': reason}))

PYEOF
unset _SECRET_SCANNER_CONTENT

exit 0
