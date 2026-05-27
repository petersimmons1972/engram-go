#!/usr/bin/env bash
# check-secrets.sh — block accidental commits of credentials.
#
# Behaviour:
#   • Exits 1 if any staged file matches an env-secret filename pattern
#     (.env, .env.*, .env.bak.*, .env.machine-identity, etc.) — except
#     .env.example which is the allowed placeholder template.
#   • Exits 1 if any staged file's diff content matches a known secret shape
#     (long hex tokens in KEY=VALUE form, Anthropic sk-ant- tokens,
#     Infisical-style base64 secrets, etc.).
#   • Exits 0 if the index is empty or every staged change is clean.
#
# Usage:
#   bash scripts/check-secrets.sh             # check current git index
#   (intended to be wired as a pre-commit hook)
#
# Source: QA sweep 2026-05-17 Wave 1 prelude (issue #657 mitigation)

set -u

# Allow override for testing (we never want a guard that fails open by accident)
GIT="${GIT:-git}"

fail=0
fail_reason=""

emit_fail() {
    fail=1
    fail_reason="${fail_reason}${fail_reason:+
}$1"
}

is_env_secret_name() {
    case "$(basename "$1")" in
        .env.example)
            return 1
            ;;
        .env|.env.local|.env.machine-identity|.env.*)
            return 0
            ;;
    esac
    return 1
}

# ── 0. Local ignored env-file mode rules ──────────────────────────────────────
# Inspect names and modes only. Do not read or print file contents.
shopt -s nullglob
for f in .env .env.*; do
    if ! is_env_secret_name "$f"; then
        continue
    fi
    if ! "$GIT" check-ignore -q -- "$f" 2>/dev/null; then
        continue
    fi
    [ -e "$f" ] || continue
    mode=$(stat -c '%a' "$f" 2>/dev/null || true)
    [ -n "$mode" ] || continue
    last_two="${mode: -2}"
    group_digit="${last_two:0:1}"
    world_digit="${last_two:1:1}"
    if [ "$group_digit" != "0" ] || [ "$world_digit" != "0" ]; then
        emit_fail "BLOCKED: ignored secret file has group/world permissions: $f mode=$mode (run: chmod 600 '$f')"
    fi
done
shopt -u nullglob

# ── Collect staged files (added or modified, ignore deletions) ────────────────
staged_files=$("$GIT" diff --cached --name-only --diff-filter=AM 2>/dev/null || true)

if [ -z "$staged_files" ] && [ "$fail" -eq 0 ]; then
    exit 0
fi

# ── 1. Filename rules ─────────────────────────────────────────────────────────
while IFS= read -r f; do
    [ -z "$f" ] && continue
    if is_env_secret_name "$f"; then
        emit_fail "BLOCKED: staged secret file: $f"
    fi
done <<< "$staged_files"

# ── 2. Content rules — scan staged diff for known secret shapes ───────────────
# Patterns are intentionally specific to keep false positives low.
patterns=(
    # KEY=<64+ hex chars> — Postgres password, Engram API key, generic 256-bit
    '^[+].*[A-Z_]+(PASSWORD|API_KEY|SECRET|TOKEN)=[0-9a-fA-F]{32,}'
    # Anthropic API keys: sk-ant-... followed by 80+ url-safe chars
    '^[+].*sk-ant-[A-Za-z0-9_-]{40,}'
    # Infisical / generic 64+ hex secrets in KEY=VALUE form (catches CLIENT_SECRET=...)
    '^[+].*CLIENT_SECRET=[0-9a-fA-F]{32,}'
)

# Get staged diff once
diff_output=$("$GIT" diff --cached --no-color -U0 2>/dev/null || true)

if [ -n "$diff_output" ]; then
    current_file=""
    while IFS= read -r line; do
        # Track which file we're inside the diff for
        case "$line" in
            "diff --git "*)
                current_file=$(echo "$line" | sed -E 's|^diff --git a/([^ ]+) .*|\1|')
                ;;
        esac
        for pat in "${patterns[@]}"; do
            if echo "$line" | grep -qE "$pat"; then
                emit_fail "BLOCKED: secret-shaped content in: ${current_file:-<unknown>}"
                break
            fi
        done
    done <<< "$diff_output"
fi

# ── 3. Report and exit ────────────────────────────────────────────────────────
if [ "$fail" -eq 1 ]; then
    {
        echo "──────────────────────────────────────────────────────────────"
        echo "  check-secrets: COMMIT BLOCKED"
        echo "──────────────────────────────────────────────────────────────"
        # Deduplicate reasons
        echo "$fail_reason" | sort -u
        echo
        echo "  If this is a false positive, override with:  git commit --no-verify"
        echo "  (but verify by hand first — this guard exists for a reason)"
        echo "──────────────────────────────────────────────────────────────"
    } >&2
    exit 1
fi

exit 0
