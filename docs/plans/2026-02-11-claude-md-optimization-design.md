# CLAUDE.md Optimization Design

**Date:** 2026-02-11
**Goal:** Reduce token usage by ~30% through targeted extraction and condensing
**Approach:** Conservative - preserve all critical content, move project-specific and detailed references external

---

## Design Overview

### Optimization Strategy

**Primary goal:** Reduce token usage to lower API costs and preserve context window

**Approach:** Conservative extraction
- Extract project-specific content to project CLAUDE.md
- Extract detailed workflows to reference docs
- Condense verbose sections while preserving essential information
- Target: ~30% reduction (396 lines → 250-270 lines)

### User Requirements

- **Most-referenced section:** Team management workflows
- **Conservative approach:** Keep all critical content, trim examples/verbosity
- **Project isolation:** Security Intelligence Business content should only load in that project

---

## Part 1: Content Extraction Plan

### Project-Specific Content → Project CLAUDE.md

**Move to `~/projects/security-intelligence-business/CLAUDE.md`:**

1. **Project-Specific Conventions** (lines 3-10)
   - Report versioning format
   - Directory structure rules
   - Generation command patterns

2. **Multi-Stage Verification** (lines 268-362)
   - Specific to chart generation → embedding → HTML pipeline
   - Detailed examples for this project's architecture
   - Red flags and checklists

**Rationale:** These only apply to security-intelligence-business project, not needed in global context

### Reference Documentation Extractions

**1. Team Management → `docs/TEAM-MANAGEMENT-WORKFLOW.md`**
- Detailed 4-step workflow (document, commit, verify, shutdown)
- Service record format and requirements
- Why it matters explanation
- Replace with one-liner in CLAUDE.md: "NEVER shut down teams without service records (see docs/TEAM-MANAGEMENT-WORKFLOW.md)"

**2. Generals Integration → `docs/GENERALS-INTEGRATION.md`**
- Commander profile updates
- XP calculation methodology
- Ribbon/medal awarding criteria
- Competence progress tracking
- Behavioral observation guidelines

**3. DNS API Reference → `docs/DNS-API-REFERENCE.md`**
- Detailed curl examples for Unifi API
- Detailed curl examples for Cloudflare API
- Pi-hole API limitations and notes
- Replace with quick reference table in CLAUDE.md

---

## Part 2: Project-Specific CLAUDE.md

### File: `~/projects/security-intelligence-business/CLAUDE.md`

```markdown
# Security Intelligence Business - Project Instructions

## Report Generation Conventions

**Versioning:** Three-digit zero-padded (001, 002, ..., 083, 084)

**Directory structure:** `output/Vendor1_v_Vendor2/{version}/`
- ✅ `output/CrowdStrike_v_SentinelOne/084/`
- ❌ `output/.../CrowdStrike_v_SentinelOne/001/` (NO nested vendor directories)

**Generation command:** Use explicit path, NOT `--auto-version` flag

**Reference:** See `.naming-conventions` file in project root

---

## Multi-Stage Verification Pattern

**Critical lesson (2026-02-09):** Charts existed in code but never appeared in reports.

### The Pattern
1. **Generation** - Code creates output (e.g., `chart_generator.py`)
2. **Integration** - Output gets wired/mapped (e.g., `CHART_PLACEMENT_MAP`)
3. **Delivery** - Final artifact contains expected elements

### Verification Checklist
- [ ] Stage 1: Code exists and returns expected data
- [ ] Stage 2: Integration/mapping connects stages
- [ ] Stage 3: Final output shows expected elements

**If you can't verify output, you don't know if it works.**

### Red Flags
- "The code looks good" → But is it USED?
- "Implementation complete" → But does output SHOW it?
- "Charts implemented" → But are they EMBEDDED?

### Application to This Project
- Don't claim charts complete until HTML shows them
- Verify `CHART_PLACEMENT_MAP` entries exist
- Count SVG tags: `grep "<svg" output/.../report.html | wc -l`
- Check PDF visually
```

**Size:** ~40 lines (vs. 100+ lines removed from global CLAUDE.md)

---

## Part 3: Condensed Sections

### Web Search Section

**Before:** 41 lines with detailed API examples, JSON parsing, K8s details

**After:** 8 lines
```markdown
## Web Search

**PRIMARY:** SearXNG (`https://searxng.petersimmons.com/search?q={query}&format=json`)
**FALLBACK:** WebSearch tool (if SearXNG fails)

Quick Reference: `/search?q={query}&format=json&language=en`
K8s: `kubectl scale deployment searxng -n default --replicas=N`
```

### DNS Management Section

**Before:** 52 lines with detailed curl examples for each API

**After:** 15 lines
```markdown
## DNS Management

**Use APIs - never ask user to do it manually**

| System | Use For | Credentials |
|--------|---------|-------------|
| Unifi API | Internal DNS | `~/.claude/.unifi-credentials` |
| Cloudflare | Public domains | `~/projects/kubernetes/cert-manager/.env` |
| Pi-hole | Monitoring only | `~/.claude/pihole-api-credentials.md` |

Full examples: `docs/DNS-API-REFERENCE.md`
```

### Team Management Section

**Before:** 44 lines with detailed workflow steps and explanations

**After:** 1-2 lines
```markdown
**BEFORE shutting down teams:** Complete workflow in `docs/TEAM-MANAGEMENT-WORKFLOW.md`
```

---

## Part 4: Final Metrics

### Size Reduction

**Current:** 396 lines
**Optimized:** ~250-270 lines
**Reduction:** ~130-150 lines (33-38% savings)
**Token savings:** ~25-30% reduction in context consumption

### Content Distribution

**Global CLAUDE.md:** Universal rules, quick references, essential workflows
**Project CLAUDE.md:** Project-specific conventions and learnings
**Reference docs:** Detailed examples and workflows

---

## Implementation Plan

### Files to Create

1. `~/projects/security-intelligence-business/CLAUDE.md` (~40 lines)
2. `~/docs/TEAM-MANAGEMENT-WORKFLOW.md` (detailed 4-step workflow)
3. `~/docs/GENERALS-INTEGRATION.md` (XP, ribbons, profiles)
4. `~/docs/DNS-API-REFERENCE.md` (detailed curl examples)

### Steps

1. Create project-specific CLAUDE.md in security-intelligence-business
2. Create reference docs in ~/docs/
3. Update global CLAUDE.md with condensed versions + pointers
4. Test by working in different projects to verify context loading
5. Commit changes with clear explanation

### Verification

- Check token usage before/after in a typical session
- Verify project-specific content loads when in that project
- Verify reference docs are accessible when needed
- Confirm all critical functionality preserved

---

## Trade-offs

### Benefits

✅ ~30% token reduction → lower API costs
✅ Faster context loading
✅ Cleaner separation of concerns
✅ Project-specific rules only load when relevant
✅ More maintainable (changes to workflow don't touch global CLAUDE.md)

### Considerations

- Occasionally need to reference external docs
- Team Management workflow pointer instead of inline details
- Need to maintain multiple files instead of one monolithic file

---

## Success Criteria

1. Token usage reduced by 25-30% in typical sessions
2. All critical rules and workflows preserved
3. Project-specific content only loads in relevant projects
4. No loss of functionality or important context
5. Easier to maintain and update individual sections
