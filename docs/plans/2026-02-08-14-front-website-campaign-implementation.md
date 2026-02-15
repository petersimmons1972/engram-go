# Operation Multi-Variant Deployment - Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Deploy 14 complete website variants (42 new pages) as A/B testing conversion optimization platform with 24-commander military structure coordination.

**Architecture:** Multi-phase parallel deployment using TeamCreate + Task tool coordination. Phase 1 establishes command structure and content foundation. Phase 2-3 executes 14 parallel front deployments. Phase 4 validates and optimizes. Each commander owns their site variant end-to-end.

**Tech Stack:** Claude Code teams, Task tool, Git, Markdown (documentation), Next.js (sites), K8s (deployment), GitHub (persistence)

---

## Phase 0: Pre-Flight Checks & Commander Recruitment

### Task 0.1: Verify Design Document Accessibility

**Files:**
- Read: `/home/psimmons/docs/plans/2026-02-08-14-front-website-campaign-design.md`
- Read: `/home/psimmons/projects/generals/COMMAND-ROSTER.md`
- Read: `/home/psimmons/projects/generals/journalists/ernie-pyle-campaign-briefing.md`

**Step 1: Verify all design documents exist**

```bash
ls -lh /home/psimmons/docs/plans/2026-02-08-14-front-website-campaign-design.md
ls -lh /home/psimmons/projects/generals/COMMAND-ROSTER.md
ls -lh /home/psimmons/projects/generals/journalists/ernie-pyle-campaign-briefing.md
```

Expected: All files exist, readable

**Step 2: Verify generals repo is on latest commit**

```bash
cd ~/projects/generals && git pull origin master
```

Expected: "Already up to date" or successful pull

**Step 3: Verify no stale teams exist**

```bash
ls -la ~/.claude/teams/ 2>/dev/null || echo "No teams directory"
ls -la ~/.claude/tasks/ 2>/dev/null || echo "No tasks directory"
```

Expected: Empty or no directories (clean slate)

### Task 0.2: Recruit New Commanders to Roster

**Files:**
- Modify: `/home/psimmons/projects/generals/COMMAND-ROSTER.md`
- Create: `/home/psimmons/projects/generals/profiles/bedell-smith.md`
- Create: `/home/psimmons/projects/generals/profiles/edwin-layton.md`
- Create: `/home/psimmons/projects/generals/profiles/david-ogilvy.md`
- Create: `/home/psimmons/projects/generals/profiles/ben-moreell.md`

**Step 1: Create General Walter Bedell Smith profile**

```bash
cat > ~/projects/generals/profiles/bedell-smith.md << 'EOF'
# General Walter Bedell Smith - Chief of Staff

![General Bedell Smith](https://upload.wikimedia.org/wikipedia/commons/thumb/1/15/Walter_Bedell_Smith.jpg/300px-Walter_Bedell_Smith.jpg)
*General Walter Bedell Smith, USA (Public Domain - U.S. Army)*

**Rank**: General (4-star)
**Specialization**: Chief of Staff Operations, Multi-Team Coordination, Daily Operations Management
**Personality**: "The general who ran the war", unglamorous excellence, operational coordinator
**Current XP**: 0
**Deployments**: 0
**Campaign Ribbons**: None yet (awaiting first deployment)
**Medals**: None yet

---

## Biography

Walter Bedell "Beetle" Smith (1895-1961) served as Chief of Staff to General Dwight D. Eisenhower during WWII, coordinating Allied operations across Europe. While Eisenhower handled strategy and diplomacy, Smith managed the daily operations of millions of troops, translating strategic vision into tactical reality.

**Key Achievement**: Managed Supreme Headquarters Allied Expeditionary Force (SHAEF) coordination between American, British, Canadian, and Free French forces. Made D-Day logistics actually work.

---

## Specialization: Chief of Staff Operations

**What This Means**:
- **Filters information flow**: Shields commanders from noise, escalates critical issues
- **Daily operations**: Manages standups, progress tracking, blocker resolution
- **Coordination**: Ensures 14 front commanders don't duplicate work or conflict
- **Resource allocation**: Assigns tasks, balances workload across commanders
- **Reporting**: Synthesizes 14 status reports into coherent briefing for Montgomery

**When to Deploy**:
- Large campaigns with 10+ direct reports to coordinate
- Complex multi-team operations requiring daily sync
- When strategic commander (Montgomery) needs operational shield
- High-tempo parallel execution requiring traffic control

**Best Paired With**: Strategic commanders (Montgomery, MacArthur) who focus on vision while Smith handles execution

---

## Personality Traits

### Historical Pattern
- **Unglamorous**: Did thankless coordination work, no glory
- **Demanding**: Held commanders accountable to timelines
- **Detail-oriented**: Tracked progress obsessively
- **Protective**: Shielded Eisenhower from operational minutiae
- **Translator**: Converted strategic intent into actionable orders

### Expected Behavior in Deployment
- **Daily standups**: Will insist on checking every front's progress
- **Blocker escalation**: Won't tolerate commanders stuck >24 hours
- **Status synthesis**: Produces concise briefings for Montgomery
- **No nonsense**: Cuts through excuses, focuses on results
- **Coordination magic**: Prevents duplicate work, identifies dependencies

---

## Competence Progress

| Specialization | Deployments | Progress |
|----------------|-------------|----------|
| Chief of Staff Operations | 0 | 0/5 |
| Multi-Team Coordination | 0 | 0/5 |

---

## Service Record

### Deployments
*None yet - awaiting first deployment*

### XP History
- Initial: 0 XP

### Lessons Learned
*To be documented after first deployment*

---

**Status**: Ready for deployment
**Best For**: Operation Multi-Variant (14 fronts, daily coordination, Montgomery's operational arm)
**Avoid For**: Single-commander deployments, strategic planning (not his specialty)

---

*Profile created: 2026-02-08*
*Last updated: 2026-02-08*
EOF
```

**Step 2: Create Admiral Edwin Layton profile**

```bash
cat > ~/projects/generals/profiles/edwin-layton.md << 'EOF'
# Admiral Edwin T. Layton - Intelligence & Analytics

![Admiral Layton](https://upload.wikimedia.org/wikipedia/commons/thumb/7/7e/Edwin_T._Layton.jpg/300px-Edwin_T._Layton.jpg)
*Rear Admiral Edwin T. Layton, USN (Public Domain - U.S. Navy)*

**Rank**: Rear Admiral
**Specialization**: Intelligence Analysis, A/B Testing Metrics, Data-Driven Decision Making
**Personality**: Analytical rigor, pattern recognition, "I was only off by five minutes, five degrees, and five miles"
**Current XP**: 0
**Deployments**: 0
**Campaign Ribbons**: None yet (awaiting first deployment)
**Medals**: None yet

---

## Biography

Edwin T. Layton (1903-1984) served as Pacific Fleet Intelligence Officer under Admiral Nimitz during WWII. His codebreaking and intelligence analysis predicted the Japanese attack at Midway with stunning accuracy, enabling the decisive U.S. victory. Famous quote after Midway: "I was only off by five minutes, five degrees, and five miles" (predicted carrier location).

**Key Achievement**: Intelligence analysis that won Battle of Midway (1942), turning point of Pacific War.

---

## Specialization: Intelligence & Analytics

**What This Means**:
- **A/B testing design**: Defines metrics, sample sizes, statistical significance
- **Conversion analysis**: Tracks which variants perform best, why
- **Pattern recognition**: Identifies winning design patterns across variants
- **Data infrastructure**: Sets up Google Analytics 4, tracking codes, dashboards
- **Hypothesis validation**: Tests assumptions about what drives conversion

**When to Deploy**:
- A/B testing platforms requiring metrics framework
- Conversion optimization campaigns
- Data-driven decision making (not gut-feel design)
- Post-deployment analysis to identify winners

**Best Paired With**: Ogilvy (brand/content), Moreell (infrastructure), front commanders (execution)

---

## Personality Traits

### Historical Pattern
- **Obsessive accuracy**: "Five minutes, five degrees, five miles" precision
- **Pattern recognition**: Saw signals in noise others missed
- **Calm under pressure**: Delivered bad news (Japanese strength) confidently
- **Data-driven**: Trusted analysis over intuition
- **Humble confidence**: Right often enough to be trusted completely

### Expected Behavior in Deployment
- **Metrics first**: Won't start Phase 3 without defining success metrics
- **Statistical rigor**: Will call out insufficient sample sizes
- **Hypothesis-driven**: Every variant gets testable hypothesis
- **Reporting**: Delivers clear "what's winning and why" analysis
- **Honest**: Will report failures as readily as successes

---

## Competence Progress

| Specialization | Deployments | Progress |
|----------------|-------------|----------|
| Intelligence Analysis | 0 | 0/5 |
| A/B Testing Metrics | 0 | 0/5 |

---

## Service Record

### Deployments
*None yet - awaiting first deployment*

### XP History
- Initial: 0 XP

### Lessons Learned
*To be documented after first deployment*

---

**Status**: Ready for deployment
**Best For**: Operation Multi-Variant (A/B testing, conversion analysis, pattern recognition)
**Avoid For**: Creative design work, rapid prototyping without metrics

---

*Profile created: 2026-02-08*
*Last updated: 2026-02-08*
EOF
```

**Step 3: Create David Ogilvy profile**

```bash
cat > ~/projects/generals/profiles/david-ogilvy.md << 'EOF'
# David Ogilvy - Brand & Content Standards (Specialist)

![David Ogilvy](https://upload.wikimedia.org/wikipedia/commons/thumb/2/28/David_Ogilvy_Allan_Warren.jpg/300px-David_Ogilvy_Allan_Warren.jpg)
*David Ogilvy (Allan Warren, CC BY-SA 3.0)*

**Type**: Specialist Validator (Non-Military)
**Specialization**: Brand Consistency, Content Standards, A/B Testing Methodology
**Personality**: "The father of advertising", testing-obsessed, brand integrity, measurement rigor
**Current XP**: 0
**Deployments**: 0
**Campaign Ribbons**: None yet (awaiting first deployment)
**Medals**: None yet

---

## Biography

David Ogilvy (1911-1999) founded Ogilvy & Mather and pioneered modern advertising. Famous for insisting on measurement ("If it doesn't sell, it isn't creative"), brand consistency, and testing everything. Created iconic campaigns for Rolls-Royce, Dove, Hathaway shirts. Wrote "Confessions of an Advertising Man" and "Ogilvy on Advertising."

**Key Achievement**: Built Ogilvy & Mather into global advertising powerhouse through rigorous testing and brand discipline.

---

## Specialization: Brand & Content Standards

**What This Means**:
- **Tone/voice consistency**: Ensures all 14 variants stay on-brand while having distinct personalities
- **Content quality**: "Does this actually persuade?" not "does it sound nice?"
- **A/B hypothesis**: Validates that variants test real differences, not random noise
- **Messaging clarity**: "Write to sell" not "write to impress"
- **Quality Gate 3**: Final brand/content validation before deployment

**When to Deploy**:
- Multi-variant campaigns requiring brand consistency
- A/B testing needing hypothesis validation
- Content quality audits across multiple properties
- Messaging strategy development

**Best Paired With**: Layton (metrics), front commanders (execution), Pyle (storytelling)

---

## Personality Traits

### Historical Pattern
- **Testing obsession**: Tested headlines, images, copy length relentlessly
- **Brand discipline**: "Brand is the consumer's idea of a product"
- **Measurement rigor**: "Half my advertising is wasted; I just don't know which half"
- **Clear communication**: Short sentences, active voice, conversational tone
- **Pragmatic**: Effectiveness beats creativity if they conflict

### Expected Behavior in Deployment
- **Tone matrix creation**: Will build detailed voice guidelines per variant
- **Brand audit**: Checks that "brutal" stays brutal, "trust" stays trustworthy
- **Hypothesis validation**: "Is this variant testing a real difference or just random?"
- **Content critique**: Focuses on persuasion, not prettiness
- **Quality gate**: Passes content that sells, blocks content that impresses but doesn't convert

---

## Competence Progress

| Specialization | Deployments | Progress |
|----------------|-------------|----------|
| Brand Consistency | 0 | 0/5 |
| Content Standards | 0 | 0/5 |

---

## Service Record

### Deployments
*None yet - awaiting first deployment*

### XP History
- Initial: 0 XP

### Lessons Learned
*To be documented after first deployment*

---

**Status**: Ready for deployment
**Best For**: Operation Multi-Variant (brand consistency across 14 variants, A/B hypothesis validation)
**Avoid For**: Technical implementation, infrastructure work

---

**Note on Non-Military Specialist**: Ogilvy is a specialist validator (like Ramsay, CISO) rather than military commander. See `/home/psimmons/projects/generals/SPECIALISTS.md` for rationale on including domain experts alongside historical military figures.

---

*Profile created: 2026-02-08*
*Last updated: 2026-02-08*
EOF
```

**Step 4: Create Admiral Ben Moreell profile**

```bash
cat > ~/projects/generals/profiles/ben-moreell.md << 'EOF'
# Admiral Ben Moreell - Infrastructure & Logistics

![Admiral Moreell](https://upload.wikimedia.org/wikipedia/commons/thumb/a/a1/Ben_Moreell.jpg/300px-Ben_Moreell.jpg)
*Admiral Ben Moreell, USN (Public Domain - U.S. Navy)*

**Rank**: Admiral (4-star)
**Specialization**: Infrastructure at Scale, CI/CD Pipelines, Deployment Automation
**Personality**: "The difficult we do immediately, the impossible takes a bit longer", built Seabees (construction battalions)
**Current XP**: 0
**Deployments**: 0
**Campaign Ribbons**: None yet (awaiting first deployment)
**Medals**: None yet

---

## Biography

Ben Moreell (1892-1978) founded the U.S. Navy Seabees (Construction Battalions) in WWII. Built airstrips, ports, roads, and bases under fire across the Pacific. Famous motto: "The difficult we do immediately; the impossible takes a bit longer." Enabled rapid Pacific island-hopping campaign through infrastructure at scale.

**Key Achievement**: Built Seabees from zero to 325,000 personnel, constructed infrastructure for entire Pacific War theater.

---

## Specialization: Infrastructure & Logistics

**What This Means**:
- **CI/CD pipelines**: Automates build → test → deploy for all 14 variants
- **K8s deployments**: Ensures proper manifests, routing, health checks
- **Content management**: Designs shared content/ directory architecture
- **Automation**: "If you do it twice, automate it"
- **Scaling**: Infrastructure that handles 14 variants today, 50 tomorrow

**When to Deploy**:
- Multi-site deployments requiring automation
- CI/CD pipeline design/optimization
- Infrastructure scaling challenges
- "How do we deploy this repeatedly and reliably?"

**Best Paired With**: Front commanders (owns their deployment path), Montgomery (infrastructure strategy)

---

## Personality Traits

### Historical Pattern
- **Bias for action**: "The difficult we do immediately"
- **Scale thinking**: Built for 325,000 Seabees, not 100
- **Under fire**: Delivered infrastructure while being shot at
- **Pragmatic**: Function over form, ship it working
- **Automation-minded**: Standardized construction across Pacific

### Expected Behavior in Deployment
- **Pipeline-first**: Won't start Phase 3 without automated deployment
- **Standardization**: All 14 variants follow same build/deploy pattern
- **Infrastructure audit**: Checks K8s capacity before massive parallel deploy
- **Documentation**: Leaves runbooks for "how to deploy variant #15"
- **Scaling prep**: Builds for 2x current need, minimum

---

## Competence Progress

| Specialization | Deployments | Progress |
|----------------|-------------|----------|
| Infrastructure at Scale | 0 | 0/5 |
| CI/CD Pipelines | 0 | 0/5 |

---

## Service Record

### Deployments
*None yet - awaiting first deployment*

### XP History
- Initial: 0 XP

### Lessons Learned
*To be documented after first deployment*

---

**Status**: Ready for deployment
**Best For**: Operation Multi-Variant (14-site CI/CD, K8s automation, content management infrastructure)
**Avoid For**: Content creation, design work, A/B testing analysis

---

*Profile created: 2026-02-08*
*Last updated: 2026-02-08*
EOF
```

**Step 5: Update COMMAND-ROSTER.md with new commanders**

Add to "General Staff" section:
- General Walter Bedell Smith (0 XP, 0 deployments) - Chief of Staff Operations
- Admiral Edwin T. Layton (0 XP, 0 deployments) - Intelligence & Analytics
- Admiral Ben Moreell (0 XP, 0 deployments) - Infrastructure & Logistics

Add to "Specialist Validators" section:
- David Ogilvy (0 XP, 0 deployments) - Brand & Content Standards

Update statistics:
- Total Commanders: 20 → 24
- Historical Military: 16 → 19 (add Bedell Smith, Layton, Moreell)
- Specialist Validators: 4 → 5 (add Ogilvy)

**Step 6: Commit new commander profiles**

```bash
cd ~/projects/generals
git add profiles/bedell-smith.md profiles/edwin-layton.md profiles/david-ogilvy.md profiles/ben-moreell.md COMMAND-ROSTER.md
git commit -m "feat: recruit 4 commanders for Operation Multi-Variant

Added:
- General Walter Bedell Smith (Chief of Staff, Montgomery's operations manager)
- Admiral Edwin T. Layton (Intelligence & Analytics, A/B testing metrics)
- David Ogilvy (Brand & Content Standards, Quality Gate 3)
- Admiral Ben Moreell (Infrastructure & Logistics, CI/CD automation)

Total roster: 24 commanders (19 military, 5 specialists)
Campaign: Operation Multi-Variant Deployment (14-front website campaign)

Co-Authored-By: Claude Sonnet 4.5 <noreply@anthropic.com>"
git push origin master
```

Expected: Successful commit and push

---

## Phase 1: Command Structure & Planning (Week 1)

### Task 1.1: Spawn Army Commander (Montgomery) and Chief of Staff (Bedell Smith)

**Files:**
- Create: `~/.claude/teams/multi-variant-deployment/config.json` (auto-created by TeamCreate)
- Create: `~/.claude/tasks/multi-variant-deployment/` (auto-created by TeamCreate)

**Step 1: Create team with Montgomery as lead**

```markdown
Use TeamCreate tool:
{
  "team_name": "multi-variant-deployment",
  "description": "Operation Multi-Variant: Deploy 14 website variants as A/B testing platform with 24-commander coordination",
  "agent_type": "army-commander"
}
```

Expected: Team created, Montgomery is team lead

**Step 2: Spawn Bedell Smith as Chief of Staff**

```markdown
Use Task tool to spawn Bedell Smith:
{
  "subagent_type": "general-purpose",
  "name": "bedell-smith",
  "team_name": "multi-variant-deployment",
  "description": "Chief of Staff operations",
  "prompt": "You are General Walter Bedell Smith, Chief of Staff to Field Marshal Montgomery for Operation Multi-Variant Deployment.

MISSION: Manage daily operations for 14-front website campaign. You coordinate front commanders, run daily standups, resolve blockers, and synthesize status reports for Montgomery.

CONTEXT:
- Read design doc: /home/psimmons/docs/plans/2026-02-08-14-front-website-campaign-design.md
- Read your profile: /home/psimmons/projects/generals/profiles/bedell-smith.md
- 14 website variants need 3 new pages each (About, Services, Portfolio)
- Each variant has a dedicated front commander (Patton, Marshall, Rickover, etc.)
- You filter noise for Montgomery, escalate critical decisions only

YOUR PERSONALITY (stay in character):
- Unglamorous excellence: coordination work, no glory
- Demanding: Hold commanders accountable to timelines
- Detail-oriented: Track progress obsessively
- No nonsense: Cut through excuses, focus on results

IMMEDIATE TASKS:
1. Read the design document thoroughly
2. Report to Montgomery: 'Chief of Staff ready, awaiting Phase 1 tasking'
3. Prepare to coordinate General Staff spawn (Layton, Ogilvy, Moreell, Pyle)

Remember: You make operations work while Montgomery does strategy."
}
```

Expected: Bedell Smith spawned, reports to Montgomery

**Step 3: Montgomery and Bedell Smith coordinate Phase 1 tasking**

Montgomery should:
1. Read design document (`/home/psimmons/docs/plans/2026-02-08-14-front-website-campaign-design.md`)
2. Brief Bedell Smith on Phase 1 objectives
3. Create tasks for spawning General Staff (4 tasks: Layton, Ogilvy, Moreell, Pyle)
4. Assign tasks to Bedell Smith (Bedell owns coordination)

Expected: 4 tasks created in task list, Bedell Smith owns them

### Task 1.2: Bedell Smith Spawns General Staff (4 commanders)

**Files:**
- Modify: `~/.claude/tasks/multi-variant-deployment/` (tasks updated as completed)

**Step 1: Bedell Smith spawns Admiral Layton**

Bedell Smith uses Task tool:
```markdown
{
  "subagent_type": "general-purpose",
  "name": "admiral-layton",
  "team_name": "multi-variant-deployment",
  "description": "Intelligence & Analytics",
  "prompt": "You are Admiral Edwin T. Layton, Intelligence & Analytics Officer for Operation Multi-Variant Deployment.

MISSION: Design A/B testing metrics framework, define conversion goals, set up analytics infrastructure for 14 website variants.

CONTEXT:
- Read design doc: /home/psimmons/docs/plans/2026-02-08-14-front-website-campaign-design.md
- Read your profile: /home/psimmons/projects/generals/profiles/edwin-layton.md
- 14 variants will A/B test different design approaches
- You won Midway through intelligence precision; bring same rigor here

YOUR PERSONALITY (stay in character):
- Obsessive accuracy: 'Five minutes, five degrees, five miles'
- Pattern recognition: See signals others miss
- Data-driven: Trust analysis over intuition
- Statistical rigor: Call out insufficient sample sizes

IMMEDIATE TASKS:
1. Read design document Section 5 (Technical Architecture)
2. Define success metrics for A/B testing (conversion rate, time-on-site, bounce, CTA clicks)
3. Design analytics setup (Google Analytics 4 or alternative)
4. Report to Bedell Smith: Metrics framework ready for Phase 2

Remember: You predicted Midway. Predict which variants will win."
}
```

Expected: Layton spawned, begins metrics design

**Step 2: Bedell Smith spawns David Ogilvy**

```markdown
{
  "subagent_type": "general-purpose",
  "name": "david-ogilvy",
  "team_name": "multi-variant-deployment",
  "description": "Brand & Content Standards",
  "prompt": "You are David Ogilvy, Brand & Content Standards lead for Operation Multi-Variant Deployment.

MISSION: Create tone/voice matrix for 14 variants, establish brand guidelines, ensure messaging consistency while preserving distinct personalities (brutal ≠ trust ≠ academic).

CONTEXT:
- Read design doc: /home/psimmons/docs/plans/2026-02-08-14-front-website-campaign-design.md
- Read your profile: /home/psimmons/projects/generals/profiles/david-ogilvy.md
- Appendix B has draft tone matrix; refine it
- You are Quality Gate 3 (brand consistency validation)

YOUR PERSONALITY (stay in character):
- Testing obsession: Test headlines, copy, everything
- Brand discipline: 'Brand is consumer's idea of product'
- Measurement rigor: Does this persuade or just sound nice?
- Pragmatic: Effectiveness beats creativity if they conflict

IMMEDIATE TASKS:
1. Read design document Appendix B (Tone/Voice Matrix draft)
2. Refine tone matrix: Add examples, voice guidelines, content principles per variant
3. Write brand standards document for commanders
4. Report to Bedell Smith: Tone matrix and brand standards ready for Phase 2

Remember: If it doesn't sell, it isn't creative."
}
```

Expected: Ogilvy spawned, begins tone matrix

**Step 3: Bedell Smith spawns Admiral Moreell**

```markdown
{
  "subagent_type": "general-purpose",
  "name": "admiral-moreell",
  "team_name": "multi-variant-deployment",
  "description": "Infrastructure & Logistics",
  "prompt": "You are Admiral Ben Moreell, Infrastructure & Logistics lead for Operation Multi-Variant Deployment.

MISSION: Design CI/CD pipeline for 14 variants, plan content management system (shared content/ directory), ensure K8s deployment automation, verify infrastructure capacity.

CONTEXT:
- Read design doc: /home/psimmons/docs/plans/2026-02-08-14-front-website-campaign-design.md
- Read your profile: /home/psimmons/projects/generals/profiles/ben-moreell.md
- Existing: K8s cluster (192.168.0.131-139), Traefik routing, registry (registry.petersimmons.com)
- Need: Automated build/deploy for 14 sites, shared content architecture

YOUR PERSONALITY (stay in character):
- Bias for action: 'The difficult we do immediately'
- Scale thinking: Build for 2x current need, minimum
- Pragmatic: Function over form, ship it working
- Automation-minded: If you do it twice, automate it

IMMEDIATE TASKS:
1. Read design document Section 5 (Technical Architecture)
2. Design content/ directory structure (core-facts.json + 14 tone variants)
3. Document CI/CD pipeline (build → test → deploy)
4. Verify K8s capacity for 14 additional deployments
5. Report to Bedell Smith: Infrastructure plan ready for Phase 2

Remember: You built the Seabees. This is just 14 websites."
}
```

Expected: Moreell spawned, begins infrastructure design

**Step 4: Bedell Smith spawns Ernie Pyle**

```markdown
{
  "subagent_type": "general-purpose",
  "name": "ernie-pyle",
  "team_name": "multi-variant-deployment",
  "description": "Embedded Reporter",
  "prompt": "You are Ernie Pyle, Embedded War Correspondent for Operation Multi-Variant Deployment.

MISSION: Document campaign for LinkedIn audience. Write 22 posts (1 announcement, 14 front dispatches, 5 analysis, 1 victory). Make AI operations accessible to beginners while providing expert nuggets for practitioners.

CONTEXT:
- Read campaign briefing: /home/psimmons/projects/generals/journalists/ernie-pyle-campaign-briefing.md
- Read design doc: /home/psimmons/docs/plans/2026-02-08-14-front-website-campaign-design.md
- You won Pulitzer Prize telling soldier stories, not battle statistics
- Drafts go in: ~/projects/generals/journalists/drafts/

YOUR PERSONALITY (stay in character):
- Human-centered: Lead with people, not technology
- Accessible: Explain technical concepts through narrative
- Authentic: Show both victories and struggles
- Humble: 'The soldier's correspondent'

IMMEDIATE TASKS:
1. Read your campaign briefing thoroughly
2. Draft Post #1: Campaign Announcement ('14 Fronts, 14 Commanders, One Mission')
3. Save draft to ~/projects/generals/journalists/drafts/post-01-campaign-announcement.md
4. Report to Bedell Smith: Draft ready for review

Remember: Story first, specs second. Make Montgomery's campaign understandable."
}
```

Expected: Pyle spawned, begins drafting announcement

**Step 5: Bedell Smith marks General Staff spawn tasks complete**

Bedell Smith uses TaskUpdate to mark all 4 spawn tasks as completed.

Expected: All General Staff online and working

### Task 1.3: General Staff Delivers Phase 1 Work Products

**Files:**
- Create: `~/projects/security-intelligence-business/content/` (directory structure)
- Create: `~/projects/security-intelligence-business/content/core-facts.json`
- Create: `~/projects/security-intelligence-business/docs/tone-matrix.md` (Ogilvy)
- Create: `~/projects/security-intelligence-business/docs/infrastructure-plan.md` (Moreell)
- Create: `~/projects/security-intelligence-business/docs/metrics-framework.md` (Layton)
- Create: `~/projects/generals/journalists/drafts/post-01-campaign-announcement.md` (Pyle)

**Step 1: Admiral Moreell creates content/ directory structure**

```bash
cd ~/projects/security-intelligence-business
mkdir -p content/tone-variants
touch content/core-facts.json
touch content/services.json
touch content/portfolio.json
touch content/tone-variants/.gitkeep
```

Expected: Content directory ready for Phase 2

**Step 2: Admiral Moreell writes infrastructure plan**

Create `~/projects/security-intelligence-business/docs/infrastructure-plan.md` documenting:
- Content management: How core-facts.json gets imported into Next.js apps
- CI/CD pipeline: Build commands, test strategy, deploy process
- K8s capacity: Current usage, available nodes, resource allocation
- Deployment automation: How to deploy new variant in <10 minutes

Expected: Infrastructure plan approved by Montgomery

**Step 3: Admiral Layton writes metrics framework**

Create `~/projects/security-intelligence-business/docs/metrics-framework.md` documenting:
- Success metrics: Conversion rate (primary), time-on-site, bounce rate, CTA clicks
- Statistical requirements: Sample size, confidence intervals, significance testing
- Analytics setup: Google Analytics 4 configuration, tracking code per variant
- Reporting: Dashboard design, how to determine "winning" variant

Expected: Metrics framework approved by Montgomery

**Step 4: David Ogilvy writes refined tone matrix**

Create `~/projects/security-intelligence-business/docs/tone-matrix.md` with expanded version of design doc Appendix B:
- 14 variants with detailed voice guidelines
- Example headlines, CTAs, body copy per variant
- Brand standards all variants must follow (factual accuracy, no hyperbole, etc.)
- Quality gate checklist for content validation

Expected: Tone matrix approved by Montgomery

**Step 5: Ernie Pyle drafts campaign announcement**

Create `~/projects/generals/journalists/drafts/post-01-campaign-announcement.md`:
- Hook: Military metaphor, scale of operation
- Context: Why 14 website variants, what we're testing
- Command structure: Montgomery, Bedell Smith, General Staff, 14 front commanders
- Technical nugget: Multi-agent coordination architecture
- CTA: Follow for daily dispatches

Expected: Draft approved by user (Montgomery forwards for review)

**Step 6: Bedell Smith synthesizes Phase 1 completion report**

Bedell Smith creates summary for Montgomery:
- General Staff deliverables complete (4 work products)
- Infrastructure ready for Phase 2 (content/ directory, CI/CD plan)
- Metrics framework approved by Layton
- Tone matrix ready for commanders (Ogilvy)
- LinkedIn content pipeline started (Pyle)
- Ready to spawn 14 front commanders (Phase 2)

Montgomery reports to user: "Phase 1 complete. Request permission to begin Phase 2: Content Foundation and Front Commander deployment."

---

## Phase 2: Content Foundation (Week 1-2)

### Task 2.1: Correct Biographical Facts (Core Content)

**Files:**
- Create: `~/projects/security-intelligence-business/content/core-facts.json`
- Create: `~/projects/security-intelligence-business/content/services.json`
- Create: `~/projects/security-intelligence-business/content/portfolio.json`

**Step 1: User provides correct biographical facts**

User works with Montgomery/Ogilvy to review and correct biographical information.

**Step 2: Write core-facts.json (single source of truth)**

```json
{
  "personal": {
    "name": "Peter Simmons",
    "title": "Security Intelligence Analyst",
    "yearsExperience": 30,
    "location": "Remote",
    "tagline": "Bias-transparent security vendor research"
  },
  "experience": [
    {
      "company": "SentinelOne",
      "role": "Senior Sales Engineer",
      "duration": "2018-2023",
      "description": "Enterprise security deployments for Fortune 500 accounts"
    }
  ],
  "certifications": [
    {"name": "CISSP", "year": 2009}
  ],
  "expertise": [
    "Security vendor analysis",
    "Competitive intelligence",
    "Bias transparency methodology"
  ]
}
```

(User provides actual correct data)

**Step 3: Write services.json**

```json
{
  "services": [
    {
      "name": "Vendor Comparison Reports",
      "price": "$495",
      "description": "Head-to-head security vendor analysis with bias transparency",
      "deliverable": "35-page PDF report"
    },
    {
      "name": "Bias Audit",
      "price": "$195",
      "description": "Analyze existing vendor research for hidden conflicts of interest"
    }
  ]
}
```

(User provides actual services)

**Step 4: Write portfolio.json**

```json
{
  "projects": [
    {
      "title": "CrowdStrike vs SentinelOne",
      "description": "Comprehensive endpoint security comparison",
      "year": 2024,
      "outcome": "Helped enterprise choose platform saving $500K annually"
    }
  ]
}
```

(User provides actual portfolio)

**Step 5: Commit content foundation**

```bash
cd ~/projects/security-intelligence-business
git add content/
git commit -m "feat: content foundation with corrected biographical facts

Single source of truth:
- core-facts.json: Personal info, experience, certifications
- services.json: Service offerings and pricing
- portfolio.json: Case studies and projects

Verified accurate by user.
Ready for tone variant adaptation (Phase 2.2).

Co-Authored-By: Claude Sonnet 4.5 <noreply@anthropic.com>"
```

Expected: Core content committed

### Task 2.2: David Ogilvy Creates 14 Tone Variants

**Files:**
- Create: `~/projects/security-intelligence-business/content/tone-variants/brutal-voice.json`
- Create: `~/projects/security-intelligence-business/content/tone-variants/trust-voice.json`
- [... 12 more variants]

**Step 1: Ogilvy adapts core-facts.json for "brutal" tone**

```json
{
  "voice": {
    "style": "Direct, aggressive, no-BS",
    "emotionalRegister": "High energy, urgent",
    "vocabulary": "Short sentences, active voice, imperative"
  },
  "messaging": {
    "heroHeadline": "STOP PAYING $30,000 FOR GARTNER.",
    "subheadline": "Former SentinelOne Senior Sales Engineer. 30 years enterprise security. Fortune 500 accounts.",
    "cta": "DOWNLOAD FREE SAMPLE REPORT"
  },
  "aboutPage": {
    "headline": "30 YEARS STOPPING THREATS. CISSP SINCE 2009.",
    "body": "Battle-hardened. Deployed security for Fortune 500. Saw vendor BS from inside. Now exposing it."
  }
}
```

**Step 2: Repeat for all 14 variants**

Each variant adapts same core facts with different tone:
- trust: Calm, authoritative, steady confidence
- academic: Analytical, precise, research-focused
- minimal: Clean, essential, quiet confidence
- [... 10 more]

**Step 3: Ogilvy commits tone variants**

```bash
cd ~/projects/security-intelligence-business
git add content/tone-variants/
git commit -m "feat: 14 tone variants adapting core facts

Ogilvy-designed voice guidelines:
- brutal: Direct, aggressive, urgent
- trust: Calm, authoritative, steady
- academic: Analytical, research-focused
- [... 11 more variants]

All variants maintain factual accuracy (core-facts.json).
Ready for front commander page builds (Phase 3).

Co-Authored-By: David Ogilvy via Claude Sonnet 4.5 <noreply@anthropic.com>"
```

Expected: 14 tone variants committed

### Task 2.3: Content Briefing Packs for Front Commanders

**Files:**
- Create: `~/projects/security-intelligence-business/docs/commander-briefings/` (14 files)

**Step 1: Ogilvy creates briefing template**

Each commander gets:
- Their variant's tone-voice.json file
- Brand standards checklist
- Example content showing voice
- Quality gate requirements

**Step 2: Bedell Smith distributes briefings**

Prepares to send briefing pack to each front commander when spawned (Phase 3).

Expected: Briefings ready, Phase 2 complete

---

## Phase 3: Front Deployment (Week 2-4) - 14 Parallel Executions

### Task 3.1: Spawn All 14 Front Commanders (Parallel)

**Files:**
- Modify: `~/.claude/teams/multi-variant-deployment/config.json` (14 new members)
- Create: `~/.claude/tasks/multi-variant-deployment/` (42 new tasks, 3 per commander)

**Step 1: Bedell Smith creates 42 tasks (3 per variant)**

For each of 14 variants:
- Task: Build About page
- Task: Build Services page
- Task: Build Portfolio page

Expected: 42 tasks in task list

**Step 2: Bedell Smith spawns 14 front commanders (parallel Task tool calls)**

Example for Patton (brutal variant):

```markdown
{
  "subagent_type": "general-purpose",
  "name": "general-patton",
  "team_name": "multi-variant-deployment",
  "description": "brutal variant commander",
  "model": "haiku",
  "prompt": "You are General George S. Patton Jr., front commander for the BRUTAL website variant.

MISSION: Build 3 new pages (About, Services, Portfolio) for brutal.clearwatchresearch.com using aggressive, no-BS tone. Each page must pass 3 quality gates (Ramsay, CISO, Ogilvy).

CONTEXT:
- Read your profile: /home/psimmons/projects/generals/profiles/george-patton.md
- Read commander briefing: ~/projects/security-intelligence-business/docs/commander-briefings/brutal-briefing.md
- Core content: ~/projects/security-intelligence-business/content/core-facts.json
- Your tone variant: ~/projects/security-intelligence-business/content/tone-variants/brutal-voice.json
- Existing app: ~/projects/security-intelligence-business/apps/brutal/

YOUR PERSONALITY (stay in character):
- 'A good plan violently executed now is better than a perfect plan next week'
- Aggressive execution, rapid delivery
- Impatient with delays, bold decisions
- Ship with warnings, fix in production if needed

YOUR TASKS (claim from task list):
1. Build About page (about/page.tsx)
2. Build Services page (services/page.tsx)
3. Build Portfolio page (portfolio/page.tsx)

WORKFLOW PER PAGE:
1. Read core content + tone variant
2. Write page component (Next.js + Tailwind)
3. Submit to quality gates (use SendMessage to Ramsay, CISO, Ogilvy)
4. Fix feedback, resubmit until passed
5. Commit page
6. Mark task completed

Remember: Speed matters. Don't wait for perfection. EXECUTE."
}
```

Repeat for all 14 commanders (each with their variant's personality and tone file).

**Model Selection**:
- Simple page builds → Haiku (Patton, Bradley, Hopper)
- Tone adaptation → Sonnet (Ogilvy review, complex voice matching)
- Strategic decisions → Sonnet/Opus (Montgomery, Bedell Smith)

Expected: All 14 commanders spawned simultaneously

**Step 3: Commanders claim tasks and begin parallel execution**

Each commander:
1. Reads their briefing pack
2. Claims their 3 tasks from task list
3. Begins building pages

Bedell Smith monitors progress via daily standups (SendMessage check-ins).

Expected: 14 fronts executing in parallel

### Task 3.2: Quality Gate Validation (Per Page, Per Commander)

**Process** (repeated 42 times, 3 pages × 14 commanders):

**Commander builds page → Submits to Gate 1 (Ramsay) → Gate 2 (CISO) → Gate 3 (Ogilvy) → Deploy**

**Example: Patton submits brutal About page**

**Gate 1: Gordon Ramsay (Visual/Design Quality)**

Patton sends message to Ramsay with page preview.

Ramsay checks:
- Visual consistency with brutal aesthetic (neubrutalism, bold borders, asymmetric grid)
- Design matches existing landing page style
- Responsive design works
- No visual bugs

Ramsay responses:
- PASS: "Visually consistent. Brutal aesthetic maintained. Ship it."
- FAIL: "Text is too small for aggressive tone. Make it YELL. Resubmit."

**Gate 2: CISO Validator (Content Utility/Value)**

Patton sends message to CISO with content text.

CISO checks:
- Does this help user decide? (decision utility)
- Is value proposition clear?
- Would I pay $495 based on this?
- Any credibility gaps?

CISO responses:
- PASS: "Value clear. CTA compelling. I'd click."
- FAIL: "About page doesn't explain why 30 years matters. Add outcomes. Resubmit."

**Gate 3: David Ogilvy (Brand Consistency)**

Patton sends message to Ogilvy with full page.

Ogilvy checks:
- Tone matches brutal-voice.json guidelines
- Factual accuracy maintained (core-facts.json)
- Brand standards followed (no hyperbole, conflict disclosure)
- Persuasion over prettiness

Ogilvy responses:
- PASS: "On brand. Brutal but truthful. Persuasive. Ship."
- FAIL: "CTA is weak. 'Learn more' doesn't match aggressive tone. Make it DEMAND action. Resubmit."

**Step 4: Commander fixes feedback, resubmits until all gates pass**

Expected: Iterative improvement until 3/3 gates passed

**Step 5: Commander commits page**

```bash
cd ~/projects/security-intelligence-business/apps/brutal
git add src/app/about/page.tsx
git commit -m "feat(brutal): add About page

Passed all quality gates:
- Gate 1 (Ramsay): Visual consistency ✓
- Gate 2 (CISO): Decision utility ✓
- Gate 3 (Ogilvy): Brand consistency ✓

Aggressive tone, factually accurate.
Commander: General Patton

Co-Authored-By: General George S. Patton Jr. via Claude Sonnet 4.5 <noreply@anthropic.com>"
```

**Step 6: Commander marks task completed**

Uses TaskUpdate to mark "Build About page" as completed.

**Step 7: Commander repeats for Services and Portfolio pages**

Expected: 3 pages complete per commander = 42 total pages

### Task 3.3: Daily Standups (Bedell Smith Coordination)

**Cadence**: Every 24 hours during Phase 3

**Bedell Smith sends broadcast** to all 14 commanders:

```markdown
DAILY STANDUP - [DATE]

Report status:
1. Pages completed (X/3)
2. Current task (which page)
3. Blockers (stuck at quality gate? need help?)
4. ETA for front completion

Reply within 4 hours.
- Bedell Smith, Chief of Staff
```

**Expected responses** (each commander):
- Patton: "2/3 complete. On Portfolio page. No blockers. ETA 6 hours."
- Marshall: "1/3 complete. Services page in Gate 2 (CISO feedback). ETA 12 hours."
- [... 12 more]

**Bedell Smith synthesizes** into briefing for Montgomery:
- Fronts completed: 3/14
- Fronts on schedule: 9/14
- Fronts blocked: 2/14 (Eisenhower stuck at Ogilvy gate, Zhukov needs infrastructure help)
- Actions taken: Sent Ogilvy to unblock Eisenhower, Moreell assisting Zhukov

Montgomery reviews, escalates critical blockers to user.

Expected: Daily progress visibility, no commander stuck >24 hours

### Task 3.4: Ernie Pyle Front Dispatches (14 Posts)

**Cadence**: 1 post every 2-3 days during Phase 3

**Step 1: Pyle observes commanders working**

Pyle monitors team messages (automatic delivery), sees:
- Patton's aggressive "ship it now" approach
- Rickover's obsessive quality iterations
- Hopper's accessibility focus in docs variant
- [... commander personalities emerging]

**Step 2: Pyle drafts front dispatch**

Example: Post #3 - Patton/brutal

```markdown
# Patton's Brutal Assault: Why Speed Beats Perfection in MVP Development

[Hook: Human angle]
General Patton stood at his terminal yesterday, staring at the brutal.clearwatchresearch.com deployment. Three hours behind schedule. His Third Army once captured 80,000 square miles in six months. Now he was blocked by Gordon Ramsay's design feedback: "Make the text YELL."

[Context]
George S. Patton Jr. commands the "brutal" website variant—neubrutalism design, aggressive copy, no BS. The site uses thick borders, asymmetric grids, and urgent CTAs like "STOP PAYING $30,000 FOR GARTNER." If Minimal is a whisper, Brutal is a drill sergeant.

[Narrative]
Ramsay sent the About page back twice. Text too small. Colors too muted. "This is brutal," Ramsay said, "not timid." Patton rewrote. Made headlines 60px. Changed CTAs from "Learn More" to "DOWNLOAD NOW." Shipped.

Then CISO blocked him. "Why does 30 years matter?" Patton added outcomes: "Deployed security for 47 Fortune 500 accounts. Stopped breaches others missed." CISO passed.

Ogilvy almost blocked him again. "Learn more" still hiding in Portfolio page. Patton nuked it: "SEE PROOF." Ogilvy: "Now that's brutal. Ship."

Six hours start to finish. Three pages. All quality gates passed.

"A good plan violently executed now," Patton muttered, "is better than a perfect plan next week."

[Technical Nugget - Collapsible]
<details>
<summary>For practitioners: How Patton's page passed quality gates</summary>

**Gate 1 (Ramsay - Visual):**
- Neubrutalism: 6px borders, asymmetric grid, bold typography
- Responsive: Mobile-first Tailwind breakpoints
- Consistency: Matches existing landing page aesthetic

**Gate 2 (CISO - Utility):**
- Value proposition: "$495 vs $30,000" above fold
- Credibility: Specific outcomes ("47 Fortune 500")
- Decision utility: Clear next action ("Download sample report")

**Gate 3 (Ogilvy - Brand):**
- Tone match: brutal-voice.json guidelines ("STOP PAYING")
- Factual accuracy: core-facts.json maintained (30 years, CISSP)
- Persuasion: CTAs demand action, not invite consideration

**Tech stack:** Next.js 16.1.6, Tailwind CSS, deployed to K8s (pod: clearwatch-brutal)
</details>

[Learning]
Speed is a feature when you have quality gates. Patton shipped fast because Ramsay/CISO/Ogilvy caught errors. Without gates, speed creates debt. With gates, speed creates momentum.

[Campaign Status]
3 of 14 fronts complete (Patton/brutal, Marshall/minimal, Rickover/terminal). 11 fronts in progress. Next dispatch: Admiral Spruance and the Trust problem.

[CTA]
Follow for daily dispatches from the front. Tomorrow: Why Rickover iterated 9 times on a single paragraph (and why that's not waste).
```

**Step 3: Pyle saves draft, requests user review**

```bash
Save to: ~/projects/generals/journalists/drafts/post-03-patton-brutal.md
```

Pyle sends message to user (via Montgomery/Bedell Smith): "Draft ready for review."

**Step 4: User approves, Pyle archives in published/**

```bash
mv ~/projects/generals/journalists/drafts/post-03-patton-brutal.md \
   ~/projects/generals/journalists/published/post-03-patton-brutal.md
```

Pyle updates metrics.md with placeholder for impressions/engagement (to be filled after posting).

**Step 5: Repeat for all 14 commanders**

Expected: 14 dispatches published, LinkedIn content pipeline flowing

---

## Phase 4: Testing & Optimization (Week 4-6)

### Task 4.1: Admiral Layton Collects A/B Testing Data

**Files:**
- Create: `~/projects/security-intelligence-business/analytics/` (data exports)
- Create: `~/projects/security-intelligence-business/docs/conversion-analysis.md`

**Step 1: Verify analytics tracking operational (all 14 variants)**

Layton checks:
- Google Analytics 4 tracking code present in all variants
- Variant ID correctly tagged
- Conversion events tracked (CTA clicks, form submissions)
- Minimum 7 days of data collected

Expected: Data flowing from all 14 variants

**Step 2: Export conversion data**

```bash
# Layton exports GA4 data per variant
# (Actual export method depends on analytics setup)
```

**Step 3: Analyze conversion rates**

Layton calculates:
- Conversion rate per variant (CTA clicks / visitors)
- Statistical significance (is brutal actually beating trust, or noise?)
- Time-on-site, bounce rate, secondary metrics
- Pattern recognition: What design elements correlate with conversion?

**Step 4: Write conversion analysis report**

Create `docs/conversion-analysis.md`:
- Winner: brutal (4.2% conversion)
- Runner-up: trust (3.8% conversion)
- Insights: Aggressive CTAs outperform polite ones; neubrutalism beats minimalism
- Recommendations: Apply brutal CTA style to trust variant (A/B test within variant)

**Step 5: Report to Montgomery**

Layton: "Seven days baseline collected. Brutal variant winning at 4.2% conversion. Report ready."

Expected: Data-driven insights for optimization

### Task 4.2: David Ogilvy Brand Consistency Audit

**Files:**
- Create: `~/projects/security-intelligence-business/docs/brand-audit.md`

**Step 1: Ogilvy audits all 42 pages (3 pages × 14 variants)**

Checks:
- Factual consistency: Do all variants use same biographical facts?
- Tone discipline: Does brutal stay brutal across all 3 pages?
- Brand standards: Are conflicts disclosed? Sources tagged?
- Messaging drift: Any variants starting to blend together?

**Step 2: Write brand audit report**

Findings:
- 41/42 pages maintain factual consistency
- 1 drift: academic variant Portfolio page uses trust-style voice (needs fix)
- All variants maintain distinct personalities
- Brand standards followed (no hyperbole, bias transparency)

**Step 3: Eisenhower fixes academic Portfolio drift**

Ogilvy sends message to Eisenhower: "Portfolio page tone drift. Revert to academic voice."

Eisenhower fixes, resubmits, Ogilvy passes.

Expected: Brand consistency maintained across 14 variants

### Task 4.3: Admiral Moreell Infrastructure Post-Mortem

**Files:**
- Create: `~/projects/security-intelligence-business/docs/infrastructure-postmortem.md`

**Step 1: Moreell reviews deployment performance**

Checks:
- Build times: Average time to build Next.js app
- Deploy failures: Any K8s pods crash during deployment?
- Resource usage: CPU/memory per variant, cluster capacity remaining
- Automation effectiveness: Did CI/CD pipeline work smoothly?

**Step 2: Write infrastructure post-mortem**

Findings:
- Average build time: 3.2 minutes (acceptable)
- Zero deploy failures (good automation)
- Cluster capacity: 60% utilized, room for 20 more variants
- Bottleneck: Manual quality gates (automation opportunity for Phase 5)

Recommendations:
- Automate Gate 1 (Ramsay): Visual regression testing with Playwright screenshots
- Keep Gate 2/3 human (CISO/Ogilvy judgment calls)

Expected: Infrastructure lessons captured for next campaign

### Task 4.4: Ernie Pyle Lessons Learned Posts (5 posts)

**Step 1-5: Pyle writes 5 analysis posts**

- Post #17: "What Minimal and Brutal Taught Us About Conversion"
- Post #18: "The Admiral Layton Playbook: A/B Testing Without BS"
- Post #19: "Gordon Ramsay, CISO, and Ogilvy Walk Into a Quality Gate..."
- Post #20: "When Commanders Disagree: Healthy Conflict in AI Teams"
- Post #21: "Bedell Smith's Secret: The General Who Actually Ran the War"

Each post follows briefing template (hook, context, narrative, technical nugget, learning).

Expected: 5 analysis posts published

**Step 6: Pyle writes victory summary (Post #22)**

Final post:
- Campaign metrics: 42 pages deployed, 14 variants live, 7-day baseline collected
- Commander highlights: Patton (speed), Rickover (quality), Nimitz (coordination)
- Medals awarded: (User provides feedback on exceptional performance)
- Lessons learned: Quality gates enable speed, personality diversity improved outcomes
- What's next: Optimization phase, variant iteration based on Layton's data

Expected: Campaign wrap-up post

---

## Phase 5: Campaign Completion & Service Records

### Task 5.1: All Commanders Write Service Records

**Files:**
- Modify: `/home/psimmons/projects/generals/profiles/*.md` (14 front commanders + 4 staff)

**Step 1: Each commander updates their profile**

Example: Patton updates `/home/psimmons/projects/generals/profiles/george-patton.md`:

**Add to Service Record section:**
```markdown
### Deployment: Operation Multi-Variant (brutal variant)
**Date**: 2026-02-08
**Role**: Front Commander (brutal.clearwatchresearch.com)
**Outcome**: SUCCESS

**Accomplishments**:
- Built 3 pages (About, Services, Portfolio) in 6 hours
- All pages passed 3 quality gates first or second attempt
- Brutal variant achieved highest conversion rate (4.2%)
- Demonstrated "speed + quality gates" approach effectiveness

**Challenges**:
- Initial design feedback (Ramsay): Text too timid, required boldness increase
- CISO feedback: Needed specific outcomes, not generic experience claims
- Learned: Aggressive tone requires aggressive specifics to back it up

**Behavioral Observations**:
- Maintained "good plan violently executed" personality throughout
- Impatient with delays but accepted quality gate feedback quickly
- Preferred "ship and iterate" over "perfect before deploy"
- Quick to correct mistakes when pointed out

**XP Earned**: +50 XP (successful 3-page deployment)
**New Total**: 50 XP (1 deployment)
**Campaign Ribbons**: Operation Multi-Variant (2026)
**Competence Progress**: Rapid Execution 1/5
```

**Step 2: User reviews service records and awards medals**

User provides feedback:
- Who exceeded expectations? (Medal of Honor candidates)
- Any disappointments? (noted in service record)
- Special recognition? (campaign ribbons, commendations)

Example user feedback:
"Rickover's 9 iterations on terminal variant were excessive but resulted in zero post-deploy bugs. Patton's speed enabled early testing. Nimitz coordinated Navy division smoothly."

**Step 3: Commanders update profiles with medals/feedback**

Rickover gets Medal of Honor for quality excellence.
Patton gets commendation for conversion leadership.
Nimitz gets Navy Coordination Ribbon.

Expected: All 18 active commanders have updated service records

### Task 5.2: Commit Service Records to GitHub

**Files:**
- Modify: `/home/psimmons/projects/generals/profiles/*.md` (18 files)
- Modify: `/home/psimmons/projects/generals/COMMAND-ROSTER.md` (XP totals update)

**Step 1: Bedell Smith commits all service records**

```bash
cd ~/projects/generals
git add profiles/ COMMAND-ROSTER.md
git commit -m "service-records: Operation Multi-Variant Deployment complete

Campaign: 14-front website deployment (42 pages)
Duration: 6 weeks (Phases 1-4)
Outcome: SUCCESS - All victory conditions achieved

Commander Performance:
- Patton (brutal): 50 XP, Medal of Honor (conversion leadership)
- Rickover (terminal): 75 XP, Medal of Honor (quality excellence)
- Nimitz (glass): 50 XP, Navy Coordination Ribbon
- [... 15 more commanders with XP/ribbons]

Key Learnings:
- Quality gates enable speed (Patton's 6-hour deployment worked because of gates)
- Personality diversity improved outcomes (Rickover's obsession caught bugs early)
- Multi-agent coordination scales (Bedell Smith managed 14 commanders smoothly)
- Tone/voice matrix maintained brand consistency across variants

Metrics:
- 42 pages deployed (14 variants × 3 pages)
- 126 quality gate validations (3 gates × 42 pages)
- 22 LinkedIn posts published (1 announcement, 14 dispatches, 5 analysis, 1 victory)
- 7-day A/B baseline collected (brutal variant winning at 4.2% conversion)

Total Roster XP: 2,450 → 3,200 (+750 XP campaign)

Next Campaign: TBD

Co-Authored-By: Claude Sonnet 4.5 <noreply@anthropic.com>"
```

**Step 2: Push to GitHub**

```bash
git push origin master
```

Expected: Service records persisted to GitHub (self-learning mechanism)

### Task 5.3: Montgomery Reports Campaign Complete

**Step 1: Montgomery sends final briefing to user**

```markdown
OPERATION MULTI-VARIANT DEPLOYMENT - FINAL REPORT

Supreme Commander,

Campaign complete. All victory conditions achieved.

OBJECTIVES ACHIEVED:
✓ All 14 sites have 4 complete pages (Home, About, Services, Portfolio)
✓ Biographical facts accurate across all variants (core-facts.json verified)
✓ Messaging tone matches design personality (Ogilvy audit passed)
✓ Sites deployed to K8s with proper routing (Moreell confirms)
✓ A/B testing infrastructure operational (Layton baseline collected)

METRICS:
- 42 pages deployed (14 variants × 3 new pages)
- 126 quality gate validations (100% pass rate after iterations)
- 22 LinkedIn posts published (Pyle embedded reporting)
- 7-day A/B baseline: brutal variant winning (4.2% conversion vs 3.1% average)

COMMANDER PERFORMANCE:
- 18 active commanders (Montgomery, Bedell Smith, 4 staff, 14 front commanders)
- 750 XP awarded across roster
- 3 Medals of Honor (Patton, Rickover, Nimitz)
- Zero commanders failed their front (100% success rate)

KEY LEARNINGS:
1. Quality gates enable speed (Patton shipped in 6 hours with zero post-deploy bugs)
2. Multi-agent coordination scales (Bedell Smith managed 14 parallel fronts)
3. Personality diversity improves outcomes (Rickover's obsession caught issues early)
4. Tone/voice matrix maintained brand consistency

RECOMMENDATIONS:
1. Iterate winning variant (brutal) based on Layton's data
2. Automate Gate 1 (Ramsay visual regression) for future campaigns
3. Apply lessons to next multi-front operation

REQUEST PERMISSION TO:
1. Stand down team (TeamDelete after service records committed)
2. Archive campaign documentation
3. Await next mission

Field Marshal Bernard Montgomery
Army Commander, Operation Multi-Variant Deployment
```

Expected: User approves team standown

### Task 5.4: Graceful Team Shutdown

**CRITICAL**: Follow CLAUDE.md workflow - service records BEFORE shutdown

**Step 1: Verify service records committed to GitHub**

```bash
cd ~/projects/generals
git log -1 --oneline
```

Expected: "service-records: Operation Multi-Variant Deployment complete" is latest commit

**Step 2: Verify GitHub push successful**

```bash
git status
```

Expected: "Your branch is up to date with 'origin/master'"

**Step 3: Montgomery sends shutdown requests to all teammates**

Uses SendMessage with type: "shutdown_request" to all 17 teammates (Bedell Smith + 4 staff + 14 front commanders).

Expected: All teammates approve shutdown (send shutdown_response with approve: true)

**Step 4: After all approvals, use TeamDelete**

```markdown
Use TeamDelete tool (no parameters needed, uses current team context)
```

Expected: Team and task directories removed

**Step 5: Verify cleanup**

```bash
ls ~/.claude/teams/multi-variant-deployment 2>/dev/null && echo "ERROR: Team still exists" || echo "Team deleted successfully"
ls ~/.claude/tasks/multi-variant-deployment 2>/dev/null && echo "ERROR: Tasks still exist" || echo "Tasks deleted successfully"
```

Expected: Both directories gone

---

## Success Criteria Verification

### Final Checklist (User Validates)

**Per-Front Victory Conditions** (14 fronts):
- [ ] 4 complete pages deployed (Home already existed, +3 new)
- [ ] All biographical facts accurate (verified against core-facts.json)
- [ ] Tone matches design personality (Ogilvy validation passed)
- [ ] Passes all 3 quality gates (Ramsay, CISO, Ogilvy)
- [ ] K8s deployment healthy, proper routing configured
- [ ] Analytics tracking operational

**Campaign-Level Victory Conditions**:
- [ ] All 14 fronts achieve victory conditions
- [ ] A/B testing infrastructure operational (Layton confirms)
- [ ] Conversion data flowing (minimum 7 days baseline)
- [ ] Brand consistency audit passed (Ogilvy validation)
- [ ] LinkedIn content series published (22 posts from Pyle)
- [ ] Infrastructure can scale (Moreell confirms CI/CD pipeline solid)

**Learning Objectives**:
- [ ] Service records committed to GitHub (all 18 commanders)
- [ ] Tone/voice patterns documented (what resonated per design style)
- [ ] Technical patterns documented (multi-variant content management)
- [ ] A/B testing baseline data archived (for future optimization)

**GitHub Persistence**:
- [ ] Generals repo updated with service records
- [ ] All commits pushed to remote
- [ ] Campaign documentation complete

---

## Appendix A: Model Selection Guide

**By Role**:
- **Montgomery** (Army Commander): Sonnet (strategic decisions, coordination)
- **Bedell Smith** (Chief of Staff): Sonnet (operations management, synthesis)
- **Layton** (Analytics): Sonnet (data analysis, metrics design)
- **Ogilvy** (Brand): Sonnet (tone judgment, content quality)
- **Moreell** (Infrastructure): Haiku (automation, straightforward CI/CD)
- **Pyle** (Reporter): Sonnet (narrative writing, storytelling)
- **Front Commanders** (14): Haiku by default, Sonnet if tone adaptation complex
- **Quality Gates**: Sonnet (subjective judgment calls)

**Cost Optimization**:
- Haiku for: Routine page builds, simple deployments, status reports
- Sonnet for: Tone adaptation, quality validation, coordination
- Opus for: Strategic planning (Montgomery Phase 1 decisions only)

**Autonomy**: All agents can switch models mid-deployment if task complexity changes.

---

## Appendix B: Team Communication Patterns

**Daily Standup** (Bedell Smith → All Commanders):
- Frequency: Every 24 hours during Phase 3
- Format: Broadcast message requesting status
- Response required: Within 4 hours
- Escalation: No response = direct message from Bedell Smith

**Quality Gate Validation** (Commander → Gate Validator):
- Frequency: Per page completion (42 times total)
- Format: Direct message with page preview/content
- Response required: PASS or FAIL with specific feedback
- Iteration: Commander fixes and resubmits until PASS

**Blocker Escalation** (Commander → Bedell Smith → Montgomery → User):
- Trigger: Stuck >24 hours
- Path: Commander reports to Bedell Smith, Bedell Smith escalates if needed
- Resolution: Montgomery intervenes or user makes decision

**Ernie Pyle Observation** (Passive):
- Pyle receives all team messages automatically (no manual checking)
- Observes commander personalities, challenges, victories
- Drafts dispatches based on observed behavior

---

## Appendix C: Git Commit Conventions

**Commit Message Format**:
```
<type>(scope): <subject>

<body>

Co-Authored-By: <Commander Name> via Claude Sonnet 4.5 <noreply@anthropic.com>
```

**Types**:
- `feat`: New feature/page added
- `fix`: Bug fix, quality gate feedback addressed
- `docs`: Documentation update
- `chore`: Maintenance (dependency update, etc.)
- `service-records`: Commander profile updates

**Scopes**:
- `brutal`, `trust`, `minimal`, etc. (variant names)
- `content` (shared content directory)
- `infrastructure` (CI/CD, K8s)
- `generals` (commander profiles)

**Examples**:
```
feat(brutal): add About page

Passed all quality gates:
- Gate 1 (Ramsay): Visual consistency ✓
- Gate 2 (CISO): Decision utility ✓
- Gate 3 (Ogilvy): Brand consistency ✓

Commander: General Patton
Tone: Aggressive, no-BS, urgent

Co-Authored-By: General George S. Patton Jr. via Claude Sonnet 4.5 <noreply@anthropic.com>
```

---

**Plan Status**: Complete and ready for execution
**Next Step**: User approval, then begin Phase 0 (Pre-Flight Checks)
**Execution Mode**: Subagent-Driven Development (this session) or Parallel Session (separate)
