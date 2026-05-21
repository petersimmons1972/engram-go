#!/usr/bin/env bash
# fetch-longmemeval.sh — download + preprocess the LongMemEval-M dataset.
#
# Closes #677. The benchmark binary (cmd/longmemeval) requires
# testdata/longmemeval/longmemeval_m_cleaned.json (~2.7 GB). It's gitignored
# because of size. This script documents how to (re-)materialize it.
#
# Usage: bash scripts/fetch-longmemeval.sh
set -euo pipefail

DEST_DIR="testdata/longmemeval"
DEST_FILE="${DEST_DIR}/longmemeval_m_cleaned.json"
SRC_URL="${LONGMEMEVAL_SOURCE_URL:-https://huggingface.co/datasets/xiaowu0162/longmemeval/resolve/main/longmemeval_m.json}"

if [ -s "$DEST_FILE" ]; then
    echo "✓ ${DEST_FILE} already exists ($(du -h "$DEST_FILE" | cut -f1)). Delete it to re-download."
    exit 0
fi

mkdir -p "$DEST_DIR"

echo "Downloading from: $SRC_URL"
echo "  (override via LONGMEMEVAL_SOURCE_URL env var)"
echo

# Use xh if available, else curl. Stream to a tmp file so a partial download
# doesn't leave a corrupted target.
TMP="${DEST_FILE}.partial"
if command -v xh >/dev/null 2>&1; then
    xh GET "$SRC_URL" --download --output "$TMP"
else
    curl -fL --progress-bar -o "$TMP" "$SRC_URL"
fi

# Preprocess: the upstream JSON has been observed with stray whitespace + a
# trailing newline that breaks streaming parsers. Normalise to one document
# per line with `jq -c`. This is the `_cleaned` step referenced in the
# filename.
echo "Preprocessing (one JSON document per line)..."
if command -v jq >/dev/null 2>&1; then
    jq -c . "$TMP" > "$DEST_FILE"
    rm "$TMP"
else
    echo "WARN: jq not installed — skipping preprocessing. The binary may"
    echo "      reject the raw upstream format. Install jq and re-run."
    mv "$TMP" "$DEST_FILE"
fi

echo
echo "✓ ${DEST_FILE} ready ($(du -h "$DEST_FILE" | cut -f1))"
