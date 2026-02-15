# 14-Front Website Campaign - Design Document

**Date**: 2026-02-08
**Campaign Name**: Operation Multi-Variant Deployment
**Supreme Commander**: User
**Army Commander**: Field Marshal Bernard Montgomery
**Chief of Staff**: General Walter Bedell Smith

---

## Executive Summary

Deploy 14 complete website variants as an A/B testing conversion optimization platform. Each variant (academic, bento, brutal, dark, docs, editorial, flat, glass, gradient, hero, minimal, selector, terminal, trust) will be expanded from landing-page-only to a complete 4-page site (Home, About, Services, Portfolio). Content strategy uses hybrid approach: core biographical facts shared across all variants, but messaging tone/voice adapted to match each design personality.

**Scale**: 42 new pages (14 sites × 3 new pages each), 14 independent commanders, multi-phase parallel deployment.

---

## 1. Strategic Overview & Battle Metaphor

### The Challenge
Deploy 14 complete website variants as a conversion optimization A/B testing platform. Each variant shares core biographical facts but adapts messaging/tone to different design personalities.

### Current State
Only landing pages exist. Need to build out 3 additional pages per variant (About/Bio, Services/Expertise, Portfolio/Projects), requiring ~56 pages total (14 sites × 4 pages each, minus 14 existing landing pages = 42 new pages).

### The Battle Metaphor
This isn't a single deployment—it's a **multi-front campaign**. Each website variant is a "front" with its own battlefield commander (General/Admiral). The Army Commander coordinates all fronts, ensures strategic coherence, and reports consolidated progress. The embedded reporter (Ernie Pyle) documents the campaign for LinkedIn content generation.

### Why Military Structure Matters
- **Parallel execution**: 14 independent fronts can operate simultaneously
- **Clear accountability**: Each site has a named commander responsible for its completion
- **Doctrine standardization**: Shared content architecture prevents drift
- **Progress visibility**: Clear chain of command for status reporting
- **Learning capture**: Service records document what works/fails across variants

### Success Criteria
1. All 14 sites have 4 complete pages (Home, About, Services, Portfolio)
2. Biographical facts are accurate across all variants
3. Messaging tone matches design personality (Brutal ≠ Trust ≠ Academic)
4. Sites are deployed to K8s with proper routing
5. A/B testing infrastructure can measure conversion metrics

---

## 2. Command Structure

### Supreme Headquarters
```
Supreme Commander (User)
    ↓
Field Marshal MONTGOMERY (Army Commander - Strategic Coordination)
    ↓
General BEDELL SMITH (Chief of Staff - Operations Management)
```

### General Staff
- **Admiral Edwin LAYTON** - Intelligence & Analytics Division (A/B testing metrics, conversion analysis)
- **David OGILVY** - Brand & Content Standards (messaging consistency across variants)
- **Admiral Ben MOREELL** - Infrastructure & Logistics (CI/CD, K8s deployments, automation)
- **Ernie PYLE** - Embedded Reporter (LinkedIn content, technical storytelling)

### Quality Gates (Always Consulted)
- **Gate 1**: Gordon Ramsay (Visual/Design Quality)
- **Gate 2**: CISO Validator (Content Utility/Value)
- **Gate 3**: David Ogilvy (Brand Consistency/A/B Hypothesis)

### Front Commanders (14 Site Deployments)

**Army Division (7 Generals)**:
| Site | Commander | Rationale |
|------|-----------|-----------|
| brutal | General George S. Patton Jr. | Aggressive execution, rapid delivery |
| minimal | General George C. Marshall | Systematic logistics, clean organization |
| terminal | Admiral Hyman G. Rickover | Quality obsession, technical precision |
| academic | General Dwight D. Eisenhower | Collaborative, analytical approach |
| editorial | Field Marshal William J. Slim | Narrative focus, clarity in communication |
| docs | Rear Admiral Grace M. Hopper | Technical documentation, accessibility |
| flat | General Omar Bradley | "Soldier's general", approachable design |

**Navy Division (7 Admirals)**:
| Site | Commander | Rationale |
|------|-----------|-----------|
| glass | Fleet Admiral Chester W. Nimitz | Transparency, elegant complexity |
| trust | Admiral Raymond A. Spruance | Methodical, calculated risk |
| hero | Fleet Admiral William F. Halsey | Bold, attention-grabbing |
| gradient | Fleet Admiral Ernest J. King | Multi-dimensional, sophisticated |
| dark | Lieutenant General Leslie R. Groves | Security-focused, serious tone |
| bento | Marshal Georgy Zhukov | Organized complexity, systematic |
| selector | Brigadier General William Mitchell | Future-forward, experimental |

### Chain of Command
- Front Commanders report to → Bedell Smith (daily ops) → Montgomery (strategic decisions) → User
- General Staff advises Montgomery & coordinates across fronts
- Quality Gates validate outputs before deployment

### Model Selection Autonomy
All agents are empowered to select cost-efficient models based on task complexity:
- **Haiku**: Simple page builds with clear templates, routine deployments
- **Sonnet**: Content adaptation requiring tone/voice judgment, integration work
- **Opus**: Strategic planning, complex architectural decisions, multi-dependency coordination

---

## 3. Site-to-Commander Personality Matching

### Army Generals (Ground/Traditional Approaches)
- **brutal** → Patton: "A good plan violently executed now" matches brutal aesthetic
- **minimal** → Marshall: Systematic, organized approach fits minimalist design
- **terminal** → Rickover: Technical precision, zero-defect culture matches terminal theme
- **academic** → Eisenhower: Collaborative, analytical style suits academic rigor
- **editorial** → Slim: Narrative clarity, communication focus fits editorial style
- **docs** → Hopper: Technical documentation expertise, accessibility pioneer
- **flat** → Bradley: Approachable, humble competence matches flat design philosophy

### Navy Admirals (Modern/Fluid Approaches)
- **glass** → Nimitz: Transparency, organizational excellence fits glassmorphism
- **trust** → Spruance: Methodical, calculated risk builds trust
- **hero** → Halsey: Bold, aggressive style matches hero-section focus
- **gradient** → King: Multi-dimensional thinking suits gradient complexity
- **dark** → Groves: Security obsession fits dark, serious aesthetic
- **bento** → Zhukov: Made complexity visible through organization (bento grid)
- **selector** → Mitchell: Visionary, future-forward matches selector/experimental theme

---

## 4. Operational Phases

### Phase 1: Planning & Reconnaissance (Week 1)
**Commanders**: Montgomery, Bedell Smith, General Staff

**Activities**:
- Montgomery + Bedell Smith: Campaign plan, resource allocation, timeline
- Admiral Layton: Define A/B testing metrics, conversion goals per variant
- David Ogilvy: Establish brand guidelines, tone matrices per design style
- Admiral Moreell: Infrastructure readiness (CI/CD, content management system)
- Ernie Pyle: Campaign announcement, "14 Fronts" LinkedIn series kickoff

**Deliverables**:
- Implementation plan with task breakdown
- A/B testing metrics framework
- Tone/voice matrix for all 14 variants
- Infrastructure readiness report
- LinkedIn campaign announcement post

### Phase 2: Content Foundation (Week 1-2)
**Commanders**: Ogilvy (lead), all Front Commanders (receive briefings)

**Activities**:
- Correct biographical facts (single source of truth in core-facts.json)
- Create tone/voice matrix (Brutal vs Trust vs Academic messaging)
- Build shared content components (bio facts, service descriptions, portfolio items)
- Each front commander receives their content briefing pack
- Ernie Pyle: "Building the Arsenal" content creation post

**Deliverables**:
- `content/core-facts.json` (verified accurate)
- `content/services.json` (factual descriptions)
- `content/portfolio.json` (case studies/projects)
- 14 tone variant files (`content/tone-variants/*.json`)
- Content briefing packs for all commanders

### Phase 3: Front Deployment (Week 2-4)
**Commanders**: All 14 Front Commanders (parallel execution)

**Activities**:
- All 14 fronts execute simultaneously (parallel deployment)
- Each commander builds 3 pages (About, Services, Portfolio) for their site
- Daily standups with Bedell Smith (blockers, progress, coordination)
- Quality gates validate each page before deployment
- Ernie Pyle: Daily "front dispatches" from different commanders (14 posts)

**Deliverables**:
- 42 new pages deployed (14 sites × 3 pages)
- All pages pass 3 quality gates
- K8s deployments healthy with proper routing
- 14 LinkedIn "front dispatch" posts

### Phase 4: Testing & Optimization (Week 4-6)
**Commanders**: Layton (lead), Ogilvy (brand audit)

**Activities**:
- Admiral Layton: A/B test data collection, conversion analysis
- David Ogilvy: Cross-variant brand consistency audit
- Winning patterns identified, shared across fronts
- Ernie Pyle: "Lessons from the front" analysis posts (3-5 posts)

**Deliverables**:
- A/B testing baseline data (minimum 7 days)
- Conversion analysis report
- Brand consistency audit report
- Pattern library of winning approaches
- LinkedIn analysis posts + victory summary

---

## 5. Technical Architecture

### Content Management Strategy
```
~/projects/security-intelligence-business/
├── content/
│   ├── core-facts.json          # Single source of truth (bio, dates, achievements)
│   ├── services.json             # Service offerings (factual)
│   ├── portfolio.json            # Projects/case studies (factual)
│   └── tone-variants/
│       ├── brutal-voice.json     # Aggressive, direct messaging
│       ├── trust-voice.json      # Calm, authoritative messaging
│       ├── academic-voice.json   # Research-focused messaging
│       ├── minimal-voice.json
│       ├── terminal-voice.json
│       ├── editorial-voice.json
│       ├── docs-voice.json
│       ├── flat-voice.json
│       ├── glass-voice.json
│       ├── hero-voice.json
│       ├── gradient-voice.json
│       ├── dark-voice.json
│       ├── bento-voice.json
│       └── selector-voice.json
├── apps/
│   ├── brutal/                   # Existing Next.js apps
│   ├── trust/
│   ├── minimal/
│   ├── [... 11 more variants]
│   └── selector/
└── k8s/clearwatch/               # Deployment manifests
    ├── namespace.yaml
    ├── [deployment configs per variant]
    └── ingress-*.yaml
```

### Hybrid Content Model
- **Core Facts** (shared): Dates, credentials, achievements, technical skills
- **Tone Variants** (unique): Voice, messaging style, emotional register
- **Example**:
  - Core: "15 years cybersecurity experience, CISSP certified"
  - Brutal tone: "Battle-hardened. 15 years stopping threats. CISSP."
  - Trust tone: "Certified CISSP with 15 years of proven security leadership"
  - Academic tone: "15 years of applied cybersecurity research (CISSP, 2009)"

### Deployment Architecture
- Each variant stays as independent Next.js deployment (current state preserved)
- Shared content imported at build time from `content/` directory
- K8s deployments use warship pod names (already established: hms-dreadnought, uss-enterprise, etc.)
- Traefik routing by subdomain: brutal.clearwatchresearch.com, trust.clearwatchresearch.com, etc.

### A/B Testing Infrastructure
- **Traffic Distribution**: Traefik weighted routing OR JavaScript redirect on main domain
- **Metrics Collection**: Admiral Layton owns Google Analytics 4, conversion tracking
- **Variant Tracking**: Each variant gets tracking code with variant ID
- **Success Metrics**: Conversion rate, time-on-site, bounce rate, CTA click-through

### Quality Gate Integration
- **Gate 1 (Ramsay)**: Visual regression testing, screenshot comparison, design consistency
- **Gate 2 (CISO)**: Content audit checklist, value proposition scoring, decision utility
- **Gate 3 (Ogilvy)**: Brand consistency matrix, tone alignment validation, A/B hypothesis soundness

### CI/CD Pipeline (Admiral Moreell)
```
Code Commit → Build (Next.js) → Quality Gates (3-stage) → K8s Deploy → Analytics Setup
```

---

## 6. Success Metrics & Victory Conditions

### Per-Front Victory Conditions (Each Commander)
- ✅ 4 complete pages deployed (Home, About, Services, Portfolio)
- ✅ All biographical facts accurate (verified against core-facts.json)
- ✅ Tone matches design personality (Ogilvy validation)
- ✅ Passes all 3 quality gates (Ramsay, CISO, Ogilvy)
- ✅ K8s deployment healthy, proper routing configured
- ✅ Analytics tracking operational

### Campaign-Level Victory Conditions
- ✅ All 14 fronts achieve victory conditions
- ✅ A/B testing infrastructure operational (Layton confirms)
- ✅ Conversion data flowing (minimum 7 days baseline)
- ✅ Brand consistency audit passed (Ogilvy validation)
- ✅ LinkedIn content series published (Ernie Pyle deliverables)
- ✅ Infrastructure can scale (Moreell confirms CI/CD pipeline solid)

### Learning Objectives (GitHub Documentation)
**Required before team shutdown (per CLAUDE.md)**:
- Service records for all commanders (what worked, what failed, personality observations)
- Tone/voice patterns that resonated per design style
- Technical patterns for multi-variant content management
- A/B testing baseline data for future optimization
- XP awards, campaign ribbons, medals (based on user feedback)
- Commit to GitHub: https://github.com/petersimmons1972/generals.git

### Failure Recovery Plan
- Field Marshal Slim on standby for disaster recovery
- Bedell Smith escalates blocked fronts to Montgomery
- Failed quality gates trigger commander consultation, not bypass
- Anti-pattern checking if commanders stuck >20 minutes

### LinkedIn Content Deliverables (Ernie Pyle)
1. **Campaign Announcement Post** (Phase 1)
   - The mission, the scale, the commanders
   - "14 Fronts, 14 Commanders, One Mission"

2. **14 Front Dispatch Posts** (Phase 3, 1 per commander)
   - Human-centered narrative from each commander's perspective
   - Technical details woven into storytelling
   - Beginner-friendly with expert nuggets
   - Example: "Patton's Brutal Assault: Why Speed Beats Perfection"

3. **3-5 Lessons Learned Posts** (Phase 4)
   - Cross-front pattern analysis
   - What worked, what failed, why
   - Example: "What Minimal and Brutal Taught Us About Conversion"

4. **Victory Summary Post** (Campaign completion)
   - Metrics, achievements, learnings
   - Commander highlights, medals awarded
   - Future campaign teasers

---

## 7. Risk Assessment & Mitigation

### High-Risk Factors
1. **Content Drift**: 14 variants could diverge from factual accuracy
   - **Mitigation**: Single source of truth (core-facts.json), Ogilvy brand audits

2. **Commander Overload**: Montgomery managing 14 direct reports
   - **Mitigation**: Bedell Smith as Chief of Staff filters daily operations

3. **Quality Gate Bottleneck**: 3 gates × 42 pages = 126 validations
   - **Mitigation**: Automated testing where possible (Ramsay visual regression), parallel reviews

4. **A/B Testing Complexity**: 14 variants = statistical power challenges
   - **Mitigation**: Layton designs metrics framework upfront, phased traffic allocation

5. **Context Window Limits**: Large teams generate conversation volume
   - **Mitigation**: GitHub commits for persistent learning, structured service records

### Medium-Risk Factors
- Infrastructure scaling (Moreell owns)
- Tone consistency (Ogilvy audits)
- Timeline slippage (Bedell Smith daily standups)

---

## 8. Resource Requirements

### Personnel
- 1 Army Commander (Montgomery)
- 1 Chief of Staff (Bedell Smith)
- 4 General Staff (Layton, Ogilvy, Moreell, Pyle)
- 14 Front Commanders (7 Generals, 7 Admirals)
- 3 Quality Validators (Ramsay, CISO, Ogilvy)
- 1 Disaster Recovery (Slim, on standby)

**Total**: 24 commanders/specialists

### Infrastructure
- K8s cluster (existing, 192.168.0.131-139)
- Traefik routing (existing)
- Container registry (registry.petersimmons.com, existing)
- Analytics platform (Google Analytics 4, to be configured)
- GitHub repository (https://github.com/petersimmons1972/generals.git, existing)

### Timeline
- **Phase 1**: 1 week (Planning)
- **Phase 2**: 1-2 weeks (Content Foundation)
- **Phase 3**: 2-4 weeks (Deployment)
- **Phase 4**: 2 weeks (Testing & Optimization)

**Total Campaign Duration**: 6-9 weeks

---

## 9. Next Steps

1. **User Approval**: Confirm design meets requirements
2. **Implementation Planning**: Use `superpowers:writing-plans` to create detailed execution plan
3. **Team Assembly**: Spawn Montgomery, Bedell Smith, General Staff
4. **Phase 1 Execution**: Planning & Reconnaissance begins
5. **Ernie Pyle Briefing**: LinkedIn content strategy and publishing cadence

---

## Appendix A: Commander Profiles

See `/home/psimmons/projects/generals/COMMAND-ROSTER.md` for detailed profiles of all commanders including:
- Specializations
- Personality traits
- Current XP and deployment history
- Campaign ribbons and medals
- Best use cases

---

## Appendix B: Tone/Voice Matrix (Draft)

| Variant | Voice | Emotional Register | Example CTA |
|---------|-------|-------------------|-------------|
| brutal | Direct, aggressive | High energy, urgent | "Stop getting hacked. Talk now." |
| trust | Calm, authoritative | Steady confidence | "Let's build security together." |
| academic | Analytical, precise | Intellectual curiosity | "Explore our research methodology." |
| minimal | Clean, essential | Quiet confidence | "See what matters." |
| terminal | Technical, precise | Expert authority | "$ secure-your-infrastructure" |
| editorial | Narrative, storytelling | Engaging, informative | "Read our security insights." |
| docs | Instructional, clear | Helpful expertise | "Learn how we protect your data." |
| flat | Approachable, simple | Friendly competence | "Security made simple." |
| glass | Transparent, elegant | Sophisticated clarity | "See through to better security." |
| hero | Bold, aspirational | Inspirational confidence | "Transform your security posture." |
| gradient | Layered, sophisticated | Complex elegance | "Navigate the spectrum of threats." |
| dark | Serious, intense | Focused intensity | "Your security. Our mission." |
| bento | Organized, systematic | Structured clarity | "Every piece has its place." |
| selector | Experimental, modern | Future-forward | "Choose your security future." |

*Note: Final tone variants will be developed by David Ogilvy in Phase 2*

---

**Document Status**: Design Complete, Ready for Implementation Planning
**Next Document**: `2026-02-08-14-front-website-campaign-implementation.md` (to be created)
