#!/usr/bin/env bash
# verify-cutover.sh — Armies cutover verification
# Run after each phase to confirm migration state.
# Exit 0 = all checks pass. Exit 1 = failures found.

set -euo pipefail

PASS=0
FAIL=0

check() {
  local desc="$1"
  local result="$2"  # "ok" or error message
  if [[ "$result" == "ok" ]]; then
    echo "  ✅ $desc"
    PASS=$((PASS + 1))
  else
    echo "  ❌ $desc — $result"
    FAIL=$((FAIL + 1))
  fi
}

echo ""
echo "=== Phase 1: Operational data ported ==="

# Service records
sr_count=$(ls ~/.armies/service-records/*.yaml 2>/dev/null | wc -l || echo 0)
check "Service records present (≥30 YAMLs)" "$([[ $sr_count -ge 30 ]] && echo ok || echo "only $sr_count found")"

# Accountability
check "malus-ledger.yaml present" "$([[ -f ~/.armies/accountability/malus-ledger.yaml ]] && echo ok || echo "missing")"
check "saves-log.yaml present" "$([[ -f ~/.armies/accountability/saves-log.yaml ]] && echo ok || echo "missing")"
check "attribution-index.yaml present" "$([[ -f ~/.armies/accountability/attribution-index.yaml ]] && echo ok || echo "missing")"

# Bin scripts
check "generate-roster-cache.py present" "$([[ -f ~/.armies/bin/generate-roster-cache.py ]] && echo ok || echo "missing")"
check "check-general-eligibility.py present" "$([[ -f ~/.armies/bin/check-general-eligibility.py ]] && echo ok || echo "missing")"
check "generate-roster-cache.py uses armies path" "$(grep -q 'armies_root' ~/.armies/bin/generate-roster-cache.py && echo ok || echo "still references generals")"
check "generate-roster-cache.py source label updated" "$(grep -q 'armies/service-records' ~/.armies/bin/generate-roster-cache.py && echo ok || echo "still references generals")"

echo ""
echo "=== Phase 2: ~/.claude/agents/ sourced from armies ==="

# Check a profile has armies-level frontmatter
check "eisenhower.md has display_name" "$(grep -q 'display_name:' ~/.claude/agents/eisenhower.md 2>/dev/null && echo ok || echo "missing display_name")"
check "eisenhower.md has roles:" "$(grep -q '^roles:' ~/.claude/agents/eisenhower.md 2>/dev/null && echo ok || echo "missing roles")"
check "eisenhower.md has xp:" "$(grep -q '^xp:' ~/.claude/agents/eisenhower.md 2>/dev/null && echo ok || echo "missing xp")"
check "eisenhower.md has rank:" "$(grep -q '^rank:' ~/.claude/agents/eisenhower.md 2>/dev/null && echo ok || echo "missing rank")"
check "founder.md in agents-archive" "$([[ -f ~/.claude/agents-archive/founder.md ]] && echo ok || echo "founder.md missing from agents-archive")"
check "sync-to-claude-agents.sh exists" "$([[ -f ~/.armies/bin/sync-to-claude-agents.sh ]] && echo ok || echo "missing")"

echo ""
echo "=== Phase 3: Hooks rewired ==="

check "sync-armies-xp.sh exists" "$([[ -f ~/.claude/hooks/sync-armies-xp.sh ]] && echo ok || echo "missing — still using generals hook")"
check "sync-armies-xp.sh references camp-david" "$(grep -q 'camp-david' ~/.claude/hooks/sync-armies-xp.sh 2>/dev/null && echo ok || echo "still references generals")"
check "sync-armies-xp.sh does NOT reference generals repo" "$(grep -qv 'petersimmons1972/generals' ~/.claude/hooks/sync-armies-xp.sh 2>/dev/null && echo ok || echo "still has generals reference")"
check "refresh-armies-cache.sh exists" "$([[ -f ~/.claude/refresh-armies-cache.sh ]] && echo ok || echo "still using generals script")"
check "refresh-armies-cache.sh references .armies" "$(grep -q '\.armies' ~/.claude/refresh-armies-cache.sh 2>/dev/null && echo ok || echo "still references generals")"
check "generals-roster-cache.json sourced from armies" "$(grep -q 'armies/service-records' ~/.claude/generals-roster-cache.json 2>/dev/null && echo ok || echo "still sourced from generals")"

echo ""
echo "=== Phase 4: Documentation updated ==="

check "AGENTS.md has no ~/projects/generals references" "$(! grep -q 'projects/generals' ~/AGENTS.md 2>/dev/null && echo ok || echo "still references generals paths")"
check "CLAUDE.md has no ~/projects/generals references" "$(! grep -q 'projects/generals' ~/CLAUDE.md 2>/dev/null && echo ok || echo "still references generals paths")"

echo ""
echo "=== Phase 5: armies pushed to camp-david ==="

armies_status=$(git -C ~/.armies status --porcelain 2>/dev/null | wc -l || echo "?")
check "armies working tree clean (committed)" "$([[ $armies_status -eq 0 ]] && echo ok || echo "$armies_status uncommitted changes")"
armies_remote=$(git -C ~/.armies remote get-url origin 2>/dev/null || echo "none")
check "armies remote is camp-david" "$([[ "$armies_remote" == *"camp-david"* ]] && echo ok || echo "remote: $armies_remote")"

echo ""
echo "=== Phase 6: generals decommissioned ==="

check "generals README has DEPRECATED header" "$(head -3 ~/projects/generals/README.md 2>/dev/null | grep -qi deprecated && echo ok || echo "no deprecation notice")"

echo ""
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "  Passed: $PASS  |  Failed: $FAIL"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo ""

[[ $FAIL -eq 0 ]] && exit 0 || exit 1
