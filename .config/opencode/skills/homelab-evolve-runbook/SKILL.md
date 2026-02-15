---
name: homelab-evolve-runbook
description: Update runbooks and playbooks based on actual effectiveness data from fix-effectiveness.yaml. Reorders steps by success rate, adds new discovered fixes, and archives ineffective approaches.
---

# Evolve Runbook

## Overview

Evolves runbooks and playbooks based on real-world effectiveness data. Compares documented steps against tracked outcomes in fix-effectiveness.yaml to reorder, add, or deprecate steps based on what actually works.

**Purpose**: Runbooks should reflect reality, not theory. This skill ensures documentation evolves with accumulated troubleshooting experience.

## When to Use

**Triggers**:
- Monthly review (as part of `homelab:monthly-review`)
- After using a runbook that didn't work as expected
- User says "update runbook" / "improve runbook" / "runbook didn't work"
- User says "evolve runbook" / "the runbook was wrong"
- After significant troubleshooting session with lessons learned

**When NOT to use**:
- Creating a new runbook (use `homelab:create-runbook` instead)
- During active incident (focus on resolution first)
- Without fix-effectiveness.yaml data (need baseline data)

## Data Sources

**Required files**:
```
/home/psimmons/.homelab/knowledge/fix-effectiveness.yaml  # Command/procedure stats
```

**Runbook/Playbook locations**:
```
/home/psimmons/RUNBOOKS/   # Single-process workflows
/home/psimmons/PLAYBOOKS/  # Diagnostic/incident response
```

## Process

### Step 1: Identify Runbook to Evolve

**From user input**:
```
User: "update the pod restart runbook"
-> /home/psimmons/RUNBOOKS/k8s-pod-restart.md
```

**From recent incident**:
```
After fixing Homepage issue, check if relevant runbook exists:
-> /home/psimmons/RUNBOOKS/homepage-network-policy.md
-> /home/psimmons/PLAYBOOKS/service-crashloop.md
```

**If runbook unclear**:
```
Which runbook should I evolve?

Available runbooks in /home/psimmons/RUNBOOKS/:
1. k8s-pod-restart.md
2. homepage-network-policy.md
3. ...

Available playbooks in /home/psimmons/PLAYBOOKS/:
1. service-404.md
2. service-crashloop.md
3. ...

Or specify a path to a runbook.
```

### Step 2: Load Effectiveness Data

**Read fix-effectiveness.yaml** and filter for relevant service/problem:

```yaml
# Extract relevant commands
relevant_commands = commands.filter(
  service == runbook.service OR
  problem_category == runbook.problem_category
)

# Extract relevant procedures
relevant_procedures = procedures.filter(
  service == runbook.service OR
  problem_category == runbook.problem_category
)
```

**Match runbook steps to tracked commands**:
```yaml
for each step in runbook:
  find matching_command in fix_effectiveness where:
    step.command ~= matching_command.command

  if found:
    step.tracked_success_rate = matching_command.success_rate
    step.tracked_attempts = matching_command.attempts
  else:
    step.tracked_success_rate = null  # Untracked step
```

### Step 3: Analyze Effectiveness

**Categorize each step**:

```yaml
for each step:
  if success_rate >= 0.90:
    category: "high_performer"
    action: "move_to_top"
    reason: ">{success_rate*100}% success rate with {attempts} attempts"

  elif success_rate >= 0.50:
    category: "moderate"
    action: "keep_position"
    reason: "Acceptable success rate, may need refinement"

  elif success_rate < 0.50 AND attempts >= 5:
    category: "deprecation_candidate"
    action: "flag_for_removal"
    reason: "Only {success_rate*100}% success after {attempts} attempts"

  elif tracked_success_rate == null:
    category: "untracked"
    action: "monitor"
    reason: "No effectiveness data - track future usage"
```

**Identify missing steps**:
```yaml
# Commands in fix-effectiveness that work but aren't in runbook
for each command in fix_effectiveness:
  if command.service == runbook.service:
    if command.success_rate >= 0.80:
      if command NOT in runbook.steps:
        suggest_adding(command)
```

### Step 4: Generate Proposed Changes

**Change types**:

1. **REORDER**: Move high-success steps earlier
   ```yaml
   change:
     type: reorder
     step: "Check network policy labels"
     from_position: 3
     to_position: 1
     reason: "90% success rate - highest of all steps"
     evidence: "18/20 attempts succeeded at this step"
   ```

2. **DEPRECATE**: Remove low-success steps
   ```yaml
   change:
     type: deprecate
     step: "Delete and recreate pod"
     current_position: 2
     reason: "Only 20% success rate"
     evidence: "3/15 attempts - treats symptom, not cause"
     alternative: "Check network policy labels first"
   ```

3. **ADD**: Insert new effective steps
   ```yaml
   change:
     type: add
     step: "Verify app.kubernetes.io/name label matches"
     suggested_position: 1
     reason: "Discovered during recent incidents"
     evidence: "100% success when label mismatch was root cause"
   ```

4. **UPDATE**: Modify step wording or commands
   ```yaml
   change:
     type: update
     step: "Check pod logs"
     modification: "Add --previous flag for crashed pods"
     reason: "Current logs often empty after crash"
   ```

### Step 5: Present Changes for Approval

**Output format**:

```
RUNBOOK EVOLUTION: [runbook-name.md]

Current runbook: [N] steps, [X]% overall tracked success rate
Data period: [date range of effectiveness data]

PROPOSED CHANGES:

1. REORDER: Move "[step name]" from step [N] -> step [M]
   Reason: [why this helps]
   Data: [X/Y] attempts, [Z]% success rate

2. DEPRECATE: Remove "[step name]" (currently step [N])
   Reason: [why this should go]
   Data: [X/Y] attempts - [explanation]
   Alternative: [what to do instead]

3. ADD NEW STEP: "[step description]"
   Suggested position: Step [N]
   Reason: [why this should be added]
   Data: [evidence from fix-effectiveness or recent incidents]

4. UPDATE: Modify "[step name]"
   Current: [current text]
   Proposed: [new text]
   Reason: [why change improves effectiveness]

---

SAFETY CHECK:
- Backup will be created: [runbook-name.md.backup-YYYYMMDD-HHMMSS]
- Changes require user approval before applying

Apply these changes? [Y/n]
```

### Step 6: Create Backup

**ALWAYS backup before modifying**:

```bash
cp /home/psimmons/RUNBOOKS/runbook-name.md \
   /home/psimmons/RUNBOOKS/runbook-name.md.backup-$(date +%Y%m%d-%H%M%S)
```

**Verify backup exists before proceeding**:
```bash
ls -la /home/psimmons/RUNBOOKS/runbook-name.md.backup-*
```

### Step 7: Apply Changes (With Approval)

**Only after explicit user approval ("yes", "y", "apply", "do it")**:

1. **Add/Update evolution metadata** at top of runbook:
   ```yaml
   ---
   # Runbook Evolution Metadata
   success_rate: 0.85
   times_used: 20
   avg_resolution_time_minutes: 5
   last_evolved: 2026-01-18
   evolution_history:
     - date: 2026-01-18
       change: "Moved 'check network policy' to step 1 (was step 3)"
       reason: "90% of failures resolved at this step"
     - date: 2026-01-15
       change: "Removed 'restart pod' from step 1"
       reason: "Only 20% success rate - treats symptom not cause"
   ---
   ```

2. **Apply each change**:
   - Reorder: Move step in document
   - Deprecate: Move to "Deprecated Steps" section at bottom (don't delete)
   - Add: Insert new step with clear instructions
   - Update: Modify step text

3. **Update "Last Updated" footer**

### Step 8: Confirm Changes

**After applying**:

```
RUNBOOK EVOLUTION COMPLETE

Backup: /home/psimmons/RUNBOOKS/runbook-name.md.backup-20260118-195500
Updated: /home/psimmons/RUNBOOKS/runbook-name.md

Changes applied:
- [x] Moved "Check network policy labels" to step 1
- [x] Deprecated "Delete and recreate pod" (moved to Deprecated section)
- [x] Added "Verify label matches" as step 2
- [x] Updated metadata with evolution history

New success rate projection: 85% (based on step ordering)

To restore previous version:
  cp /home/psimmons/RUNBOOKS/runbook-name.md.backup-20260118-195500 \
     /home/psimmons/RUNBOOKS/runbook-name.md
```

## Runbook Metadata Schema

**Added to top of evolved runbooks**:

```yaml
---
# Runbook Evolution Metadata
success_rate: FLOAT           # Calculated from step success rates (0.0-1.0)
times_used: INTEGER           # Total documented usage count
avg_resolution_time_minutes: INTEGER  # Average time to resolution
last_evolved: DATE            # ISO date of last evolution
data_source: STRING           # Path to fix-effectiveness.yaml

evolution_history:
  - date: DATE                # When change was made
    change: STRING            # What changed (human-readable)
    reason: STRING            # Why it changed (data-driven)
    previous_success_rate: FLOAT  # Before this change
    new_success_rate: FLOAT       # After this change (projected or measured)
---
```

## Deprecated Steps Section

When deprecating a step, move it to a "Deprecated Steps" section at the bottom:

```markdown
---

## Deprecated Steps

These steps have been removed from the main procedure due to low effectiveness.
Kept for historical reference.

### (Deprecated 2026-01-18) Delete and Recreate Pod

**Why deprecated**: Only 20% success rate (3/15 attempts). Treats symptom, not cause.

**Alternative**: Check network policy labels first (Step 1)

**Original step**:
kubectl delete pod homepage-xxx -n default

---
```

## Edge Cases

### No Effectiveness Data

If fix-effectiveness.yaml has no data for the runbook's service:

```
No effectiveness data found for service: [service]
Problem categories checked: [list]

Options:
1. Run the runbook on next incident and track results with homelab:log-fix-result
2. Manually assess step effectiveness based on experience
3. Skip evolution until data is available

Would you like to proceed with manual assessment?
```

### Runbook Not Found

```
Runbook not found: [specified path]

Available runbooks:
/home/psimmons/RUNBOOKS/
  - k8s-pod-restart.md
  - [list others]

/home/psimmons/PLAYBOOKS/
  - service-404.md
  - [list others]

Specify runbook path or name.
```

### Conflicting Evidence

When different incidents show conflicting success for same command:

```
Conflicting data for step: "Restart Apache"

Success cases (3):
- 2026-01-10: Resolved Nextcloud 503 after config change
- 2026-01-08: Resolved Nextcloud 503 after memory issue

Failure cases (2):
- 2026-01-15: Failed - root cause was database connection
- 2026-01-12: Failed - root cause was disk space

Analysis: Step effective for service issues, not infrastructure issues.
Suggestion: Add pre-condition check for disk space and database connectivity.
```

### All Steps Untracked

```
No effectiveness data tracked for any steps in this runbook.

To build effectiveness data:
1. Use homelab:log-fix-result after each troubleshooting session
2. Track both successes AND failures
3. Re-run homelab:evolve-runbook after 5+ incidents

Would you like me to set up tracking reminders for this runbook's steps?
```

## Integration with Other Skills

**Data sources from**:
- `homelab:log-fix-result` - Populates fix-effectiveness.yaml with command outcomes
- `homelab:log-incident` - Provides incident context for analysis

**Called by**:
- `homelab:monthly-review` - Triggers runbook evolution as part of monthly analysis

**Related skills**:
- `homelab:create-runbook` - Creates new runbooks (this skill evolves existing ones)
- `homelab:troubleshoot-common-issues` - Uses evolved runbooks for troubleshooting

## Safety Requirements

**ALWAYS**:
- Create backup before any modification
- Require explicit user approval for changes
- Track all changes in evolution_history
- Keep deprecated steps (don't delete, move to Deprecated section)
- Verify backup exists before applying changes

**NEVER**:
- Modify runbook without backup
- Apply changes without user approval
- Delete steps entirely (deprecate instead)
- Overwrite existing backup files

## Example Session

```
User: "The pod restart runbook didn't work for Homepage"

Assistant: I'll help evolve the pod restart runbook based on effectiveness data.

[Reads /home/psimmons/RUNBOOKS/k8s-pod-restart.md]
[Reads /home/psimmons/.homelab/knowledge/fix-effectiveness.yaml]

RUNBOOK EVOLUTION: k8s-pod-restart.md

Current runbook: 3 options (rollout restart, delete pod, scale down/up)
Data period: 2025-11-01 to 2026-01-18

PROPOSED CHANGES:

1. REORDER: Move "Option 2: Delete Pod" to last option (was Option 2)
   Reason: Low success rate for Homepage issues
   Data: 3/15 attempts (20%) - pod recreates with same error

2. ADD NEW STEP: "Check network policy labels first"
   Suggested position: Before all restart options
   Reason: Root cause is often network policy, not pod state
   Data: 5/5 attempts (100%) success with kubectl apply -f networkpolicy.yaml

3. UPDATE: Add Homepage-specific note to rollout restart
   Current: "Restart a deployment"
   Proposed: "Restart a deployment (for Homepage, check network policy labels first)"
   Reason: Prevents wasted restart attempts

---

SAFETY CHECK:
- Backup will be created: k8s-pod-restart.md.backup-20260118-143000
- Changes require user approval before applying

Apply these changes? [Y/n]

User: "yes"

[Creates backup]
[Applies changes]

RUNBOOK EVOLUTION COMPLETE

Backup: /home/psimmons/RUNBOOKS/k8s-pod-restart.md.backup-20260118-143000
Updated: /home/psimmons/RUNBOOKS/k8s-pod-restart.md

Changes applied:
- [x] Reordered "Delete Pod" to last option
- [x] Added "Check network policy labels" as prerequisite step
- [x] Added Homepage-specific note
- [x] Updated metadata with evolution history

New success rate projection: 90% (based on network policy check first)
```

## Files Modified

- Runbooks in `/home/psimmons/RUNBOOKS/` (modified with approval)
- Playbooks in `/home/psimmons/PLAYBOOKS/` (modified with approval)

## Notes

- Evolution is data-driven, not opinion-driven
- Low success rate is defined as <50% with at least 5 attempts
- High success rate is defined as >=90% with at least 3 attempts
- Deprecated steps are preserved for historical reference
- All changes are tracked in evolution_history for audit trail
