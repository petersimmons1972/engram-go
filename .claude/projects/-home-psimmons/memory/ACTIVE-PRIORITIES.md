---
Category: active-work
---
# Active Priorities

**Last Updated**: 2026-03-10
**Current Focus**: Clearwatch — labor cost verification complete, v190 action items pending

---

## Immediate Action Items (from labor-cost-verification-memo.md)

1. [ ] Resolve v189 internal contradiction: report disclaims FTE comparisons but shows them in 3 charts
2. [ ] Add methodology disclosure footnotes to False Positive Tax + Analyst Hour Burn Rate charts
3. [ ] Fix BLS salary figure ($75K entry-level vs $120K median — choose and label)
4. [ ] Correct false Forrester attribution in `research_prompt.md` ("1.0-1.5 FTE/1K endpoints")
5. [ ] Qualify "50-70% of TCO is labor" axiom in CORE-METHODOLOGY.md + CHART-STANDARDS.md
6. [ ] Reframe tco-methodology.md dollar ranges as "illustrative, not sourced"

## Pipeline Re-run Needed [FOUNDER ACTION]
```bash
cd /home/psimmons/projects/clearwatch && bash bin/run-tier1-reports.sh
```
Chart fixes from visual QA (2026-03-08) + prompt fixes from reviewer feedback need regeneration.

## Current Report Status

| Report | Latest Version | Notes |
|--------|----------------|-------|
| CrowdStrike_v_SentinelOne | v189 | Labor claims graded — see verification memo |
| PaloAltoNetworks_v_CrowdStrike | v059 | Chart fixes pending re-run |
| MicrosoftDefender_v_PaloAlto | v052 | Chart fixes pending re-run |
| SentinelOne_v_PaloAltoCortex | v045 | Chart fixes pending re-run |
| SentinelOne_v_MicrosoftDefender | v046 | Pricing corrected ($179.99) in commit 7281d18 |

## Blocked/Deferred

| Task | Status |
|------|--------|
| SCALE bugs (#2340-#2364) | Deferred — 25 P2 concurrency bugs, non-blocking |
| Visual smoketest full pass | Needs pipeline re-run |
