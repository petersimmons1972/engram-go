---
name: homelab-troubleshoot-common-issues
description: First-line troubleshooting that matches symptoms against failure history and suggests quick fixes with highest success rates. Invoke BEFORE deep debugging when user reports a problem.
---

# Troubleshoot Common Issues

## Overview

First-line troubleshooting skill that matches reported symptoms against failure history to suggest quick fixes with proven success rates. Uses pattern matching against past incidents to accelerate diagnosis.

**Purpose**: Get to resolution faster by checking known issues first before deep debugging.

## When to Use

**Triggers**:
- User reports any problem ("X is down/broken/slow/not working")
- Any service appears unhealthy
- User describes symptoms matching common issues
- BEFORE invoking `superpowers:systematic-debugging`

**When NOT to use**:
- After quick fix already attempted and failed (escalate to systematic-debugging)
- Novel issue with no symptom matches
- Complex multi-service outages (use incident-response)

## Process

### Step 1: Load Knowledge Base

**Required files**:
- `/home/psimmons/.homelab/knowledge/failure-history.yaml` - Past incidents
- `/home/psimmons/.homelab/knowledge/fix-effectiveness.yaml` - Command success rates
- `/home/psimmons/FAILURE-MODES-CATALOG.md` - Known failure patterns

### Step 2: Extract Symptoms

**From user report, identify**:
- Service affected (homepage, nextcloud, traefik, kubernetes, hardware, etc.)
- Symptom type:
  - HTTP errors (404, 502, 503)
  - Pod states (CrashLoopBackOff, Pending, Error)
  - Performance issues (slow, timeout)
  - Complete unavailability
  - Partial functionality loss

**Common symptom keywords**:
| Keyword              | Maps to Category            |
|----------------------|-----------------------------|
| down, unavailable    | service-down                |
| 404                  | routing-issue               |
| 502, 503             | backend-down                |
| slow, timeout        | performance-issue           |
| CrashLoopBackOff     | pod-crashloop               |
| Pending              | resource-or-storage-issue   |
| frozen, stuck        | process-hang                |
| not loading          | connectivity-issue          |

### Step 3: Match Against Known Issues

**Check CLAUDE.md common issues table first**:

| Issue             | Check First                | Don't Waste Time       |
|-------------------|----------------------------|------------------------|
| Homepage down     | Network policy labels      | Restarting Traefik     |
| Nextcloud slow    | Background jobs status     | Adding resources       |
| Mouse frozen      | ~/bin/mouse.sh             | Re-pairing             |
| 404 single svc    | Service config, ingress    | Traefik restart        |
| Proxmox slow      | zp3 storage usage          | Network speed          |
| Pod pending       | Node resources, PVC status | Cluster-wide issues    |

**Then search failure-history.yaml**:
```yaml
# Search algorithm:
# 1. Match service name
# 2. Match symptom patterns using pattern_tags
# 3. Score by recency and success rate

matches = search_failure_history(
  service=reported_service,
  symptoms=extracted_symptoms,
  pattern_tags=symptom_keywords
)
```

### Step 4: Rank by Success Rate

**Lookup fix-effectiveness.yaml for matched issues**:
```yaml
# For each matching issue, get:
# - Command success rate
# - Average resolution time
# - Last used date
# - Notes

rank_by:
  1. success_rate (highest first)
  2. avg_resolution_time (fastest first)
  3. last_used (most recent first)
```

### Step 5: Present Top 3 Matches

**Output format**:
```
Based on failure history, this looks like:

1. [Issue Type] ([X]% of [service] issues)
   Quick fix: [command]
   Success rate: [Y]%
   Avg resolution: [Z] seconds

2. [Issue Type] ([X]% of [service] issues)
   Quick fix: [command]
   Success rate: [Y]%

3. [Issue Type] ([X]% of [service] issues)
   Quick fix: [command]

Try #1 first. If that doesn't work, I'll escalate to systematic-debugging.
```

### Step 6: Execute Quick Fix

**After user confirms (or immediately for high-confidence matches)**:

1. Run the suggested command
2. Verify service status
3. **If fix works**: Invoke `homelab:log-fix-result` to record success
4. **If fix fails**: Continue to next suggestion or escalate

### Step 7: Escalation Path

**If no matches found OR all quick fixes fail**:
```
No quick fix matched (or all failed). Escalating to systematic debugging.
```

Then invoke: `superpowers:systematic-debugging`

## Quick Reference Tables

### Service-Specific Quick Fixes

**Homepage**:
| Symptom                     | Check First                                          | Success Rate |
|-----------------------------|------------------------------------------------------|--------------|
| 404 / API Error             | `kubectl describe networkpolicy -n default \| grep Selector` | 100%         |
| CrashLoopBackOff            | `kubectl describe pod homepage-xxx \| grep -A5 Limits`       | 85%          |
| Widgets not loading         | `kubectl logs -l app.kubernetes.io/name=homepage`           | 80%          |

**Nextcloud**:
| Symptom                     | Check First                                          | Success Rate |
|-----------------------------|------------------------------------------------------|--------------|
| 503 error                   | `ssh 192.168.0.200 "sudo systemctl status apache2"` | 95%          |
| Slow performance            | `ssh 192.168.0.200 "php occ background:status"`     | 90%          |
| Maintenance mode stuck      | `ssh 192.168.0.200 "sudo -u www-data php occ maintenance:mode --off"` | 100% |

**Mouse**:
| Symptom                     | Check First                                          | Success Rate |
|-----------------------------|------------------------------------------------------|--------------|
| Frozen after idle           | `~/bin/mouse.sh`                                     | 100%         |
| No movement                 | `~/bin/mouse.sh`                                     | 100%         |

**Traefik**:
| Symptom                     | Check First                                          | Success Rate |
|-----------------------------|------------------------------------------------------|--------------|
| All services 502/503        | `kubectl get pods -n kube-system -l app.kubernetes.io/name=traefik` | 80% |
| Single service 404          | Check Ingress AND IngressRoute (use only one)        | 90%          |
| Certificate errors          | `kubectl get certificate -A`                         | 85%          |

**Kubernetes Pods**:
| Symptom                     | Check First                                          | Success Rate |
|-----------------------------|------------------------------------------------------|--------------|
| Pod Pending                 | `kubectl describe pod X \| grep -A10 Events`         | 80%          |
| Pod CrashLoopBackOff        | `kubectl logs X --previous`                          | 75%          |
| Pod resource issues         | `kubectl top pods` and `kubectl describe node`       | 85%          |

**Proxmox**:
| Symptom                     | Check First                                          | Success Rate |
|-----------------------------|------------------------------------------------------|--------------|
| VMs slow/freezing           | `zfs list zp3` (check usage <85%)                    | 90%          |
| VM won't start              | Check node resources in Proxmox UI                   | 80%          |

### Anti-Patterns (Don't Waste Time)

| Tempting But Wrong            | Why Wrong                      | Do Instead                  |
|-------------------------------|--------------------------------|-----------------------------|
| Add Homepage replicas         | Doesn't fix network policies   | Fix policy labels           |
| Restart Traefik for single 404| Single svc != Traefik problem  | Check service config        |
| Re-pair mouse                 | Driver state, not pairing      | Run ~/bin/mouse.sh          |
| Check DNS for Homepage        | Usually K8s internal           | Check network policies      |
| Restart pods without logs     | Destroys evidence              | Logs before restart         |

## Output Examples

### Example 1: Homepage 404

**User**: "Homepage is showing 404"

**Output**:
```
Based on failure history, this looks like:

1. Network policy label mismatch (90% of Homepage issues)
   Quick fix: kubectl describe networkpolicy -n default | grep Selector
   Then verify: Labels should match app.kubernetes.io/name=homepage (NOT app=homepage)
   Success rate: 100% when labels are the cause

2. Pod resource limits exceeded (5% of Homepage issues)
   Quick fix: kubectl describe pod -l app.kubernetes.io/name=homepage | grep -A5 Limits

3. Ingress configuration issue (5% of Homepage issues)
   Quick fix: kubectl get ingress,ingressroute -n default

Try #1 first. If that doesn't work, I'll escalate to systematic-debugging.
```

### Example 2: Mouse Frozen

**User**: "My mouse is frozen again"

**Output**:
```
Based on failure history, this looks like:

1. USB autosuspend driver issue (100% of mouse freeze issues)
   Quick fix: ~/bin/mouse.sh
   Success rate: 100%
   Avg resolution: 30 seconds

Running the fix now...
```

### Example 3: Nextcloud Slow

**User**: "Nextcloud is really slow today"

**Output**:
```
Based on failure history, this looks like:

1. Background jobs not running (60% of Nextcloud performance issues)
   Quick fix: ssh psimmons@192.168.0.200 "sudo -u www-data php occ background:status"
   Check for: Last job should be < 5 minutes ago
   Success rate: 90%

2. Apache worker exhaustion (25% of Nextcloud performance issues)
   Quick fix: ssh psimmons@192.168.0.200 "sudo systemctl status apache2"

3. Database connection issues (15% of Nextcloud performance issues)
   Quick fix: ssh psimmons@192.168.0.200 "sudo systemctl status postgresql"

Try #1 first. If background jobs are stale, run: occ background:cron
```

### Example 4: No Matches Found

**User**: "The GPU passthrough stopped working"

**Output**:
```
No quick fix found in failure history for GPU passthrough issues.

This appears to be a novel issue. Escalating to systematic debugging.

Invoking: superpowers:systematic-debugging
```

## Integration with Other Skills

**Triggers**:
- User reports problem → This skill (first-line troubleshooting)

**Escalates to**:
- `superpowers:systematic-debugging` - When quick fixes fail or no match found

**Logs results to**:
- `homelab:log-fix-result` - Records command outcomes for learning

**Uses data from**:
- `homelab:log-incident` - Incident history feeds pattern matching
- `homelab:monthly-review` - Reviews effectiveness of quick fixes

## Files Read

- `/home/psimmons/.homelab/knowledge/failure-history.yaml`
- `/home/psimmons/.homelab/knowledge/fix-effectiveness.yaml`
- `/home/psimmons/FAILURE-MODES-CATALOG.md`
- `/home/psimmons/CLAUDE.md` (common issues table)

## Success Criteria

**Skill successful when**:
- Issue resolved without escalating to systematic-debugging
- Resolution time < expected MTTR from FAILURE-MODES-CATALOG.md
- Fix result logged to fix-effectiveness.yaml

**Skill escalates when**:
- No symptom matches found
- All quick fixes attempted and failed
- Issue complexity exceeds quick-fix scope

## Notes

- Always check logs BEFORE suggesting restarts (restarts destroy evidence)
- Prioritize non-destructive diagnostic commands first
- If unsure between multiple services, run health-check.sh first
- Track success/failure of suggested fixes to improve recommendations over time
