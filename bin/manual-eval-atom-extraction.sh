#!/usr/bin/env bash
# Runs the issue #1395 extraction-fidelity fixtures through the real Claude CLI.
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "${repo_root}"

if ! command -v claude >/dev/null 2>&1; then
  echo "manual-eval-atom-extraction: 'claude' CLI not found — the fidelity evaluation cannot run (a skipped test would exit 0 and green-wash the eval)" >&2
  exit 2
fi

export ENGRAM_MANUAL_ATOM_EVAL=1
export GOCACHE="${GOCACHE:-/tmp/engram-go-cache}"
out="$(go test ./internal/atom -run '^TestManualExtractionFidelity$' -count=1 -v 2>&1)"
status=$?
printf '%s\n' "$out"
if [[ $status -eq 0 ]] && grep -q -- "--- SKIP" <<<"$out"; then
  echo "manual-eval-atom-extraction: evaluation was SKIPPED, not run — refusing to report success" >&2
  exit 2
fi
exit "$status"
