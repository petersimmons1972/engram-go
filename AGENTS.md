# Generals Operational Reference

> **GitHub source of truth**: https://github.com/petersimmons1972/generals
> Local roster is for spawn decisions. GitHub profiles are authoritative for XP/stats.

---

## 1. Task-to-Team Quick Lookup

| Task Type            | Pattern                   | Lead              | Core Team                              | Validators        |
|----------------------|---------------------------|-------------------|----------------------------------------|-------------------|
| Content production   | Sequential Pipeline       | Pyle (draft)      | Orwell (edit), Murrow (fact-check)     | Ramsay, CISO      |
| Competitive intel    | Parallel + Coordinator    | Montgomery        | Nimitz, Halsey, MacArthur              | Spruance          |
| Code sprint          | Parallel + Coordinator    | Eisenhower        | Bradley, Layton, Mitchell, Dowding     | Spruance          |
| Rapid fix / hotfix   | Solo Deep Work            | Patton or Rommel  | —                                      | —                 |
| K8s deployment       | Solo + Skills             | Nimitz or King    | —                                      | —                 |
| Security audit       | Parallel + Coordinator    | Rickover          | Hopper, Layton                         | CISO              |
| Chart production     | Parallel + Coordinator    | Montgomery        | Zhukov, King, Spruance, Halsey         | Ramsay             |
| Full ClearWatch      | Parallel + Coordinator    | Eisenhower        | Bradley, Layton, Mitchell, Dowding     | Spruance, Ramsay  |
| Review panel         | Multi-perspective         | Spruance (cross)  | Ramsay, CISO + 1-2 zero-XP fresh eyes | —                 |

---

## 2. Commander Roster

> **XP VERIFICATION**: Before any spawn decision where XP matters, check GitHub repo `petersimmons1972/generals`. Never trust local cache alone.
> **MANDATORY**: Check `~/projects/generals/bench-roster.md` for the full 29-commander roster. **Prefer zero-XP commanders whose specialization fits** over proven ones — builds bench depth.

### High-XP Commanders

| Name       | Specialization                          | XP  | Model  |
|------------|-----------------------------------------|-----|--------|
| Rickover   | Zero-defect standards, technical excellence | 925 | Opus   |
| Eisenhower | Workflow analysis, coalition building   | 550 | Opus   |
| Spruance   | Verification, TDD, analytical cross-check | 525 | Opus   |
| Montgomery | Multi-team coordination, intel synthesis | 400 | Opus   |
| Bradley    | Methodical execution, state machines    | 350 | Opus   |
| Nimitz     | Config/manifests, competitive intel     | 325 | Sonnet |
| Zhukov     | Workflow visualization                  | 225 | Sonnet |
| Halsey     | Aggressive action, rapid response       | 225 | Sonnet |
| King       | Deployment ops, blocker identification  | 175 | Sonnet |
| Ramsay     | Visual quality control                  | 150 | Sonnet |
| CISO       | Strategic utility, decision support     | 150 | Sonnet |
| Layton     | Intelligence analysis, diagnostics      | 150 | Opus   |

Full roster including zero-XP bench: `~/projects/generals/bench-roster.md`

---

## 3. Spawn Templates

> **Full templates with code examples**: `~/projects/generals/spawn-patterns.md`

| Pattern              | When                                    | Team Size | Cost     |
|----------------------|-----------------------------------------|-----------|----------|
| Sequential Pipeline  | Content: draft → edit → validate        | 3-5       | Moderate |
| Parallel + Coord     | Independent streams + central coord     | 4-8       | High     |
| Verification Sweep   | QA across multiple items                | 2-4       | Low-Med  |
| Solo Deep Work       | Single specialist sufficient            | 1         | Low      |
| Multi-perspective    | Review panel with fresh-eyes zero-XP    | 3-5       | Moderate |

---

## 4. Model Assignment

| Role                            | Default | Override When                    |
|---------------------------------|---------|----------------------------------|
| Coordinator                     | Opus    | Never downgrade                  |
| Core specialist (complex)       | Opus    | Sonnet if routine/repetitive     |
| Core specialist (routine)       | Sonnet  | Opus if unexpectedly complex     |
| Zero-XP (first deployment)      | Sonnet  | Opus if task is complex          |
| Validator (Ramsay, CISO)        | Sonnet  | Haiku OK for simple pass/fail    |

---

## 5. Service Record Checklist (NON-NEGOTIABLE)

After every team deployment, before TeamDelete:
1. Write service record → update profiles + YAMLs in `~/projects/generals/`
2. `git add`, `git diff --staged`, commit, push, verify push
3. `TeamDelete` — only after push confirmed

---

## 6. Circuit Breakers

**Agent:** 3 consecutive failures → stop + report | Exceeds 3x time → checkpoint + escalate | Fails validation 2x → halt + human review
**Campaign:** >50% agents lost → halt | Cost >2x estimate → pause + report | Wake-the-Founder → pause
**Recovery:** Coordinator writes incident note before resuming.

---

**Detailed docs moved to `~/projects/generals/`:** XP Reference → `PROGRESSION-SYSTEM.md` | Sync Protocol → `SYNC-PROTOCOL.md` | Post-Mortems → `post-mortems/`
