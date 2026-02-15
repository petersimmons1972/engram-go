---
name: session-documentation
description: Use when completing major work or before team shutdown - ensures consistent session summaries with required sections and service records format
---

# Session Documentation

Guide for writing consistent, useful session summaries and service records.

## When to Use

- After completing major work (deployments, fixes, features)
- Before shutting down teams (REQUIRED - see CLAUDE.md)
- When creating handoff documentation
- End of significant troubleshooting sessions

## Session Summary Format

**Filename:** `SESSION-YYYY-MM-DD-BRIEF-DESCRIPTION.md`

**Required Sections:**

### 1. Header
```markdown
# [Brief Title] - Session Summary
**Date**: YYYY-MM-DD
**Duration**: ~X hours
**Status**: ✅ COMPLETE / ⚠️ PARTIAL / ❌ BLOCKED
```

### 2. Executive Summary
2-3 sentences: What was done, what worked, current state.

### 3. What We Accomplished
Bulleted list of concrete deliverables:
- Feature X deployed
- Issue Y resolved
- Documentation Z created

### 4. Critical Issues Resolved
For each major issue encountered:
- **Problem**: Brief description
- **Root Cause**: What actually caused it
- **Solution**: How it was fixed
- **Impact**: What required changes

### 5. Technical Lessons Learned
Numbered list of key insights:
1. Next.js 15+ requires async params
2. Traefik IngressRoute takes precedence over standard Ingress

### 6. Files Created/Modified
Two sections:
- **New Files**: List with brief description
- **Modified Files**: List with what changed

### 7. Next Steps
Categorized by timeframe:
- **Immediate**: Must do now
- **Short-Term**: Next 1-2 weeks
- **Medium-Term**: Next 1-3 months

## Service Records Format

**Use when shutting down teams** (Critical - see CLAUDE.md)

**For each team member:**

```markdown
### [General Name] ([Model])
**Task**: [Brief task description]

**Accomplishments**:
- Concrete achievement 1
- Concrete achievement 2
- Technical contribution with specifics

**Challenges**:
- Issue encountered 1
- Issue encountered 2
- What didn't work as expected

**Learning**: [One-sentence key insight from this work]

**XP Earned**: [Number] XP ([justification])
**Campaign Ribbons**: 🎖️ [Ribbon Name], 🎖️ [Ribbon Name]
```

**Mission Assessment:**
- **What Worked**: Team dynamics, technical approaches
- **What Failed**: Approaches that didn't work
- **Strategic Pivot**: Major direction changes (if any)
- **Final Status**: Outcome and next steps

See `docs/GENERALS-INTEGRATION.md` for XP/ribbon guidelines.

## Quick Decision Tree

```
Finished major work?
├─ Used teams? → Write service records first
├─ Major deployment? → Full session summary
├─ Quick fix? → Brief summary optional
└─ Troubleshooting? → Document issue + solution
```

## Examples

**Good Session Summary:**
- Clear executive summary
- Specific metrics (53% token reduction)
- Technical lessons with context
- Concrete next steps

**Good Service Record:**
- Quantified accomplishments (289 lines, 4,093 words)
- Honest challenges (output undersized vs requirements)
- Specific learning (Following spec exactly ≠ meeting business needs)
- Justified XP (implementation + debugging work)

## Anti-Patterns

❌ **Don't:**
- Write narrative story ("we started by...")
- Omit failures/challenges
- Skip verification status
- Forget to commit service records before team shutdown
- Use vague language ("improved things", "made progress")

✅ **Do:**
- Concrete facts and numbers
- Include what didn't work
- State current status clearly
- Commit service records to GitHub before shutdown
- Use specific examples

## Where to File

- Session summaries: `/home/psimmons/SESSION-*.md`
- Service records: Within session summary or separate file
- Commit to git: Yes (required for service records)

## Related

- `docs/TEAM-MANAGEMENT-WORKFLOW.md`: Full team shutdown process
- `docs/GENERALS-INTEGRATION.md`: XP and ribbon guidelines
- `MEMORY.md`: Recent sessions are auto-indexed
