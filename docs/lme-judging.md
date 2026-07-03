# LongMemEval Judging Harness

Use this doc to produce judge-attributed, repeatable LME scores.

## One-command judge harness

The harness entrypoint is `scripts/lme-judge.sh`:

```bash
scripts/lme-judge.sh --run results/my-run --judge qwen3
```

Required arguments:

- `--run`: result directory containing `checkpoint-run.jsonl` (and `checkpoint-ingest.jsonl`) from a completed `run`.
- `--judge`: one of `qwen3` or `gpt4o`.

Optional:

- `--thinking on|off` (default `on`): pass through to `--scorer-thinking`.
- `--compare <baseline-dir>`: print strict/lenient deltas against another run’s `score_report.json`.
- `--bundle`: run both `qwen3` and `gpt4o` judges in one invocation. `qwen3` output lands in `--run/qwen3`, `gpt4o` in `--run/gpt4o`.
- `WORKERS=<n>`: worker count (environment override).
- `SCORER_MAX_TOKENS=<n>`: score request budget for unlocked presets such as `gpt4o`.

## Judge presets

### qwen3 (default)

Use for cheap wave-over-wave scoring:

- URL: `http://192.168.0.138:30411/olla/openai/v1`
- model: `inference`
- API key: none

Qwen3 is a reasoning model and can be slower when chain-of-thought is enabled.
The `qwen3` harness path is scorer-lock-backed: the lock is the single source of
truth for lock-owned settings such as `max_tokens` and `preserve_correct`, so
`lme-judge.sh` does not forward manual overrides for those flags on this preset.

### gpt-4o (comparability)

Use for published comparability snapshots:

- URL: `https://api.openai.com/v1`
- model: default `gpt-4o-2024-11-20`
- API key from `LME_SCORER_API_KEY` or `OPENAI_API_KEY` (mapped to `--scorer-api-key`)

`gpt4o` runs are typically faster with fewer concurrency settings but carry higher cost.

## Flags passed through to `score-efficient`

`lme-judge.sh` maps judge preset selection to:

- `--scorer-url`
- `--scorer-model`
- `--scorer-api-key` (optional)
- `--scorer-thinking`
- `--scorer-max-tokens` (`gpt4o` / unlocked presets only)
- `--preserve-correct` (resume-friendly on unlocked presets only; `qwen3` defers to the scorer lock)

## Accounting: strict vs lenient

The judge report is scored in two ways:

- `strict`: `CORRECT / total`
- `lenient`: `(CORRECT + PARTIALLY_CORRECT) / total`

Use both when comparing checkpoints, and keep `compare` deltas in CI notes.

## Recommended workflow

1. Keep the judge constant across a wave before computing deltas.
2. Use `qwen3` for regular iteration (`--think on` for realism, `--think off` for throughput).
3. Use `gpt4o` only for a final comparability snapshot and publication.
4. Only compare against a baseline scored with the same judge mode.
