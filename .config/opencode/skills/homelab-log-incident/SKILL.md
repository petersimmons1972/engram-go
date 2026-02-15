---
name: homelab-log-incident
description: Use after resolving an infrastructure incident to capture structured learning data. Interactive incident capture for failure history, timeline, root cause, and prevention actions. Automatically updates knowledge base and indexes.
---

# Log Infrastructure Incident

## Overview

Interactive incident capture system that transforms troubleshooting sessions into structured learning data. Captures complete incident context, tested solutions, and prevention measures to feed the self-learning knowledge system.

**Purpose**: Every incident is a learning opportunity - this skill ensures none are lost.

## When to Use

**Triggers**:
- After resolving any infrastructure incident
- User says "log this incident" / "capture this incident"
- Production outage >5 minutes resolved
- Novel problem not seen before
- Recurring issue (helps detect patterns)

**When NOT to use**:
- Routine operations (scheduled maintenance)
- Quick fixes <5 minutes with known cause
- Non-production test environment issues

## Incident Capture Process

### Step 1: Incident Identification

**Ask user**:
```
What service or system was affected?
Examples: homepage, nextcloud, kubernetes-api, traefik, hardware-mouse
```

**Auto-detect if possible**:
- Check current working context for service names
- Review recent commands for affected services
- Scan conversation for service mentions

### Step 2: Timeline Construction

**Gather chronologically**:

1. **Detection time**: When was the issue first noticed?
   - User report
   - Monitoring alert
   - Health check failure

2. **Diagnosis timeline**: What steps were taken to diagnose?
   - Commands run
   - Logs checked
   - Hypotheses tested

3. **Resolution timeline**: What fixed it?
   - Commands that failed
   - Commands that succeeded
   - Time between attempts

4. **Verification time**: When was service confirmed healthy?
   - Health check passed
   - User verified functionality
   - Monitoring shows normal

**Format**:
```yaml
timeline:
  - time: "2026-01-17T14:32:00Z"
    event: "User reported Homepage 404"
  - time: "2026-01-17T14:33:00Z"
    event: "Confirmed pod in CrashLoopBackOff"
  - time: "2026-01-17T14:42:00Z"
    event: "Service restored and verified"
```

### Step 3: Symptoms Documentation

**Capture observable symptoms**:
- Error messages seen by user
- HTTP status codes (404, 503, etc.)
- Pod states (CrashLoopBackOff, Pending, Error)
- Log entries
- Performance degradation

**Example**:
```yaml
symptoms:
  - "404 error on https://homepage.petersimmons.com"
  - "Pod in CrashLoopBackOff"
  - "Network policy blocked metrics endpoint"
```

### Step 4: Root Cause Analysis

**Ask**:
1. What was the root cause? (Technical explanation)
2. What category?
   - configuration (wrong settings, label mismatch)
   - hardware (mouse freeze, GPU issue)
   - software (bug, crash)
   - network (DNS, connectivity)
   - storage (disk full, PVC issues)

**Example**:
```yaml
diagnosis:
  root_cause: "Network policy label mismatch"
  root_cause_category: configuration
  technical_details: "Policy used app=homepage but deployment uses app.kubernetes.io/name=homepage"
```

### Step 5: Commands Attempted

**Capture everything tried**:
- Commands that failed (and why)
- Commands that succeeded
- Order matters (for learning what to try first)

**Format**:
```yaml
commands_attempted:
  - command: "kubectl delete pod homepage-xxx"
    outcome: failed
    reason: "Pod still crashlooped after delete"
    timestamp: "2026-01-17T14:34:00Z"

  - command: "kubectl apply -f networkpolicy.yaml"
    outcome: success
    reason: "Fixed label to app.kubernetes.io/name=homepage"
    timestamp: "2026-01-17T14:40:00Z"
```

### Step 6: Prevention Measures

**Ask**:
1. What actions were taken to prevent recurrence?
2. Were they actually implemented? (implemented: true/false)

**Examples**:
- Created validation script
- Updated documentation
- Added monitoring alert
- Changed configuration defaults
- Automated the fix

### Step 7: Learning Extraction

**Auto-generate**:

1. **Warning signs**: What could have predicted this?
   ```yaml
   warning_signs_identified:
     - metric: "pod_restarts_per_hour"
       threshold: ">3"
       lead_time_seconds: 1800  # 30 minutes before crash
   ```

2. **Pattern tags**: Keywords for future searches
   ```yaml
   pattern_tags:
     - "network-policy"
     - "label-mismatch"
     - "homepage"
     - "kubernetes"
   ```

3. **Automation candidate**: Should this be automated?
   ```yaml
   automation_candidate: true
   automation_priority: high  # high/medium/low
   automation_reason: "Same issue happened 3 times in 3 weeks"
   ```

### Step 8: Documentation Updates

**Track what got updated**:
```yaml
documentation_updated:
  - file: "CLAUDE.md"
    section: "Application-Specific > Homepage"
  - file: "KNOWLEDGE-INDEX.md"
    section: "Kubernetes"
```

## Auto-Updates Performed

After capturing incident, this skill automatically:

1. **Appends to failure-history.yaml**
   - Full incident record with all details
   - Assigns unique ID: `{service}-{issue}-{YYYYMMDD-HHMM}`

2. **Updates fix-effectiveness.yaml**
   - Adds/updates command success rates
   - Tracks resolution times

3. **Updates FAILURE-MODES-CATALOG.md**
   - Updates "Last Occurrence" date
   - Updates actual MTTR from incident data
   - Increments occurrence counter

4. **Updates KNOWLEDGE-INDEX.md**
   - Adds entry linking to incident report
   - Updates document count
   - Tags by technology/date

5. **Creates incident report**
   - Full markdown report in `/home/psimmons/INCIDENTS/`
   - Filename: `YYYY-MM-DD-{service}-{issue}.md`

6. **Pattern detection**
   - Checks for similar incidents (3+ = pattern)
   - Suggests automation if pattern detected
   - Adds to warning-patterns.yaml if predictive

## Output Format

**During capture** (interactive):
```
📋 Incident Capture Starting

Service affected: homepage
Incident detected: 2026-01-17 14:32
Resolution time: 10 minutes

Timeline captured: 4 events
Commands tracked: 2 failed, 2 succeeded
Root cause identified: Network policy label mismatch

Prevention implemented: ✅ Yes
  - Created pre-apply validation script
  - Updated PRR checklist

⚠️  Pattern detected: This is the 3rd Homepage network policy issue in 3 weeks
Automation suggested: HIGH priority

Updating knowledge base...
✅ Updated failure-history.yaml
✅ Updated fix-effectiveness.yaml
✅ Updated FAILURE-MODES-CATALOG.md
✅ Updated KNOWLEDGE-INDEX.md
✅ Created incident report: /home/psimmons/INCIDENTS/2026-01-17-homepage-network-policy.md

Incident logged successfully.
```

**Files modified**:
- `.homelab/knowledge/failure-history.yaml` (appended)
- `.homelab/knowledge/fix-effectiveness.yaml` (updated)
- `FAILURE-MODES-CATALOG.md` (updated)
- `KNOWLEDGE-INDEX.md` (updated)
- `INCIDENTS/YYYY-MM-DD-service-issue.md` (created)

## Interactive Questions

This skill will ask questions one at a time to gather complete information:

1. "What service was affected?"
2. "When did you first notice the issue? (or enter 'now' for current time)"
3. "What symptoms did you observe?"
4. "Walk me through the timeline - what did you try first?"
5. "What was the root cause?"
6. "Which commands failed and why?"
7. "Which commands succeeded?"
8. "What prevention measures did you implement?"
9. "Is this similar to any past incidents you remember?"

## Pattern Recognition

After capturing, automatically check:

```yaml
# Similar incidents search
similar_incidents = search_failure_history(
    service=incident.service,
    root_cause_category=incident.diagnosis.root_cause_category,
    days_back=90
)

if len(similar_incidents) >= 3:
    alert("PATTERN DETECTED: {service} {category} happened {count} times in 90 days")
    suggest_automation(priority="high")
```

## Integration with Other Skills

**Triggers other skills**:
- If pattern detected → suggest `homelab:suggest-automation`
- If runbook used → suggest `homelab:evolve-runbook`
- If warning signs identified → update `warning-patterns.yaml`

**Used by**:
- `homelab:monthly-review` - analyzes all logged incidents
- `homelab:troubleshoot-common-issues` - uses incident history for suggestions
- `homelab:predict-failure` - uses warning signs for predictions

## Minimal Incident Capture

For quick logging without full interactive session:

```
User: "Log incident: Homepage down due to network policy, fixed in 10 min"

Skill captures:
- Service: homepage
- Issue: network policy
- Duration: 10 minutes
- Status: resolved

Prompts for critical missing info only:
- Root cause details?
- Prevention implemented?
```

## Example Session

```
User: "Log this incident"