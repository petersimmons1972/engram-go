# Issue #396 — Phase 1: Hook Timing Measurement

Status: in progress (`fix/wave4-hook-timing-phase1`)
Phase: 1 of 3 — **measurement only**
Author: Wave 4 hook-timing campaign

## Question this phase answers

> Where, exactly, does each Engram-aware hook spend its wall-clock time?

Specifically:

1. Which stage dominates total hook latency — token resolution, MCP connect,
   request send, or response wait?
2. Does the dominant stage differ across the five hooks (`engram-precheck`,
   `engram-auth-check`, `engram-token-refresh`, `engram-session-recall`,
   `engram-session-end`) and the Go `instinct` binary?
3. Is per-stage variance high enough to justify Phase 2 (session reuse, auth
   cache), or is the median already inside the budget?

Phase 1 is strictly **measurement**. No daemon. No session-reuse code. No new
auth-token cache file. No systemd unit. No Prometheus exporter. Those belong
in Phase 2 and Phase 3 and are explicitly out of scope here.

## What changes in this PR

| Path | Purpose |
|------|---------|
| `cmd/instinct/timing.go` | `stageTimes` struct, `newStageTimes`, `toTSVRow`, `appendTimingRow`, file/header bootstrap |
| `cmd/instinct/timing_test.go` | Unit tests for offset math, header bootstrap, append semantics, empty-stage rendering |
| `cmd/instinct/main.go` | Four stage marks inside `run()`: `authResolved`, `mcpConnected`, `requestSent`, `responseReceived`; exit time + TSV append in `defer` |
| `hooks/lib/timing-v2.sh` | Bash counterpart — same schema, same file, `EXIT` trap emits one row |
| `hooks/engram/engram-precheck.sh` | Marks `request_sent` before `/health`, `response_received` after |
| `hooks/engram/engram-auth-check.sh` | Marks `auth_resolved` after token load, `request_sent`/`response_received` around `/quick-recall` probe |
| `hooks/engram/engram-token-refresh.sh` | Marks `auth_resolved` after token resolution (cached or freshly fetched), `request_sent`/`response_received` around `_test_auth` |
| `hooks/engram/engram-session-recall.sh` | Marks `auth_resolved` after token load, `request_sent` before recall, `response_received` after last response |
| `hooks/engram/engram-session-end.sh` | Marks `auth_resolved` after token load, `request_sent`/`response_received` around `/quick-store` |
| `hooks/install.sh` | Installs `timing-v2.sh` to `~/.claude/hooks/lib/` and the engram hooks to `~/.claude/hooks/` |

## TSV schema

All rows — from the Go binary and from bash hooks — append to a single file:

    $HOME/.claude/hook-timings-v2.tsv

(Override via the `HOOK_TIMING_V2_LOG` environment variable.)

The file is created on first write with a single header line.

Schema (tab-separated, ten columns):

| # | Column                  | Type     | Meaning |
|---|-------------------------|----------|---------|
| 1 | `iso_ts`                | string   | UTC RFC3339 timestamp of process start |
| 2 | `hook_name`             | string   | `instinct` for the Go binary; bash hooks emit the script's `basename` |
| 3 | `exec_start_ms`         | int64    | Epoch milliseconds at process start (absolute clock) |
| 4 | `auth_resolved_ms`      | int (ms) | Offset from `exec_start` to "token usable" — **empty if not reached** |
| 5 | `mcp_connected_ms`      | int (ms) | Offset to MCP connection established (Go binary only) — empty if not reached |
| 6 | `request_sent_ms`       | int (ms) | Offset to first byte of request sent |
| 7 | `response_received_ms`  | int (ms) | Offset to final response fully received |
| 8 | `exit_ms`               | int (ms) | Offset to process exit |
| 9 | `exit_code`             | int      | Process exit code |
| 10| `pid`                   | int      | Process PID |

Design notes on the schema:

- **Offsets from `exec_start`, not absolute times** — three reasons: smaller
  numbers in the log, immune to clock skew between rows, and trivially
  comparable across hooks.
- **Empty field, not zero, for unreached stages** — a bash hook that exits
  early because no token was found should distinguish "auth resolved at t=0"
  (impossible) from "auth never resolved" (the real case). Zero is a valid
  offset value; emptiness is the right "missing" signal.
- **`mcp_connected` is Go-only** — bash hooks talk to Engram via REST, not
  MCP/SSE. They leave that column empty by construction.

## Stage definitions (which checkpoint maps to which transition)

| Stage             | Defined as (Go)                              | Defined as (bash hook) |
|-------------------|----------------------------------------------|------------------------|
| `exec_start`      | `newStageTimes()` returns, top of `run()`    | `_tv2_t0_ns` set at top of `timing-v2.sh` |
| `auth_resolved`   | `loadConfig` succeeded and we hold a token   | Token successfully read from `mcp_servers.json` or `/setup-token` |
| `mcp_connected`   | `e.connect(ctx)` returned without error      | n/a — bash hooks are REST-only |
| `request_sent`    | About to call first `writeEpisode`           | About to call the first `curl` that contacts Engram |
| `response_received` | Last write/recall completed                | After the final `curl` returns |
| `exit`            | `defer` runs at the bottom of `run()`        | `trap _tv2_on_exit EXIT` fires |

## Why bash + Go share one file

Phase 2 will compare:

- Bash hook auth probe (cold curl) vs Go binary auth probe (`hybridEngram.connect`)
- Bash recall (one POST) vs Go recall (MCP/SSE)
- Same hook re-invoked back-to-back (cache hit vs cache miss)

Joining the two TSV streams requires no schema translation: `exec_start_ms` is
an absolute clock and the hook_name disambiguates the source.

## Analysis snippet (Phase 1 reads, no writes)

Run this against `~/.claude/hook-timings-v2.tsv` after a normal day of Claude
Code use. It prints a per-hook stage breakdown (median, p90):

```bash
# Median + p90 per stage per hook
mlr --tsv stats1 \
  -a p50,p90,count \
  -f auth_resolved_ms,mcp_connected_ms,request_sent_ms,response_received_ms,exit_ms \
  -g hook_name \
  ~/.claude/hook-timings-v2.tsv
```

Equivalent using `awk` (when `mlr` is not available):

```bash
awk -F'\t' 'NR>1 && $4!="" { sum[$2"_auth"]+=$4; n[$2"_auth"]++ }
            NR>1 && $7!="" { sum[$2"_resp"]+=$7; n[$2"_resp"]++ }
            END { for (k in sum) printf "%-50s %.1f ms (n=%d)\n", k, sum[k]/n[k], n[k] }' \
  ~/.claude/hook-timings-v2.tsv | sort
```

To find the dominant stage per row (which checkpoint took longest):

```bash
mlr --tsv put '
  $delta_auth   = $auth_resolved_ms      != "" ? $auth_resolved_ms                                            : 0;
  $delta_mcp    = $mcp_connected_ms      != "" ? $mcp_connected_ms      - ($auth_resolved_ms       != "" ? $auth_resolved_ms      : 0) : 0;
  $delta_req    = $request_sent_ms       != "" ? $request_sent_ms       - ($mcp_connected_ms       != "" ? $mcp_connected_ms      : ($auth_resolved_ms != "" ? $auth_resolved_ms : 0)) : 0;
  $delta_resp   = $response_received_ms  != "" ? $response_received_ms  - ($request_sent_ms        != "" ? $request_sent_ms       : 0) : 0;
  $delta_exit   = $exit_ms               != "" ? $exit_ms               - ($response_received_ms   != "" ? $response_received_ms : 0) : 0;
' then cut -f hook_name,delta_auth,delta_mcp,delta_req,delta_resp,delta_exit \
  ~/.claude/hook-timings-v2.tsv
```

## Decision gate: when do we move to Phase 2?

The promotion criterion is **evidence-driven**, not time-driven. We move to
Phase 2 (session reuse / auth cache) only when **both** are true:

1. **Sample size**: at least **500 rows** from real Claude Code sessions,
   across at least **5 distinct calendar days**, with at least **20 rows per
   instrumented hook**. This bounds the influence of one-off cold-start
   outliers.
2. **Effect size**: a single stage's p50 contribution is **≥ 100 ms** AND
   that stage represents **≥ 40%** of total wall-clock time for its hook.

Conversely, we **do not** ship Phase 2 when:

- The dominant stage is `auth_resolved` at < 50 ms p50 (already cheap).
- The dominant stage is `response_received_ms − request_sent_ms` (that's
  server-side latency, not hook overhead — Phase 2 cannot help).
- Total p50 exec time per hook is < 200 ms across the board (no perceivable
  user impact).

The Phase 1 deliverable is therefore a **decision document**, not a fix. It
either justifies Phase 2 with numbers or closes the wave with "no action."

## Safety / risk notes

- Every timing call is wrapped (`2>/dev/null || true` in bash;
  `appendTimingRow` swallows errors in Go). A buggy `date`, a read-only home
  directory, or a missing file will not change a hook's exit code.
- `appendTimingRow` uses `O_APPEND` — concurrent invocations write whole
  lines atomically up to `PIPE_BUF` (4 KB on Linux). Our rows are < 200 B.
- File is created mode 0600. No secrets are logged; only offsets and the hook
  name.
- Disk cost: ~150 bytes per invocation. At 200 hook invocations per active
  day, the log grows ~30 KB/day. Rotation can wait until Phase 2.

## Out of scope (explicitly)

- Persistent daemon to keep MCP session warm — Phase 2.
- Auth token cache file (separate from `mcp_servers.json`) — Phase 2.
- Systemd unit, Prometheus exporter, Grafana dashboard — Phase 3.
- Any change to MCP wire protocol, server endpoints, or `engram-go` binaries
  beyond `cmd/instinct/`.
- Sampling, log rotation, structured JSON output — not justified until we see
  the data volume.
