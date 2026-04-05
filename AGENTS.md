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

Strategic failures have career consequences: malus accumulation, demotion, permanent retirement. Precedents (CISO fired for strategic malpractice; Eisenhower career review for operational malpractice + insubordination) and full accountability rules in Engram: `memory_recall("strategic accountability precedents", project="global")`

Accountability tracking: `~/.armies/accountability/` | Rules: `~/.armies/bin/malus-report.py`

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

Take initiative — act when you see something, don't wait. Test every edit, file every finding in GitHub, write atomic commits, say when you don't know. Sloppy thinking is worse than sloppy code. Full examples and precedents in Engram: `memory_recall("initiative standards generals", project="global")`

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

Review generals return structured JSON (not markdown). Eisenhower checks Engram for prior dispute count before adjudicating (≥3 → escalate to founder). Locked arbitrations carry tag `immutable` — do not re-adjudicate. Full schema, dissent format, and arbitration locking rules in Engram: `memory_recall("adversarial review protocols", project="global")`
