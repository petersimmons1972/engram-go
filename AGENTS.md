# Generals Operational Reference

> **GitHub source of truth**: https://github.com/petersimmons1972/generals
> Local roster is for spawn decisions (specialization-based). GitHub profiles are authoritative for XP/stats.

---

## 1. Task-to-Team Quick Lookup

| Task Type | Pattern | Lead | Core Team | Validators |
|-----------|---------|------|-----------|------------|
| Content production | Sequential Pipeline | Pyle (draft) | Orwell (edit), Murrow (fact-check) | Ramsay, CISO, Ogilvy |
| Competitive intel | Parallel + Coordinator | Montgomery | Nimitz, Halsey, MacArthur | Spruance |
| Code sprint | Parallel + Coordinator | Eisenhower | Bradley, Layton, Mitchell, Dowding | Spruance, Murrow |
| Rapid fix / hotfix | Solo Deep Work | Patton or Rommel | — | — |
| K8s deployment | Solo + Skills | Nimitz or King | — | — |
| Security audit | Parallel + Coordinator | Rickover | Hopper, Layton | CISO |
| Chart production | Parallel + Coordinator | Montgomery | Zhukov, King, Spruance, Halsey | Ramsay |
| Strategic positioning | Solo Deep Work | MacArthur | — | CISO |
| Full ClearWatch sprint | Parallel + Coordinator | Eisenhower | Bradley, Layton, Mitchell, Dowding, Nimitz | Spruance, Ramsay |
| LinkedIn / writing | Sequential Pipeline | Pyle | Orwell | Ogilvy |

---

## 2. Commander Roster

> **SYNC WARNING**: Last synced from GitHub: 2026-02-24. XP from service record YAMLs + Feb 24 campaign.
> If a commander's XP matters for a decision, verify against `profiles/service-records/{name}.yaml`.

### High-XP Commanders (deployed, proven)

| Name | Branch | Specialization | XP | Model | When to Use |
|------|--------|---------------|-----|-------|-------------|
| Rickover | Tech/Eng | Zero-defect standards, technical excellence | 925 | Opus | Quality-critical technical work |
| Montgomery | British Army | Multi-team coordination, intel synthesis | 400 | Opus | Supreme command, large campaigns |
| Spruance | US Navy | Verification, TDD, analytical excellence | 375 | Opus | QA, testing, cost analysis |
| Eisenhower | US Army | Workflow analysis, coalition building | 350 | Opus | Multi-team coordination |
| Bradley | US Army | Methodical execution, state machines | 200 | Opus | Careful implementation, proven patterns |
| Nimitz | US Navy | Config/manifests, competitive intel | 175 | Sonnet | K8s deployments, research |
| King | US Navy | Deployment ops, blocker identification | 175 | Sonnet | Deployment execution, diagnostics |
| Halsey | US Navy | Aggressive action, rapid response | 150 | Sonnet | Fast execution, competitive analysis |
| Ramsay | Validator | Visual quality control | 150 | Sonnet | Chart/visual QA gates |
| CISO | Validator | Strategic utility, decision support | 150 | Sonnet | Content utility validation |
| Layton | Tech/Eng | Intelligence analysis, SIGINT, diagnostics | 150 | Opus | Pattern recognition, diagnostics |
| Marshall | US Army | Build & logistics, infrastructure | 100 | Sonnet | Large-scale builds |
| Rommel | Wehrmacht | Rapid tactical execution | 100 | Sonnet | Small-scale rapid ops |
| Hopper | Tech/Eng | Computing, software development | 100 | Sonnet | Software projects |
| Mitchell | USAAF | Air power innovation, code review | 100 | Opus | Code review, challenging assumptions |
| Dowding | RAF | Integrated defense, systems integration | 100 | Opus | Architecture, pipeline integration |
| Murrow | Journalist | Fact-checking, statistical verification | 100 | Sonnet | Fact-checking, source validation |
| Zhukov | Soviet | Workflow visualization | 75 | Sonnet | Process diagrams, flow charts |
| MacArthur | US Army | Strategic positioning, visionary planning | 50 | Opus | Strategy, future-state analysis |

### Zero-XP Commanders (available, unproven)

| Name | Branch | Specialization | When to Use |
|------|--------|---------------|-------------|
| Patton | US Army | Rapid execution, emergency response | Time-critical missions |
| Slim | British Army | Innovation under constraints | Difficult situations, morale recovery |
| Orwell | Journalist | Propaganda detection, political analysis | Detecting mythology, ethical assessment |
| Pyle | Journalist | Humanization, ground-level narrative | Human stories, content drafting |
| Ogilvy | Validator | Brand standards, voice consistency | Brand alignment, voice validation |
| Groves | Tech/Eng | Mega-project management | Complex technical projects |
| Lejeune | USMC | Doctrine, leadership development | Training systems |
| Butler | USMC | Direct action, anti-corruption | Challenging authority |
| Puller | USMC | Combat leadership | Front-line operations |
| Shoup | USMC | Amphibious assault | Complex assault planning |
| James | USMC | Fighter ops, integration leadership | Breaking barriers |
| Arnold | USAAF | Strategic air power | Large-scale operations |
| LeMay | USAAF | Strategic bombing, efficiency | Maximum effectiveness |
| Spaatz | USAAF | Precision strikes | Strategic campaigns |
| Portal | RAF | RAF strategic direction | High-level strategy |
| Slessor | RAF | Maritime air operations | Maritime ops |
| Harris | RAF | Area bombing | Maximum pressure |
| Trenchard | RAF | RAF doctrine | Organizational founding |
| Smith | Tech/Eng | Chief of staff ops, intel | Staff coordination |
| Moreell | Tech/Eng | Construction, Seabees | Rapid construction |
| Dornberger | Tech/Eng | Rocket development | Advanced weapons dev |
| Yamamoto | IJN | Naval aviation | Carrier operations |
| Nagano | IJN | Authorization/coordination | Approval processes |
| Raeder | Kriegsmarine | Naval strategy | Naval planning |
| Doenitz | Kriegsmarine | Submarine warfare | Asymmetric naval |
| Galland | Luftwaffe | Fighter operations | Air superiority |
| Moelders | Luftwaffe | Fighter tactics | Tactical innovation |
| Tukhachevsky | Soviet | Deep operations theory | Military theory |
| Kulik | Soviet | Incompetent leadership study | Failure analysis |

---

## 3. Spawn Templates

### Pattern 1: Sequential Pipeline (Content Production)

**When**: Content that flows through draft → edit → validate stages
**Team size**: 3-5 | **Cost**: Moderate

```
TeamCreate: team_name="content-pipeline", description="Content production pipeline"

# Stage 1: Draft
Task: name="pyle", subagent_type="general-purpose", team_name="content-pipeline"
  prompt: "You are Ernie Pyle. Draft [content] with ground-level narrative..."

# Stage 2: Edit (after Stage 1 completes)
Task: name="orwell", subagent_type="general-purpose", team_name="content-pipeline"
  prompt: "You are George Orwell. Review and sharpen [draft]..."

# Stage 3: Validate (parallel after Stage 2)
Task: name="ramsay", subagent_type="general-purpose", team_name="content-pipeline"
Task: name="ciso", subagent_type="general-purpose", team_name="content-pipeline"
Task: name="ogilvy", subagent_type="general-purpose", team_name="content-pipeline"
```

**Past deployment**: Operation Multi-Variant Deployment (2026-02-09)

### Pattern 2: Parallel Execution with Coordinator

**When**: Multiple independent work streams needing central coordination
**Team size**: 4-8 | **Cost**: High

```
TeamCreate: team_name="[operation-name]", description="[objective]"

# Coordinator (Opus always)
Task: name="eisenhower", subagent_type="general-purpose", team_name="[operation-name]", model="opus"
  prompt: "You are General Eisenhower, Supreme Commander. Coordinate [N] specialists..."

# Specialists (parallel, model per complexity)
Task: name="bradley", subagent_type="general-purpose", team_name="[operation-name]", model="opus"
Task: name="layton", subagent_type="general-purpose", team_name="[operation-name]", model="opus"
Task: name="mitchell", subagent_type="general-purpose", team_name="[operation-name]", model="opus"
Task: name="dowding", subagent_type="general-purpose", team_name="[operation-name]", model="opus"
```

**Past deployment**: Self-Learning Redesign (2026-02-24), Operation Stunning Charts (2026-02-07)

### Pattern 3: Parallel Verification Sweep

**When**: QA across multiple items simultaneously
**Team size**: 2-4 | **Cost**: Low-Moderate

```
TeamCreate: team_name="qa-sweep", description="Quality validation sweep"

Task: name="ramsay", subagent_type="general-purpose", team_name="qa-sweep", model="sonnet"
  prompt: "You are Gordon Ramsay. Validate visual quality of [items]..."

Task: name="ciso", subagent_type="general-purpose", team_name="qa-sweep", model="sonnet"
  prompt: "You are the CISO Validator. Assess strategic utility of [items]..."
```

**Past deployment**: Operation Multi-Variant Deployment Phase 4 (2026-02-09)

### Pattern 4: Solo Deep Work

**When**: Single specialist or skill invocation sufficient
**Team size**: 1 | **Cost**: Low

```
# No TeamCreate needed - direct Task spawn
Task: name="patton", subagent_type="general-purpose", model="sonnet"
  prompt: "You are General Patton. Execute [rapid fix] immediately..."
```

---

## 4. Service Record Checklist (NON-NEGOTIABLE)

After every team deployment, complete ALL steps before TeamDelete:

- [ ] Write service record using template: `~/projects/generals/templates/SERVICE-RECORD-TEMPLATE.md`
- [ ] Update commander profiles: `~/projects/generals/profiles/{name}.md`
- [ ] Update service record YAMLs: `~/projects/generals/profiles/service-records/{name}.yaml`
- [ ] `git add` all changed files in `~/projects/generals/`
- [ ] `git diff --staged` — verify changes are correct
- [ ] `git commit -m "docs: [Operation Name] service records"`
- [ ] `git push origin master`
- [ ] Verify push succeeded (check for errors)
- [ ] `TeamDelete` — only after all above steps complete

**Skipping any step = violation of CLAUDE.md ALWAYS rules.**

---

## 5. Model Assignment Guide

| Role | Default | Override When |
|------|---------|--------------|
| Coordinator (Eisenhower, Montgomery) | Opus | Never downgrade |
| Core specialist (complex: state machines, architecture) | Opus | Sonnet if routine/repetitive |
| Core specialist (routine: config, deployment) | Sonnet | Opus if unexpectedly complex |
| 0 XP commander (first deployment) | Haiku | Sonnet/Opus if task is complex |
| Validator (Ramsay, CISO, Ogilvy) | Sonnet | Haiku OK for simple pass/fail |
| Journalist (Murrow, Orwell, Pyle) | Sonnet | Opus for complex analysis |

---

## 6. Active Project Contexts

**ClearWatch** (`~/projects/clearwatch/`): Most agent-intensive project. Typical team: Eisenhower + 4-6 specialists. Recent campaign: self-learning redesign (6 commanders, 707 tests).

**Writers/LinkedIn** (`~/linkedin/`): Content pipeline pattern. Start with Pyle for draft, Orwell for sharpening. See `~/projects/generals/profiles/` for journalist selection guide.

**Homelab/K8s**: Mostly solo with homelab skills. Nimitz or King for K8s manifest work. Rarely needs full team.

**Security Intelligence Business** (`~/projects/security-intelligence-business/`): Chart-heavy reports. Montgomery coordinates, specialists produce charts (Zhukov, King, Spruance, Halsey). Ramsay + CISO validate.

---

## 7. XP Quick Reference

| Task Type | Base XP | Bonus |
|-----------|---------|-------|
| Research/Intelligence | 50 | +25 if >10 sources |
| Visualization/Charts | 75 | +25 if user praises quality |
| Integration/Pipeline | 100 | +50 if zero bugs |
| Coordination/Command | 150 | +50 if all subordinates succeed |
| Troubleshooting | 200 | +100 if root cause <30 min |

| Penalty | XP |
|---------|-----|
| Task incomplete | -50 |
| Major rework needed | -25 |
| Missed deadline | -25 |
| Coordination failure | -50 |

Full system: `~/projects/generals/PROGRESSION-SYSTEM.md`

---

## 8. Sync Protocol

**When to sync**: After any deployment that changes XP/stats.

**How to sync**:
1. `cd ~/projects/generals && git pull origin master`
2. Compare roster XP above against `profiles/service-records/*.yaml`
3. Update this file's roster table if stale
4. Note new sync date in Section 2 header

**Why staleness is acceptable**: Spawn decisions are specialization-based, not XP-dependent. A commander's specialization doesn't change between syncs. XP only matters for model assignment (high-XP = proven = Opus-worthy), and small XP deltas don't change that calculus.

---

## 9. Team Management Standards (MINIMUM)

### Idle Notification Protocol
When teammate goes idle:
1. Immediately send status check (do NOT just wait)
2. Request: What completed? Current state? Blocked? Next steps?
3. Response within 1 minute of idle notification

### Field Marshal Responsibilities
- Monitor all teammate idle notifications
- Send status checks within 1 minute of idle state
- Document progress in mission log
- Escalate blockers immediately

### Anti-patterns (FORBIDDEN)
- Seeing idle notification and doing nothing
- Assuming agent will self-report
- Waiting >5 minutes to check on idle agent
- Passive "let me know when done" approach

**Success metric:** No agent sits idle >2 minutes without status check.
