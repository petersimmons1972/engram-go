# Active Priorities

**Last Updated**: 2026-03-05T12:35:00Z
**Session**: 20260305-073500
**Status**: Active tracking initiated

---

## Immediate (Next Session)

Work to tackle in the very next session. These are ready to start or actively blocking deliverables.

### 1. Clearwatch #1550 [CRITICAL]
**Issue**: SentinelOne pricing verification ($179.99 vs $210)
- **Impact**: Blocking report delivery to client
- **Status**: RESEARCH COMPLETE - Requires decision
- **Finding**:
  - ✅ **$179.99 is ACCURATE** — official SentinelOne Singularity Complete tier (current)
  - ❌ **$210 is QUESTIONABLE** — doesn't match official pricing ($229.99 for Commercial tier)
  - 📌 Source: Official SentinelOne pricing page + Spendflo analyst guide (conflicting data)
- **Recommendation**:
  - [ ] Verify where $210 claim originated (outdated quote? volume tier? error?)
  - [ ] If Complete→Complete comparison: use $179.99 (both vendors)
  - [ ] If mixing tiers: clarify which tier each vendor (Complete vs Commercial)
  - [ ] Add footnote: "List prices as of March 2026; typical discounts 15-25%"
  - [ ] Confirm with client before report release
- **Next Step**: Review report to identify context of $210 figure, then update/remove

### 2. Clearwatch #1640 [MEDIUM]
**Issue**: Reviewer visual defect tracking
- **Impact**: Not blocking, informational only
- **Status**: Fix merged, tracking only
- **Action Items**:
  - [ ] Monitor next report generation for visual regression
  - [ ] Document any new defects found during review

### 3. Skill Optimization [MEDIUM]
**Issue**: Monitor newly deployed skills in practice
- **Time Estimate**: Ongoing (2-week validation period)
- **Status**: Post-deployment monitoring
- **Action Items**:
  - [ ] Track skill usage and effectiveness
  - [ ] Document improvements or gaps
  - [ ] Plan iteration based on 2-week data
- **Context**: Skills deployed in February 2026; validation cycle through mid-March

---

## This Week (7 Days)

Work scheduled for the next 7 days (2026-03-05 through 2026-03-11).

| Task | Estimate | Status | Dependencies | Priority |
|------|----------|--------|--------------|----------|
| ~~Remove Alert Triage Funnel chart~~ (doesn't exist / already removed) | — | COMPLETE | Chart audit complete | — |
| **[COMPLETE] Audit all ~60 charts for meaningful vendor differentiation** | 6-8 hours | **DONE 2026-03-05** | None | HIGH |
| **[COMPLETE] Remove 5 non-comparative charts** (readiness_matrix, decision_flowchart, outage_infographic, vendor_concentration_risk, cascade_failure_timeline) | 2-3 hours | **DONE 2026-03-05** | Audit complete | HIGH |
| Review & remove remaining 10-15 low-differentiation charts (Phase 2) | 3-4 hours | Ready | Phase 1 complete | MEDIUM |
| Memory System: Complete optimization (Tasks 1-3) | 3-4 hours | Pending | None | MEDIUM |
| Update MEMORY.md with new ACTIVE-PRIORITIES reference | 30 min | Pending | This file creation | LOW |
| Review and consolidate lesson files from MEMORY.md | 2 hours | Pending | Memory optimization | MEDIUM |

### Detailed Tasks

**Chart Audit & Cleanup**
- Background: User feedback indicated identical pricing/features for both vendors on Alert Triage Funnel
- Goal: Remove charts with no meaningful differentiation, keep only those showing real vendor differences
- Success Criteria: ~60 charts reviewed, <45 retained (removing ~15 duplicates/non-differentiated)
- Risk: May reveal architectural issues with chart generation; escalate if pattern found

**Memory System Optimization**
- Task 1: Move "Key Lessons" section detailed content to separate topic file (lessons-learned.md)
- Task 2: Consolidate certificate, deployment, and incident patterns into dedicated files
- Task 3: Create ACTIVE-PRIORITIES.md (this file) as session-start reference
- Rationale: Reduce MEMORY.md from 206→150 lines, keep as concise index only

---

## Blocked Tasks

Work items waiting on external dependencies or unresolved blockers.

| Task | Blocker | Current Status | Unblock Criteria | Days Blocked |
|------|---------|----------------|-----------------|--------------|
| Clearwatch #1550 (SentinelOne pricing) | Pricing data accuracy | Research needed | Verified official source + client confirmation | 0 |
| Homepage CrashLoopBackOff | Network policy label mismatch investigation | Pending detailed log analysis | Root cause identified + fix deployed | 5+ |
| Nextcloud storage pressure | >85% disk usage on zp3 | Awaiting cleanup | Free space >20% available | 3+ |

---

## How to Use This File

### Session Start Checklist
1. **Read this file first** — it's your project roadmap for the session
2. **Review "Immediate" section** — start with these if no other guidance
3. **Check "Blocked" table** — escalate any items >14 days without progress
4. **Skim "This Week"** — understand context for multi-session work
5. **Verify dates** — if this file is >7 days old, check MEMORY.md for updates

### During Session
- **Status updates**: Mark completed tasks as done, move to "Completed (Archive)" if relevant
- **New discoveries**: Add to appropriate section (don't let ad-hoc work go untracked)
- **Blocker changes**: Update status immediately if blocker resolves/changes
- **Time estimates**: Adjust if initial estimate was significantly wrong

### End of Session
- **Commit changes**: After work is done, update this file with status + commit
- **Move completed items**: Archive tasks >2 weeks completed (keep recent for reference)
- **Escalate stale items**: If a task hasn't moved in >5 days, flag as at-risk

---

## Decision Framework (CLAUDE.md Reference)

When prioritizing within a session, use this framework:

- **100% confident this is right** → Just do it (no discussion needed)
- **80-99% confident** → Do it + explain the reasoning in a comment
- **50-80% confident** → Propose the approach first, wait for feedback
- **<50% confident** → Ask before proceeding

Apply to: task sequencing, technical approach, scope decisions

---

## Update Protocol

**Who**: Next session's Claude instance
**When**: Every session start + when status changes
**What**: Update sections below in order

### Session Start (Every Session)
1. Review "Immediate" section
2. Move any completed items to archive or remove if >2 weeks old
3. Check if any "Blocked" items have unblocking updates
4. Update last-updated timestamp at top

### Weekly (Every 7 Days, Thursdays)
1. Review entire "This Week" section
2. Escalate any items without progress >5 days
3. Adjust time estimates based on actual effort
4. Move completed items to archive

### Monthly (First of Month)
1. Archive all completed items from previous month
2. Plan next month's "This Week" priorities
3. Review "Key Lessons" in MEMORY.md for patterns
4. Link any learnings back to this file

### Commit Message Format
```
docs: update ACTIVE-PRIORITIES.md - [task-name] [status-change]

- Task: [brief description]
- Previous Status: [status]
- New Status: [status]
- Changes: [what was updated]
```

Example:
```
docs: update ACTIVE-PRIORITIES.md - complete chart audit

- Task: Audit ~60 charts for vendor differentiation
- Previous Status: In Progress (40/60 reviewed)
- New Status: Completed
- Changes: Removed 12 non-differentiated charts, retained 48 core charts
```

---

## Reference Links

- **CLAUDE.md** → Critical rules and decision framework
- **MEMORY.md** → Infrastructure health, active projects, key lessons
- **PROJECTS-CATALOG.md** → Full project list with ownership
- **lessons-learned.md** → Detailed topic files (planned)
- **homelab-k8s-patterns.md** → Infrastructure-specific patterns

---

## Notes for Next Session

- Monitor SentinelOne pricing verification closely — client is waiting on this report
- Chart audit may take longer than estimated if architectural issues discovered
- Homepage restart pattern is recurring; investigate root cause before next failure
- Skills validation at 2-week mark (2026-03-14) requires decision: keep/iterate/remove

---

## Archive (Completed >2 weeks ago)

(None yet — document will grow over time)
