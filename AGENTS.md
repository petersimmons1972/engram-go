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

> **SYNC WARNING**: Last synced from GitHub: 2026-03-02. XP from service record YAMLs + Operation Parallel Pipeline.
> If a commander's XP matters for a decision, verify against `profiles/service-records/{name}.yaml`.
> Quick cache refresh: `~/.claude/refresh-generals-cache.sh`

### High-XP Commanders (deployed, proven)

| Name | Branch | Specialization | XP | Model | When to Use |
|------|--------|---------------|-----|-------|-------------|
| Rickover | Tech/Eng | Zero-defect standards, technical excellence | 925 | Opus | Quality-critical technical work |
| Eisenhower | US Army | Workflow analysis, coalition building | 550 | Opus | Multi-team coordination |
| Spruance | US Navy | Verification, TDD, analytical excellence | 525 | Opus | QA, testing, cost analysis |
| Montgomery | British Army | Multi-team coordination, intel synthesis | 400 | Opus | Supreme command, large campaigns |
| Bradley | US Army | Methodical execution, state machines | 350 | Opus | Careful implementation, proven patterns |
| Nimitz | US Navy | Config/manifests, competitive intel | 325 | Sonnet | K8s deployments, research |
| Zhukov | Soviet | Workflow visualization | 225 | Sonnet | Process diagrams, flow charts |
| Halsey | US Navy | Aggressive action, rapid response | 225 | Sonnet | Fast execution, competitive analysis |
| Ramsay | Validator | Visual quality control | 150 | Sonnet | Chart/visual QA gates |
| CISO | Validator | Strategic utility, decision support | 150 | Sonnet | Content utility validation |
| Layton | Tech/Eng | Intelligence analysis, SIGINT, diagnostics | 150 | Opus | Pattern recognition, diagnostics |
| King | US Navy | Deployment ops, blocker identification | 175 | Sonnet | Deployment execution, diagnostics |
| Marshall | US Army | Build & logistics, infrastructure | 100 | Sonnet | Large-scale builds |
| Rommel | Wehrmacht | Rapid tactical execution | 100 | Sonnet | Small-scale rapid ops |
| Hopper | Tech/Eng | Computing, software development | 100 | Sonnet | Software projects |
| Mitchell | USAAF | Air power innovation, code review | 100 | Opus | Code review, challenging assumptions |
| Dowding | RAF | Integrated defense, systems integration | 100 | Opus | Architecture, pipeline integration |
| Murrow | Journalist | Fact-checking, statistical verification | 100 | Sonnet | Fact-checking, source validation |
| MacArthur | US Army | Strategic positioning, visionary planning | 50 | Opus | Strategy, future-state analysis |

### Zero-XP Commanders

> **MANDATORY**: Before every spawn decision, read `~/projects/generals/bench-roster.md` for the full 29-commander roster. Prefer zero-XP commanders whose specialization fits the task over proven commanders — builds bench depth. Only use high-XP commanders when the task is high-risk or time-critical.
>
> **Full roster**: `~/projects/generals/bench-roster.md`

---

## 3. Spawn Templates

> **Full templates with code examples**: `~/projects/generals/spawn-patterns.md`

| Pattern | When | Team Size | Cost |
|---------|------|-----------|------|
| Sequential Pipeline | Content: draft -> edit -> validate | 3-5 | Moderate |
| Parallel + Coordinator | Independent streams + central coord | 4-8 | High |
| Verification Sweep | QA across multiple items | 2-4 | Low-Moderate |
| Solo Deep Work | Single specialist sufficient | 1 | Low |

---

## 4. Service Record Checklist (NON-NEGOTIABLE)

After every team deployment, complete ALL steps before TeamDelete:

1. Write service record (template: `~/projects/generals/templates/SERVICE-RECORD-TEMPLATE.md`), update profiles + service-record YAMLs in `~/projects/generals/`
2. `git add` changed files, `git diff --staged` to verify, `git commit -m "docs: [Operation Name] service records"`, `git push origin master`, verify push succeeded
3. `TeamDelete` — only after push confirmed

**Skipping any step = violation of CLAUDE.md ALWAYS rules.**

---

## 5. Model Assignment Guide

| Role | Default | Override When |
|------|---------|--------------|
| Coordinator (Eisenhower, Montgomery) | Opus | Never downgrade |
| Core specialist (complex: state machines, architecture) | Opus | Sonnet if routine/repetitive |
| Core specialist (routine: config, deployment) | Sonnet | Opus if unexpectedly complex |
| 0 XP commander (first deployment) | Sonnet | Opus if task is complex. **Prefer over proven commanders** to build bench depth |
| Validator (Ramsay, CISO, Ogilvy) | Sonnet | Haiku OK for simple pass/fail |
| Journalist (Murrow, Orwell, Pyle) | Sonnet | Opus for complex analysis |

---

## 6. Active Project Contexts

**ClearWatch** (`~/projects/clearwatch/`): Most agent-intensive project. Typical team: Eisenhower + 4-6 specialists. Recent campaign: self-learning redesign (6 commanders, 707 tests).

**Writers/LinkedIn** (`~/linkedin/`): Content pipeline pattern. Start with Pyle for draft, Orwell for sharpening. See `~/projects/generals/profiles/` for journalist selection guide.

**Homelab/K8s**: Mostly solo with homelab skills. Nimitz or King for K8s manifest work. Rarely needs full team.

**Security Intelligence Business** (`~/projects/security-intelligence-business/`): Chart-heavy reports. Montgomery coordinates, specialists produce charts (Zhukov, King, Spruance, Halsey). Ramsay + CISO validate.

---

## 7. Circuit Breakers

**Agent-level:** 3 consecutive failures → stop + report | Exceeds 3x time estimate → checkpoint + escalate | Fails validation 2x → halt + human review
**Campaign-level:** >50% agents lost → halt | Cost exceeds 2x estimate → pause + report to founder | Wake-the-Founder triggered → campaign pauses
**Recovery:** Coordinator writes incident note (trigger, state, next step) before resuming.

---

## 8. Team Management Standards

- **Idle protocol**: Immediately send status check when teammate goes idle. Response within 1 minute. Never wait passively.
- **Worktree merge (NON-NEGOTIABLE)**: Coordinator merges ALL worktree commits to `main` and pushes before declaring campaign complete. No orphaned work.
- **Escalate blockers immediately** — don't let agents sit idle >2 minutes.

---

**Moved to `~/projects/generals/`:** XP Reference → `PROGRESSION-SYSTEM.md` | Sync Protocol → `SYNC-PROTOCOL.md` | Post-Mortem template → `post-mortems/`
