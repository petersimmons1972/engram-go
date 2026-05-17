#!/usr/bin/env bash
# test-regexp.sh — validates lineinfile regexp behaviour against /etc/hosts fixtures
# Usage: ./test-regexp.sh [old|new]   (default: new)
#
# Tests five fixture scenarios and compares output to .expected snapshots.
# Exits 0 if all assertions pass, non-zero on any failure.
#
# Requires ansible-core >=2.14. Fails loud if ansible is absent or any fixture invocation errors.
#
# NOTE on escaping: when passing regexp via ansible -a "..." the shell interprets
# backslashes, so each regex backslash must be doubled here (\\s, \\d, etc.).
# In the YAML playbook, single backslashes work fine because YAML handles quoting.

set -euo pipefail

command -v ansible >/dev/null 2>&1 || { echo "FATAL: ansible not installed — install ansible-core >=2.14" >&2; exit 2; }

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
FIXTURES_DIR="${SCRIPT_DIR}/registry-hosts-fixtures"

MANAGED_LINE='192.168.0.180 registry.petersimmons.com  # managed by ansible — homelab-config#2'

# OLD regexp (buggy) — no start-of-line anchor, matches commented lines, misses trailing comments
# Double-backslash for shell -a argument passing
OLD_REGEXP='\\s+registry\\.petersimmons\\.com$'

# NEW regexp (fixed) — anchored, rejects comment lines, matches any IPv4 in column 1,
# tolerates trailing whitespace/comments after the FQDN.
# Double-backslash for shell -a argument passing.
NEW_REGEXP='^\\s*[0-9]{1,3}(\\.[0-9]{1,3}){3}\\s+registry\\.petersimmons\\.com(\\s.*)?$'

MODE="${1:-new}"
if [[ "${MODE}" == "old" ]]; then
    REGEXP="${OLD_REGEXP}"
    echo "=== Running with OLD regexp (expect failures on commented.hosts and trailing-comment.hosts) ==="
else
    REGEXP="${NEW_REGEXP}"
    echo "=== Running with NEW anchored regexp (expect all 5 pass) ==="
fi

PASS=0
FAIL=0
RESULTS=()

FIXTURES=(fresh bare commented stale-ip trailing-comment)

for FIXTURE in "${FIXTURES[@]}"; do
    INPUT="${FIXTURES_DIR}/${FIXTURE}.hosts"
    EXPECTED="${FIXTURES_DIR}/${FIXTURE}.hosts.expected"
    TMPFILE="$(mktemp /tmp/hosts-test-XXXX)"

    cp "${INPUT}" "${TMPFILE}"

    # Run ansible lineinfile module locally against the temp file
    ansible localhost \
        -m ansible.builtin.lineinfile \
        -a "path=${TMPFILE} regexp='${REGEXP}' line='${MANAGED_LINE}' state=present" \
        --connection=local \
        -o \
        || { echo "FATAL: ansible invocation failed on fixture ${FIXTURE}" >&2; exit 1; }

    if diff -q "${TMPFILE}" "${EXPECTED}" >/dev/null 2>&1; then
        PASS=$((PASS + 1))
        RESULTS+=("  PASS  ${FIXTURE}.hosts")
    else
        FAIL=$((FAIL + 1))
        RESULTS+=("  FAIL  ${FIXTURE}.hosts")
        echo ""
        echo "--- FAIL: ${FIXTURE}.hosts ---"
        echo "Expected:"
        cat "${EXPECTED}"
        echo "Got:"
        cat "${TMPFILE}"
        diff "${EXPECTED}" "${TMPFILE}" || true
    fi

    rm -f "${TMPFILE}"
done

echo ""
echo "Results (${MODE} regexp):"
for R in "${RESULTS[@]}"; do
    echo "${R}"
done
echo ""
echo "Passed: ${PASS} / $((PASS + FAIL))"

if [[ ${FAIL} -gt 0 ]]; then
    exit 1
fi
exit 0
