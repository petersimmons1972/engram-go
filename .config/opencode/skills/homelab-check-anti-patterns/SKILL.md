---
name: homelab-check-anti-patterns
description: Use when stuck on problem >20 minutes to check current approach against known anti-patterns. Detects tempting-but-wrong approaches and suggests proven correct solutions. Auto-invoked when user says "stuck" or repeats same commands.
---

# Check Anti-Patterns

## Overview

Pattern-matching system that catches you before wasting time on known-bad approaches. Compares current troubleshooting actions against historical anti-patterns and redirects to proven solutions.

**Purpose**: Stop wasting time on approaches that have failed before.

**Anti-patterns file**: `/home/psimmons/.homelab/config/anti-patterns.yaml`

## When to Use

**Auto-invoke triggers**:
- Stuck on problem >20 minutes
- User says "stuck" / "not working" / "tried everything" / "same error"
- Repeating same commands without progress
- Attempting same fix for 3rd time

**Manual invoke**:
- Before trying a "desperate" fix
- When about to restart something
- When considering major changes to fix minor issues

**When NOT to use**:
- Making genuine progress
- Novel problem with no history
- Already following correct approach

## Process

### Step 1: Load Anti-Patterns Database

Read anti-patterns from configuration:

```bash
# Load anti-patterns.yaml
cat /home/psimmons/.homelab/config/anti-patterns.yaml
```

**Current tracked anti-patterns**:

| ID                              | Tempting Action                          | Correct Action                    |
|---------------------------------|------------------------------------------|-----------------------------------|
| add-homepage-replicas           | Add Homepage replicas to fix crashes     | Fix policy labels                 |
| restart-traefik-for-single-404  | Restart Traefik when one service 404     | Check service config/ingress      |
| re-pair-mouse                   | Re-pair Logitech mouse via Bluetooth     | Run ~/bin/mouse.sh                |
| check-dns-for-homepage          | Check DNS when Homepage down             | Check network policies, pod status|
| add-verbose-logging-first       | Enable verbose logging before checking   | Check existing logs first         |
| restart-pod-without-logs        | Restart pod before checking logs         | kubectl logs first, THEN restart  |
| assume-backup-worked            | Assume backup worked without testing     | Test restore procedure monthly    |
| edit-live-manifests             | kubectl edit to modify live resources    | Edit source files, commit, apply  |
| skip-health-check               | Start troubleshooting without health-check | Run ~/bin/health-check.sh first |
| replace-traefik-with-nginx      | Switch from Traefik to Nginx             | Fix actual problem                |

### Step 2: Analyze Current Approach

**Gather context**:
1. What service/system is affected?
2. What commands have been run in this session?
3. What actions have been attempted?
4. How long has troubleshooting been going?

**Detection methods**:
- Review recent bash commands in session
- Check for keywords matching anti-pattern `tempting_action`
- Look for repeated commands without state change
- Identify problem type from context

### Step 3: Match Against Anti-Patterns

**Matching algorithm**:
```
FOR each command/action in session:
  FOR each anti_pattern in anti_patterns.yaml:
    IF command matches anti_pattern.tempting_action:
      MATCH FOUND -> trigger alert
    IF problem_type matches current_problem:
      CHECK if approach matches tempting_action
```

**Match indicators**:
- Command contains keywords (restart, add replicas, kubectl edit, etc.)
- Problem type matches known anti-pattern problem_type
- Multiple attempts without progress
- Time spent > anti_pattern.time_wasted_average_minutes

### Step 4: Alert on Match

**If anti-pattern detected, output**:

```
============================================================
ANTI-PATTERN DETECTED
============================================================

You're trying: {anti_pattern.tempting_action}

Why it's wrong: {anti_pattern.why_wrong}

Do instead: {anti_pattern.correct_action}

------------------------------------------------------------
STATISTICS
------------------------------------------------------------
This anti-pattern has been violated {times_violated} times before.
Average time wasted per violation: {time_wasted_average_minutes} minutes
Last violation: {last_violation}
============================================================

RECOMMENDED NEXT STEPS:
1. STOP current approach
2. {correct_action}
3. If that doesn't work, escalate per CLAUDE.md decision matrix
```

### Step 5: Update Violation Tracking

**When anti-pattern is violated, update anti-patterns.yaml**:

```yaml
# Update fields:
times_violated: {current + 1}
last_violation: {current_timestamp}
```

**Recalculate statistics**:
```yaml
statistics:
  total_violations: {sum of all times_violated}
  total_time_wasted_hours: {recalculate from averages}
  most_common_anti_pattern: {pattern with highest times_violated}
  highest_time_waste: {pattern with highest time_wasted_average_minutes}
```

## Output Examples

### Example 1: Single Service 404

**Situation**: Homepage returning 404, user about to restart Traefik

```
============================================================
ANTI-PATTERN DETECTED
============================================================

You're trying: Restart Traefik when one service returns 404

Why it's wrong: Single service 404 means service-specific issue, not Traefik problem

Do instead: Check service config and ingress configuration

------------------------------------------------------------
STATISTICS
------------------------------------------------------------
This anti-pattern has been violated 12 times before.
Average time wasted per violation: 10 minutes
Last violation: 2025-12-15
============================================================

RECOMMENDED NEXT STEPS:
1. STOP - do not restart Traefik
2. Check service config and ingress configuration:
   kubectl get ingress -A | grep homepage
   kubectl describe ingress homepage -n default
   kubectl get svc homepage -n default
3. If that doesn't work, escalate per CLAUDE.md decision matrix
```

### Example 2: Pod Restart Before Logs

**Situation**: Pod crashlooping, user typing `kubectl delete pod`

```
============================================================
ANTI-PATTERN DETECTED
============================================================

You're trying: Restart pod before checking logs

Why it's wrong: Destroys evidence of what went wrong

Do instead: kubectl logs <pod> first, THEN restart if needed

------------------------------------------------------------
STATISTICS
------------------------------------------------------------
This anti-pattern has been violated 15 times before.
Average time wasted per violation: 10 minutes
Last violation: 2026-01-05
============================================================

RECOMMENDED NEXT STEPS:
1. STOP - do not delete/restart the pod yet
2. Capture logs first:
   kubectl logs <pod-name> -n <namespace>
   kubectl logs <pod-name> -n <namespace> --previous
3. After reviewing logs, then restart if appropriate
```

### Example 3: Mouse Re-pairing

**Situation**: Mouse frozen, user asking about Bluetooth settings

```
============================================================
ANTI-PATTERN DETECTED
============================================================

You're trying: Re-pair Logitech mouse via Bluetooth

Why it's wrong: Driver state issue, not pairing issue

Do instead: Run ~/bin/mouse.sh to reload driver

------------------------------------------------------------
STATISTICS
------------------------------------------------------------
This anti-pattern has been violated 8 times before.
Average time wasted per violation: 5 minutes
Last violation: 2026-01-10
============================================================

RECOMMENDED NEXT STEPS:
1. STOP - do not re-pair the mouse
2. Run the driver reload script:
   ~/bin/mouse.sh
3. If still not working, logout/login
```

## No Anti-Pattern Match

**If no anti-pattern matches current approach**:

```
Anti-Pattern Check Complete
---------------------------
No known anti-patterns detected in current approach.

Current troubleshooting approach appears valid. If still stuck:
1. Check KNOWLEDGE-INDEX.md for similar past issues
2. Use superpowers:systematic-debugging skill
3. Review FAILURE-MODES-CATALOG.md for this service
4. Escalate per CLAUDE.md decision matrix (50-80% confidence = propose and wait)
```

## Quick Reference Commands

**View all anti-patterns**:
```bash
cat /home/psimmons/.homelab/config/anti-patterns.yaml
```

**Most violated anti-patterns**:
```bash
grep -A1 "times_violated:" /home/psimmons/.homelab/config/anti-patterns.yaml | head -20
```

**Check statistics**:
```bash
grep -A5 "statistics:" /home/psimmons/.homelab/config/anti-patterns.yaml
```

## Adding New Anti-Patterns

When a new anti-pattern is discovered (same mistake made 3+ times):

**Add to anti-patterns.yaml**:
```yaml
- id: new-anti-pattern-id
  tempting_action: "What seems like a good idea"
  why_wrong: "Why it actually doesn't work"
  correct_action: "What to do instead"
  problem_type: category-of-problem
  times_violated: 0
  last_violation: null
  time_wasted_average_minutes: 0
```

**Criteria for new anti-pattern**:
- Same mistake made 3+ times
- Clear alternative approach exists
- Wasted significant time (>5 min) each occurrence
- Generalizable (not one-off situation)

## Integration with Other Skills

**Triggers from**:
- `superpowers:systematic-debugging` - checks anti-patterns during debugging
- `homelab:incident-response` - checks before each action

**Triggers to**:
- `homelab:log-incident` - if anti-pattern led to extended outage
- `superpowers:systematic-debugging` - if no anti-pattern match and still stuck

**Updates**:
- `/home/psimmons/.homelab/config/anti-patterns.yaml` - violation counts

## Proactive Checks

**Auto-check triggers** (assistant should invoke this skill when detecting):

| Trigger Phrase       | Example User Input                           |
|----------------------|----------------------------------------------|
| Stuck expression     | "I'm stuck", "nothing is working"            |
| Repeated failure     | "tried that already", "same error again"     |
| Frustration          | "why isn't this working"                     |
| Time mention         | "been at this for an hour"                   |
| Desperation          | "let me just restart everything"             |

**Command repetition detection**:
If same command run 3+ times without success:
- Flag potential anti-pattern
- Suggest checking anti-patterns database
- Recommend systematic debugging skill
