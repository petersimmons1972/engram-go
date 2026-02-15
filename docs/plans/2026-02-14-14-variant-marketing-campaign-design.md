# 14-Variant Marketing Campaign Design

**Date:** 2026-02-14
**Status:** Design Complete - Awaiting Implementation Approval
**Author:** Peter Simmons
**Campaign:** ClearWatch Research - Message × Aesthetic Testing Matrix

---

## Executive Summary

Deploy 14 full-featured e-commerce websites testing unique Message × Aesthetic combinations to identify optimal market positioning for ClearWatch Research's $495 bias-aware security intelligence reports. Internal-only deployment for expert validation, portfolio demonstration, and system proof-of-concept.

**Target Audience:** Decision-makers without trusted security advisors (1-3 person IT teams managing 500-2500 computers, organizations lacking MSP/consultant relationships).

**Purpose:** Expert validation + Portfolio demonstration + System proof-of-concept (NOT live public testing).

---

## Section 1: Campaign Architecture & Objectives

**Mission**: Deploy 14 full-featured e-commerce websites, each testing a unique Message × Aesthetic combination to identify optimal market positioning for ClearWatch Research's bias-aware security intelligence reports.

**Core Architecture**:
- **14 independent Next.js sites** (not a monorepo selector - each is standalone)
- **Shared product catalog** (same 6-8 comparison reports across all variants)
- **Variant-specific differentiation**: Homepage messaging, visual design, brand personality, About page narrative
- **Unified backend**: All variants connect to same WordPress + EDD headless backend, same Stripe account
- **Internal-only deployment**: No Cloudflare tunnel, DNS via Unifi only, homelab K8s cluster

**Campaign Objectives**:
1. **Expert validation**: Gordon Ramsay (presentation) + CISO Validator (decision utility) approve all 14
2. **Portfolio demonstration**: Showcase marketing capability to investors/partners
3. **System proof-of-concept**: Validate self-learning content generation system
4. **Pre-live refinement**: Test and refine before potential public deployment
5. **Learning capture**: Build reusable library of high-performing patterns

**Strategic Positioning**: Selling **decision insurance** to overwhelmed IT generalists making six-figure security commitments without expert guidance. $495 buys confidence they're not making a catastrophic mistake.

---

## Section 2: 14-Variant Marketing Matrix

| # | Deployment | Core Message | Aesthetic | Buyer Thought We're Targeting |
|---|------------|--------------|-----------|-------------------------------|
| 1 | **clearwatch-brutal** | Stop Guessing | Brutal Honesty | "I'm about to spend $100K on the wrong EDR" |
| 2 | **clearwatch-hero** | Expert at Your Side | Aspirational Bold | "I need someone who's done this before" |
| 3 | **clearwatch-minimal** | Research Already Done | Clean Scientific | "I don't have time to evaluate 5 vendors" |
| 4 | **clearwatch-trust** | Defendable Decisions | Professional Conservative | "I need to justify this to my CEO" |
| 5 | **hms-dreadnought** | Real Analysis, Real Price | Revolutionary Dominant | "There's nothing between free and unaffordable" |
| 6 | **hms-upholder** | No Vendor BS | Persistent Resilient | "Sales guys all say they're the best" |
| 7 | **hms-victory** | 30 Years Experience | Traditional Authority | "I'm not a security expert" |
| 8 | **uss-constitution** | See How We Decided | Foundational Principles | "I need to understand the methodology" |
| 9 | **uss-enterprise** | Every Source Checked | Bold Pioneering | "I'd research this myself if I had 40 hours" |
| 10 | **uss-fletcher** | Your Decision, Faster | Efficient Practical | "I need an answer by Friday" |
| 11 | **uss-monitor** | Expert Help You Can Afford | Disruptive Innovation | "$495 vs winging a $100K decision" |
| 12 | **uss-olympia** | American Security Veteran | Classic American Confidence | "Someone who knows US enterprise security" |
| 13 | **uss-tang** | Every Claim Proven | Elite Precision | "I need data, not marketing claims" |
| 14 | **uss-yorktown** | Confidence in Your Choice | Battle-Tested Resilience | "I need to sleep at night after this decision" |

**Strategic Clusters:**
- **Decision Confidence (4):** Brutal, Trust, Yorktown, Monitor
- **Expert Authority (3):** Hero, Victory, Olympia
- **Time/Efficiency (3):** Minimal, Fletcher, Enterprise
- **Transparency (2):** Constitution, Tang
- **Value/Accessibility (2):** Dreadnought, Monitor

---

## Section 3: General Assignments & Responsibilities

**Supreme Command Structure:**

**Field Marshal Montgomery** - Campaign Commander (Order of Victory recipient)
- Overall coordination of 14-site deployment
- Integration across all general assignments
- Final approval authority on campaign execution
- Synthesizes learning across all variants
- Reports to: User (Peter Simmons)

**Core Team Assignments:**

| General | Primary Role | Specific Responsibilities |
|---------|-------------|--------------------------|
| **General Marshall** | Build & Logistics Coordination | 14 parallel Next.js builds, deployment sequencing, resource allocation |
| **Admiral Nimitz** | K8s Configuration Engineering | Deployment manifests (16 pods), ingress routes, namespace organization |
| **Admiral Spruance** | Analytics & Testing | Analytics implementation, test purchase flows, verification |
| **Admiral Rickover** | Quality Control & Standards | Pipeline quality gates, regression prevention, build verification, learning synthesis |

**Content Creation Team (Journalists):**

| Journalist | Content Focus | Assignment |
|------------|---------------|------------|
| **Ernie Pyle** | Variants 1-5 (Clearwatch) | Ground-level, accessible copy for overwhelmed IT generalists |
| **Edward Murrow** | Variants 6-10 (HMS ships) | Formal authority, strategic framing |
| **George Orwell** | Variants 11-14 (USS ships) | Systematic diagnosis, anti-BS messaging |

**Quality Validation Gates:**
- **Gordon Ramsay**: Presentation quality (all 14 variants) - "Would I show this to a client?"
- **CISO Validator**: Decision utility (all 14 variants) - "$495 test" - Is this valuable to target audience?

**Rapid Response:**
- **General Patton**: Hotfixes, emergency deployments, blocker removal

---

## Section 4: Technical Stack & Infrastructure

**Frontend Architecture (14 Variants):**
- **Framework**: Next.js 16 with App Router (proven with apps/clay)
- **Styling**: Tailwind CSS 4 with variant-specific design systems
- **Deployment**: Individual Docker containers per variant
- **Hosting**: Kubernetes cluster (clearwatch namespace)
- **Domains**: Subdomain per variant (internal DNS only via Unifi)

**Shared Components (DRY principle):**
- **Product Catalog**: Same 6-8 comparison reports across all variants
- **Report Data**: Centralized JSON/API serving report metadata
- **Payment Backend**: Single Stripe account, shared checkout infrastructure (test mode)
- **PDF Delivery**: Unified fulfillment system post-purchase
- **Analytics**: Shared analytics with variant tagging

**Variant-Specific Elements:**
- Homepage hero sections (unique messaging per variant)
- About page narratives (same Peter Simmons story, different tone/emphasis)
- Visual design systems (14 distinct Tailwind configs)
- Brand personality (copy tone, imagery, micro-interactions)

**Backend/E-commerce (Option B - Headless Shared WordPress):**
- **Backend**: Single WordPress + Easy Digital Downloads (EDD) instance (headless API)
- **Database**: PostgreSQL/MySQL pod
- **Integration**: WordPress REST API serving product catalog
- **Frontends**: 14 lightweight Next.js sites consuming shared API
- **Advantages**:
  - Single product catalog (update once, reflects everywhere)
  - Proven e-commerce stack (EDD from business plan)
  - WordPress admin for non-technical product management
  - Shared checkout/payment logic

**Infrastructure Map:**
```
14 Next.js Sites → Shared WordPress + EDD API → Stripe (test) → Fulfillment → Customer
                ↓
        Shared Analytics Database
```

**K8s Pod Inventory (16 total):**
- 1 WordPress + EDD backend pod
- 1 PostgreSQL/MySQL database pod
- 14 Next.js frontend pods (one per variant)

**Network Architecture:**
- Internal-only (no Cloudflare tunnel)
- DNS via Unifi appliances only
- HTTPS via cert-manager (internal CA or Let's Encrypt)
- Traefik ingress (14 routes)

---

## Section 5: Content Generation System

**Report Generation** (proven system):
- **Reference implementations**:
  - `/home/psimmons/projects/security-intelligence-business/output/CrowdStrike_v_SentinelOne/209/` (123KB)
  - `/home/psimmons/projects/security-intelligence-business/output/CrowdStrike_v_MicrosoftDefender/214/` (133KB)
  - `/home/psimmons/projects/security-intelligence-business/output/SentinelOne_v_PaloAlto/215/` (135KB)
- **Pattern**: Modular section generation with learning capture
- **Output**: 120-135KB HTML reports (professional, substantial)
- **Reuse**: Same 6-8 reports across all 14 variants (DRY)

**Marketing Website Content** (NEW for this campaign):
- **Journalist generals** create variant-specific copy based on Message × Aesthetic assignments
- **Input**:
  - Target audience definition (overwhelmed IT generalist, no trusted advisor)
  - Business plan lessons
  - Variant assignment (message + aesthetic pairing)
- **Output**:
  - Homepage hero section
  - About page (Peter Simmons 30-year veteran story, variant-specific tone)
  - Feature descriptions
  - CTAs optimized for message
- **Quality gates**: Gordon (presentation) + CISO (decision utility)

**Content Reuse Strategy:**
- **Reports**: Same 6-8 PDFs across all variants (DRY)
- **Product Descriptions**: Shared base, variant-specific framing
- **Peter Simmons Bio**: Same facts, 14 different tones/emphasis
- **Pricing**: Consistent $495, different value framing

**Feedback Integration Pattern:**
1. Generate variant content (journalist general)
2. Gordon validates presentation quality
3. CISO validates decision utility
4. Capture feedback in structured format (`SERVICE_RECORD.md`)
5. Update generation prompts with lessons learned
6. Regenerate improved version
7. Repeat until both validators approve

---

## Section 6: Quality Gates & Feedback Loop

**Three-Layer Validation System:**

**Layer 1: General Self-Critique**
- Each general evaluates their own work before submission
- Document: What worked, what didn't, what would improve
- Format: Structured markdown in `SERVICE_RECORD.md` per variant deployment
- Accountability: No general submits without self-assessment

**Layer 2: Cross-General Review**
- Generals review each other's variants
- Focus: Consistency with target audience, message clarity, aesthetic quality
- Field Marshal Montgomery coordinates cross-reviews
- Captures: "What X did well that Y should adopt"
- Learning synthesis: Patterns that work across multiple variants

**Layer 3: Expert Validation**
- **Gordon Ramsay**: Presentation quality (all 14 variants)
  - Visual polish, typography, spacing, professionalism
  - Pass/fail gate: "Would I show this to a client?"
  - Ruthless feedback on any mediocrity
- **CISO Validator**: Decision utility (all 14 variants)
  - "$495 test" - Would overwhelmed IT generalist find this valuable?
  - Pass/fail gate: "Does this help them make a confident decision?"
  - Practical skeptic evaluation

**Feedback Capture Format:**
```markdown
## Variant: [deployment-name]
### General: [responsible-general-name]

### What Worked
- [Specific positive elements with evidence]
- [Patterns to reuse across other variants]

### What Failed
- [Specific failures with evidence]
- [Why it failed (missed target audience, wrong tone, etc.)]

### Actionable Lessons
- [Prompt modifications for next iteration]
- [Pattern to reuse across variants]
- [Anti-pattern to avoid]
- [Cross-variant learning opportunities]
```

**Integration Process:**
1. Collect feedback from all 3 layers per variant
2. Admiral Rickover synthesizes cross-variant patterns
3. Field Marshal Montgomery approves prompt updates
4. Update generation instructions with learned patterns
5. Archive anti-patterns to avoid
6. Next iteration uses improved prompts
7. Measure: Did feedback integration improve quality scores?

---

## Section 7: Deployment Strategy

**K8s Deployment Architecture:**

**Namespace**: `clearwatch` (existing)

**Parallel Deployment** (All 14 variants simultaneously):

**Infrastructure Foundation:**
- 1 WordPress + EDD headless backend pod
- 1 PostgreSQL/MySQL database pod
- 14 Next.js frontend pods (one per variant)
- Traefik ingress with 14 route definitions
- DNS: 14 subdomains configured via Unifi API (internal only)

**Subdomain Mapping:**
```
brutal.clearwatchresearch.com        → clearwatch-brutal
hero.clearwatchresearch.com          → clearwatch-hero
minimal.clearwatchresearch.com       → clearwatch-minimal
trust.clearwatchresearch.com         → clearwatch-trust
selector.clearwatchresearch.com      → clearwatch-selector
dreadnought.clearwatchresearch.com   → hms-dreadnought
upholder.clearwatchresearch.com      → hms-upholder
victory.clearwatchresearch.com       → hms-victory
constitution.clearwatchresearch.com  → uss-constitution
enterprise.clearwatchresearch.com    → uss-enterprise
fletcher.clearwatchresearch.com      → uss-fletcher
monitor.clearwatchresearch.com       → uss-monitor
olympia.clearwatchresearch.com       → uss-olympia
tang.clearwatchresearch.com          → uss-tang
yorktown.clearwatchresearch.com      → uss-yorktown
```

**Deployment Ownership:**
- **General Marshall**: Parallel build coordination (14 simultaneous builds)
- **Admiral Nimitz**: K8s manifests for all 16 pods (14 frontends + backend + DB)
- **Admiral Rickover**: Pre-deployment quality gate (all variants must pass)
- **Admiral Spruance**: Post-deployment verification (all 14 sites functional)

**Build Process:**
1. Content generation (3 journalist generals create variant-specific copy)
2. Layer 1 validation (generals self-critique)
3. Layer 2 validation (cross-general review)
4. Layer 3 validation (Gordon + CISO approval)
5. Docker builds (14 Next.js containers + 1 WordPress container)
6. K8s deployment (all 16 pods)
7. DNS configuration (Unifi API, 14 internal subdomains)
8. Ingress routes (Traefik, 14 routes)
9. Post-deployment verification (Admiral Spruance)

**Success Criteria (All-or-Nothing):**
- ✅ All 16 pods Running (2/2 ready)
- ✅ All 14 subdomains resolve (internal network)
- ✅ All 14 HTTPS certificates valid
- ✅ Checkout flow works on all 14 variants (test purchases)
- ✅ Analytics tracking on all 14 variants
- ✅ All variants pass Layer 3 validation (Gordon + CISO)

**Rollback Strategy:**
- Each variant deployed with version tags
- Failed deployment: rollback individual variant (not entire campaign)
- Shared backend failure: all variants affected (acceptable risk for internal deployment)

---

## Section 8: Success Metrics & Learning Integration

**Purpose: Internal validation, portfolio demonstration, system proof-of-concept**

**Success Criteria:**

**Tier 1: Technical Completeness**
- ✅ All 14 Next.js sites deployed and functional (internal network)
- ✅ All subdomains resolve via Unifi DNS
- ✅ WordPress headless backend operational
- ✅ Checkout flow works end-to-end (test purchases)
- ✅ All 16 K8s pods Running (2/2 ready)
- ✅ Analytics tracking implemented (even if no real traffic)

**Tier 2: Expert Validation**
- ✅ **Gordon Ramsay**: All 14 variants pass presentation quality gate
- ✅ **CISO Validator**: All 14 variants pass decision utility gate ($495 test)
- ✅ **General Self-Critique**: Each responsible general documents their work
- ✅ **Cross-Review**: Generals identify best practices across variants

**Tier 3: Portfolio Quality**
- ✅ 14 distinct Message × Aesthetic combinations clearly differentiated
- ✅ Professional quality suitable for investor/partner demonstrations
- ✅ Demonstrates range of marketing approaches for same product
- ✅ Proves capability to generate diverse, high-quality marketing sites

**Tier 4: System Validation**
- ✅ Self-learning feedback loop functional (capture → integrate → improve)
- ✅ Content generation system produces consistent quality
- ✅ Multi-general coordination successful
- ✅ Deployment pipeline repeatable

**Learning Integration Workflow:**

1. **Build & Deploy** - All 14 variants complete with expert validation

2. **Capture Feedback**:
   - General self-critiques → `SERVICE_RECORD.md` per variant
   - Gordon presentation feedback → structured notes
   - CISO decision utility feedback → structured notes
   - Cross-general learnings → synthesis document

3. **Synthesize Lessons** (Admiral Rickover):
   - What messaging patterns worked best?
   - What aesthetic approaches showed highest quality?
   - What generation prompts produced best results?
   - What failures to avoid in future iterations?
   - Which Message × Aesthetic combinations would likely perform best live?

4. **Document for Future** (Field Marshal Montgomery):
   - Update campaign generation prompts with lessons
   - Archive winning patterns for reuse
   - Document anti-patterns to avoid
   - Prepare synthesis report for user
   - Prepare system for potential live deployment

**Demonstration Use Cases:**
- **Show investors**: "Here's our marketing capability - 14 distinct approaches"
- **Show partners**: "We can target multiple segments effectively"
- **Internal planning**: "Which approach should we go live with first?"
- **System proof**: "The self-learning loop works at scale"
- **Portfolio piece**: "Example of AI-coordinated marketing campaign execution"

**Success Definition:**
- All 4 tiers complete
- No variant fails expert validation
- Clear learning captured for next iteration
- System proven ready for live deployment (when decision made)
- User satisfied with quality and completeness

---

## Target Audience Reference

**WHO:** Decision-makers without access to trusted security advisors

**Segment 1: Small IT Teams (Primary Focus)**
- 1-3 person IT teams managing 500-2500 computers
- IT generalists (NOT security specialists)
- No dedicated security team on staff
- Drowning in responsibility - huge environments, limited expertise
- Making high-stakes security decisions outside their core competency
- Critical purchase decisions: EDR for 1000 endpoints = $50K-150K/year commitment

**Segment 2: Organizations Without Advisors**
- No MSP/consultant partner relationship
- No trusted advisor from partners they do business with
- Lack internal security expertise
- Need independent guidance for security vendor decisions

**BUDGET REALITY:**
- Cannot afford $5K Gartner/Forrester analyst subscriptions
- CAN afford $495 for decision-specific guidance on critical purchase
- One-time purchase for specific vendor decision (not ongoing relationship)

**WHO WE'RE NOT TARGETING (KEY EXCLUSIONS):**
- Organizations with highly valued MSP relationships
- Organizations with trusted consulting partners
- Organizations with MDR (Managed Detection & Response) providers
- **If they have a trusted advisor, they'll ask them - not us**

**VALUE PROPOSITION:**
You're selling **decision insurance** to overwhelmed IT generalists who are about to make six-figure security commitments outside their expertise. $495 buys them confidence they're not making a catastrophic mistake.

**PRICING:** $495 per report (fixed)

**CRITICAL:** All marketing messaging, website copy, and content must speak to this specific persona - the overwhelmed IT generalist without trusted advisor, NOT security specialists or CISOs with expert teams.

---

## Implementation Phases (If Proceeding)

**Phase 1: Foundation Setup**
- WordPress + EDD deployment (K8s)
- Database setup
- Shared API configuration
- Stripe test mode integration
- Reference: Best report samples (v209, v214, v215)

**Phase 2: Content Generation**
- Journalist generals create variant-specific copy
- 14 Message × Aesthetic combinations
- Layer 1 validation (self-critique)
- Layer 2 validation (cross-review)

**Phase 3: Expert Validation**
- Gordon Ramsay presentation review (all 14)
- CISO Validator decision utility review (all 14)
- Feedback capture and iteration
- Approval for deployment

**Phase 4: Deployment**
- Docker builds (14 Next.js + 1 WordPress)
- K8s deployment (all 16 pods)
- DNS configuration (Unifi)
- Ingress routes (Traefik)
- Post-deployment verification

**Phase 5: Learning Synthesis**
- Admiral Rickover synthesizes lessons
- Field Marshal Montgomery approves
- Documentation of learnings
- System readiness assessment

---

## Open Questions (To Be Resolved During Implementation)

1. **WordPress Headless Setup**: Use existing WordPress or deploy fresh instance?
2. **Product Catalog Source**: Import from existing reports or create fresh?
3. **Stripe Test Mode**: New test account or use existing?
4. **Analytics Platform**: Plausible, Google Analytics, or custom?
5. **Certificate Authority**: Let's Encrypt or internal CA for HTTPS?
6. **Deployment Sequencing**: Content-first or infrastructure-first?

---

## Reference Materials

### Existing Assets
- `/home/psimmons/projects/clearwatch-research-website/` - Current single variant (clay)
- `/home/psimmons/projects/security-intelligence-business/output/CrowdStrike_v_SentinelOne/209/` - Best report sample
- `/home/psimmons/projects/security-intelligence-business/output/CrowdStrike_v_MicrosoftDefender/214/` - Best report sample
- `/home/psimmons/projects/security-intelligence-business/output/SentinelOne_v_PaloAlto/215/` - Best report sample
- `/home/psimmons/projects/security-intelligence-business/CLAUDE.md` - Project instructions with target audience
- `/home/psimmons/projects/generals/COMMAND-ROSTER.md` - Available generals and specializations

### Research Completed
- 14-variant Message × Aesthetic matrix (this session)
- Target audience refinement (this session)
- General assignments (this session)
- Technical stack selection (this session)

---

## Next Steps

1. Review this design document, identify any gaps
2. Decide on implementation start date
3. Resolve open questions above
4. Create implementation plan (using `superpowers:writing-plans`)
5. Set up isolated workspace (using `superpowers:using-git-worktrees`)
6. Begin Phase 1: Foundation Setup

---

**Document Version:** 1.0
**Last Updated:** 2026-02-14
**Status:** Design Complete - Awaiting Implementation Approval
