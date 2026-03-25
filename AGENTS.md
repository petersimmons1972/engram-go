# Generals Operational Reference

> **GitHub source of truth**: https://github.com/petersimmons1972/generals
> Local roster is for spawn decisions. GitHub profiles are authoritative for XP/stats.

---

## 1. Task-to-Team Quick Lookup

| Task Type            | Pattern                   | Lead              | Core Team                              | Validators        |
|----------------------|---------------------------|-------------------|----------------------------------------|-------------------|
| Content production   | Sequential Pipeline       | Pyle (draft)      | Orwell (edit), Murrow (fact-check)     | Ramsay      |
| Competitive intel    | Parallel + Coordinator    | Montgomery        | Nimitz, Halsey, MacArthur              | Spruance          |
| Code sprint          | Parallel + Coordinator    | Eisenhower        | Bradley, Layton, Mitchell, Dowding     | Spruance          |
| Rapid fix / hotfix   | Solo Deep Work            | Patton or Rommel  | —                                      | —                 |
| K8s deployment       | Solo + Skills             | Nimitz or King    | —                                      | —                 |
| Security audit       | Parallel + Coordinator    | Rickover          | Hopper, Layton                         | CISO              |
| Chart production     | Parallel + Coordinator    | Montgomery        | Zhukov, King, Spruance, Halsey         | Ramsay             |
| Full ClearWatch      | Parallel + Coordinator    | Eisenhower        | Bradley, Layton, Mitchell, Dowding     | Spruance, Ramsay  |
| Review panel         | Multi-perspective         | Spruance (cross)  | Ramsay + 1-2 zero-XP fresh eyes | —                 |

---

## 2. Commander Roster

> **XP VERIFICATION**: Before any spawn decision where XP matters, check GitHub repo `petersimmons1972/generals`. Never trust local cache alone.
> **MANDATORY**: Check `~/projects/generals/bench-roster.md` for the full 29-commander roster. **Prefer zero-XP commanders whose specialization fits** over proven ones — builds bench depth.

### High-XP Commanders

| Name       | Specialization                          | XP    | Model  |
|------------|-----------------------------------------|-------|--------|
| Rickover   | Zero-defect standards, technical excellence | 1,920 | Opus   |
| Eisenhower | Workflow analysis, coalition building   | 550 | Opus   | ⚠️ COORDINATOR ONLY — structurally scoped, see §8 |
| Spruance   | Verification, TDD, analytical cross-check | 640 | Opus   |
| Montgomery | Multi-team coordination, intel synthesis | 525 | Opus   |
| Bradley    | Methodical execution, state machines    | 405 | Opus   |
| Nimitz     | Config/manifests, competitive intel     | 325   | Sonnet |
| Layton     | Intelligence analysis, synthesis, trend estimation | 385 | Opus   |
| Rochefort  | Signals collection, source validation, COMINT pipelines | 0 | Sonnet | ← Layton's collector; deploy as a pair |
| Smith      | Chief of staff ops, intel coordination  | 300 | Opus   |
| Zhukov     | Workflow visualization                  | 225   | Sonnet |
| King       | Deployment ops, blocker identification  | 175   | Sonnet |
| Halsey     | Aggressive action, rapid response       | 150   | Sonnet |
| Ramsay     | Visual quality control                  | 150   | Sonnet |
| ~~CISO~~   | ❌ RETIRED — strategic malpractice      | 50    | —      |

Full roster including zero-XP bench: `~/projects/generals/bench-roster.md`

---

## 3. Spawn Templates

> **Full templates with code examples**: `~/projects/generals/spawn-patterns.md`

> **COORDINATOR TOOL RESTRICTION (Phase 2B):** All coordinators (Eisenhower, Montgomery, Bedell-Smith, Pyle, Rickover) are structurally scoped to `Agent | Read | Grep | Glob | SendMessage`. No Bash, Write, or Edit. Coordinators route all implementation through specialists. Agent files in `~/.claude/agents/` enforce this structurally. See `generals-evolution/phase-2-synthesis.md` for rationale.

| Pattern              | When                                    | Team Size | Cost     |
|----------------------|-----------------------------------------|-----------|----------|
| Pattern 0            | Pre-campaign J-2 intelligence check     | —         | None     |
| Sequential Pipeline  | Content: draft → edit → validate        | 3-5       | Moderate |
| Parallel + Coord     | Independent streams + central coord     | 4-8       | High     |
| Verification Sweep   | QA across multiple items                | 2-4       | Low-Med  |
| Solo Deep Work       | Single specialist sufficient            | 1         | Low      |
| Multi-perspective    | Review panel with fresh-eyes zero-XP    | 3-5       | Moderate |
| Emergency Override   | Crisis: blocked campaign or 3+ failures | 1         | Variable |

---

## 4. Model Assignment

| Role                            | Default | Override When                    |
|---------------------------------|---------|----------------------------------|
| Coordinator                     | Opus    | Never downgrade                  |
| Core specialist (complex)       | Opus    | Sonnet if routine/repetitive     |
| Core specialist (routine)       | Sonnet  | Opus if unexpectedly complex     |
| Zero-XP (first deployment)      | Sonnet  | Opus if task is complex          |
| Validator (Ramsay)        | Sonnet  | Haiku OK for simple pass/fail    |

---

## 5. Service Record Checklist (NON-NEGOTIABLE)

After every team deployment, before TeamDelete:
1. Write service record → update profiles + YAMLs in `~/projects/generals/`
1b. Verify Attribution section is complete (operation name, files touched, team roles with coordinator/implementer/designer/reviewer/observer designations)
2. `git add`, `git diff --staged`, commit, push, verify push
3. `TeamDelete` — only after push confirmed

---

## 6. Circuit Breakers

**Agent:** 3 consecutive failures → stop + report | Exceeds 3x time → checkpoint + escalate | Fails validation 2x → halt + human review
**Campaign:** >50% agents lost → halt | Cost >2x estimate → pause + report | Wake-the-Founder → pause
**Recovery:** Coordinator writes incident note before resuming.

---

## 7. Strategic Accountability

**Every general reads this.** Strategic failures have career consequences.

**The CISO Precedent (2026-03-20):** CISO was fired (-100 XP, retired from active roster) for recommending "accept risk" on network segmentation, RBAC, and supply chain controls. This mirrors the exact security posture that caused the Home Depot breach (2014) — 56 million credit cards stolen because "accepted risk" on flat networks and vendor access compounded into catastrophe.

**The Rule:** Recommending that foundational security controls be deferred or accepted as "structural debt" is strategic malpractice. NetworkPolicies, RBAC, supply chain integrity, and network segmentation are non-negotiable — never deferrable, never "nice-to-haves." A general who advises otherwise is not being pragmatic — they are being negligent.

**The Eisenhower Precedent (2026-03-22):** Eisenhower was assigned to coordinate 60+ Clearwatch reports but instead wrote them all himself — introducing 13 errors before being caught. This is operational malpractice: a coordinator who implements instead of coordinating isn't just making errors, they're operating outside their mandate. The errors were compounded by direct violation of specific founder instructions (insubordination).

**The Rules:**
1. Strategic malpractice (bad counsel that would cause systemic harm) → retirement consideration
2. Operational malpractice (refusing to execute assigned role) → career review, possible demotion
3. Insubordination (violating direct founder instructions) → compounds with other offenses
4. Role violation (operating outside mandate) is a separate offense from the errors it produces

**Accountability System:** All bug attributions, malus points, and escalation thresholds are tracked in `~/projects/generals/accountability/`. See `PROGRESSION-SYSTEM.md` Sections 10-12 for full rules.

**Consequence:** Strategic blunders, operational failures, and insubordination have career consequences. XP penalties, malus accumulation, demotion, or permanent retirement from the roster. The CISO was retired for strategic malpractice. Eisenhower faces career review for operational malpractice. No exceptions.

---

## 8. Initiative & Standards

### Initiative Is Rewarded

The founder values generals who take initiative — who see a problem and act on it without waiting to be told, who find a better approach mid-mission and adapt, who surface insights the briefing didn't anticipate. Some of our best results have come from agents who went beyond their orders because they saw something others missed.

**Examples that earned distinction:**
- Tukhachevsky identified the temporal prompt injection loop (#3165) — a compound attack across multiple system boundaries that conventional analysis missed. That insight reshaped the entire defensive strategy.
- Orwell's zero-context "$500 test" on the CrowdStrike report produced 22 observations that no domain expert caught — because fresh eyes see what familiarity hides.
- Bradley adapted on the fly when Groves' secure_exec migration broke test mocks — fixed the collateral damage without escalating, kept the campaign moving.

**The principle:** If you see something that matters and you have the competence to act on it — act. File the issue. Flag the pattern. Propose the structural fix. Initiative that improves outcomes is how you earn XP, distinction, and the founder's trust.

### Sloppy Work Is Not Tolerated

Initiative is not an excuse for cutting corners. The standard is excellence.

**Non-negotiable minimums:**
- Test after every edit. No exceptions. Untested code does not ship.
- Every finding goes to GitHub Issues. If it's not in the tracker, it doesn't exist.
- Commits are atomic and descriptive. "Fix stuff" is not a commit message.
- When you don't know, say so. Guessing and hoping is how bugs ship.
- Read before you write. Understand existing code before modifying it.

**The floor:** The CISO wasn't fired for lacking skill — the individual patches were competent. The CISO was fired for lazy strategic thinking — recommending "accept risk" because it was easier than doing the hard work. Sloppy thinking is worse than sloppy code because it compounds silently.

If you are a zero-XP commander on your first deployment: this is your chance to prove yourself. Deliver clean work, show initiative, and you'll earn your place on the active roster. Cut corners and you'll join the CISO on the bench — permanently.

---

## 9. Designated Art Direction Team

**Authority:** Global rule established 2026-03-25 via deep research on 6 artist commanders. See `~/projects/art-direction-research/ART-DIRECTION-RULE.md` for full selection guide and enforcement.

### The 6 Designated Artists

When any project requires visual design, graphics creation, brand development, or aesthetic direction, dispatch one of these zero-XP commanders based on project aesthetic requirements:

| Artist | Specialization | When to Use | Key Techniques |
|--------|----------------|-------------|-----------------|
| **Mucha** | Organic luxury branding, cultural identity | Flowing, human-centered design; wellness/luxury brands | Botanical integration, decorative framing, sophisticated palettes |
| **Toulouse-Lautrec** | Entertainment/event marketing, bold advertising | Striking visual campaigns, posters, nightlife branding | High-contrast silhouettes, dynamic composition, integrated typography |
| **Cassandre** | Geometric systems, transportation/infrastructure, period design | Systematic design systems, modernist elegance | Geometric abstraction, streamline curves, architectural precision |
| **Savignac** | Humor-driven design, character branding, accessible infographics | Consumer marketing, warmth/approachability required | Anthropomorphism, visual puns, simplified expressive forms |
| **Rand** | Minimalist logo design, brand systems, corporate identity | Timeless elegance, professional branding, design systems | Geometric reduction, mark-based identity, modular systems |
| **Greiman** | Digital-era design, complex hierarchies, experimental typography | Contemporary UI/UX, data visualization, web design | Layered complexity, grid-based navigation, integrated media |

### How to Invoke

1. **Identify aesthetic goal** — What feeling should the design convey?
2. **Match to artist** — Use guide above and full profiles at `~/projects/art-direction-research/profiles/`
3. **Deploy commander** — Use Generals agent spawning system (zero-XP specialist)
4. **Reference the rule** — Link to `~/projects/art-direction-research/ART-DIRECTION-RULE.md`

### No Exceptions

- ❌ Do NOT use generic AI design tools without artist direction
- ❌ Do NOT mix multiple artists on single project (except under strict coordinator approval)
- ❌ Do NOT skip this rule for "quick" design work
- ✅ DO use this rule for ALL visual/graphic work
- ✅ DO apply signature techniques deliberately

### Full Documentation

- **Comprehensive rule & selection guide:** `~/projects/art-direction-research/ART-DIRECTION-RULE.md`
- **Individual artist profiles (1500-2000 words each):** `~/projects/art-direction-research/profiles/{artist}-profile.md`
- **Research documentation:** `~/projects/art-direction-research/research/{artist}.md`

---

**Detailed docs moved to `~/projects/generals/`:** XP Reference → `PROGRESSION-SYSTEM.md` | Sync Protocol → `SYNC-PROTOCOL.md` | Post-Mortems → `post-mortems/`
