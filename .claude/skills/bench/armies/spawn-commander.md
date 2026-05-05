---
name: armies:spawn-commander
description: Spawn commander with personality from armies profile, generating Agent tool invocation
---

# Spawn Commander

Read commander profile from `~/.armies/profiles/`, extract personality/historical context, generate spawn prompt preserving character.

**Foundation skill** — Everything starts here.

---

## Usage

```
/armies:spawn-commander <commander-name> <task-description>
```

**Parameters**:
- `commander-name` — Profile filename without .md (e.g., "patton", "grace-hopper", "montgomery")
- `task-description` — Clear mission statement (1-2 sentences)

**Examples**:
```
/armies:spawn-commander patton "Deploy aggressive variant website with bold CTA"
/armies:spawn-commander spruance "Validate trust variant for accuracy and calm tone"
/armies:spawn-commander montgomery "Coordinate 14-front parallel deployment"
```

---

## Pre-Flight: Eligibility Check (MANDATORY)

Before spawning, run the eligibility check:

```bash
python ~/.armies/bin/check-general-eligibility.py <name> <role>
# role: coordinator | emergency_reserve | specialist | validator
# exit 0 = eligible | exit 1 = blocked (reason on stderr)
```

| Effective Malus | Eligible Roles |
|----------------|----------------|
| 0 – 99         | All roles |
| 100 – 199      | specialist, validator only; emergency_reserve requires founder approval |
| 200 – 299      | specialist only; mandatory output review by Spruance or Ramsay |
| 300 – 399      | specialist only; escalate to founder before spawn |
| 400+           | Do not spawn |

---

## Callability Note

Commanders are only callable as named subagents (`subagent_type="<name>"`) if their profile has `status: active` and has been synced via `~/.armies/bin/sync-to-claude-agents.sh`. Profiles without `status: active` must be spawned as `subagent_type="general-purpose"` with the personality prompt embedded.

---

## How It Works

### Step 1: Read Profile

Location: `~/.armies/profiles/<commander-name>.md`

**Extracts**:
- Historical context (achievements, command roles)
- Core personality traits
- Strengths and weaknesses
- Current XP/competence (from service records)

### Step 2: Analyze Task Fit

**Match task requirements to commander personality**:
- Aggressive task → Aggressive commander (Patton, Halsey)
- Methodical task → Methodical commander (Spruance, Bradley)
- Coordination task → Multi-force coordinator (Montgomery, Eisenhower)
- Technical task → Technical specialist (Hopper, Rickover)
- Creative task → Innovative thinker (Mitchell, Slim, Rommel)

**If mismatch detected**: Warn but proceed with better recommendation.

### Step 3: Generate Spawn Prompt

**Includes**:
1. Identity: "You are [Rank] [Name]"
2. Mission: Clear task statement
3. Personality traits: Core characteristics to embody
4. Historical parallel: How task relates to historical experience
5. Strengths to leverage
6. Weaknesses to watch
7. Voice/tone: Communication style

### Step 4: Return Agent Tool Invocation

**Output**: Ready-to-execute Agent tool call with generated prompt.

---

## Commander Quick Reference

| Type | Commanders | Best For |
|------|-----------|----------|
| **Aggressive** | Patton, Halsey, MacArthur | Fast execution, breakthrough |
| **Methodical** | Spruance, Bradley, Nimitz | Quality validation, accuracy-critical |
| **Coordinators** | Montgomery, Eisenhower, Marshall | Team lead, coalition management |
| **Technical** | Hopper, Rickover, Groves | Technical validation, engineering |
| **Creative** | Mitchell, Slim, Rommel | Innovation, unconventional approaches |

---

## Critical: Worktree Before Implementation

Before spawning any implementation team (Patterns 1–5), create an isolated git worktree:
- **REQUIRED:** Use `superpowers:using-git-worktrees` before TeamCreate
- Exception: read-only/validation operations

---

## Workflow Integration

1. **Eligibility** — Run `check-general-eligibility.py`
2. **Match** — Use `/armies:match-commander-to-task` (finds best fit)
3. **Spawn** — Use this skill (generate personality prompts) ← **YOU ARE HERE**
4. **Coordinate** — Use `/armies:campaign-coordinator` (manage execution)
5. **Award** — Use `/armies:award-experience` (capture lessons, update service records)

---

## Dependencies

**Required**:
- `Read` — Load commander profile from `~/.armies/profiles/`
- `Agent` — Spawn agent with generated prompt

**Files**:
- Commander profile at `~/.armies/profiles/<name>.md`
- Eligibility script at `~/.armies/bin/check-general-eligibility.py`

**Optional**:
- `/armies:match-commander-to-task` — Find best fit first
- `~/.armies/spawn-patterns.md` — Pattern reference
