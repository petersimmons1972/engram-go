---
name: active-priorities
description: Current work focus and pending action items
type: project
Category: active-work
---
# Active Priorities

**Last Updated**: 2026-03-20
**Current Focus**: Clearwatch — reviewer dedup fix, gates 33-35, effectiveness tracker wired

---

## Just Completed (2026-03-20)
- Phase 1: Issue dedup fix (extract_complaint, vendor normalization, threshold 4)
- Phase 2: Gates 33-35 (uncaveated claims, Parametrix disclosure, truncation)
- Phase 3: EffectivenessTracker wired into post-grading lifecycle

## Pending
- [ ] Phase 4: Deduplicate existing open issues (run `--backfill --dry-run` first)
- [ ] Pipeline re-run to validate all changes end-to-end

## Pipeline Re-run Needed [FOUNDER ACTION]

```bash
cd /home/psimmons/projects/clearwatch && bash bin/run-tier1-reports.sh
```

## Blocked/Deferred

| Task                       | Status                                      |
|----------------------------|---------------------------------------------|
| SCALE bugs (#2340-#2364)   | Deferred — 25 P2 concurrency bugs, non-blocking |
| NHI vendor data quality    | Open: #2904, #2923 — monitoring after RSAC  |
