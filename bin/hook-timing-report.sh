#!/usr/bin/env bash
# Summarize ~/.claude/hook-timings.tsv. Output: per-hook count, p50, p95, max ms.
# Use to make the data-driven #396 decision once a few days of telemetry have accumulated.

set -euo pipefail
LOG="${HOOK_TIMING_LOG:-$HOME/.claude/hook-timings.tsv}"
[[ -f "$LOG" ]] || { echo "No data yet at $LOG"; exit 0; }

since=$(head -1 "$LOG" | cut -f1)
rows=$(wc -l <"$LOG")
sessions_est=$(cut -f5 "$LOG" | sort -u | wc -l)
echo "Hook timing report"
echo "  Log:       $LOG"
echo "  Since:     $since"
echo "  Rows:      $rows"
echo "  Distinct pids (rough session proxy): $sessions_est"
echo

awk -F'\t' '
$3 ~ /^-?[0-9]+$/ && $3 >= 0 {
  count[$2]++
  vals[$2] = vals[$2] " " $3
  if ($3 > max[$2]) max[$2] = $3
  sum[$2] += $3
}
END {
  printf "%-42s %7s %8s %8s %8s %8s\n", "hook", "count", "mean_ms", "p50_ms", "p95_ms", "max_ms"
  printf "%-42s %7s %8s %8s %8s %8s\n", "----", "-----", "-------", "------", "------", "------"
  for (n in count) {
    split(vals[n], a, " ")
    k = 0
    for (i in a) if (a[i] != "") arr[++k] = a[i] + 0
    for (i = 1; i < k; i++) for (j = i+1; j <= k; j++) if (arr[i] > arr[j]) { t = arr[i]; arr[i] = arr[j]; arr[j] = t }
    p50 = arr[int(k * 0.50) + 1]
    p95 = arr[int(k * 0.95) + 1]
    mean = sum[n] / count[n]
    printf "%-42s %7d %8d %8d %8d %8d\n", n, count[n], mean, p50, p95, max[n]
    for (i in arr) delete arr[i]
  }
}' "$LOG" | (read -r h1; read -r h2; echo "$h1"; echo "$h2"; sort -k4 -n -r)

echo
echo "Decision criteria for #396 reopen:"
echo "  - Daemon (Option A) justified if: p95 of any engram hook > 200ms AND fires >50/day"
echo "  - Static shim (Option B) sufficient if: p95 < 100ms but Python startup hooks > 80ms"
echo "  - Close as won't-fix if: all p95 < 50ms"
