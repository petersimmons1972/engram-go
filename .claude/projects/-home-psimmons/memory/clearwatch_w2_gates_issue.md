---
name: Clearwatch W2 Gates - GOLD report regeneration issue
description: Issue #4711 blocking tier-1 GOLD regeneration due to incomparable MTTE metrics in dossiers
type: project
originSessionId: edc0c1e3-3de0-453a-a83b-b7d45a8646c2
---
## Problem

Issue #4711 requires regenerating 6 Tier-1 GOLD reports under the new W2 prompt (executive_verdict requirement). Reports were failing at Stage 2 (InputQualityGate) with P5 blocking error:

```
"P5: 'mttd_minutes' mixes incomparable measurement types: 
  mttd_analyst_notification vs mtte_customer_notification"
```

## Root Cause

Three dossiers were mixing incomparable MTTD metric types:
- **CrowdStrike metrics**: MTTD (detection time) - analyst notification - COMPARABLE
- **SentinelOne metrics**: MTTE (customer notification / eradication time) - NOT comparable to MTTD

This violates Clearwatch's strict MTTD metric restriction (CLAUDE.md):
> `mtte_customer_notification` may ONLY be used in charts that explicitly compare MDR service notification times

## Solution

Nullified the incomparable SentinelOne MTTE metrics in three dossiers:
1. `crowdstrike_vs_sentinelone.json` - removed mtte_customer_notification (47 min)
2. `sentinelone_vs_microsoft_defender.json` - removed mtte_eradication (47 min)
3. `sentinelone_vs_paloalto_cortex.json` - removed mtte_customer_notification (47 min)

Commit: f9e9337c "fix(dossiers): remove incomparable MTTE metrics blocking validation"

## Next Steps

After reports complete:
1. Verify new versions pass Stage-7 with F1/F2/F3 gates
2. Update GOLD_VERSIONS dict with new version numbers (likely v255+)
3. Remove gates 9 & 42 from DEGRADED_GATES
4. Run regression tests - should pass without DEGRADED workarounds

**Why:** MTTE (time for MDR to notify customer) and MTTD (detection time) measure different steps in the threat response timeline and cannot be compared directly.
