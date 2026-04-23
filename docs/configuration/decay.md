# Decay Configuration

Three environment variables tune engram's memory decay behavior at runtime.
None are required; all default to values that preserve existing behavior.

> **Note:** Default rate tuning is deferred pending LongMemEval baseline results.
> Do not change `ENGRAM_DECAY_RATE_PER_HOUR` in production until that work lands.

---

## `ENGRAM_DECAY_RATE_PER_HOUR`

Controls the exponential decay rate used by `RecencyDecay(hours)`.

| | |
|---|---|
| Formula | `exp(-rate * hours)` |
| Default | `0.01` (1% weight loss per hour; ~50% at 69 hours, ~1/1300 at one month) |
| Sanity bounds | Must be in `(0, 10]`. Values outside this range fall back to the default. |
| On parse failure | Falls back to default; emits one `slog.Warn` |

```
ENGRAM_DECAY_RATE_PER_HOUR=0.005   # slower decay — ~50% at 139 hours
ENGRAM_DECAY_RATE_PER_HOUR=0.02    # faster decay — ~50% at 35 hours
```

---

## `ENGRAM_DECAY_FLOOR`

Sets a minimum value that `RecencyDecay` will never return below.
Use this to prevent very old memories from scoring near zero.

| | |
|---|---|
| Default | `0.0` (no floor; current behavior) |
| Sanity bounds | Must be in `[0, 1]`. Values outside this range fall back to `0.0`. |
| On parse failure | Falls back to `0.0`; emits one `slog.Warn` |

```
ENGRAM_DECAY_FLOOR=0.1    # memories never score below 10% of their recency weight
```

---

## `ENGRAM_DECAY_INTERVAL_HOURS`

Controls how often the background spaced-repetition decay worker runs.
Only used when `NewDecayWorker` is called with `interval=0`.
A non-zero explicit interval passed to `NewDecayWorker` always wins.

| | |
|---|---|
| Default | `8` (8 hours) |
| Sanity bounds | Must be a positive number. Zero or negative falls back to default. |
| On parse failure | Falls back to default; emits one `slog.Warn` |

```
ENGRAM_DECAY_INTERVAL_HOURS=4     # run decay pass every 4 hours
ENGRAM_DECAY_INTERVAL_HOURS=24    # run decay pass once a day
```

---

## Notes

- All three values are read once at first use via `sync.Once` and cached for
  the process lifetime. Changing env vars after startup has no effect.
- The `decayFactor` (0.95) used by the spaced-repetition worker is intentionally
  not configurable — it is part of the algorithm's math, not a tuning curve.
