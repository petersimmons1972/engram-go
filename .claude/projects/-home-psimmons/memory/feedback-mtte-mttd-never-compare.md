---
name: feedback-mtte-mttd-never-compare
description: NEVER compare SentinelOne 47-min MTTE (human escalation) with CrowdStrike 4-min MTTD (machine detection) as equivalent speeds
type: feedback
Category: feedback
---

SentinelOne's 47-minute figure is MTTE — Mean Time To Escalate to a human customer. CrowdStrike's 4-minute figure is MTTD — automated machine detection. These measure fundamentally different things and MUST NOT be compared on the same chart axis or in the same sentence as equivalent "response times."

**Why:** The founder has flagged this multiple times. DI-001 was filed and "fixed" but the chart renderers (kill_chain_velocity, mdr_timeline) still juxtapose the two figures. "15min vs 47min" in a chart title implies they're measuring the same thing. They're not.

**How to apply:**
1. Charts comparing response speeds must label the measurement type: "Machine MTTD: 4 min" vs "Human Escalation MTTE: 47 min"
2. If both figures appear on the same chart, the chart MUST include a visible disclaimer: "These metrics measure different stages of the response pipeline and are not directly comparable"
3. The kill_chain_velocity and mdr_timeline renderers need this enforcement
4. Domain knowledge DI-001 is documented but not fully enforced in chart output
