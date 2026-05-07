# Design: AIMD Adaptive Concurrency for `engram-reembed`

**Issue:** engram-go#610
**Date:** 2026-05-06
**Scope:** Rust binary only (`reembed-rs/src/main.rs`)

## Problem

`engram-reembed` uses a fixed semaphore (`ENGRAM_REEMBED_CONCURRENCY`, default 8). At high values it monopolises both GPUs, causing foreground `memory_store`/`memory_recall` MCP calls to time out and Claude Code to synthesise "user denied" results (see #611). Lowering the value to 2 keeps the foreground responsive but makes the ~727k-chunk NULL backlog take days to clear.

## Goal

Burst to high concurrency when the GPU is idle; pull back automatically when embed latency rises (indicating foreground contention or thermal throttle). No cross-service dependencies.

## Approach: AIMD on Per-Batch Latency

Measure wall-clock time of each `run_batch` call divided by chunks processed. Use that as the contention signal. Apply Additive Increase / Multiplicative Decrease to the semaphore width.

This is the "simplest first cut" explicitly recommended in #610. It needs no coupling to Olla internals or the engram-go process.

## Architecture

### New struct: `AdaptiveConcurrency`

```
AdaptiveConcurrency {
    current: usize,       // live concurrency level
    min: usize,
    max: usize,
    latency_high_ms: u64, // above this → halve
    latency_low_ms: u64,  // below this → increment clean counter
    ramp_after: usize,    // consecutive clean batches required to step up
    clean_count: usize,   // consecutive batches below low threshold
}
```

### Control loop (called after each non-empty batch)

| Condition | Action |
|-----------|--------|
| `per_chunk_ms >= latency_high_ms` | `current = max(current / 2, min)`, reset clean counter |
| `per_chunk_ms < latency_low_ms` | increment clean counter; if `clean_count >= ramp_after` → `current = min(current + 1, max)`, reset clean counter |
| dead band `[latency_low_ms, latency_high_ms)` | no change |
| empty batch or timeout/error | timeout/error counts as high latency; empty batch skipped entirely |

### Integration with `run_batch`

`run_batch` gains a `concurrency: usize` parameter and returns `(usize, Duration)` — chunks processed and wall-clock elapsed. The main loop passes `controller.current` in, receives the duration back, calls `controller.update(chunks, elapsed)`.

Timeouts and embed errors inside `run_batch` do not propagate as `Err` today (they log and continue). For the latency signal, the full batch wall time (including any chunk-level timeouts) is used — no special handling needed.

## Configuration

All new env vars are optional. Existing deployments need no changes.

| Env var | Default | Notes |
|---------|---------|-------|
| `ENGRAM_REEMBED_CONCURRENCY_MAX` | `ENGRAM_REEMBED_CONCURRENCY` or `16` | Ceiling. Existing `ENGRAM_REEMBED_CONCURRENCY` becomes an alias for this. |
| `ENGRAM_REEMBED_CONCURRENCY_MIN` | `1` | Floor. |
| `ENGRAM_REEMBED_LATENCY_HIGH_MS` | `2000` | Halve concurrency above this per-chunk latency. |
| `ENGRAM_REEMBED_LATENCY_LOW_MS` | `400` | Count as clean below this per-chunk latency. |
| `ENGRAM_REEMBED_RAMP_AFTER` | `3` | Consecutive clean batches before stepping up. |

**Backward compatibility:** `ENGRAM_REEMBED_CONCURRENCY=2` continues to work as a ceiling (max=2, controller starts at 2 and can only go down). The current `.env` mitigation value of 2 will be respected automatically.

Initial concurrency at startup = `min(CONCURRENCY_MAX, CONCURRENCY_MAX)` = max, so the binary starts aggressive and backs off if needed (matches expected idle-at-startup behaviour).

## Observability

Log at `info` on every concurrency change:

```json
{"level":"INFO","msg":"reembed concurrency adjusted","prev":8,"next":4,"reason":"latency_high","per_chunk_latency_ms":3200}
{"level":"INFO","msg":"reembed concurrency adjusted","prev":4,"next":5,"reason":"latency_low","per_chunk_latency_ms":180}
```

Log at `debug` on every batch: current concurrency, chunks processed, per-chunk latency.

## Testing

Unit tests for `AdaptiveConcurrency` in isolation (no tokio runtime needed):

- High latency fires MD: `current` halves, floors at `min`
- Low latency increments clean counter; no ramp until `ramp_after` batches
- Low latency for `ramp_after` batches ramps `current` by 1, resets counter
- Dead-band latency leaves `current` unchanged
- Empty batch (n=0) is skipped: no state change
- Ceiling and floor are both respected

Integration: existing `run_batch` unit tests continue to pass unchanged (signature change is additive — `concurrency` param replaces the constant).

## Out of Scope

- Go `GlobalReembedder` adaptive concurrency (separate issue if needed)
- Olla queue-depth polling
- Postgres coordination flag
