---
name: analyzing-companies-for-interviews
description: "Use when preparing for interviews at a specific company, conducting due diligence, or assessing culture fit before accepting an offer. Triggers on company mentions in job search context, explicit commands, or when role-specific research is needed for interview preparation."
---

# Analyzing Companies for Interviews

## Overview

Structured company analysis for job search decisions requires systematic research across eight distinct domains:
1. **Financial & Legal** (10-Ks, litigation, management changes)
2. **Employee Sentiment** (Blind, Reddit, Glassdoor, role-specific reviews)
3. **Customer & Market** (G2, TrustRadius, case studies, churn patterns)
4. **Competitive Intelligence** (SWOT, positioning, compensation benchmarking)
5. **Analyst Coverage** (Gartner, Forrester, GigaOm, trend reports)
6. **News & Social** (Recent developments, layoffs, LinkedIn company page)
7. **Interview Preparation** (Talking points, questions to ask, common interview topics)
8. **Output Organization** (Proper markdown structure, Obsidian integration, version history)

Skipping any domain creates blind spots that cost candidates leverage in salary negotiation and acceptance decisions.

## When to Use

- **Explicit**: `/analyze-company [Name]` or "analyze [Company] for my interview"
- **Conversational**: User mentions company in job search context ("I'm interviewing at Acme Corp")
- **Update trigger**: Analysis exists but >30 days old or major news appeared

**Prerequisites**: Company name confirmed (apply intelligence: "Google" → "Alphabet Inc."), role specified (default: Senior Sales Engineer unless otherwise stated)

## Core Pattern: The Eight-Domain Research Framework

### Pre-Research
1. Confirm company name (catch aliases, parent companies, renamed entities)
2. Clarify role and geography if relevant
3. Check if analysis already exists in `/home/psimmons/projects/interview-tracking/companies/`
4. Show user what will be researched before starting (expensive operation)

### Domain 1: Financial & Legal (15 min)
- **Search**: "[Company] 10-K", "[Company] SEC filings", "[Company] litigation"
- **Extract**: Debt concerns, customer concentration, recent management changes, insider trading activity
- **Flag**: Unusual items only (not boilerplate disclosure)

### Domain 2: Employee Sentiment (25 min) — HIGHEST PRIORITY
- **Sources**: Glassdoor (deep reading, not just top reviews), **Blind.com** (anonymous, more honest), Reddit (r/[company], r/cscareerquestions), role-specific job boards
- **Extract**: Recent reviews (90-120 days priority), broader sentiment patterns, culture red flags
- **Focus**: Role-specific feedback (e.g., "Solutions Engineer at [Company]" experiences)
- **Include**: Atlanta/geography-specific patterns if relevant

### Domain 3: Customer & Market (20 min)
- **Sources**: G2, TrustRadius, PeerSpot, vendor review sites, customer case studies
- **Extract**: Customer satisfaction patterns, complaint themes, churn signals
- **Skip**: Competitor reviews (Domain 4)

### Domain 4: Competitive Intelligence (20 min)
- **Identify**: Top 3-5 competitors per major business line
- **Create**: SWOT analysis (Strengths, Weaknesses, Opportunities, Threats vs each competitor)
- **Search**: "[Company] vs [Competitor]" comparisons, competitor positioning
- **Benchmark**: Solutions Engineer compensation in Atlanta (vs competitors)

### Domain 5: Analyst Coverage (10 min)
- **Search**: Gartner Magic Quadrant, Forrester Wave, GigaOm reports, analyst trend reports
- **Extract**: Market positioning, recognition, strategic direction

### Domain 6: News & Social (15 min)
- **Search**: "[Company] news" (90-120 days), LinkedIn company page, Twitter/X updates, layoff announcements
- **Flag**: Recent controversies, hiring freezes, product launches, customer complaints
- **Separate**: Dedicated section for negative information (transparency required)

### Domain 7: Interview Preparation (15 min)
- **Generate**:
  - "Why do you want to work here?" talking points (connect your experience to company problems)
  - 5-10 strong questions to ask the interviewer (shows domain knowledge, tests culture fit)
  - Common interview questions about company (product strategy, culture, recent news)
- **Customize**: By role and your background

### Domain 8: Output & Organization (10 min)
- **Create**: `/home/psimmons/projects/interview-tracking/companies/[company-name].md`
- **Include**: YAML frontmatter (tags, date, role, company name), clear section headers, update history
- **Format**: Obsidian-compatible (markdown, tags for linking)
- **Track**: Version history and deltas when refreshing

## Implementation Strategy

### Sequential vs Parallel
- **Quick turnaround** (<15 min): Run domains 2, 5, 7 in sequence (critical path)
- **Deep analysis** (45+ min): Spawn parallel background agents for domains 1-6, consolidate results, then handle domain 7-8

### Confirmation Checklist
Before starting, confirm with user:
```
I'll analyze [Company Name] for your [Role] interview:
- Financial & legal risks (10-K, litigation)
- Employee sentiment (Blind, Glassdoor, Reddit)
- Customer satisfaction & reviews
- Competitive positioning & compensation
- Analyst coverage (Gartner, Forrester, GigaOm)
- Recent news & social sentiment
- Interview talking points & questions
- Organized in: /home/psimmons/projects/interview-tracking/companies/

Ready to start? (This takes 20-45 minutes)
```

### Handling Existing Analyses
If analysis exists:
1. Show user the existing file (date and key findings)
2. Ask: "Refresh all sections, or just update with recent news?"
3. Create delta section showing what changed (new reviews, news, management changes)
4. Preserve original analysis with update timestamp

## Common Mistakes

| Rationalization | Reality | Fix |
|-----------------|---------|-----|
| "Basic web search is enough" | Misses 80% of critical interview data | Requires 8-domain systematic approach |
| "Glassdoor covers employee sentiment" | Blind & Reddit have more honest feedback | Search all three + role-specific sources |
| "Found competitors" | Didn't analyze positioning or create SWOT | Build formal competitive matrix |
| "Salary data is generic" | Need role-specific + location + competitor comparison | Benchmark against Atlanta Sales Engineers at 3+ competitors |
| "Sequential searches are fine" | Takes 45 min; parallel agents save 20+ min | Spawn 5-10 background agents for independent domains |
| "Manual organization is acceptable" | Creates friction for future updates & Obsidian linking | Auto-generate structured markdown with metadata |
| "I'll just do quick research" | Interview prep requires DEEP research | Allocate 30-45 minutes, not 10 |
| "10-K is overkill for interview" | Unusual items reveal risks interviewers won't mention | Extract legal, debt, management changes only |
| "Don't need negative information" | All companies have problems; omitting them creates bias | Separate "Concerns" section with honest assessment |
| "One analysis is forever" | Companies change (new CEO, layoffs, pivot); sentiment ages poorly | Refresh if >30 days old or major news appeared |

## Keywords

company analysis, interview preparation, due diligence, cultural fit, competitive intelligence, employee sentiment, financial health, market research, salary negotiation, job search

