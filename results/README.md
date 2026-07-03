# results/

This directory is **gitignored by default** (see `.gitignore`).

Campaign log files that should persist as documentation are tracked deliberately
via `git add -f`. The currently-tracked files are:

- `lme-camp-report.md` — live campaign-log artifact
- `lme-camp-state.json`, `lme-camp-stratified-100.json` — campaign-state machine data
- `next-campaign-manifest.md` — next-campaign starting point + post-campaign outcomes block
- `layerb-corruption-probe.md` — Layer B additive-schema integrity probe for issue #1289
- `wisdom/bottleneck-symmetry.md` — durable pattern doc: bottleneck symmetry analysis
- `wisdom/temporal-reasoning-mechanism-analysis.md` — durable pattern doc: temporal reasoning failure mechanisms

## When to commit a file under `results/`

Track via `git add -f` if **all** apply:
- The file is durable wisdom or a pinned campaign reference, not transient per-run data
- It will be read by future contributors looking to orient themselves
- It is not regeneratable (it documents a one-time analysis or campaign synthesis)

Do **not** track per-run outputs: `checkpoint-*.jsonl`, `score_report.json`, `run.log`,
intermediate scoring artifacts, ad-hoc benchmarks. Those stay gitignored.

## Conventions

- File names use `lme-camp-*` prefix for LME campaign artifacts, `wisdom/*.md` for
  durable pattern docs.
- When adding a new tracked file, add it to the inventory above in this README in
  the same commit.

Closes #769.
