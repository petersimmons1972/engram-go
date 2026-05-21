#!/usr/bin/env bash
# psql-exec.sh — safely run a SQL file against the engram-postgres container.
#
# WHY THIS SCRIPT EXISTS:
# `docker exec ... psql <<'SQL' ... SQL` silently discards stdin without `-i`.
# Rather than require operators to remember `-i`, this script uses the
# unambiguous `docker cp` + `psql -f` pattern (issue #646).
#
# Usage: ./scripts/psql-exec.sh <path-to-file.sql> [container-name]

set -euo pipefail

SQL_FILE="${1:?Usage: psql-exec.sh <file.sql> [container]}"
CONTAINER="${2:-engram-postgres}"

if [[ ! -f "$SQL_FILE" ]]; then
  echo "error: file not found: $SQL_FILE" >&2
  exit 1
fi

REMOTE="/tmp/psql-exec-$$.sql"
docker cp "$SQL_FILE" "$CONTAINER:$REMOTE"
docker exec "$CONTAINER" psql -U engram -d engram -f "$REMOTE"
docker exec "$CONTAINER" rm -f "$REMOTE"
