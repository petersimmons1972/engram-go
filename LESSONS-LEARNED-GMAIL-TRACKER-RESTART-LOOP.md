# Lessons Learned: Gmail-Tracker Restart Loop (850 restarts in 8 days)

**Date**: 2026-01-25
**Duration**: Ongoing since deployment
**Impact**: Job runs every 15 minutes instead of every 12 hours (100+ unnecessary restarts/day)
**Severity**: Medium (job is functional, but inefficient)

---

## Executive Summary

The gmail-tracker pod restarts ~100 times per day (850 restarts in 8 days) with exit code 137 (OOMKilled). Root cause: **Liveness probe conflicts with sleep-loop architecture.**

The container is designed to:
- Run job every 12 hours with sleep loop (`while true; do job; sleep 12h; done`)

But Kubernetes configuration expects:
- Long-running process with constant health signals
- Liveness probe checks every 5 minutes for `/app/logs/gmail_tracker.log`
- Pod is killed after 3 failed checks (15 minutes)

**Result**: Pod completes job, enters 12-hour sleep, fails health check after 15 minutes, gets killed, restarts immediately, runs job again.

---

## Technical Diagnosis

### Actual Behavior Flow

```
Pod Start
  ↓
Wait for database (30s) ✓
  ↓
Enter while loop
  ↓
Run job (9s) ✓
  ↓
Log "Next check in 12 hours"
  ↓
Call sleep(43200) for 12-hour sleep
  ↓
Liveness probe runs (every 300s) → Check: test -f /app/logs/gmail_tracker.log
  ↓
Probe fails (log file doesn't exist) ✗
  ↓
Probe fails 3 times (15 minutes total)
  ↓
Kubernetes kills pod with SIGKILL (exit code 137)
  ↓
restartPolicy: Always → Pod restarts immediately
  ↓
Back to "Pod Start"
```

### Why This Looks Like OOMKilled

- **Exit code 137** = 128 + 9 = SIGKILL
- OOMKiller sends SIGKILL when memory exceeds limit
- But memory usage is only 3Mi (out of 512Mi limit)
- **Real cause**: Liveness probe timeout causing kill signal
- **Misleading**: Same exit code as memory kill, different root cause

### Why Logs Show Success

The pod logs show:
```
[Sun Jan 25 14:33:28 UTC 2026] Running Gmail job tracker...
[Sun Jan 25 14:03:36 UTC 2026] Loaded existing credentials from token file
[2026-01-25 14:03:36] INFO: Authenticated with Gmail API
[2026-01-25 14:03:37] INFO: Found 0 emails to process
[Sun Jan 25 14:03:37 UTC 2026] Next check in 12 hours
```

This logs completion successfully, then exits normally. The liveness probe sees no log file, kills the pod, and it restarts immediately. Each cycle takes ~15 minutes, not 12 hours.

---

## Root Cause Layers

### Layer 1: Immediate (Why Pod Restarts)
- Log file location: `/app/logs/gmail_tracker.log`
- Log file status: Does not exist (app logs to stdout, not disk)
- Liveness probe: Checks every 300 seconds for file existence
- Failure threshold: 3 failures = 900 seconds (~15 minutes)
- Result: Pod killed by liveness probe, not memory

### Layer 2: Architecture Mismatch
- **Container design**: "Job with sleep loop" pattern
  - Runs once every 12 hours
  - Sleeps between iterations
  - Single long-running process
- **Kubernetes expectation**: "Always-running service" pattern
  - Liveness probe expects constant health signals
  - Sleep without signals = unhealthy
  - Restarts = fix for failed health check

### Layer 3: Configuration Error
- `Deployment` with `restartPolicy: Always` is wrong for periodic jobs
- Should be: `CronJob` or `Job` with sleep loop in container
- Mixing deployment (for services) with job logic (periodic execution) creates confusion

---

## Five Critical Lessons

### 1. Liveness Probes + Sleep Loops Don't Mix
**Lesson**: Liveness probes are for health monitoring. Sleep loops indicate the process is intentionally not responding.

- Probe expects: "Service is running and responding normally"
- Sleep loop delivers: "Service is sleeping and not responding for 12 hours"
- Result: Probe fails, pod killed

**Fix**: Either:
- Remove liveness probe if sleep is intentional
- Use HTTP health endpoint instead of file check
- Convert to CronJob (no probe needed)

### 2. Exit Code 137 is Ambiguous
**Lesson**: Exit code 137 (SIGKILL) is used by both OOMKiller and liveness probe failures. You need full context to diagnose.

- OOMKiller kills with SIGKILL (137)
- Liveness probe kills with SIGKILL (137)
- But root causes are completely different
- Must check: logs, memory usage, probe config, not just exit code

**Prevention**: Check multiple signals:
- Exit code (tells you it was killed)
- Memory usage (tells you if OOM was involved)
- Pod events (tells you reason: "Liveness probe failed" vs "OOMKilled")
- Container logs (tells you what was running)

### 3. Job vs Deployment Pattern Confusion
**Lesson**: Kubernetes has different patterns for different workload types. Mixing them causes subtle bugs.

| Pattern | Best For | Behavior | Config |
|---------|----------|----------|--------|
| **Job** | Single execution | Run once, exit | `RestartPolicy: Never` |
| **CronJob** | Periodic execution | Run on schedule | `schedule: "0 */12 * * *"` |
| **Deployment** | Long-running service | Restart on failure | `restartPolicy: Always`, liveness probe |
| **gmail-tracker** | Periodic job | Sleep 12 hours between runs | Designed wrong (using Deployment) |

**Fix**: Use CronJob for periodic tasks, not Deployment with sleep loop.

### 4. File-Based Liveness Probes Are Fragile
**Lesson**: Checking for file existence is unreliable because:

- File might not be created (app logs to stdout)
- `emptyDir` volumes get cleaned on restart
- Filesystem operations timeout under load
- Single file = single point of failure

**Better options**:
- HTTP health endpoint (app responds when healthy)
- TCP port check (app accepting connections)
- Remove probe if intentional sleep is part of design
- Persistent volume if file tracking is necessary

### 5. Silent Failures Hide in Logs
**Lesson**: The pod "looks working" because the job completes successfully every time. The restart loop is hidden in the restart count.

- Log inspection shows: "Job completed successfully"
- Memory inspection shows: "Only 3Mi usage, well under limit"
- Status inspection shows: "Running" (because it always restarts and runs)
- But restart count shows: 850 in 8 days = 100/day

**Prevention**: Monitor restart rate, not just pod status.

---

## Why This Wasn't Caught Earlier

1. **Pod status appears healthy**: Shows "Running"
2. **Job output appears successful**: Logs show completed execution
3. **Memory appears fine**: Current usage 3Mi, limit 512Mi
4. **Metric hidden in plain sight**: 850 restarts visible in pod age/restarts, but easy to miss
5. **Restart loop continues indefinitely**: Job never gets "stuck", it always runs and exits

---

## The Fix (Three Options)

### Option 1: Convert to CronJob (RECOMMENDED)
**Time**: 15 minutes
**Risk**: Low (simpler design)

```yaml
apiVersion: batch/v1
kind: CronJob
metadata:
  name: gmail-tracker
spec:
  schedule: "0 */12 * * *"  # Every 12 hours
  jobTemplate:
    spec:
      template:
        spec:
          restartPolicy: Never  # Jobs should never restart
          containers:
          - name: gmail-tracker
            image: registry.petersimmons.com/gmail-job-tracker:latest
            # Remove liveness probe
            # No sleep loop needed (CronJob handles scheduling)
```

**Why this is best**:
- CronJobs are designed for periodic tasks
- No liveness probe conflicts
- Job runs once, completes, deleted
- Transparent: Look at CronJob logs to see when it runs
- Next run scheduled automatically

### Option 2: Fix Liveness Probe
**Time**: 5 minutes
**Risk**: Medium (probe still fragile)

Remove the problematic probe:
```yaml
# DELETE this from Deployment spec:
livenessProbe:
  exec:
    command:
    - /bin/sh
    - -c
    - test -f /app/logs/gmail_tracker.log
```

Since the sleep loop is intentional, the pod doesn't need health monitoring. The job either succeeds (emits "Next check in X hours") or fails (logs error).

### Option 3: Fix Log File Handling
**Time**: 20 minutes
**Risk**: High (multiple moving parts)

Make probe pass by ensuring log file always exists:
1. Change container to write log file on startup
2. Touch log file before sleep
3. Use persistent volume instead of emptyDir
4. Add periodic file update during sleep (every few minutes)

**Not recommended**: More complex, still fragile

---

## Implementation Plan

```bash
# Step 1: Backup current Deployment
kubectl get deployment gmail-tracker -n job-search -o yaml > /tmp/gmail-tracker-deployment-backup.yaml

# Step 2: Delete current Deployment
kubectl delete deployment gmail-tracker -n job-search

# Step 3: Create CronJob
kubectl apply -f /path/to/new-cronjob.yaml

# Step 4: Verify
kubectl get cronjob gmail-tracker -n job-search
kubectl describe cronjob gmail-tracker -n job-search

# Step 5: Wait for next scheduled run (up to 12 hours or manually trigger)
kubectl create job --from=cronjob/gmail-tracker gmail-tracker-manual -n job-search

# Step 6: Monitor logs
kubectl logs -f -l app=gmail-tracker -n job-search --tail=50
```

---

## Verification

After conversion to CronJob:

- [ ] CronJob created: `kubectl get cronjob -n job-search`
- [ ] Next schedule time visible: `kubectl describe cronjob gmail-tracker -n job-search`
- [ ] Job completes successfully: Check pod logs
- [ ] Pod doesn't restart: Check pod status (no restart count increment)
- [ ] Health check clean: `health-check.sh` should show no pod errors for gmail-tracker

---

## Knowledge Transfer

| What | Why | How |
|------|-----|-----|
| Exit code 137 is ambiguous | Both OOMKiller and liveness kills use SIGKILL | Check pod events and memory usage, not just code |
| File-based probes fail silently | Files can be missing/deleted, operations timeout | Use HTTP health or remove probe if not needed |
| Sleep loops need CronJob not Deployment | Deployment expects running service, not periodic job | Use `CronJob` with `schedule` for periodic tasks |
| Pod restart loop hidden in metrics | Status shows "Running", metrics show 100+/day restarts | Monitor `pod.restarts` counter continuously |
| Silent success hides failures | Job completes, logs show success, but restarting constantly | Check restart count, not just execution logs |

---

## Files Updated

- ✅ This document: `LESSONS-LEARNED-GMAIL-TRACKER-RESTART-LOOP.md`
- Will update: CLAUDE.md (add Job/Deployment patterns section)
- Will update: knowledge/failure-history.yaml (incident record)
- Will create: CronJob YAML definition

---

## Related References

- Container code: `/home/psimmons/bin/gmail_tracker/`
- Entrypoint: `/home/psimmons/docker-entrypoint.sh` (has while loop)
- Deployment: `kubectl get deployment gmail-tracker -n job-search -o yaml`
- Pod logs: `kubectl logs -n job-search <pod-name>`

---

**Version**: 1.0
**Status**: Analysis complete, implementation pending
**Last Updated**: 2026-01-25
