---
name: armies:match-commander-to-task
description: Analyze task requirements and recommend commanders from ~/.armies/profiles/ based on personality fit
---

# Match Commander to Task

Analyze task requirements and recommend best-fit commander(s) based on personality traits, strengths, and past deployment experience.

**Prevents mismatches** (Patton for methodical validation, Spruance for aggressive breakthrough).

---

## Usage

```
/armies:match-commander-to-task <task-description>
```

**Example**:
```
/armies:match-commander-to-task "Write technical white paper on enterprise EDR deployment"
```

---

## How It Works

### Step 1: Analyze Task Requirements

Extract task characteristics:
- **Pace**: Fast/aggressive vs. methodical/careful
- **Style**: Bold/dramatic vs. calm/measured
- **Complexity**: Simple execution vs. complex coordination
- **Domain**: Technical, strategic, creative, operational
- **Risk tolerance**: High-risk breakthrough vs. low-risk validation
- **Communication**: Direct/forceful vs. measured/diplomatic

### Step 2: Scan Commander Profiles

Read all profiles in `~/.armies/profiles/` (authoritative source — `camp-david` repo):
- Personality alignment (1-5 stars)
- Domain expertise
- **PRIORITY: Give experience to underutilized commanders (0 XP when possible)**
- Past deployment success in similar tasks (check `~/.armies/service-records/`)

Run eligibility check before recommending:
```bash
python ~/.armies/bin/check-general-eligibility.py <name> <role>
```

Only recommend commanders who pass eligibility for the intended role.

### Step 3: Recommend Top 3

Return:
1. **Best match** — Highest fit, detailed rationale
2. **Alternative 1** — Second-best, why they'd also work
3. **Alternative 2** — Third option, different approach

For each:
- Personality fit explanation
- What they'll excel at
- What to watch for
- Expected timeline/approach

---

## Output

```
TASK ANALYSIS
═══════════════════════════════════════════════════════════════
Task: [Description]

Requirements:
• Pace: [Fast/Methodical]
• Style: [Bold/Measured]
• Domain: [Technical/Strategic/Creative]
• Risk: [High/Low]

RECOMMENDED COMMANDERS
═══════════════════════════════════════════════════════════════

🥇 BEST MATCH: [Commander]
Personality Fit: ⭐⭐⭐⭐⭐ (5/5)
Domain Expertise: [Experience]
Eligibility: [ELIGIBLE / BLOCKED]

Why:
• [Reason 1]
• [Reason 2]

Excel At:
• [Strength 1]

Watch For:
• [Potential issue]

Timeline: [Fast/Medium/Slow]

---

🥈 ALTERNATIVE 1: [Commander]
[Similar format...]

---

🥉 ALTERNATIVE 2: [Commander]
[Similar format...]

═══════════════════════════════════════════════════════════════
RECOMMENDATION: Use [Best Match] unless [specific reason for alternative]

Next: /armies:spawn-commander [best-match] "[task]"
```

---

## Commander Types Quick Reference

| Type | Commanders | Best For |
|------|-----------|----------|
| **Aggressive** | Patton, Halsey, MacArthur | Fast execution, breakthrough |
| **Methodical** | Spruance, Bradley, Nimitz | Quality validation, accuracy |
| **Coordinators** | Montgomery, Eisenhower, Marshall | Team lead, coalition |
| **Technical** | Hopper, Rickover, Groves | Technical depth, engineering |
| **Creative** | Mitchell, Slim, Rommel | Innovation, adaptation |

---

## Dependencies

**Required**:
- `Read` — Scan profiles from `~/.armies/profiles/`
- `Glob` — Find all profiles
- `Bash` — Run eligibility check
