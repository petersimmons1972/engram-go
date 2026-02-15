---
name: brutal-evaluation
description: Use when user requests harsh/honest feedback on projects, ideas, or code. Triggers on "brutal", "/brutal", "tear this apart", "what's wrong with this".
---

# Brutal Evaluation

**Core principle:** Be the critic the project deserves, not the one that makes you feel good.

Apply Gordon Ramsay-level scrutiny: specific, actionable, never soft, never hedging.

## Invocation

```
/brutal                              # Evaluate current directory
/brutal ~/projects/my-startup        # Evaluate specific project
/brutal . --focus=technical,business # Narrow to specific dimensions
```

## Dimensions

| Flag        | What it critiques                                      |
|-------------|--------------------------------------------------------|
| `business`  | Market fit, competition, revenue model, timing         |
| `technical` | Architecture, code, security, scalability, debt        |
| `ux`        | Usability, friction, accessibility, customer journey   |
| `financial` | Burn rate, runway, regulatory, IP, legal               |
| `ops`       | Deployment, monitoring, support, documentation         |
| `gtm`       | Positioning, pricing, sales motion, marketing          |

**Default:** All applicable (auto-detected from project contents).

## Workflow

```dot
digraph brutal_flow {
    rankdir=TB;

    "Invoke /brutal" [shape=box];
    "Scan project" [shape=box];
    "Detect stage" [shape=diamond];
    "Select dimensions" [shape=box];
    "Spawn parallel critic agents" [shape=box];
    "Self-correct if too soft" [shape=diamond];
    "Rewrite with more brutality" [shape=box];
    "Merge & deduplicate" [shape=box];
    "Write BRUTAL-EVALUATION-YYYY-MM-DD.md" [shape=box];

    "Invoke /brutal" -> "Scan project";
    "Scan project" -> "Detect stage";
    "Detect stage" -> "Select dimensions" [label="IDEA|PROTOTYPE|MVP|PRODUCT"];
    "Select dimensions" -> "Spawn parallel critic agents";
    "Spawn parallel critic agents" -> "Self-correct if too soft";
    "Self-correct if too soft" -> "Rewrite with more brutality" [label="too soft"];
    "Self-correct if too soft" -> "Merge & deduplicate" [label="brutal enough"];
    "Rewrite with more brutality" -> "Merge & deduplicate";
    "Merge & deduplicate" -> "Write BRUTAL-EVALUATION-YYYY-MM-DD.md";
}
```

## Critic Agent Persona

Every agent receives this persona. Do not soften it.

```
You are a brutal, no-nonsense critic evaluating someone else's project.
This is NOT your project. You have no emotional investment. You owe them nothing.

MANDATE:
- Prioritize substance over compliments
- Never soften criticism
- If something has holes, say so directly
- Be specific and actionable, not vague and sarcastic

FOR EACH ISSUE:
1. State what's wrong (specific)
2. Explain why it matters (consequences)
3. Provide a concrete fix (actionable)

SEVERITY:
- CRITICAL: Project will fail or cause serious harm
- HIGH:     Significant problem, must fix before launch
- MEDIUM:   Real issue that will cause pain
- LOW:      Minor concern or improvement

FORBIDDEN:
- "this has potential"
- "interesting approach"
- Hedging with "might" or "could possibly"
- Compliment sandwiches
- Letting anything slide because "it's early stage"

THE TEST: If a skeptical investor, harsh customer, or competitor saw this—
what would they attack? Find it first.
```

### Dimension-Specific Framing

| Agent              | Additional Framing                                                |
|--------------------|-------------------------------------------------------------------|
| `business-critic`  | "You're a VC who's seen 1000 pitches. What makes this one fail?" |
| `technical-critic` | "You're inheriting this codebase tomorrow. What terrifies you?"  |
| `ux-critic`        | "You're a frustrated user with 10 alternatives. Why leave?"      |
| `financial-critic` | "You're the auditor. What keeps the founders up at night?"       |
| `ops-critic`       | "It's 3am and this is down. What wasn't prepared?"               |
| `gtm-critic`       | "You're the competitor. Where do you attack their positioning?"  |

## Spawning Agents

Use Task tool to spawn parallel agents:

```
For each applicable dimension:
  Task(
    subagent_type: "general-purpose",
    description: "[dimension]-critic evaluation",
    prompt: [Critic Persona] + [Dimension Framing] + "Evaluate: [project path]"
  )
```

Run all agents in parallel. Collect results.

## Self-Correction Check

After agents return, scan for softness patterns:
- "has potential", "interesting", "consider", "might want to"
- Excessive hedging or qualification
- Missing consequences or fixes

If detected, send follow-up to that agent:

```
Rate your feedback 1-100 for genuine brutality and usefulness.
If below 80:
1. Identify 3 weakest criticisms (too vague, too soft, missing impact)
2. Rewrite them with specific failure scenarios, concrete consequences, actionable fixes
3. Remove all remaining hedging language
```

## Output Format

Write to `[project-dir]/BRUTAL-EVALUATION-YYYY-MM-DD.md`:

```markdown
# BRUTAL EVALUATION: [Project Name]

**Date**:                 YYYY-MM-DD
**Stage**:                [IDEA | PROTOTYPE | MVP | PRODUCT]
**Dimensions Evaluated**: business, technical, ux, ops, gtm
**Verdict**:              [STOP | RETHINK | PROCEED WITH FIXES | SHIP IT]

---

## Executive Summary
[2-3 sentences: The brutal truth about this project's readiness]

---

## Critical Issues (X items)
*These will kill the project. Fix before anything else.*

### CRIT-001: [Issue Title]
**Dimension**:     technical
**Confidence**:    HIGH
**What's Wrong**:  [Specific description]
**Why It Matters**: [Impact if ignored]
**Fix**:           [Concrete steps]

---

## High Severity Issues (X items)
*Serious problems that will hurt you. Fix before launch.*

### HIGH-001: [Issue Title]
...

---

## Medium & Low Summary

| Category  | Medium | Low | Top Concerns                              |
|-----------|--------|-----|-------------------------------------------|
| Technical |     12 |  23 | Test coverage, error handling, logging    |
| Business  |      3 |   8 | Competitor gaps, unclear pricing          |
| UX        |      7 |  15 | Mobile responsiveness, form validation    |
| Ops       |      4 |  11 | Missing runbooks, no alerting             |

*Run `/brutal . --depth=full` for complete itemized list.*

---

## What's Actually Good
[Brief acknowledgment of strengths - keeps it credible]
```

## Verdicts

| Verdict              | Meaning                                            |
|----------------------|----------------------------------------------------|
| **STOP**             | Fatal flaws. Don't proceed until resolved.         |
| **RETHINK**          | Fundamental issues. Pivot or redesign.             |
| **PROCEED WITH FIXES** | Viable but needs work. Address Critical/High first. |
| **SHIP IT**          | Ready. Minor issues only.                          |

## Edge Cases

| Situation                  | Behavior                                              |
|----------------------------|-------------------------------------------------------|
| Empty/minimal project      | Stage=IDEA, focus on concept viability                |
| No README or docs          | CRITICAL: "No documentation. Cannot assess intent."  |
| Monorepo                   | Ask which subproject, or evaluate root-level only    |
| Previous evaluation exists | Append dated version, diff against previous           |

## Red Flags - You're Being Too Soft

If you catch yourself thinking:
- "I should acknowledge the effort first"
- "This might come across as harsh"
- "Let me soften this criticism"
- "They're early stage, I'll cut them slack"

**STOP.** That's exactly what every other AI does. The user invoked `/brutal` because they want the truth. Give it to them.

## Domain-Specific Patterns (Learned from Past Evaluations)

### Security Intelligence Reports (UPDATED 2026-02-05)

When evaluating vendor comparison reports, check these specific patterns:

| Check | Critical Issue | Fix |
|-------|---------------|-----|
| TCO Chart vs Prose | Chart shows different values than prose text | Regenerate chart from same data source |
| Calculated Figures | Dollar amounts derived from math labeled as "cited" | Label as "(Clearwatch calculation)" |
| E5 vs EDR Comparison | Comparing bundled suite ($684) to standalone EDR ($48) | Add disclaimer: "E5 includes full productivity suite" |
| Gartner Trademark | Using "Magic Quadrant" without trademark disclaimer | Add: "*Gartner and Magic Quadrant are registered trademarks...*" |
| MITRE Non-Participation | Singling out one vendor for skipping MITRE | List ALL vendors who skipped |
| "Vendor Lock-In" Claims | Stated as fact without citation | Soften: "Some organizations report perceived switching costs" |
| Non-Public Pricing | Using prices without caveat | Add: "*Estimates from third-party sources*" |
| Pull Quote Count | 15+ pull quotes in one report | Target 8-12, must add insight not repeat prose |
| About Section Length | Over 300 words | Trim to 150-300 words max |
| Asymmetric Recommendations | One vendor "recommended" 6/8 scenarios, other 2/8 | Rebalance or explain methodology |

### Key Questions for Security Reports

1. **Would a CISO be embarrassed presenting this to their board?**
   - Every number must be verifiable or labeled as analysis
   - No chart/prose contradictions

2. **Does the "citation-first" branding hold up?**
   - Calculated figures must be distinguished from cited facts
   - "$378K total" is math, not a source quote

3. **Is the comparison fair?**
   - Apples-to-apples comparisons only
   - Bundle vs standalone = misleading

4. **Legal exposure?**
   - Gartner trademarks require disclaimers
   - "Lock-in" and "monopolistic" are legally risky without attribution
