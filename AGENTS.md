# Generals Operational Reference

> **GitHub source of truth**: https://github.com/petersimmons1972/camp-david
> Local roster is for spawn decisions. GitHub profiles are authoritative for XP/stats.

---

## 1. Task-to-Team Quick Lookup

| Task Type            | Pattern                   | Lead              | Core Team                              | Validators        |
|----------------------|---------------------------|-------------------|----------------------------------------|-------------------|
| Content production   | Sequential Pipeline       | Groves (coord)    | Pyle (draft), Orwell (edit), Murrow (fact-check) | Ramsay |
| Competitive intel    | Parallel + Coordinator    | Montgomery        | Nimitz, Halsey, MacArthur              | Spruance          |
| Code sprint          | Parallel + Coordinator    | Eisenhower        | Bradley, Layton, Mitchell, Dowding     | Spruance          |
| Rapid fix / hotfix   | Solo Deep Work            | Patton or Rommel  | —                                      | —                 |
| Emergency override   | Pattern 5 (pre-auth req)  | Patton (force) or Rommel (cunning) | — | Spruance or Ramsay (post-deployment) |
| K8s deployment       | Solo + Skills             | Nimitz or King    | —                                      | —                 |
| Security audit       | Parallel + Coordinator    | Rickover          | Hopper, Layton                         | CISO              |
| Chart production     | Parallel + Coordinator    | Montgomery        | Zhukov, King, Spruance, Halsey         | Ramsay             |
| Full ClearWatch      | Parallel + Coordinator    | Eisenhower        | Bradley, Layton, Mitchell, Dowding     | Spruance, Ramsay  |
| Review panel         | Multi-perspective         | Spruance (cross)  | Ramsay + 1-2 zero-XP fresh eyes | —                 |

---

## 2. Commander Roster

> **XP VERIFICATION**: Before any spawn decision where XP matters, check GitHub repo `petersimmons1972/camp-david`. Never trust local cache alone.
> **MANDATORY**: Check `~/.armies/bench-roster.md` for the full roster. **Prefer zero-XP commanders whose specialization fits** over proven ones — builds bench depth.

### High-XP Commanders

| Name       | Specialization                          | XP    | Model  |
|------------|-----------------------------------------|-------|--------|
| Rickover   | Zero-defect standards, technical excellence | 850 | Opus   |
| Eisenhower | Workflow analysis, coalition building   | 0 | Opus   | ⚠️ COORDINATOR ONLY — structurally scoped, see §8 |
| Spruance   | Verification, TDD, analytical cross-check | 900 | Opus   |
| Montgomery | Multi-team coordination, intel synthesis | 1,000 | Opus   |
| Bradley    | Methodical execution, state machines    | 0 | Opus   |
| Nimitz     | Config/manifests, competitive intel     | 325   | Sonnet |
| Layton     | Intelligence analysis, synthesis, trend estimation | 385 | Opus   |
| Rochefort  | Signals collection, source validation, COMINT pipelines | 0 | Sonnet | ← Layton's collector; deploy as a pair |
| Smith      | Chief of staff ops, intel coordination  | 900 | Opus   |
| Zhukov     | Workflow visualization                  | 225   | Sonnet |
| King       | Deployment ops, blocker identification  | 175   | Sonnet |
| Halsey     | Aggressive action, rapid response       | 150   | Sonnet |
| Ramsay     | Visual quality control                  | 0 | Sonnet |
| ~~CISO~~   | ❌ RETIRED — strategic malpractice      | 50    | —      |

Full roster including zero-XP bench: `~/.armies/bench-roster.md`

---

## 3. Spawn Templates

> **Full templates with code examples**: `~/.armies/spawn-patterns.md`

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

> **Research-heavy dispatches:** Include graceful degradation instruction — "If you reach turn 8 of 10 without a complete answer, stop tool calls and return a partial summary labeled `PARTIAL:` with what you have gathered." See CLAUDE.md §Workflow.

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
1. Write service record → update profiles + YAMLs in `~/.armies/`
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

**Accountability System:** All bug attributions, malus points, and escalation thresholds are tracked in `~/.armies/accountability/`. See `~/.armies/bin/malus-report.py` for full rules.

**Consequence:** Strategic blunders, operational failures, and insubordination have career consequences. XP penalties, malus accumulation, demotion, or permanent retirement from the roster. The CISO was retired for strategic malpractice. Eisenhower faces career review for operational malpractice. No exceptions.

### Spawn Eligibility (machine-enforceable)

Run before every spawn: `python ~/.armies/bin/check-general-eligibility.py <name> <role>`

| Effective Malus | coordinator | emergency_reserve | specialist | validator |
|----------------|-------------|-------------------|------------|-----------|
| 0 – 99         | ✅          | ✅                | ✅         | ✅        |
| 100 – 199      | ❌          | ⚠️ founder approval | ✅       | ✅        |
| 200 – 299      | ❌          | ❌                | ✅ + review | ✅      |
| 300 – 399      | ❌          | ❌                | ✅ + escalate | ✅    |
| 400+           | ❌          | ❌                | ❌         | ❌        |

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

Six designated artists for all visual work: Mucha, Toulouse-Lautrec, Cassandre, Savignac, Rand, Greiman.

**Full selection guide and enforcement rules:** `~/projects/art-direction-research/ART-DIRECTION-RULE.md`
**Artist profiles (1500–2000 words each):** `~/projects/art-direction-research/profiles/{artist}-profile.md`

❌ Do NOT use generic AI design tools without artist direction. ❌ Do NOT mix artists on a single project.

---

**Detailed docs:** XP Reference → `~/.armies/bin/malus-report.py` | Spawn Patterns → `~/.armies/spawn-patterns.md` | How it works → `~/.armies/docs/how-it-really-works.md`

---

## 10. Adversarial Review Protocols

### Structured Output Schema

All review-mode generals return structured JSON. Eisenhower reads JSON, not free-form markdown. Include in every adversarial review dispatch: *"Return findings as JSON matching the schema below. Do not return free-form markdown."*

```json
{
  "findings": [
    {
      "location": "file:line or section description",
      "description": "specific problem statement",
      "severity": "critical|high|medium|low",
      "correction": "what fix is required",
      "confidence": "high|medium|low",
      "blocking": true
    }
  ],
  "summary": "one paragraph",
  "verdict": "HOLD|SHIP"
}
```

### Dissent Mode

When a review general's finding is overruled by coordinator adjudication:
1. The overruled general writes a minority opinion (2–4 sentences): specific passage/location, what they would have required, whether this should be treated as a precedent concern.
2. Eisenhower posts this as a GitHub Issue comment tagged `[DISSENT]`, attributed by general name.
3. Dissent opinions are not re-opened. They are the permanent record of the minority view.

**Format:** `[DISSENT — {general name}] {specific location}: {what I required} | {precedent concern: yes/no — reason}`

### Stall Detection (Eisenhower Dispatch Rule)

Before adjudicating any dispute, Eisenhower recalls from Engram: `memory_recall("dispute-tracker <issue description>", project="<project>")`. If count ≥ 3, do not adjudicate — escalate to founder. Stalled = no new information since last round. See CLAUDE.md §Engram Memory for storage format.

### Arbitration Locking

After Eisenhower adjudicates a dispute, store in Engram:
- `memory_type="decision"`, `tags=["adjudicated", "immutable", "<project>"]`
- `content="ARBITRATION: <issue> | DECISION: <summary> | DATE: <date>"`

On recall: if a memory has tag `immutable`, report the prior decision — do not re-adjudicate. Escalate to founder if the locked decision is challenged.
