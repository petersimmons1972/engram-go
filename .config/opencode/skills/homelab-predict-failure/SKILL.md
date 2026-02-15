---
name: homelab-predict-failure
description: Use after running health-check.sh or when asked "everything ok?", "check health", "any warnings?" - predicts failures using warning pattern thresholds.
---

# Predict Failure

## Overview

Proactive failure prediction system that checks warning signs from historical patterns. Compares current system metrics against known failure precursors and predicts failures before they happen.

**Purpose**: Predict failures before they happen. Prevention is better than recovery.

**Warning patterns file**: `/home/psimmons/.homelab/knowledge/warning-patterns.yaml`

## When to Use

**Invoke triggers**:
- After running `~/bin/health-check.sh`
- User asks "everything ok?" / "check health" / "any warnings?"
- User asks about system status or stability
- After major changes to infrastructure

**When NOT to use**:
- At session startup (use `homelab:session-startup` instead)
- During active incident (use `homelab:incident-response` instead)
- When user has specific problem to troubleshoot

## Process

### Step 1: Load Warning Patterns

Read warning patterns from the knowledge base:

```bash
cat /home/psimmons/.homelab/knowledge/warning-patterns.yaml
```

**Currently tracked patterns**:

| ID                                  | Warning Sign                  | Predicted Failure          | Confidence |
|-------------------------------------|-------------------------------|----------------------------|------------|
| storage-pressure-vm-freeze          | Proxmox zp3 >85%              | VM freeze within 48h       | 95%        |
| homepage-restart-network-policy-drift | Pod restarts >3/hr          | CrashLoopBackOff within 30m | 75%        |
| k8s-node-memory-pressure            | Node memory >90%              | Pod evictions within 10m   | 90%        |
| cert-expiry-imminent                | Cert expiry <7 days           | HTTPS errors               | 100%       |

### Step 2: Run Check Commands

For each pattern, execute the check_command and capture the result.

**Pattern 1: Proxmox Storage Pressure**

```bash
ssh psimmons@192.168.0.100 'pvesm status' | grep zp3
```

Parse output for usage percentage. Threshold: >85%

**Pattern 2: Homepage Pod Restarts**

```bash
kubectl get pods -n default -l app.kubernetes.io/name=homepage --no-headers | awk '{print $4}'
```

Check restart count. Threshold: >3 in last hour

**Pattern 3: Node Memory Pressure**

```bash
kubectl top nodes | awk 'NR>1 {gsub("%","",$5); if ($5 > 90) print $1, $5"%"}'
```

Check for any nodes >90% memory. Threshold: >90%

**Pattern 4: Certificate Expiry**

```bash
kubectl get certificate -A -o json | jq -r '.items[] | select(.status.renewalTime) | [.metadata.namespace, .metadata.name, .status.renewalTime] | @tsv'
```

Calculate days until expiry for each cert. Threshold: <7 days

### Step 3: Evaluate Results

For each check:
1. Parse the command output
2. Compare against threshold
3. If threshold exceeded:
   - Calculate time to predicted failure
   - Determine urgency level
   - Prepare warning output

**Urgency mapping**:
- HIGH: Tier 1 failures OR time to failure <1 hour
- MEDIUM: Tier 2 failures OR time to failure <24 hours
- LOW: Tier 3 failures OR time to failure >24 hours

### Step 4: Generate Output

**If warnings detected, output this format**:

```
FAILURE PREDICTION ANALYSIS
============================================================

WARNING: {pattern.name}
------------------------------------------------------------
Current value: {measured_value}
Threshold: {pattern.warning_sign.threshold}
Predicted failure: {pattern.predicted_failure.description}
Time to failure: {calculated from pattern.evidence.avg_time_to_failure}
Confidence: {pattern.predicted_failure.confidence * 100}% (based on {pattern.evidence.occurrences} past occurrences)
Urgency: {pattern.recommended_action.urgency | uppercase}

Recommended action: {pattern.recommended_action.action}
Commands:
  {pattern.recommended_action.commands[0]}
  {pattern.recommended_action.commands[1]}
Estimated time: {pattern.recommended_action.estimated_time_minutes} minutes

============================================================
{repeat for each warning}

SUMMARY: {count} warning(s) found. Address {urgency} urgency items first.
```

**If all clear, output this format**:

```
FAILURE PREDICTION ANALYSIS
============================================================

All warning patterns checked:
  Proxmox storage: {actual}% (threshold: 85%)
  Homepage restarts: {actual}/hr (threshold: 3/hr)
  Node memory: {actual}% max (threshold: 90%)
  Certificate expiry: {actual} days min (threshold: 7 days)

No warnings detected. System healthy.
```

## Output Examples

### Example 1: Storage Warning

**Situation**: Proxmox zp3 at 87%

```
FAILURE PREDICTION ANALYSIS
============================================================

WARNING: Proxmox zp3 storage pressure causes VM freeze
------------------------------------------------------------
Current value: 87%
Threshold: >85%
Predicted failure: VMs will freeze within 48 hours
Time to failure: ~36 hours (based on past incidents)
Confidence: 95% (based on 3 past occurrences)
Urgency: HIGH

Recommended action: Run cleanup immediately to prevent VM freeze
Commands:
  ~/bin/check_current_zp3_usage.sh
  # If needed: cleanup old logs, remove unused snapshots
Estimated time: 15 minutes

============================================================

SUMMARY: 1 warning found. Address HIGH urgency items immediately.
```

### Example 2: Multiple Warnings

**Situation**: Node memory high + cert expiring

```
FAILURE PREDICTION ANALYSIS
============================================================

WARNING: Node memory pressure causes pod evictions
------------------------------------------------------------
Current value: worker133 at 92%
Threshold: >90%
Predicted failure: Pod evictions will occur within minutes
Time to failure: ~10 minutes
Confidence: 90% (based on 2 past occurrences)
Urgency: HIGH

Recommended action: Identify and address memory-consuming pods
Commands:
  kubectl top pods -A --sort-by=memory | head -20
  # Investigate top memory consumers
Estimated time: 10 minutes

============================================================

WARNING: Certificate expiry within 7 days
------------------------------------------------------------
Current value: traefik-tls expires in 5 days
Threshold: <7 days
Predicted failure: HTTPS errors imminent
Time to failure: 5 days
Confidence: 100% (date-based prediction)
Urgency: MEDIUM

Recommended action: Verify cert-manager renewal process
Commands:
  kubectl get certificate -A
  kubectl logs -n cert-manager -l app=cert-manager --tail=50
Estimated time: 10 minutes

============================================================

SUMMARY: 2 warnings found. Address HIGH urgency items first.
```

### Example 3: All Clear

**Situation**: All metrics within healthy ranges

```
FAILURE PREDICTION ANALYSIS
============================================================

All warning patterns checked:
  Proxmox storage: 72% (threshold: 85%)
  Homepage restarts: 0/hr (threshold: 3/hr)
  Node memory: 65% max (threshold: 90%)
  Certificate expiry: 45 days (threshold: 7 days)

No warnings detected. System healthy.
```

## Quick Reference Commands

**Run all checks manually**:

```bash
# Storage check
ssh psimmons@192.168.0.100 'pvesm status' | grep zp3

# Homepage restarts
kubectl get pods -n default -l app.kubernetes.io/name=homepage --no-headers | awk '{print $4}'

# Node memory
kubectl top nodes

# Certificate expiry
kubectl get certificate -A
```

**View warning patterns**:

```bash
cat /home/psimmons/.homelab/knowledge/warning-patterns.yaml
```

## Adding New Warning Patterns

When a failure occurs that had detectable warning signs:

**Add to warning-patterns.yaml**:

```yaml
- id: new-pattern-id
  name: "Descriptive name"

  warning_sign:
    metric: "metric_name"
    threshold: ">value or <value"
    check_command: "command to run"

  predicted_failure:
    description: "What will happen"
    tier: 1-3
    services_affected:
      - service-name
    confidence: 0.0-1.0

  evidence:
    occurrences: 0
    prediction_accuracy: 0.0
    avg_time_to_failure_hours: 0
    last_occurrence: null

  recommended_action:
    urgency: high|medium|low
    action: "What to do"
    commands:
      - "command 1"
      - "command 2"
    estimated_time_minutes: 0
```

**Criteria for new pattern**:
- Warning sign detected before failure 2+ times
- Warning provides actionable lead time (>5 minutes)
- Prevention is possible within lead time
- Measurable threshold exists

## Integration with Other Skills

**Triggers from**:
- `homelab:session-startup` - may recommend running prediction after health check
- Manual user request after health check

**Triggers to**:
- `homelab:incident-response` - if urgent warning requires immediate action
- `superpowers:systematic-debugging` - if warning leads to active problem

**Updates**:
- `/home/psimmons/.homelab/knowledge/warning-patterns.yaml` - evidence counts when predictions prove accurate

## Urgency Response Guidelines

| Urgency | Response                                      |
|---------|-----------------------------------------------|
| HIGH    | Address immediately, before other work        |
| MEDIUM  | Address within 24 hours                       |
| LOW     | Schedule for next maintenance window          |

**Rule**: Act proactively. A predicted failure caught early is much cheaper than an incident response.
