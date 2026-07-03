# LongMemEval Judging Harness

Use this doc to produce judge-attributed, repeatable LME scores.

## One-command judge harness

The harness entrypoint is `scripts/lme-judge.sh`:

```bash
scripts/lme-judge.sh --run results/my-run --judge qwen3 --gold-version gold-zfs-2026-07-03
```

Required arguments:

- `--run`: result directory containing `checkpoint-run.jsonl` (and `checkpoint-ingest.jsonl`) from a completed `run`.
- `--judge`: one of `qwen3` or `gpt4o`.
- `--gold-version`: frozen gold snapshot/version tag for this eval run. Use the operational snapshot name (ZFS snapshot, `pg_dump` tag, or equivalent) rather than a free-form note.

Optional:

- `--thinking on|off` (default `on`): pass through to `--scorer-thinking`.
- `--compare <baseline-dir>`: print strict/lenient deltas against another run’s `score_report.json`.
- `--bundle`: run both `qwen3` and `gpt4o` judges in one invocation. `qwen3` output lands in `--run/qwen3`, `gpt4o` in `--run/gpt4o`.
- `ITEM_SET=<name>`: item cohort identifier stamped onto every score row. Default: `lme-s-500q`.
- `SCORER_LOCK=<path>`: scorer lock manifest for the `qwen3` path. Default: `docs/lme-campaign/scorer-lock.json`.
- `SYSTEM=<name>`: system-under-test identifier stamped onto every score row.
- `WORKERS=<n>`: worker count (environment override).
- `SCORER_MAX_TOKENS=<n>`: score request budget.

## Locked scorer workflow

The Tier-1 honest-baseline scorer manifest lives at:

```text
docs/lme-campaign/scorer-lock.json
```

For `qwen3`, `lme-judge.sh` treats that manifest as authoritative for:

- scorer URL
- scorer model
- scorer thinking mode
- scorer max tokens

The wrapper does **not** override those lock-owned fields. If you need a different judge identity, use `gpt4o` or point `SCORER_LOCK` at another reviewed manifest.

Every score row now carries:

- `gold_version`
- `scorer_version`
- `feature_flags`
- `system`
- `item_set`
- `run_id`
- `harness_sha`

When `--scorer-lock` is set, `--gold-version` and `--item-set` are mandatory so the run cannot silently score against an untagged moving target.

## Judge presets

### qwen3 (default)

Use for the locked honest baseline and cheap wave-over-wave scoring:

- manifest-backed via `docs/lme-campaign/scorer-lock.json`
- scorer version: `tier1-qwen3-2026-06-22`
- API key: none

Qwen3 lock settings are fixed by the manifest so same-scorer comparisons stay honest.

### gpt-4o (comparability)

Use for published comparability snapshots:

- URL: `https://api.openai.com/v1`
- model: default `gpt-4o-2024-11-20`
- API key from `LME_SCORER_API_KEY` or `OPENAI_API_KEY` (mapped to `--scorer-api-key`)

`gpt4o` runs are typically faster with fewer concurrency settings but carry higher cost.

## Flags passed through to `score-efficient`

`lme-judge.sh` maps judge preset selection to:

- `qwen3`: `--scorer-lock`, `--gold-version`, `--item-set`, `--system`
- `gpt4o`: `--scorer-url`, `--scorer-model`, `--scorer-api-key` (optional), `--scorer-thinking`, `--scorer-max-tokens`, `--preserve-correct`

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
5. Treat `score_report.json` as invalid for campaign comparison if `provenance.gold_version` is empty or if `baseline_comparison.status` is `diverges` and you have not explained why.
