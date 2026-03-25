---
name: analyzing-companies
description: "Use when preparing for interviews at a specific company, conducting due diligence, or assessing culture fit. Triggers on company mentions in job search context, explicit commands like /analyze-company, or when role-specific research is needed. Also covers storing results in the job-search PostgreSQL database and generating an HTML interview brief."
---

# Analyzing Companies for Interviews

## Overview

Full company analysis for interview prep requires three outputs:
1. **Research** — 8 domains of information gathered via parallel agents
2. **Database** — results stored in job-search-system PostgreSQL
3. **HTML Brief** — dark mode, portrait-monitor-optimized interview reference

Skipping any domain creates blind spots that cost leverage in negotiation and acceptance decisions.

## When to Use

- **Explicit**: `/analyze-company [Name]` or "analyze [Company] for my interview"
- **Conversational**: Company mentioned in job search context ("I'm interviewing at Acme Corp")
- **Update trigger**: Analysis exists but >30 days old or major news appeared

**Prerequisites**: Company name confirmed (apply intelligence: "Google" → "Alphabet Inc."), role specified (default: Senior Sales Engineer unless otherwise stated)

## Phase 1: Research — The Eight Domains

### Parallel Dispatch (proven pattern)

Spawn 3 agents simultaneously:

**Agent 1 — Company fundamentals:**
- What does it do, what problem does it solve
- Product catalog with pricing tiers
- Funding history (rounds, amounts, investors, valuation)
- Founded when/by whom, HQ, employee count
- Key differentiators, core technology claims
- Recent news (last 6 months)
- Top 5–7 direct competitors

**Agent 2 — Employee & analyst sentiment:**
- Glassdoor (all sub-scores, look for solicitation patterns)
- Blind.com (WLB score especially — more honest than Glassdoor)
- Reddit (r/[company], r/cscareerquestions)
- LinkedIn headcount trend
- Gartner Magic Quadrant / Forrester Wave placement (or absence)
- G2, TrustRadius customer reviews
- Layoffs, controversies, negative press
- Leadership team bios

**Agent 3 — Existing DB check:**
- `SELECT id, name, updated_at FROM companies WHERE name ILIKE '%[Company]%';`
- `SELECT dossier_section, created_at FROM research_artifacts WHERE company_id = <id>;`
- Report: does company exist? Are artifacts fresh (<30 days)?

### Domain Coverage Checklist

| Domain | Sources | Extract |
|--------|---------|---------|
| Financial & Legal | 10-K, SEC, litigation search | Debt, customer concentration, management changes |
| Employee Sentiment | Glassdoor + **Blind** (Blind is more honest) + Reddit | WLB score, culture red flags, role-specific feedback |
| Customer & Market | G2, TrustRadius, PeerSpot | Satisfaction patterns, complaint themes, churn signals |
| Competitive Intel | "[Company] vs [Competitor]" comparisons | SWOT per competitor, positioning gaps |
| Analyst Coverage | Gartner MQ, Forrester Wave, GigaOm | Placement or notable absence |
| News & Social | 90–120 day news sweep, LinkedIn | Layoffs, pivots, product launches, controversies |
| Interview Prep | Synthesize from all above | Talking points, 8 questions to ask, red flags to probe |
| Output | DB + HTML brief | See Phases 2 & 3 below |

## Phase 2: Database Storage

### Finding Credentials

**⚠️ No .env files exist. Credentials are in Kubernetes.**

```bash
# Get DB credentials from K8s secret
kubectl get secret postgres-secret -n job-search -o jsonpath='{.data.DATABASE_URL}' | base64 -d

# Port-forward (required — postgres-service is ClusterIP only)
kubectl port-forward svc/postgres-service 15432:5432 -n job-search &

# Connect
psql "postgresql://jobsearch_user:<password>@127.0.0.1:15432/jobsearch"
```

### Schema Facts (do not guess)

**`companies` table** — key columns: `id, name, industry, size, website, headquarters, description, created_at, updated_at`
- No unique constraint on `name` — use SELECT check before INSERT, then UPDATE if found

**`research_artifacts` table** — key columns: `id, company_id, artifact_type, content, metadata_json (JSONB), created_by, created_at`
- `artifact_type` has a **CHECK constraint** — only these values are valid: `swot`, `competitive`, `linkedin`, `salary`, `market`, `technical`, `other`
- Store dossier section identity in `metadata_json->>'dossier_section'`

### Artifact Type Mapping

| Dossier Section | artifact_type to use |
|-----------------|----------------------|
| company_overview | `other` |
| products_and_services | `other` |
| market_position | `market` |
| competitive_landscape | `competitive` |
| swot_analysis | `swot` |
| recent_news | `other` |
| interview_angle | `other` |

### Insert Pattern

```sql
-- 1. Upsert company (check first, no ON CONFLICT available)
SELECT id FROM companies WHERE name = 'Company Name';
-- If not found:
INSERT INTO companies (name, industry, size, website, headquarters, description)
VALUES (...) RETURNING id;
-- If found: UPDATE companies SET description=..., updated_at=NOW() WHERE id=<id>;

-- 2. Insert each section
INSERT INTO research_artifacts (company_id, artifact_type, content, metadata_json, created_by)
VALUES (
  <company_id>,
  'other',  -- use correct type from mapping above
  'Section title',
  '{"dossier_section": "company_overview", "data": {...}}',
  'manual-research-agent'
);
```

### Verify

```sql
SELECT id, name, updated_at FROM companies WHERE name = 'Company Name';
SELECT artifact_type, metadata_json->>'dossier_section' AS section, created_at
FROM research_artifacts WHERE company_id = <id>
ORDER BY created_at;
-- Expect 7 rows
```

## Phase 3: HTML Brief Generation

### Display Target

Portrait 2K monitor rotated 90°: **1000px wide** (user preference — fits portrait viewport without horizontal scroll).

### Required Design

- **Color scheme: dark mode** — user preference. Use `#0F1117` background, `#00D8B4` teal accent, white body text
- **Zero external dependencies** except Google Fonts CDN
- **No JavaScript** — pure HTML + inline CSS + inline SVG charts
- **Output location**: `~/[company-name]-interview-brochure.html`
- Open with: `xdg-open ~/[company-name]-interview-brochure.html`

### Required Sections (in order)

1. **Masthead** — company name, "Interview Intelligence Brief", date, company vitals bar (valuation, employees, raised, founded, HQ)
2. **Lead Story** — founding thesis, what problem they solve, pull quote from CEO (magazine callout style)
3. **Product Lineup** — card grid, flagship product highlighted with accent border
4. **Market Position** — SVG positioning map (AI maturity × platform breadth axes), market size table
5. **Competitor Matrix** — 7 rows × 6 columns table with 🟢🟡🔴 rating dots
6. **SWOT Analysis** — 2×2 colored grid (green/red/blue/amber), white text in each quadrant
7. **Employee Pulse** — Glassdoor vs Blind score cards; always include review solicitation warning if Glassdoor >4.0 and Blind materially lower
8. **Customer Voice** — G2/TrustRadius rating + SVG horizontal bar chart of satisfaction dimensions
9. **Interview Prep** — "Why [Company]?" talking points + numbered questions to ask + red flag cards
10. **Footer** — sources bar

### CSS Skeleton

```css
:root {
  --bg: #0F1117;
  --surface: #1A1D27;
  --teal: #00D8B4;
  --text: #E8E8F0;
  --muted: #9CA3AF;
}
body { width: 1000px; margin: 0 auto; background: var(--bg); color: var(--text); }
```

## Common Mistakes

| Rationalization | Reality | Fix |
|-----------------|---------|-----|
| "Glassdoor 4.0+ means good culture" | Glassdoor scores are systematically inflated by new-hire solicitation; Blind WLB is more reliable | Always compare Glassdoor vs Blind side-by-side |
| "I'll find DB credentials in .env" | No .env files — credentials are in K8s secret `postgres-secret` in `job-search` namespace | Use kubectl port-forward pattern above |
| "I can use any artifact_type" | CHECK constraint rejects unknown values silently or with error | Use the mapping table above |
| "ON CONFLICT works on companies.name" | No unique constraint on name — upsert will fail | SELECT first, then INSERT or UPDATE |
| "960px or 1440px for portrait monitor" | User preference is 1000px | Always use 1000px width |
| "Light mode is fine" | User prefers dark mode for briefs | Always use dark `#0F1117` background |
| "Sequential agent research is fine" | Takes 3× longer unnecessarily | Spawn 3 parallel research agents as described in Phase 1 |
| "No analyst quadrant = not worth noting" | Absence from Gartner/Forrester is meaningful signal about maturity | Explicitly call out if company is not yet in any quadrant |

## Keywords

company analysis, interview preparation, job search, due diligence, Glassdoor, Blind, G2, SWOT, competitive intelligence, job-search-system, research_artifacts, postgres, dossier, HTML brief, portrait monitor, dark mode, Gartner, Forrester, K8s, kubectl
