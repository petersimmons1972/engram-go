---
name: homelab:incident-response
description: Use when production service outage or incident occurs - systematic troubleshooting with timeline tracking
---

# Incident Response Procedure

**Announce:** "Initiating incident response procedure. Recording timeline..."

## Step 1: Record Incident Start

```
INCIDENT START: [current timestamp]
SERVICE: [affected service]
SYMPTOMS: [what user reported]
```

## Step 2: Check Failure Mode Catalog

Reference CLAUDE.md "Known Failure Modes & Blast Radius" section.

Check the Tier 1/2/3 tables in CLAUDE.md. If this matches a known pattern, follow the documented Recovery Procedure column.

Ask: Does this match a known failure pattern?
- If YES: Follow documented recovery procedure
- If NO: Continue systematic troubleshooting

## Step 3: Quick Health Check

```bash
~/bin/health-check.sh
```

Identify what's failing.

## Step 4: Systematic Troubleshooting

For Kubernetes services:
```bash
# 1. Check pod status
kubectl get pods -A | grep <service-name>

# 2. Get pod details and events
kubectl describe pod <pod-name> -n <namespace>

# 3. Check recent pod logs
kubectl logs <pod-name> -n <namespace> --tail=50

# 4. Check previous pod logs (if pod restarted)
kubectl logs <pod-name> -n <namespace> --previous

# 5. Check service endpoints
kubectl get endpoints <service-name> -n <namespace>

# 6. Check ingress/ingressroute
kubectl get ingress -A | grep <service-name>
kubectl get ingressroute -A | grep <service-name>

# 7. Check network policies (especially for Homepage!)
kubectl get networkpolicy -A | grep <service-name>
kubectl describe networkpolicy <policy-name> -n <namespace>

# 8. Check recent events in namespace
kubectl get events -n <namespace> --sort-by='.lastTimestamp' | tail -20
```

For VMs:
```bash
# Check service status
ssh <user>@<ip> "systemctl status <service>"

# Check logs
ssh <user>@<ip> "journalctl -u <service> --since '1 hour ago'"
```

**Note:** For service-specific log locations (Nextcloud, Homepage, Traefik, Pi-hole, etc.), see the Service-Specific Log Locations table in CLAUDE.md.

## Step 3.5: Check Dependencies

Reference the Service Dependency Map in CLAUDE.md to check if upstream dependencies are failing. Understanding the dependency chain helps identify if the issue is in the service itself or in a service it depends on.

## Step 5: Track Timeline

Record each diagnostic step and finding:
```
HH:MM - Checked X, found Y
HH:MM - Tried Z, result was W
```

## Step 6: Escalation Check

**If stuck for >30 minutes on Tier 1 failure:**
- Post to relevant community (see Communities table in CLAUDE.md)
- Use structured help template

**If stuck for >2 hours on any failure:**
- Stop and reassess approach
- Consider asking for external help

## Step 7: Resolution

When service is restored:
1. Record resolution time
2. Document root cause
3. Document prevention steps

## Step 8: Create Incident Report

Ensure directory exists:
```bash
mkdir -p /home/psimmons/INCIDENTS/
```

Create file: `/home/psimmons/INCIDENTS/[YYYY-MM-DD]-[service]-[issue].md`

```markdown
# Incident: [Service] [Issue Type]

**Date:** [YYYY-MM-DD]
**Duration:** [start time] - [end time] ([X] minutes)
**Severity:** Tier [1/2/3]

## Timeline
- HH:MM - [First symptom]
- HH:MM - [Diagnosis step]
- HH:MM - [Resolution action]
- HH:MM - Service restored

## Root Cause
[Technical explanation]

## Resolution
[What fixed it]

## Prevention
[What was done to prevent recurrence]

## Lessons Learned
1. [Lesson 1]
2. [Lesson 2]

## Documentation Updated
- [ ] KNOWLEDGE-INDEX.md
- [ ] Failure Mode Catalog (if new pattern)
- [ ] Runbook (if new procedure)
```

## Step 9: Update Knowledge Base

Add entry to KNOWLEDGE-INDEX.md under appropriate date and category.
