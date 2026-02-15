# CLAUDE.md Optimization Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Reduce global CLAUDE.md token usage by ~30% while preserving all critical content through project-specific extraction and condensing.

**Architecture:** Extract project-specific content to `~/projects/security-intelligence-business/CLAUDE.md`, move detailed workflows to reference docs in `~/docs/`, condense verbose sections in global CLAUDE.md to quick references with pointers.

**Tech Stack:** Markdown, Git

**Design Doc:** `docs/plans/2026-02-11-claude-md-optimization-design.md`

---

## Task 1: Create Project-Specific CLAUDE.md

**Files:**
- Create: `~/projects/security-intelligence-business/CLAUDE.md`

**Step 1: Create project-specific CLAUDE.md**

Create file with content:

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

**Thoughts that mean STOP:**
- "The code looks good" → But is it USED?
- "Implementation complete" → But does output SHOW it?
- "Charts implemented" → But are they EMBEDDED?

**Before claiming done:**
1. Verify Stage 1 (generation code)
2. Verify Stage 2 (integration/wiring)
3. Verify Stage 3 (final output)
4. Check output against user request

### Application to This Project

**For chart implementations:**
- Don't claim charts complete until HTML shows them
- Verify `CHART_PLACEMENT_MAP` entries exist
- Count SVG tags: `grep "<svg" output/.../report.html | wc -l`
- Check PDF visually

**For service deployments:**
- Don't claim service deployed until pod is Running
- Verify Service exists AND routes to pod
- Verify Ingress exists AND DNS resolves
- Test actual endpoint, not just manifests

**General principle:**
- Work backwards from user experience
- Verify each integration point
- Trust output, not code alone
- If output missing, integration failed
```

**Step 2: Verify file created**

Run: `cat ~/projects/security-intelligence-business/CLAUDE.md | head -5`
Expected: See "# Security Intelligence Business - Project Instructions"

**Step 3: Commit**

```bash
cd ~/projects/security-intelligence-business
git add CLAUDE.md
git commit -m "docs: add project-specific CLAUDE.md with conventions and verification patterns

- Report generation conventions (versioning, directory structure)
- Multi-stage verification pattern (2026-02-09 lesson)
- Project-specific application examples

Co-Authored-By: Claude Sonnet 4.5 <noreply@anthropic.com>"
```

---

## Task 2: Create Team Management Workflow Reference

**Files:**
- Create: `~/docs/TEAM-MANAGEMENT-WORKFLOW.md`

**Step 1: Create team management workflow doc**

Create file with content:

```markdown
# Team Management Workflow

## CRITICAL: Required Workflow Before Team Shutdown

**NEVER shut down a team without completing this workflow.**

### Step 1: Document Service Records

Write comprehensive records for each team member:

**Required elements:**
- **Accomplishments** - What succeeded, why it worked, what was learned
- **Failures/Issues** - What failed, root causes, lessons learned, how to prevent
- **Behavioral observations** - Personality traits demonstrated, consistency with profile
- **XP earned** - Calculate based on task completion and quality
- **Competence progress** - N/5 deployments toward next star
- **Campaign ribbons and medals** - Based on user feedback and achievements

**Format:** Structured markdown in team member's profile or service record file

### Step 2: Commit to GitHub

All service records must be committed and pushed:

1. Update commander profiles with deployment experience
2. Stage changes: `git add profiles/*.md` (or relevant files)
3. Commit with descriptive message explaining what was learned
4. Push to remote repository

**For generals project:**
- Repository: `https://github.com/petersimmons1972/generals.git`
- Profiles location: `profiles/*.md`
- Include deployment summary in service record section

### Step 3: Verify Commit

Confirm git push succeeded:

```bash
git log --oneline -1  # Check commit exists
git status           # Should show "up to date with origin"
```

### Step 4: Shutdown Team

Only after steps 1-3 complete:

```bash
rm -rf ~/.claude/teams/{team-name}
rm -rf ~/.claude/tasks/{team-name}
```

Or use TeamDelete tool if available.

---

## Why This Matters

**This is the self-learning mechanism.**

Without service records:
- We lose all operational knowledge
- We repeat the same mistakes
- Commanders don't improve across deployments
- No learning curve or skill progression

Service records are how the system:
- Captures what works and what doesn't
- Tracks commander development over time
- Builds institutional knowledge
- Improves performance across sessions

**Penalty for Skipping:** Regression to zero knowledge = wasted effort

---

## Quick Checklist

Before team shutdown, verify:
- [ ] Service records written for all team members
- [ ] Records include accomplishments + failures + observations
- [ ] XP/competence/ribbons updated
- [ ] Changes committed to git
- [ ] Changes pushed to remote
- [ ] `git status` shows up to date
- [ ] THEN shutdown team directories

---

## Integration with Generals System

See `GENERALS-INTEGRATION.md` for detailed guidance on:
- XP calculation methodology
- Ribbon and medal criteria
- Competence progression system
- Profile update formats
```

**Step 2: Verify file created**

Run: `cat ~/docs/TEAM-MANAGEMENT-WORKFLOW.md | head -3`
Expected: See "# Team Management Workflow"

**Step 3: Commit**

```bash
cd ~
git add docs/TEAM-MANAGEMENT-WORKFLOW.md
git commit -m "docs: extract team management workflow to reference doc

4-step workflow: document, commit, verify, shutdown
Explains why service records matter for self-learning
Quick checklist for verification

Co-Authored-By: Claude Sonnet 4.5 <noreply@anthropic.com>"
```

---

## Task 3: Create Generals Integration Reference

**Files:**
- Create: `~/docs/GENERALS-INTEGRATION.md`

**Step 1: Create generals integration doc**

Create file with content:

```markdown
# Generals System Integration

## Overview

The generals system tracks commander development across deployments using profiles, XP, competence ratings, and campaign ribbons/medals.

**Location:** `~/projects/generals/`
**Repository:** `https://github.com/petersimmons1972/generals.git`
**Profiles:** `profiles/*.md`

---

## Profile Updates After Deployment

### Service Record Section

Add deployment entry to commander's service record:

```markdown
## Service Record

### Deployment: [Project/Mission Name]
**Date:** YYYY-MM-DD
**Role:** [e.g., Backend Engineer, QA Lead, Documentation Specialist]
**Outcome:** [Success/Partial/Failure]

**Accomplishments:**
- [Specific achievement with impact]
- [What worked well and why]

**Challenges:**
- [What was difficult]
- [Root causes of failures]
- [Lessons learned]

**XP Earned:** +X XP
**Total XP:** Y/Z to next rank
```

---

## XP Calculation

**Base XP per deployment:**
- Simple task completion: 10 XP
- Standard deployment: 25 XP
- Complex multi-service deployment: 50 XP
- Emergency incident response: 75 XP

**Bonuses:**
- Zero defects: +10 XP
- Under time estimate: +5 XP
- Exceptional code quality: +15 XP
- Innovation/creative solution: +20 XP

**Penalties:**
- Critical bug shipped: -10 XP
- Missed deadline: -5 XP
- Requires rework: -10 XP

**XP to Rank:**
- ⭐ (1 star): 0-100 XP
- ⭐⭐ (2 stars): 100-250 XP
- ⭐⭐⭐ (3 stars): 250-500 XP
- ⭐⭐⭐⭐ (4 stars): 500-1000 XP
- ⭐⭐⭐⭐⭐ (5 stars): 1000+ XP

---

## Competence Progression

**Track deployments in specific roles:**

Each commander has competence ratings (1-5 stars) in different areas.

**Progression:** N/5 deployments toward next star

Example:
```markdown
**Competencies:**
- Backend Development: ⭐⭐⭐ (3/5 deployments to ⭐⭐⭐⭐)
- Testing: ⭐⭐ (1/5 deployments to ⭐⭐⭐)
- Documentation: ⭐⭐⭐⭐ (4/5 deployments to ⭐⭐⭐⭐⭐)
```

After each deployment, increment the deployment counter for relevant competencies.

---

## Campaign Ribbons and Medals

**Ribbons** - Awarded for participation/completion
**Medals** - Awarded for exceptional performance

### Common Ribbons

- 🎖️ **First Deployment** - Completed first assignment
- 🎖️ **Multi-Service Campaign** - Deployed 3+ services in one mission
- 🎖️ **Emergency Response** - Responded to production incident
- 🎖️ **Perfect Deployment** - Zero defects, all tests pass, on time

### Medal Criteria

- 🏅 **Bronze Star** - Exceptional technical achievement
- 🏅 **Silver Star** - Heroic debugging/problem solving
- 🏅 **Gold Star** - Strategic innovation that changes project direction
- 🏅 **Distinguished Service** - Sustained excellence over multiple deployments

**Award based on user feedback and objective outcomes.**

---

## Behavioral Observations

Document personality traits demonstrated during deployment:

**Consistency with profile:**
- Did behavior match expected personality?
- Any surprising responses or approaches?
- Evolution of communication style?

**Examples:**
- "Maintained methodical approach under pressure (consistent with profile)"
- "Showed creativity in solution design (emerging trait)"
- "Effective communication with teammates (improved from last deployment)"

**Purpose:** Track personality consistency and development over time

---

## Update Workflow

1. Read current profile: `cat ~/projects/generals/profiles/{commander}.md`
2. Add deployment to service record section
3. Calculate and add XP earned
4. Update total XP and check for rank increase
5. Increment competence deployment counters
6. Award ribbons/medals based on performance
7. Document behavioral observations
8. Commit and push changes
```

**Step 2: Verify file created**

Run: `cat ~/docs/GENERALS-INTEGRATION.md | head -3`
Expected: See "# Generals System Integration"

**Step 3: Commit**

```bash
cd ~
git add docs/GENERALS-INTEGRATION.md
git commit -m "docs: extract generals integration to reference doc

XP calculation methodology, competence progression
Ribbon/medal criteria, behavioral observations
Profile update workflow

Co-Authored-By: Claude Sonnet 4.5 <noreply@anthropic.com>"
```

---

## Task 4: Create DNS API Reference

**Files:**
- Create: `~/docs/DNS-API-REFERENCE.md`

**Step 1: Create DNS API reference doc**

Create file with content:

```markdown
# DNS API Reference

## Overview

You have direct API access to manage DNS records. **ALWAYS use APIs instead of asking user to do it manually.**

---

## 1. Unifi Controller (Internal DNS)

**Best for:** Internal network DNS (*.petersimmons.com, etc.)
**Endpoint:** `https://192.168.0.1/proxy/network/v2/api/site/default/static-dns`
**Credentials:** `~/.claude/.unifi-credentials`
**API Key:** `${UNIFI_API_KEY}`

### Add A Record

```bash
API_KEY="${UNIFI_API_KEY}"

curl -k -X POST "https://192.168.0.1/proxy/network/v2/api/site/default/static-dns" \
  -H "X-API-KEY: ${API_KEY}" \
  -H "Content-Type: application/json" \
  -d '{
    "enabled": true,
    "key": "subdomain.petersimmons.com",
    "record_type": "A",
    "value": "192.168.0.100"
  }'
```

### Add CNAME Record

```bash
curl -k -X POST "https://192.168.0.1/proxy/network/v2/api/site/default/static-dns" \
  -H "X-API-KEY: ${API_KEY}" \
  -H "Content-Type: application/json" \
  -d '{
    "enabled": true,
    "key": "subdomain.petersimmons.com",
    "record_type": "CNAME",
    "value": "target.petersimmons.com"
  }'
```

### List All DNS Records

```bash
curl -k -X GET "https://192.168.0.1/proxy/network/v2/api/site/default/static-dns" \
  -H "X-API-KEY: ${API_KEY}"
```

### Delete DNS Record

```bash
curl -k -X DELETE "https://192.168.0.1/proxy/network/v2/api/site/default/static-dns/{record_id}" \
  -H "X-API-KEY: ${API_KEY}"
```

**Advantages:**
- Instant propagation (no DNS TTL wait)
- Works immediately on internal network
- No external dependencies

---

## 2. Cloudflare DNS (Public Domains)

**Best for:** Public DNS records, external access
**Credentials:** `~/projects/kubernetes/cert-manager/.env`
**API Token:** `${CF_TOKEN}`

### Managed Zones

| Domain | Zone ID |
|--------|---------|
| clearwatchresearch.com | 684f1eed00746f0fc7f2b718b21d983c |
| clearwatchintelligence.com | 96a43875170aff855a6bf9a033d234f1 |
| clearwatch.io | 018e8292ffd3e2e501771617bf82ba71 |
| petersimmons.com | 460653bc26ff94fdc0910a13defa4afb |

### Add A Record

```bash
CF_TOKEN="${CF_TOKEN}"
ZONE_ID="460653bc26ff94fdc0910a13defa4afb"  # petersimmons.com

curl -X POST "https://api.cloudflare.com/client/v4/zones/${ZONE_ID}/dns_records" \
  -H "Authorization: Bearer ${CF_TOKEN}" \
  -H "Content-Type: application/json" \
  --data '{
    "type": "A",
    "name": "subdomain",
    "content": "192.168.0.100",
    "ttl": 1,
    "proxied": false
  }'
```

### Add CNAME Record

```bash
curl -X POST "https://api.cloudflare.com/client/v4/zones/${ZONE_ID}/dns_records" \
  -H "Authorization: Bearer ${CF_TOKEN}" \
  -H "Content-Type: application/json" \
  --data '{
    "type": "CNAME",
    "name": "subdomain",
    "content": "target.domain.com",
    "ttl": 1,
    "proxied": false
  }'
```

### List DNS Records

```bash
curl -X GET "https://api.cloudflare.com/client/v4/zones/${ZONE_ID}/dns_records" \
  -H "Authorization: Bearer ${CF_TOKEN}"
```

### Delete DNS Record

```bash
curl -X DELETE "https://api.cloudflare.com/client/v4/zones/${ZONE_ID}/dns_records/{record_id}" \
  -H "Authorization: Bearer ${CF_TOKEN}"
```

**Advantages:**
- Accessible both internally and externally
- Cloudflare's global DNS network
- Additional features (proxy, DDoS protection)

---

## 3. Pi-hole API (Monitoring Only)

**Credentials:** `~/.claude/pihole-api-credentials.md`
**Primary:** 192.168.0.231 (App Password: `gLS37GXVK1pG04BW5/GLvHUb+noHHHKI4ulI8zAIpTQ=`)
**Secondary:** 192.168.0.232

**Limitation:** Pi-hole API v6 does NOT support managing local DNS/CNAME records

**Use for:**
- Monitoring DNS queries
- Managing adblock lists
- Viewing statistics
- Checking DNS server health

**Note:** Local DNS management requires editing dnsmasq config files (needs SSH + sudo access)

---

## DNS Workflow Decision Tree

```
Need to add DNS record?
├─ Internal-only access?
│  └─ Use Unifi API (fastest, no propagation delay)
│
├─ Public domain access?
│  └─ Use Cloudflare API (accessible everywhere)
│
└─ Both internal and external?
   └─ Add to BOTH:
      1. Unifi (immediate internal access)
      2. Cloudflare (external + backup)
```

**NEVER ask user to manually add DNS records when you have API access.**

---

## Common Patterns

### Add Internal Service

```bash
# Unifi API for internal access
curl -k -X POST "https://192.168.0.1/proxy/network/v2/api/site/default/static-dns" \
  -H "X-API-KEY: ${UNIFI_API_KEY}" \
  -H "Content-Type: application/json" \
  -d '{
    "enabled": true,
    "key": "myservice.petersimmons.com",
    "record_type": "CNAME",
    "value": "traefik.petersimmons.com"
  }'
```

### Add Public Service

```bash
# Cloudflare API for external access
CF_TOKEN="${CF_TOKEN}"
ZONE_ID="460653bc26ff94fdc0910a13defa4afb"

curl -X POST "https://api.cloudflare.com/client/v4/zones/${ZONE_ID}/dns_records" \
  -H "Authorization: Bearer ${CF_TOKEN}" \
  -H "Content-Type: application/json" \
  --data '{
    "type": "CNAME",
    "name": "myservice",
    "content": "traefik.petersimmons.com",
    "ttl": 1,
    "proxied": false
  }'
```

### Add Both Internal and External

```bash
# Step 1: Unifi (internal)
curl -k -X POST "https://192.168.0.1/proxy/network/v2/api/site/default/static-dns" \
  -H "X-API-KEY: ${UNIFI_API_KEY}" \
  -H "Content-Type: application/json" \
  -d '{
    "enabled": true,
    "key": "app.petersimmons.com",
    "record_type": "CNAME",
    "value": "traefik.petersimmons.com"
  }'

# Step 2: Cloudflare (external)
curl -X POST "https://api.cloudflare.com/client/v4/zones/460653bc26ff94fdc0910a13defa4afb/dns_records" \
  -H "Authorization: Bearer ${CF_TOKEN}" \
  -H "Content-Type: application/json" \
  --data '{
    "type": "CNAME",
    "name": "app",
    "content": "traefik.petersimmons.com",
    "ttl": 1,
    "proxied": false
  }'
```
```

**Step 2: Verify file created**

Run: `cat ~/docs/DNS-API-REFERENCE.md | head -3`
Expected: See "# DNS API Reference"

**Step 3: Commit**

```bash
cd ~
git add docs/DNS-API-REFERENCE.md
git commit -m "docs: extract DNS API reference with detailed examples

Complete curl examples for Unifi, Cloudflare, Pi-hole
Decision tree for DNS workflow
Common patterns for internal/external/both

Co-Authored-By: Claude Sonnet 4.5 <noreply@anthropic.com>"
```

---

## Task 5: Update Global CLAUDE.md - Remove Extracted Sections

**Files:**
- Modify: `~/CLAUDE.md`

**Step 1: Remove Project-Specific Conventions section**

Delete lines 3-10 (entire "Project-Specific Conventions" section including header)

**Step 2: Remove Multi-Stage Verification section**

Delete lines 268-362 (entire "Multi-Stage Verification (CRITICAL)" section)

**Step 3: Remove Team Management detailed workflow**

Delete lines 166-190 (the "Required Workflow Before Team Shutdown" subsection)

Keep the section header "## Team Management & Self-Learning (CRITICAL)" but replace content with pointer (we'll do this in Task 8)

**Step 4: Remove Generals Integration section**

Delete lines 192-199 ("Generals Project Integration" subsection)

**Step 5: Remove Model Selection section**

Delete lines 201-203 ("Model Selection for Team Members" subsection)

**Step 6: Verify removals**

Run: `wc -l ~/CLAUDE.md`
Expected: Significantly fewer lines (should be around 250-270, down from 396)

Run: `grep -n "Project-Specific Conventions" ~/CLAUDE.md`
Expected: No results

Run: `grep -n "Multi-Stage Verification" ~/CLAUDE.md`
Expected: No results

**Step 7: Stage changes (don't commit yet)**

```bash
cd ~
git add CLAUDE.md
```

---

## Task 6: Update Global CLAUDE.md - Condense Web Search Section

**Files:**
- Modify: `~/CLAUDE.md:49-90`

**Step 1: Replace Web Search section**

Replace the entire "Web Search (CRITICAL)" section (lines 49-90) with:

```markdown
## Web Search

**PRIMARY:** SearXNG (`https://searxng.petersimmons.com/search?q={query}&format=json`)
**FALLBACK:** WebSearch tool (if SearXNG fails)

Quick Reference: `/search?q={query}&format=json&language=en`
K8s: `kubectl scale deployment searxng -n default --replicas=N`
```

**Step 2: Verify condensing**

Run: `grep -A 6 "## Web Search" ~/CLAUDE.md`
Expected: See only 8 lines (header + 6 content lines)

**Step 3: Stage changes**

```bash
cd ~
git add CLAUDE.md
```

---

## Task 7: Update Global CLAUDE.md - Condense DNS Management Section

**Files:**
- Modify: `~/CLAUDE.md:93-145`

**Step 1: Replace DNS Management section**

Replace the entire "DNS Management (CRITICAL)" section (lines 93-145) with:

```markdown
## DNS Management

**Use APIs - never ask user to do it manually**

| System | Use For | Credentials |
|--------|---------|-------------|
| Unifi API | Internal DNS (fastest) | `~/.claude/.unifi-credentials` |
| Cloudflare | Public domains | `~/projects/kubernetes/cert-manager/.env` |
| Pi-hole | Monitoring only | `~/.claude/pihole-api-credentials.md` |

**Workflow:** Internal-only → Unifi; Public → Cloudflare; Both → Add to both

Full examples: `docs/DNS-API-REFERENCE.md`
```

**Step 2: Verify condensing**

Run: `grep -A 12 "## DNS Management" ~/CLAUDE.md`
Expected: See only 15 lines total

**Step 3: Stage changes**

```bash
cd ~
git add CLAUDE.md
```

---

## Task 8: Update Global CLAUDE.md - Condense Team Management Section

**Files:**
- Modify: `~/CLAUDE.md:162-204`

**Step 1: Replace Team Management section**

Replace the entire "Team Management & Self-Learning (CRITICAL)" section with:

```markdown
## Team Management

**BEFORE shutting down teams:** Complete workflow in `docs/TEAM-MANAGEMENT-WORKFLOW.md`
- Document service records (accomplishments + failures + observations)
- Commit to GitHub with descriptive message
- Verify push succeeded
- THEN shutdown team directories

**Generals integration:** See `docs/GENERALS-INTEGRATION.md` for XP, ribbons, competence tracking

**Model selection:** Field Marshal assigns models case-by-case (Haiku/Sonnet/Opus)
```

**Step 2: Verify condensing**

Run: `grep -A 10 "## Team Management" ~/CLAUDE.md`
Expected: See condensed version with pointers to reference docs

**Step 3: Stage changes**

```bash
cd ~
git add CLAUDE.md
```

---

## Task 9: Verify Final Global CLAUDE.md

**Files:**
- Verify: `~/CLAUDE.md`

**Step 1: Check line count**

Run: `wc -l ~/CLAUDE.md`
Expected: ~250-270 lines (down from 396)

**Step 2: Verify structure intact**

Run: `grep "^## " ~/CLAUDE.md`
Expected: See all main section headers still present:
- Critical Rules
- Quick Reference
- Web Search
- DNS Management
- Triage
- Team Management
- Skills
- Indexes
- Decisions
- Infrastructure
- Quality
- Principles
- Collaboration Style
- Visual Output Preference
- Learning System

**Step 3: Verify no broken sections**

Run: `cat ~/CLAUDE.md`
Expected: Read through, verify all sections have content, no orphaned headers

**Step 4: Check for references to extracted content**

Run: `grep -i "docs/" ~/CLAUDE.md`
Expected: See references to:
- `docs/TEAM-MANAGEMENT-WORKFLOW.md`
- `docs/GENERALS-INTEGRATION.md`
- `docs/DNS-API-REFERENCE.md`

---

## Task 10: Test Context Loading

**Step 1: Test in home directory**

```bash
cd ~
# Check what CLAUDE.md is loaded (should be global only)
ls CLAUDE.md
```

Expected: Only global CLAUDE.md, no project-specific content

**Step 2: Test in security-intelligence-business project**

```bash
cd ~/projects/security-intelligence-business
# Check what CLAUDE.md files exist
ls CLAUDE.md
ls ~/CLAUDE.md
```

Expected: Both files exist - Claude Code should load both

**Step 3: Verify project-specific content**

Run: `cat ~/projects/security-intelligence-business/CLAUDE.md | head -5`
Expected: See project-specific conventions

**Step 4: Test in different project**

```bash
cd ~/projects/generals
ls CLAUDE.md
```

Expected: No CLAUDE.md in generals project, should only load global

---

## Task 11: Commit Global CLAUDE.md Changes

**Files:**
- Commit: `~/CLAUDE.md`

**Step 1: Review staged changes**

```bash
cd ~
git diff --staged CLAUDE.md
```

Expected: See removals of project-specific and verbose sections, condensed replacements

**Step 2: Commit optimized CLAUDE.md**

```bash
git commit -m "feat: optimize CLAUDE.md - 33% token reduction

REMOVED (moved to external references):
- Project-specific conventions → security-intelligence-business/CLAUDE.md
- Multi-stage verification → security-intelligence-business/CLAUDE.md
- Team management workflow → docs/TEAM-MANAGEMENT-WORKFLOW.md
- Generals integration → docs/GENERALS-INTEGRATION.md
- DNS API examples → docs/DNS-API-REFERENCE.md

CONDENSED:
- Web Search: 41 lines → 8 lines (keep endpoint + fallback)
- DNS Management: 52 lines → 15 lines (table + pointer)
- Team Management: 44 lines → 11 lines (checklist + pointers)

RESULT: 396 lines → ~260 lines (136 line reduction, 34% savings)

Co-Authored-By: Claude Sonnet 4.5 <noreply@anthropic.com>"
```

**Step 3: Verify commit**

Run: `git log --oneline -1`
Expected: See commit message about CLAUDE.md optimization

---

## Task 12: Final Verification and Measurement

**Step 1: Measure token reduction**

Count tokens in old vs. new:

```bash
# Approximate line count comparison
echo "Old: 396 lines"
wc -l ~/CLAUDE.md
```

Expected: ~250-270 lines

**Step 2: Verify all reference docs exist**

```bash
ls -lh ~/docs/TEAM-MANAGEMENT-WORKFLOW.md
ls -lh ~/docs/GENERALS-INTEGRATION.md
ls -lh ~/docs/DNS-API-REFERENCE.md
ls -lh ~/projects/security-intelligence-business/CLAUDE.md
```

Expected: All files exist

**Step 3: Verify git status clean**

```bash
cd ~
git status
cd ~/projects/security-intelligence-business
git status
```

Expected: Working trees clean, all changes committed

**Step 4: Create summary**

Document final metrics:
- Original size: 396 lines
- Optimized size: [actual line count]
- Reduction: [lines] ([percentage]%)
- Files created: 4 reference docs
- Project-specific CLAUDE.md: 1

---

## Success Criteria

- [ ] Global CLAUDE.md reduced by 30%+ (130+ lines)
- [ ] Project-specific CLAUDE.md created in security-intelligence-business
- [ ] 4 reference docs created (team management, generals, DNS, removed from global)
- [ ] All sections condensed appropriately with pointers
- [ ] All commits made with clear messages
- [ ] Context loading verified (global + project-specific when in project)
- [ ] No loss of critical information
- [ ] All reference docs accessible and complete
