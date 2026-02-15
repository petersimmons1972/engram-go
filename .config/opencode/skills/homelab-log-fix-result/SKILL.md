---
name: homelab-log-fix-result
description: Passive command effectiveness tracking. Records command outcomes after any fix is applied to update success/failure statistics in fix-effectiveness.yaml.
---

# Log Fix Result

## Overview

Silent command effectiveness tracker that records outcomes after any fix is applied. Updates fix-effectiveness.yaml with success rates, resolution times, and usage statistics to learn what works over time.

**Purpose**: Track command success/failure to build institutional knowledge of what actually works.

## When to Use

**Triggers**:
- After applying ANY fix command (auto-invoked silently)
- User explicitly says "log this fix" / "record fix result"
- After troubleshooting session concludes with a fix
- After runbook/playbook step completes

**When NOT to use**:
- Read-only diagnostic commands (kubectl get, logs, describe)
- Information gathering (no fix attempted)
- Partial fixes (wait for final resolution)

## Invocation

This skill can be invoked:
1. **Automatically** - After any fix command in troubleshooting
2. **Explicitly** - User says "log fix result" or similar
3. **Silently** - Integrated into other skills (systematic-debugging, incident-response)

## Required Information

### Minimum Required
- **command**: The exact command that was executed
- **service**: Service affected (homepage, nextcloud, traefik, etc.)
- **outcome**: success | failed

### Optional (Auto-Inferred When Possible)
- **problem_category**: Type of issue (pod-crashloop, network-policy-issue, etc.)
- **resolution_time_seconds**: Time from command execution to service healthy
- **notes**: Brief context about why it worked/failed

## Process

### Step 1: Capture Fix Details

**Gather from context**:
```yaml
command: "kubectl rollout restart deployment/homepage"
service: homepage
outcome: success
problem_category: pod-crashloop
resolution_time_seconds: 120
timestamp: 2026-01-18T14:30:00Z
notes: "Fixed after correcting network policy labels"
```

**Auto-detect when possible**:
- Service: From recent kubectl commands or conversation context
- Problem category: From error messages or symptoms discussed
- Resolution time: From timestamps in session

### Step 2: Update fix-effectiveness.yaml

**Location**: `/home/psimmons/.homelab/knowledge/fix-effectiveness.yaml`

**If command exists in file**:
```yaml
# Before
- command: "kubectl rollout restart deployment/homepage"
  service: homepage
  problem_category: pod-crashloop
  success_rate: 0.85
  attempts: 20
  successes: 17
  failures: 3
  avg_resolution_time_seconds: 120
  last_used: 2025-12-20T14:41:00Z

# After (if outcome=success)
- command: "kubectl rollout restart deployment/homepage"
  service: homepage
  problem_category: pod-crashloop
  success_rate: 0.857  # (17+1)/(20+1)
  attempts: 21
  successes: 18
  failures: 3
  avg_resolution_time_seconds: 120  # Updated running average
  last_used: 2026-01-18T14:30:00Z
```

**If new command (not in file)**:
```yaml
# Append new entry
- command: "kubectl delete pvc stuck-pvc"
  service: kubernetes
  problem_category: storage-issue
  success_rate: 1.0  # First attempt succeeded
  attempts: 1
  successes: 1
  failures: 0
  avg_resolution_time_seconds: 30
  last_used: 2026-01-18T14:30:00Z
  notes: "Initial entry"
```

### Step 3: Calculate Statistics

**Success rate formula**:
```
success_rate = successes / attempts
```

**Running average resolution time**:
```
new_avg = ((old_avg * (attempts - 1)) + new_time) / attempts
```

### Step 4: Update Statistics Section

Update the statistics block in fix-effectiveness.yaml:
```yaml
statistics:
  total_commands_tracked: N  # Increment if new command
  total_procedures_tracked: N
  highest_success_rate_command: "command-name"  # Recalculate if changed
  lowest_success_rate_command: "command-name"   # Recalculate if changed
  most_used_command: "command-name"             # Recalculate if changed
```

## Output Format

**Standard (silent mode)**:
```
Fix logged: kubectl rollout restart deployment/homepage [success]
```

**Verbose (when explicitly invoked)**:
```
Fix Result Logged
  Command: kubectl rollout restart deployment/homepage
  Service: homepage
  Outcome: success
  Resolution time: 120s
  Updated success rate: 85.7% (18/21)
  Updated fix-effectiveness.yaml
```

## Schema Reference

**Command entry format**:
```yaml
commands:
  - command: STRING              # Exact command executed
    service: STRING              # Service affected
    problem_category: STRING     # Issue category
    success_rate: FLOAT          # 0.0 to 1.0
    attempts: INTEGER            # Total attempts
    successes: INTEGER           # Successful attempts
    failures: INTEGER            # Failed attempts
    avg_resolution_time_seconds: INTEGER  # Average time to healthy
    last_used: ISO8601_TIMESTAMP # Last execution time
    notes: STRING                # Optional context
```

**Problem categories** (standard values):
- pod-crashloop
- network-policy-issue
- storage-issue
- configuration-error
- apache-down
- database-issue
- mouse-freeze
- dns-issue
- certificate-issue
- resource-exhaustion

## Integration with Other Skills

**Called by**:
- `homelab:incident-response` - Logs fix results during incident resolution
- `superpowers:systematic-debugging` - Logs fixes attempted during debugging
- `homelab:log-incident` - Updates fix effectiveness after incident capture

**Informs**:
- `homelab:evolve-runbook` - Uses success rates to improve runbooks
- `homelab:monthly-review` - Analyzes fix effectiveness trends
- `homelab:troubleshoot-common-issues` - Suggests highest success rate fixes first

## Examples

### Example 1: Successful Fix

```
Context: Fixed Homepage pod crashloop with rollout restart

Skill captures:
  command: kubectl rollout restart deployment/homepage
  service: homepage
  outcome: success
  problem_category: pod-crashloop
  resolution_time_seconds: 120

Output: Fix logged: kubectl rollout restart deployment/homepage [success]
```

### Example 2: Failed Fix Attempt

```
Context: Deleted pod but issue persisted

Skill captures:
  command: kubectl delete pod homepage-abc123
  service: homepage
  outcome: failed
  problem_category: pod-crashloop
  resolution_time_seconds: 0
  notes: Pod recreated with same error

Output: Fix logged: kubectl delete pod homepage-abc123 [failed]
```

### Example 3: New Command First Use

```
Context: First time using a specific fix

Skill captures:
  command: kubectl patch pvc data-pvc -p '{"metadata":{"finalizers":null}}'
  service: kubernetes
  outcome: success
  problem_category: storage-issue
  resolution_time_seconds: 15

Output: Fix logged: kubectl patch pvc... [success] (new command added)
```

## Minimal Quick Logging

For rapid logging without full context:

```
User: "log fix: mouse.sh worked"

Skill captures:
  command: ~/bin/mouse.sh
  service: hardware
  outcome: success
  problem_category: mouse-freeze

Output: Fix logged: ~/bin/mouse.sh [success]
```

## Files Modified

- `/home/psimmons/.homelab/knowledge/fix-effectiveness.yaml` (updated)

## Notes

- This skill prioritizes minimal output to avoid interrupting workflow
- Statistics are recalculated on each update to maintain accuracy
- Low success rate commands (<50%) may trigger suggestions to improve or replace the approach
- Commands with 100% success rate are candidates for automation
