# LongMemEval Judging Harness

Use this doc to produce judge-attributed, repeatable LME scores.

## Locked Tier-1 scorer

Repeatable campaign runs must use the scorer lock at [docs/lme-campaign/scorer-lock.json](./lme-campaign/scorer-lock.json).

- Locked scorer version: `tier1-qwen3-32b-nonthinking-v1`
- Harness stage: `score-efficient`
- Backend: Spark/olla
- Mode: Qwen3-32B, non-thinking, `max_tokens=2048`

The lock is the single source of truth for `--scorer-url`, `--scorer-model`,
`--scorer-thinking`, and `--scorer-max-tokens`. `score-efficient` now records
`scorer_version` into `checkpoint-score.jsonl`, `score_report.json`,
`run_manifest.json`, and `RUN_STATUS.json`.

## One-command judge harness

The harness entrypoint is `scripts/lme-judge.sh`:

```bash
scripts/lme-judge.sh --run results/my-run --judge qwen3
```

Required arguments:

- `--run`: result directory containing `checkpoint-run.jsonl` (and `checkpoint-ingest.jsonl`) from a completed `run`.
- `--judge`: one of `qwen3` or `gpt4o`.

Optional:

- `--thinking on|off` (default `off`): `qwen3` is pinned by the scorer lock and must stay `off`; `gpt4o` passes through to `--scorer-thinking`.
- `--compare <baseline-dir>`: print strict/lenient deltas against another run’s `score_report.json`.
- `--bundle`: run both `qwen3` and `gpt4o` judges in one invocation. `qwen3` output lands in `--run/qwen3`, `gpt4o` in `--run/gpt4o`.
- `LOCK_PATH=<path>`: override the scorer-lock manifest path (defaults to `docs/lme-campaign/scorer-lock.json`).
- `WORKERS=<n>`: worker count (environment override).
- `SCORER_MAX_TOKENS=<n>`: score request budget.

## Judge presets

### qwen3 (default)

Use for cheap wave-over-wave scoring:

- Config source: `docs/lme-campaign/scorer-lock.json`
- Version tag emitted in artifacts: `tier1-qwen3-32b-nonthinking-v1`
- URL/model come from the lock and must not be overridden ad hoc
- API key: none

Qwen3 thinking mode is explicitly disallowed for repeatable runs. The lock pins
non-thinking mode because campaign history already showed lenient drift from a
local thinking-enabled scorer.

### gpt-4o (comparability)

Use for published comparability snapshots:

- URL: `https://api.openai.com/v1`
- model: default `gpt-4o-2024-11-20`
- API key from `LME_SCORER_API_KEY` or `OPENAI_API_KEY` (mapped to `--scorer-api-key`)

`gpt4o` runs are typically faster with fewer concurrency settings but carry higher cost.

## Flags passed through to `score-efficient`

`lme-judge.sh` maps judge preset selection to:

- `--scorer-lock` for `qwen3`
- `--scorer-url` and `--scorer-model` for `gpt4o`
- `--scorer-api-key` (optional)
- `--scorer-thinking`
- `--scorer-max-tokens`
- `--preserve-correct` (resume-friendly)

## Accounting: strict vs lenient

The judge report is scored in two ways:

- `strict`: `CORRECT / total`
- `lenient`: `(CORRECT + PARTIALLY_CORRECT) / total`

`score_report.json` now records both summaries in machine-readable form under
`overall.{strict,lenient}` and `by_type.<question_type>.{strict,lenient}`. This
keeps the frozen phase-0 strict counts intact while exposing a separate
partial-credit baseline for categories such as `single-session-preference`.

When publishing benchmark updates:

- Keep the original strict baseline for backward comparability.
- Treat the lenient/partial-credit view as a new baseline artifact, not a
  rewrite of the existing phase-0 scores.

Use both when comparing checkpoints, and keep `compare` deltas in CI notes.

## Recommended workflow

1. Keep the judge constant across a wave before computing deltas.
2. Use locked `qwen3` for regular iteration; future result references should cite `scorer_version`, not just a model family name.
3. Use `gpt4o` only for a final comparability snapshot and publication.
4. Only compare against a baseline scored with the same judge mode.
