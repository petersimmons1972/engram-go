---
name: debugging-longhorn-storage
description: Use when troubleshooting Longhorn persistent volume attachment failures, PVC binding issues, or storage-related pod deployment problems in Kubernetes. Triggers on keywords like "longhorn", "PVC stuck", "volume not ready", or when describing storage attachment errors.
---

# Debugging Longhorn Storage Issues

## Overview

**Data-first, automation-second** troubleshooting system for Longhorn storage issues. **PRESERVES DATA AT ALL COSTS** - no destructive actions without explicit user consent and backup verification. Prioritizes data-preserving solutions, requires backups before any risk, and treats data loss as unacceptable. Automation serves data protection, not speed.

**Storage Context:** Environment has **massive disk space** (Proxmox + TrueNAS with huge capacity, 100GB+ per pod). Disk pressure issues should be **extremely rare** - if detected, alert user immediately for investigation.

## When to Use

**Triggers:**
- Keywords: "longhorn", "PVC stuck", "volume not ready", "AttachVolume failed"
- Error patterns: "volume is not ready for workloads", "CSINode does not contain driver"
- Context: Stateful workloads failing to start, storage-related pod issues
- Commands: `/debug-longhorn`, `/troubleshoot-storage`

**Automatic activation when:**
- User mentions Longhorn storage problems
- Pod stuck in ContainerCreating with volume attachment errors
- PVC shows Bound but volume won't mount
- Disk pressure taints preventing scheduling
- **NEW:** CrashLoopBackOff pods with storage events (disguised issues)

**Automation Levels (Data Preservation First):**
- **Full Auto (50% of cases):** Only completely safe, data-preserving actions (remove taints, restart components)
- **Semi-Auto (40% of cases):** Data-preserving but requires confirmation (emptyDir switch for dev workloads)
- **Manual (10% of cases):** ANY action with data loss risk - requires explicit user approval + backup verification
- **NEW: Pods with multiple CrashLoopBackOff failures** (may indicate storage corruption)
- **NEW: I/O errors in application logs** (may be disguised storage issues)
- **NEW: FailedMount with "no Pending workload pods"** (volume attachment deadlock)

## Debugging Workflow

### ⚠️ DATA PROTECTION FIRST - AUTOMATION SECOND

**CRITICAL DATA PROTECTION RULES:**
- **NO destructive action** without verified backup
- **NO data loss** without explicit user "DELETE DATA" consent
- **DEFAULT to safe solutions** - only escalate when safe options exhausted
- **STOP automation** when data risk exists - ask user instead

**Automation Mode (Conservative - Data Protection First):**
- **Safe actions only** applied automatically
- **Backup verification** required before any data risk
- **User consent** mandatory for destructive operations
- **Data integrity** checked after each attempted fix

**Automated Sequence:**
1. **Parallel Data Collection** (no user input needed)
2. **Pattern Matching** against known issues
3. **Automated Fixes** for safe, reversible actions
4. **Progress Reporting** every 30 seconds
5. **Loop Detection** - stops if repeating same steps

### Step 0: Automated Initial Triage (0-30 seconds)

**AUTOMATED: Runs without user input, detects issue patterns automatically**

**Auto-Detects:**
- Storage-disguised crashes (CrashLoopBackOff with storage events)
- Volume attachment failures
- I/O errors in application logs
- Multiple pod failures on same PVC

**Auto-Decides (Conservative - Data First):**
- If 3+ pod failures in <10 minutes → **FLAG: Investigate storage issue - NO automatic destructive action**
- If FailedMount + no Pending workload pods → **FLAG: Possible attachment issue - try safe recovery first**
- If I/O errors in logs → **FLAG: Storage corruption possible - require user decision**

**Auto-Actions (Safe Only):**
- If attachment deadlock suspected → Try Solution C (Component restart - data safe)
- If disk pressure taint → **ALERT USER IMMEDIATELY** (do NOT auto-remove - investigate first)
- **NEVER auto-applies destructive solutions - always requires user consent**

**Disk Space Context:**
- **Environment has massive storage:** Proxmox + TrueNAS with huge capacity
- **Each pod allocated 100GB+** - disk pressure should be extremely rare
- **If disk pressure detected:** This is unusual and requires user investigation

**Data Protection Rules:**
- **No destructive action without backup verification**
- **No data loss without explicit user "DELETE DATA" confirmation**
- **Default to data-preserving solutions only**
- **Escalate to user for any uncertainty**

### Step 1: Parallel Data Collection (30-60 seconds)

**AUTOMATED: Gathers all diagnostic data simultaneously**

**Parallel Collection:**
```bash
# Run all checks concurrently (scripted for speed)
~/bin/health-check.sh &
kubectl get pods -n <namespace> -o wide &
kubectl get pvc,pv -n <namespace> &
kubectl get events --all-namespaces --sort-by='.lastTimestamp' | grep -E "(FailedMount|FailedAttach|I/O|longhorn)" | tail -5 &
kubectl get volumes.longhorn.io -n longhorn-system &
kubectl get nodes -o custom-columns=NAME:.metadata.name,TAINTS:.spec.taints &
```

**Auto-Analysis:**
- Pod status: Running/Error/Pending/CrashLoopBackOff
- PVC state: Bound/Pending/Lost
- Volume state: Attached/Attaching/Degraded/Faulted
- Node taints: disk-pressure/other issues
- Recent events: storage-related failures

**Auto-Progress:** Reports findings every 15 seconds

### Step 2: Automated Pattern Matching (30-45 seconds)

**AUTOMATED: Matches collected data against known issue patterns**

**Decision Engine (Data Preservation Priority):**
```
Data Analysis Results:
├── STORAGE CORRUPTION SUSPECTED → REQUIRE USER DECISION (backup first)
├── ATTACHMENT DEADLOCK → Solution C (Restart Components - SAFE)
├── VOLUME EXPANSION ISSUE → Solution F (Clear PendingNodeID - SAFE)
├── BACKING IMAGE MISSING → Solution G (Image Recovery - SAFE)
├── NODE AFFINITY CONFLICT → REQUIRE USER DECISION (PVC recreation risk)
├── DISK PRESSURE TAINT → ALERT USER (investigate - should be rare with 100GB+ per pod)
└── UNKNOWN PATTERN → Escalate to user with SAFE options only
```

**Data Protection Scoring:**
- **SAFE (100% data preserved):** Apply automatically (component restarts, taint removal, PendingNodeID clear)
- **REQUIRES BACKUP (data at risk):** User must confirm backup exists before proceeding
- **DATA LOSS (destructive):** User must explicitly approve with "DELETE DATA" confirmation
- **UNCERTAIN:** Always escalate to user - never guess with data at stake

**Loop Prevention:** If same pattern detected 3+ times, stop automation and ask user

## Root Causes Identified

### 1. Longhorn Volume Stuck in "Attaching" State
- Longhorn volume remains in attaching state indefinitely
- Engine manager on target node may be unresponsive
- Replica rebuilding in progress blocking attachment
- Previous attachment not properly cleaned up

### 2. Node Affinity Conflicts
- When using local-path storage class, PV is bound to specific node
- If that node has taints or issues, pod cannot schedule anywhere else
- Creates scheduling deadlock

### 3. Disk Pressure Taints (RARE - Alert User Immediately)
- Nodes with `node.kubernetes.io/disk-pressure` taint reject new workloads
- **CRITICAL: With 100GB+ per pod and massive backend storage, this should be extremely rare**
- **If detected: IMMEDIATELY alert user** - investigate root cause before any automated action
- Often false positives, but in this environment may indicate real issues
- **User has instructed: Alert immediately if disk space concerns arise - they can fix it**

### 4. Stale Volume Attachments
- Previous pod deletion may not have fully detached volume
- Volume attachment object stuck in Kubernetes
- Prevents new attachment to different pod/node

### 5. Filesystem Corruption (NEW)
- EXT4 filesystem corruption on Longhorn volume
- fsck finds uncorrectable errors during mount
- Pod stuck in ContainerCreating with MountVolume.SetUp failed
- **Symptoms:** "fsck found errors", "could not correct them"

### 6. Backing Image Unavailability (NEW)
- No available copy of backing image in cluster
- Replicas using same backing image cannot start
- Volumes using backing images cannot attach
- **Check:** `kubectl get backingimages.longhorn.io -n longhorn-system`

### 7. Volume Expansion Issues (NEW)
- Unexpected volume expansion leads to degradation
- Volume attach/detach loop during expansion
- Replicas fail to rebuild after expansion
- **Affected versions:** Longhorn v1.3.2-v1.5.0

### 8. Post-Upgrade Attachment Failures (NEW)
- Leftover `volume.status.PendingNodeID` after v1.4.x→v1.5.x upgrade
- Orphaned non-empty PendingNodeID prevents attachment
- **Fix:** `kubectl edit volumes -n longhorn-system VOLUME-NAME --subresource=status` → set PendingNodeID to ""

### 9. Race Conditions in Replica Scheduling (NEW)
- Multiple controllers scheduling replicas simultaneously
- Race condition causes over-provisioning or scheduling failures
- Insufficient storage errors despite available disk space
- **Versions:** Affects all versions, worse with high replica counts

### 10. Undersized Node Disks vs Terraform Configuration Drift (NEW - 2026-01-13)
- **Symptom:** All nodes show 29-30GB disks despite Terraform config specifying 100GB
- **Root Cause:** Terraform disk_size applied to VM but guest filesystem never extended after resize
- **Impact:** Systemic disk exhaustion across entire cluster, Longhorn schedulability failures on multiple nodes
- **Detection:** Multiple nodes approaching Longhorn's 25% minimum threshold simultaneously, degraded/faulted volumes on nodes with low disk space, CSI component failures correlating with disk pressure
- **Solution:** Online hot-expansion using Proxmox API `qm resize` + guest-side `growpart` + `resize2fs` (zero downtime)
- **Prevention:** Validate Terraform state matches filesystem reality, automated capacity monitoring at 50% threshold

### Step 3: Automated Solution Application (1-5 minutes)

**AUTOMATED: Applies solutions based on pattern matching confidence**

**SAFE Auto-Apply (No Confirmation - Zero Data Risk):**
- Restart Longhorn components (non-destructive)
- Clear PendingNodeID after upgrade
- Check volume status and health
- **DISK PRESSURE TAINTS: ALERT USER (do not auto-remove - investigate first)**

**REQUIRES BACKUP VERIFICATION (Data Potentially At Risk):**
- Switch to emptyDir (acceptable for development, but verify no production data)
- Switch storage classes (PVC recreation - backup required)
- Force pod restart (may interrupt data operations)

**REQUIRES EXPLICIT APPROVAL (Data Loss Possible):**
- Filesystem repair (fsck can cause data loss - backup mandatory)
- Clean slate recreation (definitive data loss - backup required)
- Volume deletion or recreation

**Backup Verification Required:**
```bash
# BEFORE any risky action, verify backup exists and is recent
~/bin/backup_script.sh status
kubectl get backups.longhorn.io -n longhorn-system
# Must show recent backup (<24h old) before proceeding
```

**Progress Monitoring:**
- Checks solution effectiveness every 30 seconds
- Auto-escalates if solution fails after 2 minutes
- Reports status: "Solution applied, monitoring..."

### Step 4: Automation Controls & Loop Detection

**Automation Levels (Data Protection First):**
- **Full Auto:** Only 100% data-safe actions (monitoring, restarts, taint removal)
- **Semi-Auto:** Requires confirmation but data-safe (with backup verification)
- **Manual:** ANY action with data risk - explicit approval + backup proof required

**Data Protection Guards:**
- **Backup Verification:** Required before any action with >0% data loss risk
- **User Consent:** Explicit approval needed for destructive actions
- **Conservative Defaults:** When in doubt, ask user - never risk data
- **Progress Monitoring:** Data integrity checks after each attempted fix

**Loop Detection (Secondary to Data Safety):**
- **Repetition Check:** If same diagnostic step repeated >3 times, stop BUT preserve data
- **Progress Stagnation:** If no progress for 5 minutes, ask user (don't auto-escalate destructively)
- **Solution Failure:** If safe solutions fail, escalate to user - don't try risky fixes
- **Time Budget:** No hard time limit when data preservation is at stake

**Data Protection Metrics (Primary Goals):**
- Data loss incidents: 0 (absolute priority)
- Backup verification: 100% before risky actions
- User consent: Required for any destructive action
- Safe resolution rate: Maximize safe solutions over risky ones

### Solution Priority Matrix (Data Preservation Ordered)

**CRITICAL: Solutions ordered by data safety, not speed**

| Priority | Data Risk | Solution | Automation Level | Backup Required | Use Case |
|----------|-----------|----------|------------------|-----------------|----------|
| **1 - SAFE** | None | Component restart (C) | Auto-apply | No | Most common - try first |
| **2 - SAFE** | None | Clear PendingNodeID (F) | Auto-apply | No | Post-upgrade issues |
| **3 - SAFE** | None | Online disk expansion (H) | User confirm | No | Systemic disk exhaustion |
| **4 - ALERT** | Investigation needed | Disk pressure taints | Alert user | No | Rare with 100GB+ per pod - investigate |
| **5 - LOW RISK** | Minimal | Backing image recovery (G) | User confirm | No | Image availability issues |
| **6 - MEDIUM RISK** | Development only | emptyDir switch (A) | User confirm + verify dev | No | Development workloads only |
| **7 - HIGH RISK** | Production data | local-path migration (B) | User confirm + backup verify | **YES** | Single-node persistent needs |
| **8 - DESTRUCTIVE** | Data corruption risk | Filesystem repair (E) | User "DELETE DATA" consent | **YES** | EXT4 corruption detected |
| **9 - DESTRUCTIVE** | Complete loss | Clean slate (D) | User "DELETE DATA" consent | **YES** | Absolute last resort |

**Data Protection Rules:**
- **Priority 1-4:** Apply automatically or with minimal confirmation
- **Priority 5:** Only for development workloads, never production
- **Priority 6+:** Require backup proof + explicit user consent
- **Never auto-escalate** from safe to destructive solutions
- **When in doubt:** Ask user - data loss is unacceptable

## Solution Implementations

### Solution A: emptyDir (Priority 5 - MEDIUM RISK)

**DATA LOSS WARNING:** This solution DESTROYS all data in the volume
**ONLY USE FOR:** Development/testing workloads where data loss is acceptable
**NEVER USE FOR:** Production databases, user data, or any persistent storage

**Requirements before applying:**
- Confirm this is a development/test workload
- Verify no production data is stored
- User must explicitly confirm data loss is acceptable

```yaml
# Replace PVC reference with emptyDir
spec:
  template:
    spec:
      containers:
      - name: app
        volumeMounts:
        - name: data
          mountPath: /data
      volumes:
      - name: data
        emptyDir: {}
```

**Apply changes:**
```bash
kubectl apply -f <updated-manifest.yaml>
```

### Solution B: local-path Storage (Priority 6 - HIGH RISK)

**DATA MIGRATION REQUIRED:** Existing data must be backed up and restored
**BACKUP MANDATORY:** Recent backup (<24h) must exist before proceeding

**Requirements before applying:**
- Verify backup exists: `~/bin/backup_script.sh status`
- Confirm backup is recent and tested
- User must acknowledge data migration effort required

```yaml
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: data-pvc
spec:
  accessModes:
    - ReadWriteOnce
  storageClassName: local-path
  resources:
    requests:
      storage: 10Gi
```

**If switching from existing PVC:**
```bash
# Delete old PVC/PV
kubectl delete pvc <old-pvc> -n <namespace>
kubectl delete pv <old-pv>  # If necessary

# Apply new PVC
kubectl apply -f <local-path-pvc.yaml>
```

### Solution C: Longhorn Component Restart (15-30 minutes)

**Best for:** When Longhorn is truly stuck, preserving data

```bash
# Restart instance manager on affected node
kubectl delete pod -n longhorn-system -l longhorn.io/component=instance-manager

# Or restart entire Longhorn system
kubectl rollout restart deployment -n longhorn-system
kubectl rollout restart daemonset -n longhorn-system
```

### Solution D: Clean Slate (Priority 8 - DESTRUCTIVE - LAST RESORT)

**COMPLETE DATA LOSS:** This destroys ALL data - no recovery possible without backup
**ONLY USE WHEN:** All other solutions failed AND backup exists AND data recreation is feasible
**REQUIRES:** Explicit user consent with "DELETE ALL DATA" confirmation

**Requirements before applying:**
- Verified recent backup exists and is restorable
- User explicitly types "DELETE ALL DATA" to confirm
- All dependent services prepared for downtime
- Data recreation plan documented and feasible

```bash
# Delete everything
kubectl delete statefulset <name> -n <namespace>
kubectl delete pvc <name> -n <namespace>
kubectl delete pv <pv-name>

# Wait for cleanup
sleep 30

# Reapply fresh manifests
kubectl apply -f <manifests/>
```

### Solution E: Filesystem Repair (Priority 7 - DESTRUCTIVE)

**DATA CORRUPTION RISK:** fsck can cause additional data loss or corruption
**BACKUP REQUIRED:** Must have verified backup before attempting repair
**USER CONSENT:** Must explicitly approve potential data loss

**Requirements before applying:**
- Recent backup exists and tested for restore capability
- User understands fsck may make data unrecoverable
- Alternative solutions (C, F, G) attempted first

```bash
# VERIFY BACKUP EXISTS FIRST
~/bin/backup_script.sh status

# Stop the problematic pod
kubectl delete pod <pod-name> -n <namespace> --grace-period=0

# Run fsck on the Longhorn device (requires access to node)
# SSH to the node where replica is running
ssh user@<node-ip>
sudo fsck.ext4 /dev/longhorn/<volume-name>

# If fsck can repair, restart pod
kubectl apply -f <pod-manifest.yaml>

# If fsck fails, DO NOT recreate - restore from backup
```

**CRITICAL:** If fsck fails, restore from backup - do NOT proceed to Solution D

### Solution F: Clear PendingNodeID After Upgrade (NEW - 2 minutes)

**Best for:** Post v1.4.x→v1.5.x upgrade attachment failures

```bash
# Find volumes with non-empty PendingNodeID
kubectl get volumes.longhorn.io -n longhorn-system -o json | jq -r '.items[] | select(.status.pendingNodeID != "") | .metadata.name'

# Clear the PendingNodeID
kubectl edit volumes.longhorn.io <volume-name> -n longhorn-system --subresource=status
# In editor: set spec.pendingNodeID to ""
```

### Solution G: Backing Image Recovery (NEW - 10-20 minutes)

**Best for:** Backing image unavailability preventing volume attachment

```bash
# Check backing image status
kubectl get backingimages.longhorn.io -n longhorn-system

# Identify which disks should have the backing image
kubectl get backingimages.longhorn.io <backing-image-name> -n longhorn-system -o yaml

# If no available copy, you may need to:
# 1. Restore from backup
# 2. Recreate volumes without backing image
# 3. Or wait for Longhorn to recover automatically
```

### Solution H: Online Disk Expansion (NEW - 2026-01-13) (ZERO DOWNTIME)

**Best for:** Systemic disk exhaustion across multiple nodes (Terraform config drift, undersized disks)

**ZERO DATA LOSS - ZERO DOWNTIME - Proven on 9-node production cluster**

**Requirements before applying:**
- Proxmox VE API access with API token
- Guest VMs run Ubuntu/Debian with `cloud-guest-utils` (provides `growpart`)
- VM disks on expandable storage (zp3 ZFS, local-lvm, etc.)
- Sufficient storage pool capacity for expansion
- SSH access to guest VMs

**Pre-Flight Checks (CRITICAL):**
```bash
# 1. Verify all VMs on correct storage backend
for vmid in {131..139}; do
  curl -s -k -H "Authorization: PVEAPIToken=<token>" \
    "https://<proxmox>:8006/api2/json/nodes/pve/qemu/${vmid}/config" \
    | jq -r '.data | to_entries[] | select(.key | test("^(virtio|scsi|sata)0$")) | "\(.key): \(.value)"'
done

# 2. Check storage pool capacity (need ~70GB × number of nodes)
curl -s -k -H "Authorization: PVEAPIToken=<token>" \
  "https://<proxmox>:8006/api2/json/nodes/pve/storage/zp3/status" \
  | jq '.data | {total_gb: (.total/1024/1024/1024|floor), avail_gb: (.avail/1024/1024/1024|floor)}'

# 3. Verify Longhorn replica distribution (no single-point-of-failure)
kubectl get replicas.longhorn.io -n longhorn-system -o json \
  | jq -r '.items[] | "\(.spec.volumeName): \(.spec.nodeID)"' | sort | uniq -c

# 4. Verify virtio-scsi/block device support on guest
ssh user@<node-ip> "lsblk | grep -E '(vd|sd)'"
```

**STOP if any pre-flight check fails!**

**Expansion Procedure (Per Node):**

**Step 1: Backup current VM config**
```bash
curl -s -k -H "Authorization: PVEAPIToken=<token>" \
  "https://<proxmox>:8006/api2/json/nodes/pve/qemu/<vmid>/config" \
  > /tmp/worker<vmid>-config-backup.json
```

**Step 2: Proxmox-side online resize (VM stays running)**
```python
import requests
from urllib3 import disable_warnings
from urllib3.exceptions import InsecureRequestWarning
disable_warnings(InsecureRequestWarning)

TOKEN = "root@pam!<token-name>=<token-secret>"
VMID = 139  # Example
NEW_SIZE = "100G"

headers = {"Authorization": f"PVEAPIToken={TOKEN}"}
payload = {"disk": "virtio0", "size": NEW_SIZE}  # or scsi0, sata0 depending on config

resp = requests.put(
    f"https://<proxmox>:8006/api2/json/nodes/pve/qemu/{VMID}/resize",
    headers=headers,
    data=payload,
    verify=False
)

if resp.status_code == 200:
    print(f"✅ Resize task started: {resp.json()['data']}")
else:
    print(f"❌ Resize failed: {resp.status_code}")
```

**Step 3: Guest-side partition expansion (hot)**
```bash
# Grow partition to use full disk (online, no reboot)
ssh user@<node-ip> "sudo growpart /dev/vda 1"

# Verify partition expanded
ssh user@<node-ip> "lsblk /dev/vda"
```

**Step 4: Filesystem expansion (hot)**
```bash
# Extend ext4 filesystem (online resize)
ssh user@<node-ip> "sudo resize2fs /dev/vda1"

# Verify new capacity
ssh user@<node-ip> "df -h / | grep vda1"
# Expected: ~98GB total for 100GB disk
```

**Step 5: Verify Longhorn detected new capacity**
```bash
# Longhorn updates within 30-60 seconds
kubectl get nodes.longhorn.io -n longhorn-system <node-name> -o json \
  | jq '.status.diskStatus | to_entries[0].value | {maxGB: (.storageMaximum/1024/1024/1024|floor), availGB: (.storageAvailable/1024/1024/1024|floor), schedulable: .conditions[]|select(.type=="Schedulable").status}'
```

**Multi-Node Expansion Strategy:**

**For 9+ nodes, use batched expansion to avoid Longhorn replica thrashing:**

```bash
#!/bin/bash
# Batch 1: Workers 134-136 (parallel)
for vmid in 134 135 136; do
  ./expand-node.sh $vmid &
done
wait

# Wait for Longhorn stability (30-60s)
sleep 60

# Batch 2: Workers 137-139 (parallel)
for vmid in 137 138 139; do
  ./expand-node.sh $vmid &
done
wait

# Control plane: Serial expansion with health checks
for vmid in 131 132 133; do
  ./expand-node.sh $vmid
  kubectl get nodes | grep worker${vmid}
  sleep 30  # Allow cluster to stabilize
done
```

**Success Criteria:**
- ✅ VM remained running throughout (check `uptime`)
- ✅ Filesystem shows expected size (~98GB for 100GB disk)
- ✅ Longhorn disk status updated (storageMaximum increased)
- ✅ All Longhorn disks Schedulable=True
- ✅ No pod evictions or restarts during expansion
- ✅ Degraded/faulted volumes begin recovering

**Validation Commands:**
```bash
# 1. Verify all nodes expanded
for vmid in {131..139}; do
  ssh user@192.168.0.${vmid} "hostname && df -h / | grep vda1"
done

# 2. Longhorn disk health
kubectl get nodes.longhorn.io -n longhorn-system -o json \
  | jq -r '.items[] | "\(.metadata.name): max=\(.status.diskStatus | to_entries[0].value.storageMaximum / 1024/1024/1024|floor)GB avail=\(.status.diskStatus | to_entries[0].value.storageAvailable / 1024/1024/1024|floor)GB"'

# 3. Check for degraded/faulted volumes recovering
kubectl get volumes.longhorn.io -n longhorn-system \
  | grep -vE "attached-healthy"
```

**Timeline (Production-Proven):**
- Single node: 5-10 minutes
- 9 nodes (batched): 4-6 hours total
- No downtime, no data loss

**Troubleshooting:**

**If growpart fails:**
- Check partition table type: `sudo fdisk -l /dev/vda` (GPT recommended)
- Verify cloud-guest-utils installed: `dpkg -l | grep cloud-guest-utils`

**If resize2fs fails:**
- Check filesystem type: `mount | grep vda1` (ext4 required, xfs uses `xfs_growfs`)
- Verify partition grew: `lsblk /dev/vda`

**If Longhorn doesn't detect:**
- Wait 60 seconds for sync
- Check node conditions: `kubectl describe node <name>`
- Restart longhorn-manager: `kubectl delete pod -n longhorn-system -l app=longhorn-manager --field-selector spec.nodeName=<name>`

**Rollback:**
- Not needed - disk expansion is safe and non-destructive
- If filesystem expansion fails, VM remains operational with original filesystem size
- Larger disk without expanded filesystem is harmless

**Prevention (Post-Expansion):**
- Set up monitoring at 50% disk utilization threshold
- Automate Terraform state validation against actual disk sizes
- Document actual disk sizes in infrastructure docs
- Schedule quarterly capacity reviews

## Prevention Strategies

### Critical: Proactive Disk Monitoring & Alerting

**USER REQUIREMENT:** "I wish someone had alerted me that we were approaching disk limits"

**Set up automated monitoring to prevent future disk exhaustion:**

#### 1. Immediate Alerting Setup (Priority 1)

**Longhorn Built-in Alerting:**
```bash
# Configure Longhorn to alert at 50% disk usage (not 25% default)
kubectl patch settings.longhorn.io storage-minimal-available-percentage -n longhorn-system \
  --type merge -p '{"value":"50"}'

# Set up webhook notification for Longhorn events
kubectl edit settings.longhorn.io backup-target -n longhorn-system
# Add webhook URL for Slack/Discord/email alerts
```

**Prometheus AlertManager Rules:**
```yaml
# /etc/prometheus/rules/longhorn-disk-alerts.yaml
groups:
  - name: longhorn_disk_alerts
    interval: 60s
    rules:
      - alert: LonghornDiskUsageHigh
        expr: |
          (longhorn_disk_capacity_bytes - longhorn_disk_usage_bytes)
          / longhorn_disk_capacity_bytes * 100 < 50
        for: 5m
        labels:
          severity: warning
          component: longhorn
        annotations:
          summary: "Longhorn disk {{ $labels.node }} approaching capacity"
          description: "Node {{ $labels.node }} has less than 50% disk free. Current: {{ $value }}%"

      - alert: LonghornDiskUsageCritical
        expr: |
          (longhorn_disk_capacity_bytes - longhorn_disk_usage_bytes)
          / longhorn_disk_capacity_bytes * 100 < 25
        for: 2m
        labels:
          severity: critical
          component: longhorn
        annotations:
          summary: "CRITICAL: Longhorn disk {{ $labels.node }} near exhaustion"
          description: "Node {{ $labels.node }} has less than 25% free. IMMEDIATE ACTION REQUIRED. Current: {{ $value }}%"
```

**Simple Bash Monitoring Script (if no Prometheus):**
```bash
#!/bin/bash
# /home/psimmons/bin/longhorn-disk-monitor.sh
# Run via cron every hour: 0 * * * * /home/psimmons/bin/longhorn-disk-monitor.sh

ALERT_THRESHOLD=50  # Alert at 50% usage
CRITICAL_THRESHOLD=75  # Critical at 75% usage

kubectl get nodes.longhorn.io -n longhorn-system -o json | jq -r '
  .items[] |
  .status.diskStatus |
  to_entries[0].value |
  select(.storageMaximum > 0) |
  {
    node: .diskName,
    used_pct: ((.storageMaximum - .storageAvailable) / .storageMaximum * 100 | floor),
    avail_gb: (.storageAvailable / 1024 / 1024 / 1024 | floor)
  } |
  select(.used_pct >= '"$ALERT_THRESHOLD"') |
  "\(.node): \(.used_pct)% used, \(.avail_gb)GB available"
' | while read alert; do
  echo "[$(date)] ALERT: $alert" | tee -a /var/log/longhorn-disk-monitor.log

  # Send notification (customize for your setup)
  # curl -X POST https://your-webhook-url -d "Longhorn disk alert: $alert"
  # mail -s "Longhorn Disk Alert" your@email.com <<< "$alert"
done
```

**Install monitoring script:**
```bash
sudo cp /tmp/longhorn-disk-monitor.sh /home/psimmons/bin/
sudo chmod +x /home/psimmons/bin/longhorn-disk-monitor.sh
(crontab -l 2>/dev/null; echo "0 * * * * /home/psimmons/bin/longhorn-disk-monitor.sh") | crontab -
```

#### 2. Weekly Capacity Review

```bash
#!/bin/bash
# Weekly capacity report - runs every Monday 9am
# cron: 0 9 * * 1 /home/psimmons/bin/weekly-capacity-report.sh

echo "=== Longhorn Capacity Report - $(date) ===" > /tmp/capacity-report.txt

echo "Node Disk Usage:" >> /tmp/capacity-report.txt
kubectl get nodes.longhorn.io -n longhorn-system -o json | \
  jq -r '.items[] | .status.diskStatus | to_entries[0].value |
  "\(.diskName): \((.storageMaximum / 1024/1024/1024|floor))GB max, \
  \((.storageAvailable / 1024/1024/1024|floor))GB free (\
  ((1 - .storageAvailable/.storageMaximum) * 100|floor))% used)"' \
  >> /tmp/capacity-report.txt

echo -e "\nVolume Health:" >> /tmp/capacity-report.txt
kubectl get volumes.longhorn.io -n longhorn-system | \
  grep -vE "attached-healthy" >> /tmp/capacity-report.txt || echo "All volumes healthy"

echo -e "\nProjected exhaustion (at current growth rate):" >> /tmp/capacity-report.txt
# TODO: Calculate growth rate from historical data

cat /tmp/capacity-report.txt
# Email report: mail -s "Longhorn Weekly Report" your@email.com < /tmp/capacity-report.txt
```

#### 3. Infrastructure Validation (Monthly)

**Verify Terraform state matches reality:**
```bash
#!/bin/bash
# Monthly infrastructure audit
# Checks that actual disk sizes match Terraform config

for vmid in {131..139}; do
  # Get actual filesystem size
  ACTUAL=$(ssh psimmons@192.168.0.${vmid} "df -h / | grep vda1 | awk '{print \$2}'")

  # Get Terraform expected size (from tfvars)
  EXPECTED=$(grep -A 10 "worker${vmid}" /path/to/terraform.tfvars | grep vm_disk_size | awk '{print $3}')

  if [ "$ACTUAL" != "$EXPECTED" ]; then
    echo "WARNING: worker${vmid} disk mismatch! Actual: $ACTUAL, Expected: $EXPECTED"
  fi
done
```

#### 4. Dashboard Recommendations

**Grafana Dashboard Panels:**
- Longhorn disk usage per node (gauge, alert at 50%)
- Longhorn volume health status
- Disk usage growth rate (7-day trend)
- Projected days until exhaustion
- PVC count per namespace

**Key Metrics to Track:**
- `longhorn_disk_usage_bytes`
- `longhorn_disk_capacity_bytes`
- `longhorn_volume_robustness` (healthy/degraded/faulted)
- `node_filesystem_avail_bytes{mountpoint="/"}`

#### 5. Action Thresholds

| Usage Level | Action | Timeline |
|-------------|--------|----------|
| <50% | Normal operation | Monitor weekly |
| 50-65% | Review capacity plan | Within 2 weeks |
| 65-75% | Plan expansion | Within 1 week |
| 75-85% | Execute expansion | Within 3 days |
| >85% | EMERGENCY expansion | IMMEDIATE |

**Automated Response at 75%:**
```bash
# Trigger expansion workflow automatically
if [ "$DISK_USAGE" -gt 75 ]; then
  echo "Disk usage critical, generating expansion plan..."
  /home/psimmons/bin/generate-expansion-plan.sh
  # Notify user for approval
fi
```

### Proactive Monitoring (Legacy Section)
```bash
# Check disk usage regularly
kubectl top nodes

# Monitor Longhorn volumes
kubectl get volumes.longhorn.io -n longhorn-system

# Clean up unused PVCs
kubectl get pvc --all-namespaces | grep -v Bound
```

### Development Best Practices
- Use `emptyDir` for dev databases until production-ready
- Switch to Longhorn only when data persistence is critical
- Set up local container registry to avoid image distribution issues

### Production Readiness
- Monitor disk space (with 100GB+ per pod, usage should remain low - alert if >50% utilization)
- Regular PVC cleanup (unused test PVCs)
- Backup critical volumes before major changes
- **Disk Space Context:** Massive Proxmox + TrueNAS storage available - disk pressure should be extremely rare

## Common Mistakes

| Mistake | Why Bad | Do Instead |
|---------|---------|------------|
| Restart Traefik for storage issues | Different systems | Check pod events first |
| Delete PVC without checking PV | Leaves orphaned PV | Check PV status before deletion |
| Use StatefulSet without planning PVC cleanup | PVCs persist after deletion | Use Deployment + PVC for simpler cleanup |
| Ignore disk pressure taints | False positives block scheduling | Verify actual disk usage first |
| Sequential troubleshooting | Takes too long | Use parallel checks (pod + PVC + Longhorn status) |

## Emergency Commands

**If completely stuck:**
```bash
# Nuclear option - restart everything (last resort)
kubectl delete pod -n longhorn-system --all
kubectl delete volumeattachment --all
```

**Quick pod restart:**
```bash
kubectl delete pod <stuck-pod> -n <namespace> --grace-period=0 --force
```

## Integration Points

**Works with these homelab systems:**
- Health check script: `~/bin/health-check.sh`
- Backup validation: `homelab:test-backups`
- Incident response: `homelab:incident-response`
- Production readiness: `homelab:production-readiness`

**Related issues to check:**
- Image distribution across nodes (causes ImagePullBackOff)
- Network policies blocking CSI traffic
- Node resource constraints
- DNS resolution for Longhorn services

## Success Metrics

**Problem resolved when:**
- Pod reaches `Running` status
- Volume mounts successfully (`kubectl exec -it <pod> -- df -h`)
- Application can read/write data
- No volume attachment errors in logs

**Time targets:**
- emptyDir switch: <5 minutes
- local-path migration: <15 minutes
- Longhorn restart: <30 minutes
- Clean slate: <60 minutes

## Automation Philosophy

### Data Protection First, Efficiency Second
- **DATA PRESERVATION IS ABSOLUTE PRIORITY** - no automation if data is at risk
- **Conservative automation** - only apply solutions with 100% data safety automatically
- **User consent required** for any action with data loss potential
- **Backup verification mandatory** before destructive actions
- **Fail safe** - when uncertain, ask user instead of risking data

### Token Conservation Rules
1. **Stop repeating the same action** - indicates stuck in loop
2. **Escalate after 2 failed attempts** - don't keep trying broken solutions
3. **Limit automated attempts to 15 minutes** - force user intervention if needed
4. **Report progress every 30 seconds** - transparency without verbosity
5. **Auto-learn from successes** - immediately add working solutions to automation

### When Automation Stops
**User intervention required when:**
- Destructive actions needed (data loss risk)
- Unfamiliar error patterns (low confidence matching)
- Multiple solution failures (automation exhausted)
- Time budget exceeded (15+ minutes of automated attempts)
- User explicitly requests manual control

### Data Protection Monitoring (Primary Metrics)
**Track these metrics per session:**
- **Data loss incidents:** Must be 0 (absolute requirement)
- **Backup verification rate:** 100% for risky actions
- **User consent obtained:** For all destructive actions
- **Safe solution success rate:** Percentage resolved without data risk
- **Escalation rate:** How often user intervention was needed

**Secondary Metrics:**
- Time to resolution (data preservation comes first)
- Token efficiency (secondary to data safety)
- Automation coverage (must not compromise data safety)

**Goal:** 0 data loss incidents, 100% backup verification, maximum safe resolutions

## Self-Learning Updates

**This skill evolves automatically as new Longhorn issues are discovered and solved.**

### Learning Triggers
- User encounters new Longhorn error patterns
- Online research reveals additional failure modes
- Cluster events show previously unseen storage issues
- Pod crash analysis reveals storage-disguised failures

### Update Process
1. **Detect new issue:** When automation fails (unfamiliar pattern or repeated failures)
2. **Research solution:** Check Longhorn KB, GitHub issues, forums automatically
3. **Test fix:** Apply solution and verify effectiveness
4. **Automate solution:** Add to automated decision engine if safe/reliable
5. **Update skill:** Add new root cause, symptoms, and solution steps
6. **Document incident:** Add to troubleshooting guide for future reference

### Automation Learning
**When solution works reliably:**
- **Safe solutions:** Add to full automation (no user interaction)
- **Medium-risk:** Add to semi-auto (confirmation required)
- **High-risk:** Keep manual (always require user approval)

**Continuous Improvement:**
- **Success tracking:** Which automated solutions work most often
- **Failure analysis:** Why automation fails and how to improve
- **Pattern recognition:** Learn new disguised storage issue patterns
- **Efficiency tuning:** Adjust automation thresholds based on results

### Recent Additions (2026-01-11)

| Date | Addition | Source | Status |
|------|----------|--------|---------|
| 2026-01-11 | **Step 0: Disguised Storage Issues** | Analysis of CrashLoopBackOff patterns | ✅ Added |
| 2026-01-11 | **Root Cause #5: Filesystem Corruption** | Longhorn KB: filesystem corruption | ✅ Added |
| 2026-01-11 | **Root Cause #6: Backing Image Unavailability** | Longhorn KB: backing image issues | ✅ Added |
| 2026-01-11 | **Root Cause #7: Volume Expansion Issues** | Longhorn KB: expansion degradation | ✅ Added |
| 2026-01-11 | **Root Cause #8: Post-Upgrade Attachment** | Longhorn KB: v1.5.x upgrade issues | ✅ Added |
| 2026-01-11 | **Root Cause #9: Replica Scheduling Race** | GitHub issues: scheduling race conditions | ✅ Added |
| 2026-01-11 | **Solution E: Filesystem Repair** | Longhorn KB: fsck repair procedures | ✅ Added |
| 2026-01-11 | **Solution F: Clear PendingNodeID** | Longhorn KB: post-upgrade fixes | ✅ Added |
| 2026-01-11 | **Solution G: Backing Image Recovery** | Longhorn KB: backing image troubleshooting | ✅ Added |
| **2026-01-11** | **EXPERIMENTAL VALIDATION: Volume Attachment Deadlock** | **Live Linkwarden issue resolution** | **✅ SUCCESSFUL** |
| **2026-01-11** | **EXPERIMENTAL: Disk Space Exhaustion Pattern** | **Live Prometheus volume investigation** | **✅ IDENTIFIED** |
| **2026-01-11** | **EXPERIMENTAL: Setting Adjustment Techniques** | **Live storage-minimal-available-percentage modification** | **✅ TESTED** |
| **2026-01-11** | **EXPERIMENTAL: Salvage Operation Methods** | **Live replica salvage attempt** | **✅ ATTEMPTED** |
| **2026-01-11** | **ARCHITECTURE INSIGHT: Proxmox ZFS NFS Solution** | **User technical analysis + storage assessment** | **✅ SUPERIOR APPROACH** |
| **2026-01-11** | **SYSTEMIC ISSUES IDENTIFIED: Image Pull + Resource + Monitoring** | **Cluster-wide assessment** | **📋 OPPORTUNITIES CATALOGED** |
| **2026-01-13** | **Root Cause #10: Undersized Node Disks (Terraform Drift)** | **Live cluster disk expansion 30GB→100GB** | **✅ RESOLVED** |
| **2026-01-13** | **PRODUCTION VALIDATION: Zero-Downtime Online Disk Expansion** | **9-node expansion using Proxmox API + growpart + resize2fs** | **✅ PROVEN** |

### Experimental Findings (2026-01-11 Session)

**🔍 Disk Space Exhaustion Pattern:**
- **Issue:** Node disk 94% full (1.9GB available) vs Longhorn minimum requirement (8GB)
- **Root Cause:** Local node storage exhausted, external storage (TrueNAS) not configured
- **Impact:** Critical volumes (Prometheus) become faulted, monitoring lost
- **Solution:** Configure Longhorn to use external storage or expand node disks

**🛠️ Setting Adjustment Techniques:**
- **Method:** `kubectl patch settings.longhorn.io storage-minimal-available-percentage`
- **Effect:** Temporarily allows scheduling on low-space disks (experimental)
- **Risk:** Can lead to disk-full scenarios if not monitored
- **Best Use:** Emergency recovery, not permanent solution

**🔧 Salvage Operation Methods:**
- **Method:** `kubectl patch replica --type merge -p '{"spec":{"salvageRequested": true}}'`
- **Effect:** Attempts to recover stopped replicas
- **Limitations:** May not work if underlying storage issues persist
- **Use Case:** Last resort before volume recreation

**🏗️ Architecture Optimization Insight:**
- **Current Issue:** VMs have tiny local disks (30GB) while Proxmox ZFS has 14TB free
- **User's Superior Solution:** Proxmox NFS exports from ZFS pool to VMs
- **Benefits:** Access to full storage capacity, same hardware, no TrueNAS dependency
- **Performance:** Direct ZFS access via NFS maintains SAS controller benefits
- **Reliability:** Eliminates disk exhaustion while maintaining single point of failure avoidance

**🔬 Systemic Infrastructure Opportunities Identified:**
- **Image Management Issues:** 104+ pods with problems, ImagePullBackOff suggests container registry optimization needed
- **Resource Optimization:** Linkwarden using 513m CPU/1266Mi RAM - potential for tuning or resource limits
- **CSI Plugin Cleanup:** FailedPreStopHook warnings indicate shutdown cleanup issues
- **Storage Alerting:** Need monitoring at 50% utilization once NFS storage is implemented
- **Automated Maintenance:** Good backup system exists, but could add disk space monitoring alerts

### Future Learning Targets
- Longhorn v1.6.0+ features and issues
- Integration with Longhorn backup/restore workflows
- Performance optimization patterns
- Multi-cluster Longhorn configurations

## Automation Implementation Details

### SAFE Auto-Apply Actions (Zero Data Risk)
```bash
# These run automatically without user confirmation:
kubectl rollout restart deployment -n longhorn-system           # Component restart (non-destructive)
kubectl edit volumes.longhorn.io <vol> --subresource=status     # Clear PendingNodeID
kubectl get volumes.longhorn.io -n longhorn-system              # Status monitoring only
kubectl describe volume <vol> -n longhorn-system                # Diagnostic info only

# DISK PRESSURE DETECTED (RARE - Environment has massive storage):
# ALERT USER IMMEDIATELY - Do NOT auto-remove taints
echo "⚠️  DISK PRESSURE TAINT DETECTED - This should be rare with 100GB+ per pod!"
echo "Investigate immediately - user can fix disk space issues"
```

### SEMI-AUTO Actions (Requires User Confirmation)
```bash
# These prompt user and require backup verification:
kubectl apply -f emptydir-pod.yaml                              # emptyDir switch (dev only)
kubectl delete pvc <pvc> && kubectl apply -f local-path-pvc.yaml # Storage class switch
kubectl delete pod <pod> --grace-period=0                       # Force pod restart
```

### MANUAL-ONLY Actions (Data Loss Risk - Explicit Consent Required)
```bash
# These ALWAYS require explicit user approval + backup verification:
kubectl delete pvc <pvc> && kubectl delete pv <pv>               # Clean slate (data loss)
ssh <node> && sudo fsck.ext4 /dev/longhorn/<vol>                # Filesystem repair
kubectl delete volumes.longhorn.io <vol>                        # Volume deletion

# BACKUP VERIFICATION REQUIRED BEFORE ANY MANUAL ACTION:
~/bin/backup_script.sh status
kubectl get backups.longhorn.io -n longhorn-system
echo "Type 'DELETE DATA' to confirm you understand data loss risk:"
read confirmation
if [ "$confirmation" != "DELETE DATA" ]; then exit 1; fi
```

### Progress Reporting Format
```
[30s] Collecting diagnostic data...
[45s] Pattern matched: DISK_PRESSURE_TAINT
[50s] Applying solution: remove disk-pressure taint
[55s] Monitoring effectiveness...
[2m] SUCCESS: Pod now running. Resolution time: 2m 15s
```

### Loop Detection Logic
```python
# Pseudo-code for loop prevention
attempts = {}
if action in attempts:
    attempts[action] += 1
    if attempts[action] >= 3:
        stop_automation("Loop detected: " + action)
else:
    attempts[action] = 1
```