---
name: clearwatch-github-issues-architecture
description: Clearwatch learning system architecture — key files and anti-patterns
type: reference
Category: active-work
---
# Clearwatch Learning System Architecture

## Key Files
- `learning/lessons.yaml` — lesson database
- `pipeline/learning/extractor.py` — GradeExtractor: grades → lessons
- `pipeline/learning/injector.py` — LessonInjector: lessons → Stage 3 prompts
- `pipeline/learning/tracker.py` — EffectivenessTracker: tracks runs_applied and state transitions
- `pipeline/learning/storage.py` — LessonStorage: read/write lessons.yaml (atomic YAML write)
- `bin/stage7_to_github_issues.py` — grades → GitHub issues (complaint-only dedup)

## Anti-Patterns Discovered
- **LLM-fabricated ROI/timeline figures** (M4.6, M6.5 months) with no primary source. Pattern: specific sub-year decimals with no citation. PROHIBITED in affected dossiers.
- **O(n^2) fixture setup** (500x save_lesson() loops) caused hanging tests — not fcntl deadlock. Fix: direct yaml.dump() write.
- **Background nested `claude --print`** can hang indefinitely if context is full — kill and restart.
