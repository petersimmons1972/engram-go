---
name: advisory-gate
description: >
  Advisory Protocol checkpoint — invoke BEFORE selecting any implementation approach
  when 2+ options exist with meaningfully different long-term consequences (ADV.1 trigger).
  Spawns opus-advisor, waits for RECOMMENDATION, then returns. Use inside brainstorming,
  writing-plans, or any workflow where multiple paths diverge.
---

# Advisory Gate

A wrapper around the Advisory Protocol that enforces the ADV.1-ADV.5 check before any approach is selected. Use this skill when you recognize — or suspect — that a decision here will have consequences that are hard to reverse or diverge significantly between options.

## When This Applies (ADV.1-ADV.5 Triggers)

| Code | Trigger | Example |
|------|---------|---------|
| **ADV.1** | Architecture fork: 2+ approaches with meaningfully different long-term consequences | Hook vs. MCP config vs. two-stage recall |
| **ADV.2** | Infrastructure change: K8s, DNS, cert-manager, Cloudflare, storage | Changing a Cloudflare rule that affects prod routing |
| **ADV.3** | Large refactor: restructuring a module/class/boundary >100 lines | Splitting a 400-line hook into a library |
| **ADV.4** | Stuck on reasoning: same root cause failed twice, failure is logic not capacity | Debugging a race condition with two wrong theories |
| **ADV.5** | Irreversible + ambiguous: can't easily undo and right answer isn't clear | Deleting a database table with no backup |

**If in doubt, trigger. The cost of an unnecessary advisory call is low. The cost of a wrong architecture choice is high.**

## How to Use

1. **Recognize the trigger.** Look at the options you're considering. If any ADV.1-ADV.5 code applies, proceed with this skill.

2. **Construct the briefing** using this format (from `~/docs/advisory-protocol.md`):
   - **Decision** — one sentence
   - **Options** — A, B, (C) each with one-sentence tradeoffs
   - **Lean** — current preference and source of uncertainty
   - **Context** — relevant file paths, constraints, prior attempts

3. **Spawn `opus-advisor`:**
   ```
   Agent(
     subagent_type: "opus-advisor",
     prompt: "<your briefing>"
   )
   ```

4. **Wait for RECOMMENDATION.** Do not proceed to select an approach until the advisor returns. The RECOMMENDATION is the output — accept it or explicitly note why you're overriding it.

5. **Continue the workflow** with the recommended approach.

## For Subagents

If you are a dispatched subagent and you encounter 2+ implementation approaches with meaningfully different consequences, you MUST invoke this skill (or call opus-advisor directly) before proceeding. Do not make the architecture decision unilaterally.

## Escalation Threshold

- **Haiku**: No advisory calls — defer to coordinator
- **Sonnet**: Check ADV.1-ADV.5; if triggered, invoke advisory-gate before proceeding
- **Opus**: Already the advisor tier; use judgment directly

## Reference

Full detail, examples, and briefing format: `~/docs/advisory-protocol.md`
