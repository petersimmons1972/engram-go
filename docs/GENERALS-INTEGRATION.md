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
