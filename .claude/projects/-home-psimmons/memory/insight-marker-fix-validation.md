---
name: Insight marker validation fix results
description: Commit 62b37ffc reduced compiler errors from 0-insight-markers blocker
type: error
originSessionId: edc0c1e3-3de0-453a-a83b-b7d45a8646c2
---
## Issue: Report 1 insight marker validation

**Commit**: 62b37ffc (fix(stage-5): validate <<insight>> markers before accepting prose)

### Previous Run (v280, 2026-05-07 03:37)
- Primary error: "Found 0 <<insight>> marker(s), need at least 1"
- Secondary errors: 19 uncited percentage errors
- **Total pass1_errors**: 20
- Result: FAILED Stage 6

### Current Run (v298, 2026-05-07 09:07)
- Primary error: GONE ✅
- Secondary errors: 1 uncited percentage error (98%)
- **Total pass1_errors**: 1
- Result: FAILED Stage 6

### Validation
✅ **Insight marker validation fix is working** — reduced errors from 20 to 1

The fix successfully enforced the requirement that prose sections must contain `<<insight:high>>` or `<<insight:medium>>` markers before being accepted by the pipeline. This eliminated the "Found 0 <<insight>>" blocker.

The remaining F2.a error (uncited percentage) is a separate data quality issue unrelated to insight markers.

### Root Cause of Uncited Percentage Error
The error "Uncited percentage '98%' has no <<claim:N>> within 150 chars" indicates that somewhere in the generated prose, a "98%" figure appears without a citation (source reference) nearby. This is likely:
- A prose generation issue where the generator used a percentage without proper citation
- A specific claim in the dossier that didn't cite its percentage properly
- Expected behavior requiring manual dossier/prose adjustment

### Action Items
1. Monitor whether Reports 2-5 have similar single uncited percentage errors
2. If pattern is consistent, investigate which percentage is causing the issue
3. Determine if this is dossier data quality or prose generator issue
