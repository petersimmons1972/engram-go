---
name: armies:campaign-coordinator
description: Coordinate multi-commander operations using ~/.armies/spawn-patterns.md patterns, protocols, and quality gates
---

# Campaign Coordinator

Coordinate operations involving multiple commanders. Provides team structures, communication protocols, quality gates, and proven patterns.

**Use when**: Deploying 2+ commanders in parallel or complex multi-phase operations.

**Don't use**: Single commander tasks — just spawn directly.

---

## Canonical Reference

Full patterns, team structures, and the HALT/RESUME protocol live in:

```
~/.armies/spawn-patterns.md
```

Read it before designing any campaign.

---

## Pre-Campaign Requirements (MANDATORY)

### 1. Intelligence Check

Before any Pattern 2 (Parallel + Coordinator) campaign, check J-2 conditions. If CRITICAL conditions are active, acknowledge them in your plan before proceeding.

### 2. Eligibility Check

Run for every commander before spawn:
```bash
python ~/.armies/bin/check-general-eligibility.py <name> <role>
```

### 3. Worktree Isolation

Before spawning any implementation team (Patterns 1–5):
- **REQUIRED:** Use `superpowers:using-git-worktrees` before TeamCreate
- Exception: read-only/validation operations

### 4. Coordinator Tool Restriction

All coordinators are structurally scoped to: `Agent | Read | Grep | Glob | SendMessage`

**No Bash. No Write. No Edit.** Enforced at the agent file level.

---

## Campaign Patterns (Summary)

| Pattern | Use Case | Structure | Speed |
|---------|----------|-----------|-------|
| **Pattern 1: Sequential Pipeline** | Draft → Edit → Validate | 3-5 commanders in stages | Slow |
| **Pattern 2: Parallel + Coordinator** | Independent work streams | Coordinator + N specialists | Fast (50% of sequential) |
| **Pattern 3: Parallel Verification Sweep** | QA across multiple items | 2-4 validators in parallel | Low-Moderate cost |
| **Pattern 4: Solo Deep Work** | Single specialist sufficient | 1 commander, no TeamCreate | Low cost |
| **Pattern 5: Emergency Override** | Structurally blocked campaign | Pre-authorized Patton/Rommel | Emergency only |

Full invocation syntax for each pattern → `~/.armies/spawn-patterns.md`

---

## Team Structures

| Size | When | Structure |
|------|------|-----------|
| **Minimal** | <5 tasks, low complexity | You + 1-3 commanders |
| **Standard** | 5-20 tasks, medium complexity | Lead + Chief of Staff + 3-10 commanders + 2-3 validators |
| **Large** | 20+ tasks, high complexity | Full staff |

---

## CRITICAL: HALT/RESUME Protocol

**Problem**: Methodical commanders (Spruance, Bradley) wait for explicit RESUME. Aggressive commanders (Patton, Halsey) infer resumption from context.

**Rule**: After any HALT, send an explicit RESUME broadcast to ALL commanders:

```
ALL COMMANDERS: RESUME OPERATIONS

Blocker resolved: [description]
RESUME ALL WORK. You are authorized to proceed.
No further approvals required.

[Team Lead]
```

Never assume commanders will infer resumption.

---

## Quality Gates

| Gate | Purpose | Who | Pass Criteria |
|------|---------|-----|---------------|
| **Build** | Code compiles, no syntax errors | Moreell, Rickover | 100% build success |
| **Functional** | Features work as specified | Layton, Spruance | 100% test pass rate (20-50% sample) |
| **Brand** | Content accuracy, voice | Ogilvy | 100% consistency (30-50% spot-check) |

---

## Commander Assignment Priority

1. **Check `~/.armies/service-records/`** for current XP
2. **Prefer 0 XP commanders** when specialization matches
3. Only use heavily deployed commanders if no underutilized alternative exists

---

## Attribution (MANDATORY before TeamDelete)

Every campaign must capture attribution before TeamDelete. See `~/.armies/spawn-patterns.md` Attribution Requirements section.

All commits during an operation must include structured trailers:
```
Operation: [Operation-Name]
Team: [Name](role), [Name](role)
```

---

## Campaign Workflow

1. **Plan** — Define goals, choose pattern/structure, break into tasks
2. **Match** — Use `/armies:match-commander-to-task` for each role
3. **Spawn** — `TeamCreate` + `/armies:spawn-commander` for each member
4. **Execute** — Apply pattern, HALT/RESUME protocol
5. **Quality Gates** — Validate, fix, re-validate
6. **Award** — Use `/armies:award-experience` for each commander
7. **Retrospective** — Document lessons in service records

---

## Dependencies

**Required**:
- `TeamCreate` — Create team structure
- `Agent` — Spawn commanders
- `SendMessage` — Coordinate communication
- `TaskCreate/TaskUpdate` — Track work
- `~/.armies/spawn-patterns.md` — Full pattern reference
- `~/.armies/bin/check-general-eligibility.py` — Eligibility check
