---
name: feedback-no-minimal-security
description: Never recommend minimal/accept-risk security posture — user rejects CISO-style patching philosophy
type: feedback
Category: feedback
---

Never recommend "minimal change" security approaches that accept structural security debt as residual risk. The user explicitly rejected the CISO's "patch CRITICAL/HIGH only, accept 27 open issues" philosophy.

**Why:** User has direct experience with the Home Depot breach aftermath ($5.5M engagement). The HD breach was caused by exactly this pattern — flat network + vendor credential trust + deferred segmentation = 56M cards stolen. "Accepted risk" on network isolation and supply chain is where real breaches live.

**How to apply:** When presenting security remediation options, always lead with defense-in-depth and structural fixes. Quick patches are fine as Week 1 triage, but never frame them as the complete solution. NetworkPolicies, RBAC, supply chain integrity, and network segmentation are non-negotiable foundations, not "nice-to-haves."
