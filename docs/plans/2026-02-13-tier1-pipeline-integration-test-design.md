# Tier 1 Pipeline Integration Test - Design Document

**Date:** 2026-02-13
**Designer:** Field Marshal (Claude Sonnet 4.5)
**Approved By:** User
**Status:** Ready for Implementation

---

## Overview

Comprehensive integration test of the security intelligence report generation pipeline across 6 major Tier One vendor pairs. This test validates:

1. Report generation at scale (6 concurrent reports)
2. New 12-layer QA system deployed 2026-02-13
3. Researcher agent coordination via SearXNG
4. K8s infrastructure (Playwright testing at reports.petersimmons.com)
5. Multi-stage verification pattern effectiveness

## Objectives

### Primary Objectives
- Generate 6 fresh reports (Reports 218-223)
- Execute full QA suite on each report
- Test parallel execution and resource coordination
- Document systematic vs random failure patterns
- Validate end-to-end pipeline resilience

### Secondary Objectives
- Stress test K8s Playwright infrastructure
- Measure SearXNG performance under load
- Validate multi-stage verification pattern
- Identify template improvements
- Generate reusable service records

## Vendor Pairs (Reports 218-223)

1. **Report 218:** CrowdStrike vs SentinelOne
2. **Report 219:** CrowdStrike vs Microsoft Defender
3. **Report 220:** SentinelOne vs Microsoft Defender
4. **Report 221:** CrowdStrike vs Palo Alto Cortex XDR
5. **Report 222:** SentinelOne vs Palo Alto Cortex XDR
6. **Report 223:** Microsoft Defender vs Palo Alto Cortex XDR

---

## Architecture

### Team Structure

**Command Hierarchy:**
- **Field Marshal** (Sonnet): Coordination, task assignment, results compilation
- **6 Report Generals** (model assigned per task):
  - General 1: Report 218 (CrowdStrike vs SentinelOne)
  - General 2: Report 219 (CrowdStrike vs Microsoft Defender)
  - General 3: Report 220 (SentinelOne vs Microsoft Defender)
  - General 4: Report 221 (CrowdStrike vs Palo Alto)
  - General 5: Report 222 (SentinelOne vs Palo Alto)
  - General 6: Report 223 (Microsoft Defender vs Palo Alto)

**Model Selection Strategy:**
- **Haiku:** Web research, data extraction, test execution, simple verification
- **Sonnet:** Report generation, content synthesis, quality review
- **Opus:** Complex architectural decisions, deep debugging (if systematic failures)

### Task Coordination

**Shared Task List:** 18 tasks total (3 per report)
1. Research/Dossier validation
2. Report generation
3. QA testing & documentation

**Dependencies:**
- No cross-report dependencies (fully parallel)
- Within each report: Research → Generation → QA (sequential)

**Communication Protocol:**
- Generals report after task completion
- Field Marshal doesn't micromanage
- Intervention only on systematic failures (3+ reports)

**Resource Contention Handling:**
- SearXNG: Service handles load (no coordination needed)
- Storage: Isolated directories per report
- K8s testing: Sequential coordination via task list

---

## Execution Workflow

### Phase 1: Team Initialization (~10 minutes)

**Field Marshal Tasks:**
1. Create team: `team-tier1-pipeline-test`
2. Create 18 tasks in shared task list
3. Spawn 6 generals using proper personalities from `~/projects/generals/`
4. Assign initial research validation tasks
5. Set baseline expectations

### Phase 2: Parallel Execution (~3-6 hours)

**Each General Independently:**

**Step 1: Research Phase (Haiku)**
```bash
# Check existing dossier
cd ~/projects/security-intelligence-business
ls -lh dossiers/vendor1_vs_vendor2.json

# If stale (>7 days): Spawn researcher to update via SearXNG
# If fresh: Proceed with existing data
```

**Step 2: Generation Phase (Sonnet)**
```bash
cd ~/projects/security-intelligence-business/apps/minimal

# Generate report following ARMY-ORDERS-V11
python -m src.cli \
  dossiers/vendor1_vs_vendor2.json \
  output/

# Follow TDD validation loop:
# - Generate each chart
# - Validate geometry: bin/validate_report_charts.py
# - Sync to K8s: bin/sync-reports-to-k8s.sh
# - Browser test: pytest tests/playwright/test_chart_visibility.py
```

**Step 3: QA Phase (Haiku)**
```bash
# Sync to K8s infrastructure
./bin/sync-reports-to-k8s.sh

# Run full 12-layer QA suite
./bin/run-qa-tests.sh

# Document results per layer (pass/fail/expected-fail)
# Report findings to Field Marshal
```

### Phase 3: Results Compilation (~30 minutes)

**Field Marshal Tasks:**
1. Collect 6 service records
2. Create QA comparison matrix (6 reports × 12 layers)
3. Identify systematic vs random failures
4. Generate final integration test report
5. Commit all documentation to GitHub
6. Gracefully shutdown team

---

## Expected Failures & Handling

### Known Systematic Failures (All 6 Reports)

**1. PDF Generation** (Disabled in pipeline)
- **Expected:** 6/7 PDF tests fail per report
- **Action:** Document, don't fix (known limitation)
- **Impact:** Layer 12 failures expected

**2. Accessibility Landmarks** (Missing `<main>` element)
- **Expected:** 2/6 accessibility tests fail per report
- **Action:** Document as template issue
- **Impact:** Layer 9 partial failures expected

### Potential Random Failures

**1. Chart Integration** (Multi-stage verification gaps)
- **Risk:** Charts generated but not embedded in HTML
- **Detection:** `grep "<svg" report.html | wc -l` should return 9
- **Action:** Generator documents integration gap in service record

**2. SearXNG Availability** (Service downtime)
- **Risk:** Research phase stalls
- **Detection:** HTTP errors from searxng.petersimmons.com
- **Action:** Fall back to existing dossiers, document unavailability

**3. Resource Contention** (6 concurrent pytest runs)
- **Risk:** K8s overwhelmed, Playwright timeouts
- **Detection:** Test timeout errors in logs
- **Action:** Coordinate sequential K8s testing via task list

### Failure Documentation Strategy

**Categorization:**
- **Systematic:** Template/architecture issues affecting all reports
- **Random:** Generation-specific issues affecting 1-2 reports

**Quantification:**
- "4/6 reports failed Layer 9" (systematic)
- "Only Report 220 failed Layer 5" (random)

**Learning Loop:**
- Systematic failures → Update templates/ARMY-ORDERS
- Random failures → Improve generation prompts/validation

---

## Deliverables

### Per-Report Service Records (6 total)

Each general creates:
```markdown
# Report [218-223]: [Vendor] vs [Vendor] - Pipeline Test

**Mission:** Full pipeline integration test
**Date:** 2026-02-13
**Status:** [✅ Complete / ⚠️ Partial / ❌ Failed]
**Models Used:**
  - Research: [Haiku/Sonnet]
  - Generation: [Sonnet/Opus]
  - QA: [Haiku]

## Accomplishments
- Report generated: [Report number, file path]
- QA tests executed: [12/12 layers]
- Issues found: [Count]

## QA Results (12 Layers)
1. Structural Snapshots (syrupy): [PASS/FAIL]
2. Metric Regressions (pytest-regressions): [PASS/FAIL]
3. Prose Quality (textstat): [PASS/FAIL]
4. Citation URLs (linkchecker): [PASS/FAIL]
5. Visual Regression (Playwright): [PASS/FAIL]
6. Vale Prose Linting: [PASS/FAIL/SKIPPED]
7. G-Eval Rubrics: [PASS/FAIL]
8. HTML Validation (html5lib): [PASS/FAIL]
9. Accessibility (axe-core): [PASS/FAIL - Expected 2 failures]
10. Internal Links: [PASS/FAIL]
11. Performance Budgets: [PASS/FAIL]
12. PDF Quality (PyMuPDF): [EXPECTED FAIL - PDF disabled]

## Issues Found

### Systematic Issues
[Template/architecture issues that affect all reports]

### Random Issues
[Generation-specific issues unique to this report]

## Technical Challenges
[Obstacles encountered and solutions applied]

## XP Gained
[Based on complexity, learnings, problem-solving]
```

### Final Integration Report (Field Marshal)

**Contents:**
1. **Executive Summary**
   - Mission objectives vs results
   - Overall success rate
   - Key findings

2. **QA Comparison Matrix**
   ```
   Layer | R218 | R219 | R220 | R221 | R222 | R223 | Pattern
   ------|------|------|------|------|------|------|--------
   L1    | PASS | PASS | PASS | PASS | PASS | PASS | 100%
   L9    | FAIL | FAIL | FAIL | FAIL | FAIL | FAIL | Systematic (landmarks)
   L12   | FAIL | FAIL | FAIL | FAIL | FAIL | FAIL | Systematic (PDF disabled)
   ```

3. **Systematic Failure Analysis**
   - Issues affecting all/most reports
   - Root cause identification
   - Template improvement recommendations

4. **Infrastructure Performance**
   - SearXNG availability and response times
   - K8s Playwright service metrics
   - Storage utilization
   - Team coordination efficiency

5. **Recommendations**
   - Pipeline improvements
   - Template updates
   - QA suite enhancements
   - Process optimizations

---

## Success Criteria

**Mandatory:**
- ✅ All 6 reports generated (HTML files exist)
- ✅ All 6 reports synced to K8s
- ✅ Full QA suite executed on each (12 layers)
- ✅ Results documented (pass/fail/expected-fail)
- ✅ Patterns identified (systematic vs random)
- ✅ Service records created for each report
- ✅ Final integration report created
- ✅ All documentation committed to GitHub
- ✅ Team gracefully shut down

**Nice-to-Have:**
- 🎯 >80% QA pass rate (excluding expected failures)
- 🎯 <5 hour total execution time
- 🎯 Zero infrastructure failures
- 🎯 Systematic failures documented with fix recommendations

---

## Timeline Estimate

**Phase 1 (Initialization):** 10 minutes
- Team creation, task list setup, general spawning

**Phase 2 (Parallel Execution):** 3-6 hours
- Research: 15-30 min per report (parallel)
- Generation: 45-90 min per report (parallel)
- QA: 20-30 min per report (parallel)

**Phase 3 (Compilation):** 30 minutes
- Service record collection
- Matrix creation
- Final report generation

**Total: 4-7 hours**

---

## Context Files

**QA Infrastructure:**
- `~/SESSION-2026-02-13-PLAYWRIGHT-QA-INFRASTRUCTURE.md`
- `~/projects/playwright-testing-knowledge/`

**Report Generation:**
- `~/projects/security-intelligence-business/ARMY-ORDERS-V11.md`
- `~/projects/security-intelligence-business/CLAUDE.md`
- `~/projects/security-intelligence-business/apps/minimal/`

**Testing:**
- `~/projects/security-intelligence-business/bin/run-qa-tests.sh`
- `~/projects/security-intelligence-business/tests/qa/`

**Infrastructure:**
- K8s namespace: `security-intelligence-business`
- Service: `reports.petersimmons.com`
- Storage: Longhorn PVC (20Gi)

---

## Risk Mitigation

**High Risk:**
- 6 concurrent generations overwhelming system
  - **Mitigation:** Model-efficient resource allocation (Haiku where possible)

**Medium Risk:**
- SearXNG unavailable during research
  - **Mitigation:** Fall back to existing dossiers

**Low Risk:**
- K8s service downtime
  - **Mitigation:** Can test locally if needed

---

## Approval & Sign-off

**Designer:** Field Marshal (Claude Sonnet 4.5)
**Design Date:** 2026-02-13
**Approved By:** User
**Approval Date:** 2026-02-13
**Status:** ✅ Approved - Ready for Implementation

---

**Next Steps:**
1. ✅ Design approved
2. ⏭️ Create implementation plan (superpowers:writing-plans)
3. ⏭️ Execute Phase 1 (team initialization)
4. ⏭️ Monitor Phase 2 (parallel execution)
5. ⏭️ Complete Phase 3 (results compilation)
