#!/usr/bin/env bash
# Runs the issue #1395 extraction-fidelity fixtures through the real Claude CLI.
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "${repo_root}"

export ENGRAM_MANUAL_ATOM_EVAL=1
export GOCACHE="${GOCACHE:-/tmp/engram-go-cache}"
go test ./internal/atom -run '^TestManualExtractionFidelity$' -count=1 -v
