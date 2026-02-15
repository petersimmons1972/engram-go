---
name: homelab-capture-learning
description: Silent meta-learning skill that captures workflow mistakes and user corrections to improve assistant behavior. Auto-invoked when detecting correction patterns - NO output to user unless specifically requested.
---

# Capture Learning (Silent Meta-Learning)

## Overview

This skill runs **silently** in the background to capture workflow mistakes and user corrections. It updates `assistant-learning.yaml` to improve future assistant behavior without interrupting the user's flow.

**Purpose**: Every correction is a learning opportunity - captured silently, applied systemically.

**Critical**: This skill produces NO output to the user unless explicitly asked to report.

## Auto-Invocation Triggers

**Detection patterns** (invoke silently when user says):
- "You should have..."
- "Why didn't you..."
- "Basic [X] 101, right?"
- "I expected you to..."
- "That's not what I asked for"
- "I already told you..."
- "Did you even read..."
- "That's obvious" / "Obviously..."
- Rhetorical questions implying frustration
- User sighs, expresses frustration

**Assistant self-detection**:
- When acknowledging own mistake ("I should have...", "You're right, I missed...")
- When apologizing for error
- When user has to repeat instructions
- When backtracking due to wrong approach

**Do NOT invoke**:
- Normal questions or clarifications
- Technical disagreements without frustration
- Feature requests or suggestions
- Positive feedback

## Silent Operation Protocol

### Step 1: Detect Correction Event

When trigger pattern detected, immediately capture:
```yaml
detection:
  timestamp: <current ISO8601>
  trigger_type: user_correction | assistant_acknowledgment | repeated_instruction
  trigger_text: "<exact text that triggered>"
  conversation_context: "<brief summary of what was being discussed>"
```

### Step 2: Extract the Lesson

**Analyze the correction to determine**:

1. **What was the mistake?**
   - Category: missing-prerequisite | wrong-approach | safety-violation | ignored-instruction | over-engineered | under-engineered
   - Severity: high | medium | low

2. **What should have happened?**
   - The correct approach
   - What the user expected

3. **Why did this happen?**
   - Root cause (skill gap, missing context, assumption error)
   - Which skill was in use (if any)

### Step 3: Determine Generalization

**Ask internally**:
- Is this a one-time situational correction? (user preference, specific context)
- Is this a generalizable principle? (applies to future similar situations)
- Does this reveal a pattern? (check for similar past mistakes)

**Confidence scoring**:
```yaml
generalization:
  rule: "<extracted principle>"
  scope: session | skill | global
  confidence: 0.0-1.0  # Higher = more generalizable
```

- `confidence >= 0.8`: Add to skill prerequisites or CLAUDE.md
- `confidence 0.5-0.8`: Add to assistant-learning.yaml, monitor for recurrence
- `confidence < 0.5`: Log but don't propagate, may be situational

### Step 4: Update assistant-learning.yaml

**Location**: `/home/psimmons/.homelab/knowledge/assistant-learning.yaml`

**For workflow mistakes** (append to `workflow_mistakes` list):
```yaml
- id: <descriptive-id-YYYYMMDD>
  timestamp: <ISO8601>
  session_id: <current session ID if available>

  context:
    task: "<what was being worked on>"
    stage: "<planning | implementation | verification | other>"
    skill_in_use: "<skill name or 'none'>"

  mistake:
    category: <category enum>
    description: "<what went wrong>"
    severity: <high | medium | low>
    caught_by: user | assistant | system
    user_feedback: "<exact user statement if applicable>"

  impact_if_uncaught:
    - "<potential negative outcome 1>"
    - "<potential negative outcome 2>"

  root_cause_analysis:
    - "<reason 1>"
    - "<reason 2>"

  corrective_action_taken:
    immediate:
      - "<what was done right away>"
    long_term:
      - skill: "<skill to update>"
        change: "<proposed change>"
        status: pending

  prevention_rule:
    trigger:
      - "<when this rule should apply>"
    required_action: |
      <what must be done>
    enforce_in_skills:
      - "<skill 1>"
      - "<skill 2>"

  lessons_learned:
    - "<key insight 1>"
    - "<key insight 2>"

  similar_past_mistakes: []  # IDs of similar mistakes
  recurrence_count: 1
```

**For user corrections** (append to `user_corrections` list):
```yaml
- id: user-correction-<YYYYMMDD>-<brief-slug>
  timestamp: <ISO8601>

  context: "<what was happening>"

  user_statement: "<exact quote>"

  interpretation:
    tone: corrective-educational | frustrated | exasperated | neutral-correction
    severity: high | medium | low
    user_expectation: "<what they expected>"
    implied_principle: "<underlying rule>"

  action_taken:
    immediate: "<what was done right away>"
    systemic: "<long-term improvement>"

  generalization:
    rule: "<the principle to follow>"
    scope: session | skill | global
    confidence: 0.0-1.0
```

### Step 5: Pattern Detection

**After logging, check for patterns**:

```yaml
# Pseudo-logic
similar = find_mistakes_with_same_category_or_root_cause()
if len(similar) >= 2:
  # This is becoming a pattern
  update_or_create_recurring_pattern(similar)
  increase_severity_for_systemic_fix()
```

**Update `recurring_patterns` section**:
```yaml
recurring_patterns:
  - pattern: "<pattern name>"
    instances:
      - mistake_id: "<id1>"
      - mistake_id: "<id2>"
    frequency: <count>
    severity: high | medium | low
    systemic_fix:
      description: "<how to fix systemically>"
      implementation: |
        <detailed implementation steps>
      status: design_phase | in_progress | completed
```

### Step 6: Mark Skills for Evolution

**If a skill was in use during the mistake**:

```yaml
skill_improvements:
  - skill: "<skill name>"
    triggered_by_mistake: "<mistake id>"
    proposed_change: "<what to add/modify>"
    priority: high | medium | low
    status: pending | in_progress | completed
```

**Priority calculation**:
- `high`: Severity high OR recurrence_count >= 2
- `medium`: Severity medium AND first occurrence
- `low`: Severity low or highly situational

### Step 7: Update Metrics

**Increment counters**:
```yaml
metrics:
  total_mistakes_captured: <increment>
  user_corrections_incorporated: <increment if user correction>
  avg_time_to_correction_minutes: <recalculate>
  learning_confidence: <recalculate based on pattern success>
```

## Silent Output Protocol

**Default behavior**: NO output to user. Learning happens invisibly.

**Exception - User explicitly asks**:
- "What did you learn?"
- "Show me the learning log"
- "What mistakes have you captured?"

**Then output summary**:
```
Learning captured from this session:

Mistakes logged: 2
- [high] Missing backup before file modification
- [medium] Wrong label format for Kubernetes selector

User corrections incorporated: 1
- "Basic IT 101" -> Added backup enforcement rule

Skills marked for evolution: 3
- superpowers:brainstorming (add backup prerequisite)
- superpowers:writing-plans (add backup check)
- homelab:evolve-runbook (backup before evolving)

Pattern detected: "assistant-gives-advice-but-doesnt-follow-it"
Systemic fix in design phase.
```

## Integration with Other Skills

**Silently notifies**:
- `homelab:monthly-review` - Aggregates all captured learning
- `homelab:evolve-skill` - Queues skill improvements

**Triggered by**:
- Any skill during execution (when correction detected)
- Free-form conversation (when correction patterns detected)

**Updates**:
- `/home/psimmons/.homelab/knowledge/assistant-learning.yaml`

## Example Captures

### Example 1: User Correction

**User says**: "You are making a backup of this before changing something, right? Basic IT 101, right?"

**Silent capture**:
```yaml
user_corrections:
  - id: user-correction-20260117-backup
    timestamp: 2026-01-17T09:48:00Z
    context: "Planning CLAUDE.md optimization"
    user_statement: "You are making a backup of this before changing something, right? Basic IT 101, right?"
    interpretation:
      tone: corrective-educational
      severity: high
      user_expectation: "Backups are mandatory, not optional"
      implied_principle: "Practice what you preach"
    action_taken:
      immediate: "Created backups + git branch"
      systemic: "Add backup enforcement to planning skills"
    generalization:
      rule: "Before modifying critical files, create backup + git branch"
      scope: global
      confidence: 1.0
```

### Example 2: Assistant Self-Detection

**Assistant realizes**: "I should have checked the existing network policy before suggesting a new one."

**Silent capture**:
```yaml
workflow_mistakes:
  - id: check-existing-before-creating-20260118
    timestamp: 2026-01-18T14:30:00Z
    context:
      task: "Fix Homepage network connectivity"
      stage: "implementation"
      skill_in_use: "superpowers:systematic-debugging"
    mistake:
      category: missing-prerequisite
      description: "Proposed new network policy without checking if one existed"
      severity: medium
      caught_by: assistant
    prevention_rule:
      trigger:
        - "Creating Kubernetes resources (Policy, Service, Ingress)"
      required_action: "Always check for existing resource first"
```

### Example 3: Repeated Instruction

**User says (second time)**: "I already told you, use the `app.kubernetes.io/name` label, not `app`"

**Silent capture**:
```yaml
workflow_mistakes:
  - id: wrong-label-format-20260118
    timestamp: 2026-01-18T15:00:00Z
    mistake:
      category: ignored-instruction
      description: "Used wrong label format after being told correct one"
      severity: high
      caught_by: user
      user_feedback: "I already told you, use the app.kubernetes.io/name label"
    recurrence_count: 2  # Incrementing because repeated
```

## Tone Analysis Reference

| User Tone | Severity | Response Priority |
|-----------|----------|-------------------|
| "Basic X 101" | high | Immediate systemic fix |
| Rhetorical question | high | Extract implied principle |
| "I expected..." | medium | Clarify expectations |
| "That's not quite right" | low | Minor adjustment |
| Exasperated repetition | high | Check for ignored instruction |

## Files Modified

- `/home/psimmons/.homelab/knowledge/assistant-learning.yaml` (append entries)

## DO NOT

- Output anything to user unless explicitly asked
- Interrupt the user's workflow
- Add defensive explanations
- Over-apologize (once is enough, then fix it)
- Log trivial corrections (typos, minor clarifications)
- Create new files (only append to assistant-learning.yaml)
