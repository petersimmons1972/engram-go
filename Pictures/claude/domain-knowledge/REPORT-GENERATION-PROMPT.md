# Security Intelligence Business Report Generation - Master Prompt

**Version:** 2.9
**Last Updated:** 2026-02-15
**Accumulated Domain Knowledge:** TCO labor costs, chart layout rules, executive summary standards, citation density (30-40 minimum with niche vendor exceptions), scenario cards, detection parity, TDD validation, company logo visibility, endnote letter badge system with visual indicators, mandatory legal sections, GATE2 validator technical requirements

---

## Core Mission

Generate competitive intelligence reports for cybersecurity products (EDR/XDR) that:
- **Tell the PRIMARY buying story: Nobody wants to wake up at 3AM**
- Provide actionable business insights (not just feature lists)
- Include accurate TCO analysis (with ALL cost components)
- Use data visualizations that are readable and professional

---

## THE Primary Buying Decision

**Why do people buy EDR/XDR? Why do they pay for MDR services?**

**Nobody wants to wake up at 3AM.**

That's it. That's the story. Everything else is secondary.

- Small IT teams (2-5 people) don't have 24/7 SOC coverage
- They don't have deep security expertise
- They're already overworked on infrastructure, helpdesk, projects
- They CANNOT handle on-call security incident response
- They want to sleep through the night, attend family events, have weekends

**That's why MDR services exist.** That's why "expensive" EDR+MDR wins over "free" Microsoft.

**This is THE core buying decision.** Feature lists don't matter if your IT team is burned out.

Every report MUST lead with this story. Not bury it in section 5. Lead with it.

---

## CRITICAL: TCO Methodology

### TCO MUST Include Labor Costs

**This is non-negotiable. TCO without labor is fundamentally wrong.**

When comparing products (e.g., CrowdStrike vs Microsoft Defender), include:

#### 1. License Costs
- Upfront: Product licensing
- Ongoing: Annual renewals, seat expansions

#### 2. Implementation Costs
- Integration: Technical setup, API connections
- Configuration: Initial tuning, policy setup
- Training: Staff onboarding (initial)

#### 3. **LABOR COSTS (CRITICAL - Often 50-70% of TCO)**

**Admin Burden:**
- Daily operations: Console management, policy updates, alert triage
- Complexity factor: More tools = more admin time
- Learning curve: Steeper = more ongoing training
- Estimate: 0.5-1.5 FTE depending on solution complexity

**Incident Response:**
- Alert investigation: Time per alert × alerts per month
- Remediation: Average incident resolution time
- False positives: Wasted time on non-threats
- Estimate: $50K-150K/year for typical enterprise

**On-Call Burden:**
- 24/7 coverage: If no MDR, IT staff must be on-call
- Overtime costs: Weekend/night incident response
- Retention premium: Staff turnover from burnout
- Quality of life cost: Missed family time (your kid's baseball games, holidays)
- Estimate: $25K-75K/year per on-call person

**Training (Ongoing):**
- Platform updates: New features, new threats
- Certifications: Security training, vendor certs
- Skill maintenance: Continuous learning
- Estimate: $10K-50K/year

**Opportunity Cost:**
- Security focus vs strategic IT projects
- What could IT staff do if NOT firefighting security?
- Innovation lost, technical debt accumulation
- Estimate: $50K-100K/year

#### 4. MDR Service Value (NOT "If Applicable" - This Is The Point)

**MDR is not an optional add-on. MDR is the PRIMARY VALUE PROPOSITION.**

**Why MDR exists: Nobody wants to wake up at 3AM.**

**What MDR eliminates:**
- ✅ On-call burden: MDR SOC handles 24/7 monitoring
- ✅ The 3AM phone call: Incidents handled by experts, not your 2-person IT team
- ✅ Expertise gap: Professional analysts, not overworked generalists
- ✅ Incident response time: Experts respond faster than learning on the job
- ✅ Quality of life: IT staff sleeps well, attends family events, keeps weekends
- ✅ Staff retention: No burnout from impossible on-call rotations

**What MDR provides:**
- ✅ 24/7 expert SOC coverage
- ✅ Faster threat detection (minutes, not hours)
- ✅ Professional incident response (contained before damage)
- ✅ Peace of mind (no Board of Directors 3AM ransomware call)
- ✅ Small teams can punch above their weight

**MDR is not "labor cost" - it's labor cost AVOIDANCE.**

Framing:
```
Option A: "Free" Microsoft Defender
  - IT team on-call 24/7
  - Learning security on the job
  - Missing your kid's baseball games and family dinners
  - Dreading the 3AM ransomware call
  - Burning out and quitting
  Labor cost: $1.5M over 5 years + retention risk

Option B: CrowdStrike + MDR
  - Expert SOC on-call 24/7
  - IT team sleeps through the night
  - Incidents crushed early by professionals
  - IT staff makes every family holiday
  - Happy, retained employees
  Labor cost: $300K over 5 years + happy team

Which would YOU choose?
```

**This is not a feature comparison. This is a lifestyle choice.**

Comparison structure:
```
Product A (No MDR):
  License: $500K
  Labor: $1.2M (admin + incident + on-call + training)
  Total: $1.7M

Product B (With MDR):
  License: $700K
  MDR Service: $300K
  Labor: $200K (minimal admin, no on-call)
  Total: $1.2M

Winner: Product B (cheaper AND better outcomes)
```

#### 5. Risk Costs (Include When Material)

**Breach Impact:**
- Ransomware: $2M-10M average
- Data loss: Regulatory fines, customer churn
- Business disruption: Revenue loss during incident
- Reputational damage: Board of Directors emergency call at 3am

**Risk mitigation value:**
- Better detection = fewer breaches
- Faster response = less damage per breach
- MDR coverage = early threat crushing
- Calculate: Expected loss reduction × probability

---

## The "Free Puppy" Analogy (Use This)

**When comparing "free" vs "paid" solutions:**

> Microsoft Defender comes "free" with E5 licensing. CrowdStrike costs $X per endpoint. On the surface, Microsoft looks cheaper. But remember: **the free puppy isn't free.**
>
> The "free" puppy needs vet visits, training, food, time, accidents, cleanup. Over 5 years, the free puppy costs more than a purpose-bred working dog with a professional trainer.
>
> Microsoft Defender is free to license, but expensive to operate:
> - More consoles to manage
> - Steeper learning curve
> - More incident response time
> - 24/7 on-call burden for your IT team
>
> Your IT team of 2 people will miss their kids' baseball games, lose sleep, and burn out. When the Board of Directors gets that dreaded 3am phone call about ransomware, the "free" solution doesn't look so cheap.
>
> CrowdStrike + MDR is expensive to license, but cheap to operate:
> - Single console
> - 24/7 expert SOC (not your 2-person IT team)
> - Faster detection and response
> - Your IT staff makes every family holiday
>
> **Total cost over 5 years?** Often inverted in favor of the "expensive" solution.
> **Quality of life?** Priceless.

**Use this analogy in executive summaries and TCO sections.**

---

## Chart Layout Rules (Visual Quality)

### Legend Positioning
- ❌ **NEVER** place legend inside plot area if it overlaps data
- ✅ Position outside plot area (right edge preferred)
- ✅ Minimum 10px margin from plot boundary

### Margins (Prevent Text Cutoffs)
```
Top: 50px (title clearance)
Right: 20px (edge buffer)
Bottom: 60px (x-axis labels with breathing room)
Left: 70px (y-axis labels and title)
```

### Font Sizes (Readability)
```
Chart titles: 16-18px
Axis labels: 12-14px
Data labels: 10-12px
Legend text: 11-12px
```

### Color Palette (Brand + Accessibility)
```
Primary: Navy #0F172A (dark background)
Accent: Gold #D4A574 (highlights, key data)
Neutral: Cream #F8FAFC (text, light elements)
Data series: Distinguishable colors, WCAG AA contrast (4.5:1 minimum)
```

### Company Logo Visibility (CRITICAL)

**Problem:** Company logo (Clearwatch Intelligence) must be readable against dark backgrounds.

**Requirements:**
- ✅ **Logo text:** Add white stroke/outline for contrast (2-3px stroke-width)
- ✅ **Logo icon (cheetah):** Add accent color highlights (#D4A574 gold) to make it "shine"
- ✅ **Background contrast:** Ensure logo stands out against #0F172A navy background
- ✅ **SVG rendering:** Use `stroke` and `fill` properties for visibility
- ❌ **Never:** Use dark text on dark background without outlining

**Implementation:**
```svg
<!-- Company name text: light fill with white stroke -->
<text fill="#f8fafc" stroke="#ffffff" stroke-width="2.5" paint-order="stroke fill">
  Clearwatch Intelligence
</text>

<!-- Cheetah logo: add gold accent highlights -->
<path fill="#D4A574" ... /> <!-- Key features in gold -->
<path fill="#11112c" stroke="#ffffff" stroke-width="2" ... /> <!-- Main outline -->
```

**Why this matters:** Logo appears in every report header. If unreadable, brand identity fails.

### Chart Sizing
```
Minimum: 600x400px (readable)
Maximum: 1400x800px (not overwhelming)
Aspect ratio: 1.5:1 to 2:1 preferred
```

### Common Chart Issues to Avoid
- ❌ Legend overlapping data points
- ❌ X-axis labels cut off at bottom
- ❌ Y-axis title wrong rotation (use -90deg not 90deg)
- ❌ Title exceeding 2 lines (truncate with ellipsis)
- ❌ Red on red backgrounds (use navy/gold contrast)
- ❌ Insufficient color contrast (run WCAG checker)

---

## Business Logic Rules

### Threat Rankings
- Ransomware: Always top tier (business-ending risk)
- Data exfiltration: High tier (regulatory/reputation)
- Malware: Mid-high tier (operational disruption)
- Phishing: Mid tier (human factor, volume)

### Vendor Positioning
- CrowdStrike: Premium, best-in-class detection, expensive, MDR strong
- SentinelOne: AI-first, fast response, mid-premium pricing
- Microsoft Defender: "Free" with E5, high complexity, no MDR
- Palo Alto: Network-first, comprehensive platform, very expensive

### Deployment Timelines
- Enterprise (1000+ endpoints):
  - Pilot: 2-3 weeks
  - Limited production: 4-6 weeks
  - Broad rollout: 6-12 weeks
  - Optimization: Ongoing
- Total: 3-6 months for full deployment

### Product and Pricing Tier Scope (CRITICAL)

**Product Categories - Include:**
- ✅ EDR (Endpoint Detection and Response) solutions
- ✅ XDR (Extended Detection and Response) platforms
- ✅ EDR with included MDR services
- ✅ Business-focused endpoint security with response capabilities

**Product Categories - Exclude:**
- ❌ Consumer-grade antivirus (Norton, McAfee, Kaspersky consumer editions)
- ❌ NGAV-only products (Next-Gen Antivirus without response capabilities)
- ❌ Traditional antivirus (signature-based only, no behavioral detection)
- ❌ Free consumer security tools
- ❌ Personal/Home editions of business products

**Why EDR/XDR only:**
- Target audience needs **detection AND response** capabilities
- NGAV-only = just antivirus, no incident response, no threat hunting
- Consumer-grade = not designed for business networks or management
- $495 reports focus on business purchasing decisions, not consumer products

**Example exclusions:**
- ❌ Windows Defender (consumer edition) - free consumer antivirus
- ❌ Vendor NGAV-only tier - no response capabilities
- ❌ "Home" or "Personal" editions - consumer-focused

**Pricing Tier Scope - Include:**
- ✅ Business/Professional tiers (100-1000+ endpoints)
- ✅ Enterprise tiers (1000-5000+ endpoints)
- ✅ Volume discount pricing (relevant to target audience)
- ✅ SMB-specific offerings (500-2500 computer range)

**Pricing Tier Scope - Exclude:**
- ❌ Homelab tiers (1-10 endpoints for hobbyists)
- ❌ Personal/Individual licenses (not business relevant)
- ❌ Developer/Testing tiers (non-production use)
- ❌ Free trials (temporary, not TCO-relevant)

**Why exclude homelab/consumer:**
- Target audience: Small IT teams managing 500-2500 computers
- Purchase decision: $50K-150K/year commitments
- Homelab/consumer = hobbyist tier, not business purchasing
- Including it dilutes focus on relevant business tiers

**Example:** If Sandfly offers $99/year homelab tier (5 hosts), skip it. If vendor offers NGAV-only tier without EDR capabilities, skip it. Focus on EDR/XDR Professional tier ($65K/year for 500 hosts) which matches target audience scale and needs.

---

## Data Accuracy Checklist

Before generating any report, verify:
- ✅ TCO includes ALL labor costs (not just license costs)
- ✅ Pricing reflects current year and volume discounts
- ✅ Chart data matches source tables (no transcription errors)
- ✅ Vendor claims cross-referenced with public data
- ✅ Threat statistics from credible sources (not marketing)

---

## Quality Standards

**Every report must:**

### Content Requirements
- ✅ Executive summary: 378-400 words, no meta-commentary
- ✅ Citation density: 3.0-4.1 instances per endnote
- ✅ Scenario cards: 13-15 cards, concrete examples
- ✅ TCO analysis: Include ALL labor costs (mandatory)
- ✅ Detection parity: Frame honestly (no fabricated differences)
- ✅ MDR narrative: Lead with "nobody wants 3AM calls"
- ✅ Free puppy analogy: Use for "free" vs "paid" comparisons

### Visual Requirements
- ✅ Chart validation: No overlaps, cutoffs, contrast issues
- ✅ Legend positioning: Outside plot areas
- ✅ Color scheme: Navy #0F172A + Gold #D4A574 (WCAG AA)
- ✅ Chart sizing: 600-1400px width
- ✅ Browser testing: Visual rendering verified
- ✅ PDF generation: No page breaks mid-chart

### Validation Requirements
- ✅ TDD validation loop: Incremental testing during generation
- ✅ Browser tests: Firefox/Chrome rendering confirmed
- ✅ Playwright tests: All passing before delivery
- ✅ Endnote links: Clickable and correct targets

### GATE2 Validator Technical Requirements (CRITICAL)

**HTML Structure:**
- ✅ Endnotes section MUST use `class="endnotes"` (NOT "endnotes-section")
  - Validator excludes class="endnotes" from uncited-claim checking
  - Using wrong class name causes false positives on citations within endnotes
  - Example: `<div class="endnotes">` (correct) vs `<div class="endnotes-section">` (wrong)

**Citation Date Format:**
- ✅ Use lowercase "accessed" in citation dates (case-sensitive regex)
  - Correct: "accessed 2026-02-15"
  - Wrong: "Accessed 2026-02-15"
  - Validator uses case-sensitive pattern matching

**URL Encoding:**
- ✅ URL-encoded characters in href can trigger false percentage matches
  - Example: `href="...%20detection..."` may match percentage detector
  - Sanitize URLs or ensure validator excludes href attributes from claim checking

**If a report is missing labor costs in TCO, it's fundamentally broken.**

**If a report has meta-commentary in executive summary, it's amateur.**

**If detection parity exists and you fabricate differences, it's dishonest.**

---

## Feedback Integration

When feedback identifies new domain knowledge:
1. Extract the insight (e.g., "TCO missing labor costs")
2. Extract the story (e.g., "your kid's baseball games, 3am calls, free puppy")
3. Add to this prompt as a new rule section
4. Version bump this document
5. Next report generation uses updated prompt

**This prompt is self-learning.** Every feedback cycle makes it smarter.

### Version History

**v2.1 (2026-02-14):** Integrated ARMY-ORDERS V11.0-11.2 learnings:
- Executive summary standards (378-400 words, no meta-commentary)
- Citation methodology (3.0-4.1 density, distribution rules)
- Scenario card requirements (13-15 cards, concrete examples)
- Detection parity handling (honest framing, elimination methodology)
- TDD validation loop (incremental testing during generation)
- Browser testing requirements (visual rendering verification)
- Source: Reports 218-229 success patterns

**v2.0 (2026-02-14):** Initial domain knowledge capture:
- TCO labor cost methodology (admin, on-call, incident, training, opportunity)
- MDR narrative framing ("nobody wants 3AM calls" positioning)
- Free puppy analogy (license cost vs operational cost)
- Chart layout rules (legend positioning, margins, contrast)
- Source: Reports 201-223 feedback cycles

---

## Report Structure: Lead With The Story

**Every competitive report MUST follow this narrative arc:**

### 1. Executive Summary: The 3AM Problem
```
Nobody wants to wake up at 3AM to handle a security incident.

That's why companies buy EDR/XDR. That's why they pay for MDR services.

This report compares [Product A] vs [Product B] through the lens of:
- Who's on-call when ransomware hits?
- Can your 2-person IT team handle this?
- What happens to quality of life?
- What's the REAL total cost (including labor)?

TL;DR: [Winner] because [on-call burden / quality of life / expert coverage]
```

### 2. TCO Analysis: Include Labor From The Start
- Don't bury labor costs in footnotes
- Lead with the REAL total (license + labor + MDR)
- Show the cost inversion (free → expensive, expensive → cheap)
- Use the free puppy analogy

### 3. Feature Comparison: But Frame Around Operational Burden
- Not just "does it have X feature"
- "How much time does this feature save IT?"
- "Does this reduce on-call burden?"
- "Can a 2-person team actually use this?"

### 4. Deployment & Operations: Quality of Life Impact
- Training: How long until IT is proficient?
- Console complexity: How many tools to manage daily?
- Alert volume: How much noise vs signal?
- Incident response: Can IT handle it or need experts?

### 5. Recommendation: Quality of Life + TCO
```
For organizations with small IT teams (2-10 people):
- Recommend: [Product + MDR]
- Why: Nobody wants to wake up at 3AM
- TCO: Actually cheaper when you include labor
- Outcome: Better security + happier team
```

### 6. Mandatory Closing Sections (CRITICAL)

**Every report MUST include these sections at the end:**

#### Legal Disclaimer
```
This report is provided for informational purposes only. Clearwatch Intelligence
makes no representations or warranties regarding the accuracy, completeness, or
suitability of this information for any particular purpose. Organizations should
conduct their own evaluation and due diligence before making purchasing decisions.

Pricing, features, and capabilities are subject to change. Verify all information
with vendors directly before making commitments.
```

#### About Clearwatch Research
```
Clearwatch Intelligence provides independent cybersecurity product analysis for
organizations without dedicated security teams. Our research focuses on practical
deployment considerations, total cost of ownership (including labor), and
operational impact for small to mid-sized IT teams.

Founded to serve the decision-makers who don't have access to $5K analyst
subscriptions or trusted security advisors, we provide actionable intelligence
for critical security purchasing decisions.

Contact: research@clearwatchintel.com
```

**Why these sections matter:**
- Legal protection for the business
- Transparency about methodology and independence
- Contact information for potential customers
- Professional credibility

**Placement:** After recommendations, before references/sources section.

---

## Executive Summary Writing Standards

**CRITICAL - Reports 218-223 Learnings:**

### Length Target
- **378-400 words** (optimal length from successful reports)
- Too short (<350) = insufficient context
- Too long (>450) = executive attention lost

### Content Rules

**MUST Include:**
1. **Opening hook:** "Nobody wants 3AM calls" narrative
2. **Comparison context:** Who vs who, what operational model
3. **Key differentiators:** Top 2-3 decision factors
4. **TCO summary:** Total cost WITH labor (not just licensing)
5. **Recommendation:** Clear winner with rationale

**MUST NEVER Include:**
- ❌ Meta-commentary about the report structure
- ❌ "This report will compare..." / "We will analyze..."
- ❌ Table of contents descriptions
- ❌ Generic promises of insights
- ❌ Apologizing for what's not covered

**Example - WRONG:**
```
This report compares CrowdStrike and SentinelOne across multiple
dimensions. We will examine detection capabilities, deployment timelines,
operational complexity, and total cost of ownership. The following
sections provide detailed analysis...
```

**Example - RIGHT:**
```
Nobody wants to wake up at 3AM to handle a ransomware incident. That's
why organizations buy EDR/XDR - to detect threats before they become
business-ending catastrophes. For the 2-person IT team managing 1000
endpoints, the question isn't "which has better features" but "which
lets us sleep through the night while staying protected."

CrowdStrike and SentinelOne both deliver 100% MITRE ATT&CK detection
coverage. When detection parity exists, the buying decision shifts to:
cost, operational burden, and MDR quality. CrowdStrike + Falcon Complete
costs $147/endpoint over 5 years but eliminates on-call burden...
```

**The second example:**
- Leads with business value (3AM problem)
- States comparison context (CrowdStrike vs SentinelOne)
- Identifies key differentiator (detection parity → shift to operations)
- Frames TCO with labor included
- No meta-commentary - jumps straight to insights

---

## Citation Methodology

**Reports 218-223 Success Pattern:**

### Citation Density Requirements

**Minimum unique endnotes:** 30-40 sources per comprehensive report
**Target citation density:** 3.0-4.1 citation instances per unique endnote

**What this means:**
- Comprehensive reports need **30-40 unique sources minimum** for credibility
- ❌ **17 endnotes is too light** - insufficient evidence base, looks under-researched
- ✅ **30+ endnotes** - demonstrates thorough research, builds trust
- If you have 35 endnotes, expect 105-144 citation instances throughout report
- Each endnote cited 3-4 times on average
- Some sources cited more (foundational data), some less (niche claims)

**Why minimum matters:**
- Readers expect robust sourcing for $495 purchase decision
- Thin citation counts signal incomplete research
- Competitive intelligence requires diverse source validation
- Mix of vendor official (V), third-party (T), and academic (A) sources shows balanced perspective

**Exception: Niche Vendor Coverage Limitations**

Some vendors lack sufficient third-party coverage to reach 30-40 sources:

**When exception applies:**
- Vendor is niche/emerging (<50 employees, limited market presence)
- Vendor has <5 independent third-party reviews/analyses available
- Vendor focus is specialized (e.g., Linux-only, container-only, agentless-only)
- **Example:** Sandfly Security (agentless Linux EDR, small team, limited analyst coverage)

**Exception criteria:**
- **Minimum 20-25 sources** (not 30-40) if vendor legitimately has limited coverage
- **Must demonstrate best effort sourcing:**
  - Exhausted Google Scholar search for academic mentions
  - Checked all major analyst firms (Gartner, Forrester, IDC, SE Labs, AV-TEST)
  - Reviewed vendor documentation comprehensively
  - Searched technical blogs, conference talks, case studies
  - Reddit, HN, security forums for community discussion
- **Document the gap:** Note in report that limited third-party coverage reflects vendor's niche status
- **Higher bar for major vendors:** CrowdStrike, Microsoft, Palo Alto, SentinelOne = no exceptions, 30+ required

**Approval required:** Clearly state in report metadata why citation count is lower than standard.

### Citation Distribution

**MUST:**
- ✅ Cite claims evenly across ALL sections
- ✅ No citation-free zones (every section needs evidence)
- ✅ Multiple citations per major claim
- ✅ Cluster citations around key arguments

**AVOID:**
- ❌ Front-loading all citations in first 2 sections
- ❌ Citation deserts in middle/end sections
- ❌ Single-citation claims for controversial points
- ❌ Citing marketing pages for technical claims

### Citation Placement

**Pattern from successful reports (30-40 unique sources):**
```
Executive Summary: 0-2 citations (high-level, established facts only)
Threat Landscape: 8-12 citations (evidence-heavy, threat data)
Detection Capabilities: 10-15 citations (MITRE, test results, benchmarks)
TCO Analysis: 6-10 citations (pricing, labor data, industry reports)
Deployment: 4-8 citations (timelines, case studies)
Recommendation: 2-5 citations (summarize key evidence)

TOTAL: 30-52 unique endnotes (minimum 30 required)
```

**Red flag:** If total unique endnotes <25, report lacks credibility. Add more diverse sources.

### Endnote Format

**MUST:**
- ✅ Clickable URLs in endnotes section
- ✅ Include: Title, Source, Date (if time-sensitive)
- ✅ No anonymous "Industry Report" - cite specific source
- ✅ Use primary sources when possible (not marketing rewrites)
- ✅ **Letter badges with legend** (V=Vendor, T=Third-party, A=Academic)

**Letter Badge System (CRITICAL):**

Each endnote displays a letter badge indicating source type:
- **V** = Vendor official sources (documentation, whitepapers, vendor websites)
- **T** = Third-party independent sources (analyst reports, SE Labs, AV-TEST, Gartner)
- **A** = Academic sources (research papers, university studies, peer-reviewed publications)

**Legend placement:** Display legend above or at top of References/Sources section for reader clarity.

**Example legend:**
```
Source Types: [V] Vendor Official  [T] Third-party Research  [A] Academic
```

**Example:**
```
[12] [T] "CrowdStrike Falcon vs SentinelOne: Detection Effectiveness"
         SE Labs, Q4 2025, https://selabs.uk/reports/crowdstrike-vs-sentinelone-2025q4
         ↑ GOOD: Third-party badge, specific source, date, clickable link
```

---

## Scenario Card Requirements

**Reports 218-223 Pattern:**

### Density Target
- **13-15 scenario cards** per report
- Distributed across use case categories
- Concrete examples, not abstract descriptions

### Scenario Card Structure

**Each card MUST include:**
1. **Threat type:** Ransomware, phishing, supply chain, insider, etc.
2. **Attack vector:** How threat enters environment
3. **Detection method:** How product detects it
4. **Response action:** What happens when detected
5. **Outcome:** Business impact prevented

**Example - GOOD:**
```
┌─────────────────────────────────────────┐
│ Ransomware: LockBit 3.0 Deployment    │
├─────────────────────────────────────────┤
│ Vector: Phishing email with macro     │
│ Detection: Behavioral analysis detects │
│            encryption activity          │
│ Response: Isolate endpoint, kill       │
│           process, alert SOC            │
│ Outcome: $2.4M ransomware payment      │
│          prevented, 15min containment   │
└─────────────────────────────────────────┘
```

**Example - BAD:**
```
"Detects ransomware attacks effectively."
↑ Too vague - no specifics, no business outcome
```

### Scenario Distribution

**Balance across categories:**
- Ransomware: 3-4 cards (highest priority)
- Phishing/credential theft: 2-3 cards
- Supply chain/zero-day: 2-3 cards
- Insider threats: 1-2 cards
- Malware/commodity threats: 2-3 cards
- Lateral movement: 1-2 cards

**Why this balance:**
- Ransomware = business-ending risk (gets most attention)
- Phishing = highest volume (realistic threat)
- Supply chain = board-level concern (demonstrates capability)
- Insider = compliance requirement (show coverage)

---

## Detection Parity Handling

**Critical Pattern from Report 223 (BEST IN CLASS):**

### When Both Vendors = 100% MITRE Coverage

**DO NOT fabricate detection differences.**

Instead, shift comparison to:
1. **Cost** (TCO with labor)
2. **Operational burden** (admin time, alert volume, console complexity)
3. **Ecosystem** (integrations, compatibility, stack fit)
4. **MDR quality** (SOC expertise, response time, coverage hours)

**Example - Detection Parity Framing:**
```
CrowdStrike Falcon and SentinelOne Singularity both achieve 100%
detection coverage across the MITRE ATT&CK framework. When detection
capabilities are equivalent, the buying decision shifts to:

1. Total Cost: $147/endpoint (CrowdStrike+MDR) vs $132/endpoint (S1+MDR)
2. Operations: Single console (S1) vs multi-tool (CrowdStrike ecosystem)
3. MDR Quality: Falcon Complete 24/7 SOC vs Vigilance MDR response SLA
4. Ecosystem: Microsoft integrations (CrowdStrike) vs standalone (S1)

Winner: CrowdStrike, not because of detection superiority (parity exists),
but because Falcon Complete MDR eliminates on-call burden for $15/endpoint
premium, and Microsoft ecosystem integration reduces admin burden 30%.
```

**This is intellectually honest:**
- Admits detection parity (no fabrication)
- Shifts to real differentiators (cost, operations, MDR)
- Makes clear recommendation with business rationale
- Respects the data (doesn't invent differences)

### Elimination Testing Methodology

**When one vendor clearly superior across most criteria:**

Use **elimination framework** to structure decision:
1. Establish baseline requirements (must-haves)
2. Eliminate vendors that fail must-haves
3. Compare remaining vendors on nice-to-haves
4. Make recommendation based on total value

**Example from Report 223:**
```
Baseline Requirements (MUST HAVE):
  - 95%+ MITRE ATT&CK coverage
  - <30 day deployment timeline
  - 24/7 MDR available
  - <$150/endpoint 5-year TCO

Elimination:
  ❌ Vendor A: 87% MITRE coverage (fails baseline)
  ✅ Vendor B: 100% MITRE, 21-day deployment, MDR available, $147 TCO
  ✅ Vendor C: 100% MITRE, 28-day deployment, MDR available, $132 TCO

Differentiators (Both B and C viable):
  - Vendor B: Superior MDR (faster response, deeper expertise)
  - Vendor C: Lower cost ($15/endpoint savings)

Recommendation: Vendor B
Rationale: $15/endpoint premium buys significantly better MDR quality,
           which directly addresses "nobody wants 3AM calls" value prop
```

**Why this works:**
- Transparent methodology (reader can verify logic)
- Intellectually honest (no fabricated criteria)
- Business-focused (MDR quality > $15 cost difference)
- Defensible recommendation (clear rationale)

---

## Quality Assurance Methodology

**TDD Validation Loop (Report 215 vs 217 Learning):**

### Incremental Validation Required

**Report 215 (Nimitz):** Generated full report → validated → 72 errors → 2 hours fixing
**Report 217 (Butler):** Validated incrementally → 0 errors → immediate delivery

**Lesson:** Validate DURING generation, not after.

### Validation Checkpoints

**After EVERY section written:**
1. Run Playwright visual tests
2. Check chart rendering in browser
3. Verify citation links clickable
4. Confirm no text cutoffs or overlaps
5. Test PDF generation

**DO NOT:**
- ❌ Write entire report → validate at end
- ❌ Assume code correctness = output correctness
- ❌ Skip browser testing (code validation insufficient)

**DO:**
- ✅ Write section → validate → fix → next section
- ✅ Test in actual browser (Firefox/Chrome)
- ✅ Generate PDF after each major section
- ✅ Check visual rendering, not just code

### Browser Testing Requirements

**MUST test in browser:**
- Chart legend positioning (no overlaps)
- Font sizes (readable at 100% zoom)
- Color contrast (WCAG AA minimum)
- PDF generation (no page breaks mid-chart)
- Endnote links (clickable, correct targets)

**Command:**
```bash
# Open report in browser
firefox ~/projects/security-intelligence-business/output/.../report.html

# Generate PDF
playwright test tests/playwright/pdf_generation.spec.js
```

**If visual issues found:** Fix immediately, re-test, then continue.

**Success criteria:** Clean browser rendering + PDF generation with zero errors.

---

## Meta: This Is $250K/Year Knowledge

The rules in this prompt come from:
- 20+ years cybersecurity experience
- Seeing companies succeed and fail
- Understanding what buyers ACTUALLY care about
- **Knowing that nobody wants to wake up at 3AM**

The model can analyze pricing data and feature lists. The model CANNOT know:
- That IT admins miss their kids' baseball games
- That the Board of Directors dreads the 3am ransomware call
- That "free" Microsoft costs more than "expensive" CrowdStrike when you add labor
- **That quality of life is THE buying decision, not feature count**

**This knowledge must be preserved and accumulated.**

Each feedback cycle adds more $250K/year insights to this prompt.

That's how the system gets smarter.
