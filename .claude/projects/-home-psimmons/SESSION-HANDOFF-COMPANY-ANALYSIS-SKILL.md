# Session Handoff: Company Analysis Skill Creation

**Date**: 2026-01-07
**Status**: RED Phase Complete (Baseline Testing Done)
**Next Phase**: GREEN Phase (Write Skill)

---

## What We Accomplished Today

### 1. CLAUDE.md Optimization Project (60% → 60%)
- **Phase 1 Quick Wins**: ✅ Complete (#14 Incident Templates, #12 Communication)
- **Phase 2 High Value**: ✅ Complete (#3 Runbooks vs Playbooks Separation)
- **Status**: 9 of 15 suggestions implemented
- **Location**: `/home/psimmons/projects/claude-md-improvements/`

**Key Deliverables**:
- Added "Runbooks vs Playbooks" distinction to CLAUDE.md (line 66)
- Created example runbook: `/home/psimmons/RUNBOOKS/k8s-pod-restart.md`
- Created example playbook: `/home/psimmons/PLAYBOOKS/service-404.md`
- Updated project README with completion status

### 2. New Skill: Company Analysis for Interviews (RED Phase Complete)

**Skill Name**: `analyzing-companies-for-interviews`

**Description** (Draft):
```yaml
description: Use when user mentions a company name in the context of job interviews, career research, or employment opportunities. Triggers on explicit commands (/analyze-company), conversational requests ('research Microsoft'), or when discussing interviewing with a specific company. Primarily for tech companies and Sales Engineer roles.
```

**Current Status**: Completed baseline testing WITHOUT skill to document failures

---

## Baseline Testing Results (RED Phase)

### Test Case: Cyberhaven Analysis
**Company**: Cyberhaven (not Cloud5 - caught name confusion)
**Role**: Solutions Engineer, Florida (Remote), $140k base + $60k commission
**Job Link**: https://jobs.ashbyhq.com/Cyberhaven/1bd7164b-6347-45bb-91f2-e5306c1a8077

### What I Did (Superficially):
- ✅ Basic web searches for company info
- ✅ Checked Glassdoor reviews (3.8/5 stars, polarized CEO feedback)
- ✅ Found competitor names (Strac, Nightfall, Forcepoint, Symantec, etc.)
- ✅ Found Gartner Peer Insights rating (4.6/5, 44 reviews)
- ✅ Found generic Solutions Engineer salary data ($132k-$157k Atlanta avg)
- ✅ Identified company rename issue (caught it - good behavior to preserve)

### Critical Failures (What the Skill Must Fix):

#### Process Failures
- ❌ Did NOT check if analysis already exists in `/home/psimmons/projects/interview-tracking/companies/`
- ❌ Did NOT ask role-specific questions before starting
- ❌ Did NOT spawn parallel background agents (used slow sequential searches)
- ❌ Did NOT organize output in proper markdown format for Obsidian
- ❌ Did NOT create proper file structure with metadata/frontmatter

#### Financial & Legal Research Failures
- ❌ Did NOT search for 10-K filings or SEC documents
- ❌ Did NOT identify unusual 10-K items (legal risks, debt, customer concentration, litigation)
- ❌ Did NOT search for recent lawsuits or regulatory issues
- ❌ Did NOT check insider trading or management changes

#### Employee Sentiment Failures (CRITICAL - User Priority)
- ❌ Did NOT search Blind.com for anonymous employee feedback
- ❌ Did NOT search Reddit (r/cscareerquestions, company-specific subreddits)
- ❌ Did NOT search for role-specific reviews (Solutions Engineer experiences)
- ❌ Did NOT search for Atlanta-specific office culture
- ❌ Only checked surface Glassdoor (top reviews, not deep patterns)

#### Customer & Market Research Failures
- ❌ Did NOT search for customer reviews/testimonials
- ❌ Did NOT search G2, TrustRadius, or PeerSpot reviews
- ❌ Did NOT check customer case studies
- ❌ Did NOT analyze customer complaints or churn patterns

#### Competitive Intelligence Failures
- ❌ Did NOT create formal SWOT analysis
- ❌ Did NOT identify top 3-5 competitors per business line
- ❌ Did NOT compare Cyberhaven's positioning vs each competitor
- ❌ Did NOT search for "Cyberhaven vs [Competitor]" head-to-head comparisons
- ❌ Did NOT check competitor compensation for Solutions Engineers in Atlanta

#### Analyst & Industry Coverage Failures
- ❌ Did NOT search GigaOm (only checked Gartner/Forrester)
- ❌ Did NOT check all relevant analyst coverage (found Gartner Cool Vendor, but no Forrester Wave)
- ❌ Did NOT search for industry trend reports relevant to data security/DLP/DSPM

#### News & Social Media Failures
- ❌ Did NOT systematically search for controversies/bad news
- ❌ Did NOT check LinkedIn company page for updates
- ❌ Did NOT check Twitter/X for company updates or customer complaints
- ❌ Did NOT search for layoff announcements or hiring freezes
- ❌ Did NOT search "Cyberhaven news" for last 90-120 days systematically

#### Interview Preparation Failures
- ❌ Did NOT create "Why work here?" talking points
- ❌ Did NOT create questions to ask the interviewer
- ❌ Did NOT highlight role-relevant business units/products (DLP, DSPM, Insider Risk)
- ❌ Did NOT identify common interview questions about the company
- ❌ Did NOT create role-specific insights (Solutions Engineer vs general research)

#### Output Organization Failures
- ❌ Did NOT create file in `/home/psimmons/projects/interview-tracking/companies/cyberhaven.md`
- ❌ Did NOT include metadata/frontmatter for Obsidian
- ❌ Did NOT include tags (#company, #interview-prep, #data-security, #solutions-engineer)
- ❌ Did NOT version/timestamp the analysis
- ❌ Did NOT create update history section (for future refreshes)

### Rationalizations Documented (For Skill's "Common Mistakes" Table)

| Excuse | Reality |
|--------|---------|
| "Basic web search is enough" | Missed 80% of critical interview prep data |
| "Glassdoor covers employee sentiment" | Blind and Reddit have more honest, unfiltered feedback |
| "Found the competitors" | Didn't analyze competitive positioning or create SWOT |
| "Salary data is generic enough" | Need Atlanta-specific + role-specific + competitor comparison |
| "Sequential searches are fine" | Could spawn 5-10 parallel agents, save 10+ minutes |
| "Manual organization is okay" | Should auto-generate proper markdown structure with metadata |
| "I'll just do quick research" | Interview prep requires DEEP research, not surface-level |
| "Don't need 10-K for interview" | Unusual 10-K items reveal risks interviewers won't mention |

---

## User Requirements Summary

### Skill Behavior

**Triggering**:
- **A**: Automatic when company mentioned in interview context
- **B**: Explicit command `/analyze-company [Company Name]`
- **C**: Conversational ("analyze Microsoft", "research this company")

**Company Name Intelligence**:
- Prompt if ambiguous ("Apple" could be multiple companies)
- Apply intelligence: "Google" → "Alphabet Inc." automatically
- Ask for clarification if unclear

**Scope & Focus**:
- Primary: Tech companies (user's job search focus)
- Default role: **Senior Sales Engineer** (unless specified otherwise)
- Can expand to non-tech in future (customer research use case)
- Analysis depth: **Moderate** (5-10 pages, not 20+ pages)
- Focus: **Current state** (last 90-120 days unless historical crucial)

**Output Location**:
- Create: `/home/psimmons/projects/interview-tracking/companies/[company-name].md`
- Integrate with Obsidian (tags, frontmatter, links)

**Update Behavior**:
- **Automatic update** if analysis already exists
- **Track changes over time** (version history)
- **Highlight deltas**: Good news that aged well/poorly, bad news developments
- **Date-stamp** each update

**Financial Research**:
- Pull important 10-K items (unusual, not boilerplate)
- Look for: Legal risks, litigation, debt concerns, management changes, insider trading, customer concentration

**Employee Sentiment (HIGH PRIORITY)**:
- **Deep research** on what customers AND employees say
- Sources: Glassdoor, **Blind.com**, Reddit, role-specific reviews
- Look for: Recent reviews (90-120 days priority), broader patterns
- Include: Role-specific reviews (Solutions Engineer experiences)
- Include: **Compensation data** for Sales Engineers in Atlanta (and competitors)

**Competitive Analysis**:
- Identify top 3-5 competitors **per major business line**
- Create **SWOT analysis** for company vs each competitor
- Focus on business line relevant to role (ask if unclear)

**Analyst Coverage**:
- Search: Gartner, Forrester, **GigaOm**
- Include: Magic Quadrants, Waves, Cool Vendor, trend reports

**Social Media & News**:
- Focus: What **customers say** and **current/former employees say**
- Platform agnostic (find where sentiment lives)
- **Deep research** required (user priority)
- Separate section for **negative information** (all companies have it)

**Interview Preparation Sections**:
- ✅ Common interview questions about the company
- ✅ "Why do you want to work here?" talking points
- ✅ Questions to ask the interviewer

**Confirmation Before Analysis**:
- Always confirm company name and show what will be researched
- Expensive operation (multiple web searches, agent spawns)
- Verify before starting

---

## What's Next (GREEN Phase)

### Step 1: Write Minimal Skill
Create `/home/psimmons/.claude/skills/analyzing-companies-for-interviews/SKILL.md`

**Sections to include** (based on baseline failures):
1. Overview & core principle
2. When to use / triggers
3. Confirmation checklist (company name, role, business line)
4. Research workflow (what to search, in what order)
5. Parallel agent strategy (spawn multiple background agents)
6. Output structure (markdown template)
7. Update/refresh logic (detect existing, show deltas)
8. Common mistakes table (from rationalizations above)

### Step 2: Test with Skill Present
- Re-run Cyberhaven analysis WITH skill loaded
- Verify all baseline failures are now caught
- Document any new rationalizations

### Step 3: Close Loopholes (REFACTOR Phase)
- Identify new rationalizations from testing
- Add explicit counters to skill
- Re-test until bulletproof

### Step 4: Deploy
- Commit skill to git
- Test skill invocation in fresh session

---

## Partial Research Gathered (To Incorporate Later)

### Cyberhaven Quick Facts
- **Industry**: Cybersecurity - Data Security (DLP, DSPM, Insider Risk Management)
- **Funding**: $250M from Khosla Ventures, Redpoint
- **Recognition**:
  - Gartner Cool Vendor 2023
  - Deloitte Fast 500 (#51, Nov 2025)
  - Built In Best Places to Work 2026 (Jan 6, 2026)
  - Gartner Peer Insights: 4.6/5 (44 reviews)
- **Technology**: Data lineage-first architecture, AI-enabled
- **Customers**: Motorola, Zoom, JAMF, Iron Mountain, law firms
- **Recent**: Product launch Fall 2025, next phase Feb 3, 2026

### Glassdoor Summary (3.8/5, 86 reviews)
- **Positive**: Great team, growth opportunities, product quality
- **Negative**: CEO leadership concerns (toxic culture reports), high turnover
- **Polarized**: 66% recommend, but strong negative sentiment from some
- **Comp**: 4.0/5 rating for compensation

### Competitors (Partial List)
- Modern: Strac, Nightfall AI
- Traditional: Forcepoint, Symantec, Proofpoint
- Cloud-focused: Netskope, Cisco Umbrella
- Microsoft: Purview DLP (built into M365)

### Salary Data (Partial)
- **Role offer**: $140k base + $60k commission = $200k total
- **Atlanta avg**: $132k-$157k (Indeed/Glassdoor)
- **Need**: Competitor comparison for Sales Engineers in Atlanta

---

## Files & Locations

**CLAUDE.md Optimization Project**:
- `/home/psimmons/projects/claude-md-improvements/README.md` - Status tracker
- `/home/psimmons/projects/claude-md-improvements/RESEARCH-BASED-SUGGESTIONS.md` - All 15 suggestions
- `/home/psimmons/CLAUDE.md` - Updated with runbooks/playbooks (line 66)
- `/home/psimmons/RUNBOOKS/k8s-pod-restart.md` - Example runbook
- `/home/psimmons/PLAYBOOKS/service-404.md` - Example playbook

**Skill Creation (In Progress)**:
- This handoff: `/home/psimmons/.claude/skills/SESSION-HANDOFF-COMPANY-ANALYSIS-SKILL.md`
- Skill location (to create): `/home/psimmons/.claude/skills/analyzing-companies-for-interviews/`

**Interview Tracking**:
- Output directory (to use): `/home/psimmons/projects/interview-tracking/companies/`

---

## Resume Commands

When you return with credits:

1. **Continue CLAUDE.md optimization**:
   ```
   cd /home/psimmons/projects/claude-md-improvements
   cat README.md
   # Consider implementing #6 (Architecture Decision Records)
   ```

2. **Continue skill creation (GREEN phase)**:
   ```
   cat /home/psimmons/.claude/skills/SESSION-HANDOFF-COMPANY-ANALYSIS-SKILL.md
   # Write the skill addressing all documented baseline failures
   ```

3. **Complete Cyberhaven analysis** (using new skill once ready):
   ```
   /analyze-company Cyberhaven
   # Or: "Please complete the Cyberhaven analysis for my interview"
   ```

---

## Key Decisions Made

1. **Skill triggers**: All three (automatic, command, conversational)
2. **Default role**: Senior Sales Engineer (Atlanta)
3. **Update behavior**: Automatic with delta tracking
4. **Depth**: Moderate (5-10 pages)
5. **Employee sentiment**: HIGH PRIORITY - deep research required
6. **Confirmation**: Always confirm before starting (expensive operation)

---

## Questions for Next Session

1. Should we create the skill first, or finish the Cyberhaven analysis manually to gather more baseline data?
2. Do you want to review the skill structure before I write it?
3. Should the skill create a separate file for each business line analysis, or keep everything in one file?

---

**Created**: 2026-01-07
**Next Session**: Write skill (GREEN phase) or continue CLAUDE.md optimization
**Priority**: User's choice - both are valuable next steps
