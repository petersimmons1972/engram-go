# Duramind M0 Scorer Qualification Record

## Locked scorer

- Scorer version: `tier1-qwen3-32b-nonthinking-v1`
- Manifest: [docs/lme-campaign/scorer-lock.json](./scorer-lock.json)
- Harness stage: `score-efficient`
- Backend: Spark/olla
- Served model: Qwen3-32B via route alias `inference`
- Locked flags: `--scorer-thinking=false`, `--scorer-max-tokens=2048`, `--preserve-correct=true`, `--force-rescore=false`

## Fixed sample and reference policy

- Tier-1 repeatable scorer: local Spark/olla Qwen3-32B in non-thinking mode
- Tier-2 audit reference: GPT-5.4 or Sonnet, run only on the fixed `v9-failures-135` panel named in [results/next-campaign-manifest.md](../../results/next-campaign-manifest.md)
- Tier-3 anchor judge: out of scope for this lock

## Iteration record

### Iteration 0 — reject thinking-enabled local scorer

The campaign already had two independent leniency signals against local thinking-capable scoring:

1. Duramind campaign history recorded an approximately `+11 pp` headline inflation on `lme-s-baseline-0622` when a local thinking-enabled scorer was used (`81.4%` vs a real `~69–70%` plateau on the same scorer family).
2. The repo-local campaign state in [results/lme-camp-state.json](../../results/lme-camp-state.json) records a fixed 50-item partial-panel rescore where local Qwen scoring was more generous than the fallback batch judge:
   - prior labels: `12C / 12PC / 26I`
   - local Qwen rescore: `16C / 12PC / 22I`
   - bias direction: lenient
   - strict delta: `+4/50 = +8.0 pp`
   - lenient delta: `0.0 pp`

Verdict: any thinking-enabled local scorer mode is disqualified for repeatable campaign measurement.

### Iteration 1 — lock Qwen3-32B non-thinking

The approved Tier-1 scorer is the non-thinking Qwen3-32B route pinned in [docs/lme-campaign/scorer-lock.json](./scorer-lock.json).

Reasons for the lock:

- It removes the known leniency source: local thinking mode.
- It stays local and free for every repeatable run.
- The harness now stamps a stable `scorer_version` into every score artifact, so future baselines can require an exact scorer tag instead of informal model names.

## Artifact tagging rule

Every future repeatable score run must cite `scorer_version=tier1-qwen3-32b-nonthinking-v1`.

The harness now records that tag in:

- `checkpoint-score.jsonl`
- `score_report.json`
- `run_manifest.json`
- `RUN_STATUS.json`

## Qualification status

Operational verdict: **approved for Tier-1 repeatable use**.

Control caveat: the checked-in repo evidence proves the failure mode that must be excluded and records the locked non-thinking replacement. The fixed Tier-2 per-item audit sample remains `v9-failures-135`; when that audit is rerun, its result should be appended to this file under the same scorer version rather than creating a new ad hoc lock.
