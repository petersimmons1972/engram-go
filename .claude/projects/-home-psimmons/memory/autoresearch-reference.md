---
Category: reference
name: autoresearch-reference
description: Karpathy's autoresearch repo cloned as reference for autonomous experimentation patterns applicable to Clearwatch prompt/chart optimization
type: reference
---

Karpathy's `autoresearch` repo at `~/projects/autoresearch`. Reference for modify→evaluate→keep/discard loop pattern applied to Clearwatch prompt/chart optimization. ML training is irrelevant — the experimentation pattern is what matters.

**Key reference files**: `program.md` (agent instruction design), `train.py:428-451` (hyperparameter format), `prepare.py:343-365` (evaluation function separation)

**Clearwatch integration (2026-03-16)**:
- `experiments/prompt-experiments.tsv` / `chart-experiments.tsv` — track variant → grade → keep/discard
- `bin/ab-test-grades.py` — compare CISO+Gordon grades between report HTML variants
- `bin/sweep-chart-params.py` — sweep chart params, run SVG validators (71 chart types, 14 params)
- `bin/calibrate-research-depth.py` — research depth calibration study (~$1-5/pair)
- `pipeline/research_agent.py` accepts `research_depth=0..3`

**A/B results (2026-03-16)**: v165→v180 Gordon +1.00 (exec summary length), v165→v199 Gordon +0.70 (recommendation summary) — both KEPT.

**Next**: Run calibration study — one CrowdStrike/SentinelOne pair at ~$1-5 cost.
