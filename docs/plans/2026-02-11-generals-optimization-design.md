# Generals Optimization Design
**Date:** 2026-02-11
**Priority:** A) Cost Efficiency → C) User Experience → D) System Intelligence
**Target:** 60-70% token reduction + 50% cost reduction via intelligent model selection

---

## Problem Statement

Multi-commander campaigns (Operation Stunning Charts: 10 commanders) consume excessive tokens:
- **Profile reads:** 10 commanders × 250 lines avg = 2,500 lines
- **Spawn prompts:** 10 prompts × 80 lines avg = 800 lines
- **Total overhead:** ~3,300 lines BEFORE any work starts
- **Model costs:** All commanders default to Sonnet, even for routine research tasks

**Cost Impact:** $3-15 per million tokens (Sonnet), when 70% of tasks could use $0.80-4 (Haiku)

---

## Solution 1: Tiered Profile Architecture

### Three-Tier Structure

```
projects/generals/
├── profiles/
│   ├── core/                    # ALWAYS loaded on spawn (40-60 lines)
│   │   ├── nimitz.md           # Combat essentials
│   │   ├── patton.md
│   │   └── [20+ commanders]
│   ├── extended/                # OPTIONAL: historical deep-dive (150-200 lines)
│   │   ├── nimitz.md           # Full WWII context
│   │   ├── patton.md
│   │   └── [20+ commanders]
│   └── service-records/         # NEVER in spawn context
│       ├── nimitz.yaml          # Deployment history, XP tracking
│       └── [20+ commanders]
```

### Core Profile Template (40-60 lines)

```markdown
# [Rank] [Full Name]
Rank: [Historical Rank] | XP: [Total] | Deployments: [Count]

## Combat Traits
- [Trait 1]: [Brief description]
- [Trait 2]: [Brief description]
- [Trait 3]: [Brief description]
- [Trait 4]: [Brief description]

## Strengths
- [Strength 1]
- [Strength 2]
- [Strength 3]

## Weaknesses
- [Weakness 1]
- [Weakness 2]

## Recent Lessons (last 3 deployments)
- [Lesson 1]
- [Lesson 2]
- [Lesson 3]

## Voice
[1-2 sentence communication style]
```

### Extended Profile (150-200 lines)

- Full WWII biography
- Detailed battle descriptions
- Historical quotes
- Photos/images
- Source citations
- Deep personality analysis
- Historical parallels

### Service Records (YAML)

```yaml
commander: Admiral Chester W. Nimitz
rank: Fleet Admiral
total_xp: 175
deployments: 2

competence_stars:
  configuration: 1/5
  research: 1/5

ribbons:
  - name: "ClearWatch Campaign"
    date: 2026-02-07
    citation: "..."

medals:
  - type: "Commendation Medal"
    date: 2026-02-07
    citation: "..."

deployment_history:
  - date: 2026-02-07
    project: "security-intelligence-business"
    task: "K8s manifests"
    xp_earned: 100
    outcome: "success"
```

### Loading Strategy

**spawn-commander skill logic:**
```python
# Always read core
core_profile = read("profiles/core/{commander}.md")

# Read extended ONLY if:
# - User explicitly requests historical context
# - First-time spawn (no prior deployments)
# - Complex coordination requiring deep personality understanding

if needs_extended_context:
    extended_profile = read("profiles/extended/{commander}.md")
```

**Expected Token Reduction:**
- Profile reads: 2,500 lines → 500 lines (80% reduction)
- Spawn prompts: Can be compressed using core traits
- Overall campaign overhead: 60% reduction

---

## Solution 2: Model Selection Matrix

### Task-to-Model Mapping

**Haiku ($0.80/$4 per MTok) - 70% of deployments:**
- Research/Intelligence gathering
- Simple chart generation
- Manifest/config creation
- Routine verification
- Documentation writing
- **Criteria:** Clearly defined task, established patterns, low ambiguity

**Sonnet ($3/$15 per MTok) - 25% of deployments:**
- Campaign coordination
- Complex analysis
- Multi-stage integration
- Quality validation
- Creative problem-solving
- **Criteria:** Requires reasoning, coordination, or creative synthesis

**Opus ($15/$75 per MTok) - 5% of deployments:**
- CRITICAL production incidents only
- User explicitly frustrated ("stuck for hours")
- Multiple failures require different approach
- High-stakes customer-facing work
- **Criteria:** User language signals urgency/importance

### Decision Logic

**In spawn-commander skill, add model selection:**

```markdown
## Step 2.5: Select Model

Analyze task + commander + urgency:

IF task is routine AND commander has ⭐ in category:
  → model: "haiku"  # Proven competence = efficient model

IF task requires coordination OR creative analysis:
  → model: "sonnet"  # Default for complex work

IF user says "CRITICAL"/"URGENT"/"stuck for hours":
  → model: "opus"  # Break through blockers

IF user specifies model explicitly:
  → Use user's choice (override auto-selection)
```

### Integration with Task Tool

**Current spawn:**
```json
{
  "subagent_type": "general-purpose",
  "description": "Research data retention",
  "prompt": "[Nimitz personality...]",
  "name": "nimitz"
}
```

**With model selection:**
```json
{
  "subagent_type": "general-purpose",
  "model": "haiku",  // Auto-selected based on task
  "description": "Research data retention",
  "prompt": "[Nimitz core profile...]",
  "name": "nimitz"
}
```

**Expected Cost Reduction:**
- 70% of tasks on Haiku instead of Sonnet
- Haiku: $0.80 input vs Sonnet $3 input (73% cheaper)
- Haiku: $4 output vs Sonnet $15 output (73% cheaper)
- **Overall: ~50% cost reduction**

---

## Implementation Plan

### Phase 1: Proof of Concept (Top 5 Commanders)

**Migrate Priority 1 (Most Used):**
1. Field Marshal Montgomery (coordination, 5 deployments)
2. Fleet Admiral Nimitz (research, 3 deployments)
3. Admiral Spruance (analysis, 2 deployments)
4. Fleet Admiral King (visualization, 2 deployments)
5. Marshal Zhukov (workflow diagrams, 1 deployment)

**Tasks:**
- [ ] Create core profiles (5 × 40-60 lines)
- [ ] Move existing profiles to extended/ (5 files)
- [ ] Extract service records to YAML (5 files)
- [ ] Update spawn-commander skill with tiered loading
- [ ] Add model selection logic to spawn-commander
- [ ] Test spawning with core-only profiles
- [ ] Validate personality preservation
- [ ] Measure token reduction

### Phase 2: Update Skills

**Files to modify:**
- `skills/spawn-commander.md` - Add tiered loading + model selection
- `skills/award-experience.md` - Write lessons to core, history to service-records
- `skills/campaign-coordinator.md` - Include model recommendations

### Phase 3: Migrate Remaining Commanders

**Priority 2 (Active Roster):**
- Patton, Halsey, Eisenhower, Bradley, Rommel (5 commanders)

**Priority 3 (Specialists):**
- Hopper, Rickover, Groves, Ramsay, CISO (5 commanders)

**Priority 4 (Remaining):**
- All other commanders (~10 more)

### Phase 4: Documentation Updates

**For Cost Efficiency (Priority A):**
- Update README with token savings metrics
- Document model selection in PROGRESSION-SYSTEM.md
- Add "Cost Optimization" section to README

**For User Experience (Priority C):**
- Simplify Quick Start section
- Add visual diagrams for architecture
- Create "5-Minute Tutorial" for newcomers
- Improve GitHub site navigation

**For System Intelligence (Priority D):**
- Refine model selection criteria based on deployment data
- Enhance historical accuracy verification
- Improve spawn prompt quality

---

## Backward Compatibility

**Graceful Fallback in spawn-commander:**
```python
# Try new structure first
if exists("profiles/core/{commander}.md"):
    profile = read_core_profile()
else:
    # Fallback: read old monolithic profile
    profile = read("profiles/{commander}.md")
    warn("Legacy profile format - consider migrating")
```

**Migration is gradual:** Commanders migrate one at a time, system works with mixed states.

---

## Success Metrics

### Token Reduction (Priority A - Cost Efficiency)
- **Before:** 10-commander campaign = 3,300 lines overhead
- **After:** 10-commander campaign = 1,000 lines overhead
- **Target:** 60-70% reduction

### Cost Reduction (Priority A - Cost Efficiency)
- **Before:** All commanders on Sonnet ($3/$15 per MTok)
- **After:** 70% on Haiku ($0.80/$4 per MTok)
- **Target:** 50% cost reduction

### Personality Preservation (Priority D - System Intelligence)
- Commanders still behave distinctly (Patton ≠ Spruance)
- Historical traits preserved in core profiles
- Learning continues (lessons in core, always loaded)

### User Experience (Priority C)
- Simpler onboarding (measure time-to-first-spawn)
- Clearer documentation (measure GitHub engagement)
- Better readability for humans AND Claude Code

---

## Risks & Mitigations

**Risk 1: Personality Loss**
- *Concern:* Compressing profiles loses nuance
- *Mitigation:* Extended profiles available when needed, core captures combat essentials
- *Validation:* Test spawn behavior before/after migration

**Risk 2: Model Selection Errors**
- *Concern:* Haiku chosen for tasks requiring Sonnet
- *Mitigation:* User can override, Field Marshal learns from failures
- *Validation:* Track model selection success rates

**Risk 3: Migration Complexity**
- *Concern:* 20+ profiles to split, risk of data loss
- *Mitigation:* Migrate 5 at a time, validate each batch
- *Validation:* Git commits after each batch, easy rollback

**Risk 4: Service Record Extraction**
- *Concern:* Parsing deployment history from markdown to YAML
- *Mitigation:* Manual review of top 5, automated for remainder
- *Validation:* Compare XP/ribbons/medals before/after

---

## Next Steps

1. **Write implementation plan** (using superpowers:writing-plans)
2. **Create git worktree** (isolated workspace for migration)
3. **Migrate top 5 commanders** (proof of concept)
4. **Update spawn-commander skill** (tiered loading + model selection)
5. **Test multi-commander campaign** (validate token reduction)
6. **Measure cost savings** (track model selection distribution)
7. **Document learnings** (update CAMPAIGN_SUMMARY.md)
8. **Iterate on remaining commanders** (Phases 2-4)

---

## Trade-offs Accepted

✓ **Core profiles less detailed** - Extended available when needed
✓ **Model selection may err** - User override available, system learns
✓ **Migration requires work** - But 60-70% token savings justifies effort
✓ **Backward compatibility adds code** - Graceful degradation worth complexity

---

## Expected Outcome

**After full migration:**
- **Token usage:** 60-70% reduction on multi-commander campaigns
- **API costs:** 50% reduction via intelligent model selection
- **Personality:** Preserved in core profiles, extended available when needed
- **Learning:** Enhanced (lessons in core, always loaded)
- **User experience:** Simpler onboarding, clearer documentation
- **System intelligence:** Better model selection, refined spawn prompts

**Campaign Example (Operation Stunning Charts):**
- **Before:** 10 commanders × 250 lines = 2,500 lines, all Sonnet
- **After:** 10 commanders × 50 lines = 500 lines, 7 Haiku + 3 Sonnet
- **Savings:** 80% token reduction + 50% cost reduction = ~65% total cost savings

---

**Philosophy:** Optimize for cost without sacrificing personality. The commanders earned their reputations through sustained excellence - the system should honor that while being economically sustainable.
