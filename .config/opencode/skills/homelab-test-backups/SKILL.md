---
name: homelab:test-backups
description: Use monthly to test backup restore procedures - if you can't restore it, you don't have a backup
---

# Backup Restore Testing Procedure

**Announce:** "Running backup restore testing procedure..."

## Critical Rule

**If you can't restore it, you don't have a backup.**

## Step 1: Check What's Due for Testing

Review Backup Validation Matrix in CLAUDE.md:

| Service | Backup Frequency | Last Restore Test | Next Test Due |
|---------|------------------|-------------------|---------------|
| Nextcloud | Daily 2:00 AM | [check] | [check] |
| Longhorn Critical | Every 12 hours | [check] | [check] |
| Longhorn Daily | Daily 3:00 AM | [check] | [check] |
| Projects (Git) | On demand | [check] | [check] |

Identify backups that are overdue for testing.

## Step 2: Select Test Target

**CRITICAL:** Never restore to production!

Test targets:
- Kubernetes: Create test namespace (`kubectl create ns restore-test`)
- VMs: Use test VM or temporary container
- Files: Restore to `/tmp/restore-test/`

## Step 3: Execute Restore Test

**Before starting:** Note the current time - you'll use this to calculate Recovery Time (RTO) for the test report.

### For Nextcloud:
```bash
# 1. Identify latest backup
ssh psimmons@192.168.0.200 "ls -la /backup/nextcloud/ | tail -5"

# 2. Restore to test location (DO NOT restore to production)
# Follow documented restore procedure
# See: /home/psimmons/projects/nextcloud-deployment/MAINTENANCE-AUTOMATION-STATUS.md
```

### For Longhorn:
```bash
# 1. List available backups
kubectl get backup -n longhorn-system

# 2. Create test PVC from backup
# Follow Longhorn restore documentation
# See: Longhorn UI (https://longhorn-ui-endpoint) or kubectl describe backup <backup-name>
```

### For Git Projects:
```bash
# 1. Clone from backup location
cd /tmp
git clone TrueNAS:/mnt/public/git-repos/projects.git restore-test

# 2. Verify content
ls -la restore-test/
```

## Step 4: Verify Data Integrity

- [ ] Files/data present and readable
- [ ] No corruption errors
- [ ] Recent data included (check timestamps)
- [ ] Application can use restored data (if applicable)

**IMPORTANT:** If any verification step fails, do NOT mark the test as PASS. Document the failure and investigate the backup procedure immediately. A failed restore test means the backup is not reliable.

## Step 5: Cleanup Test Resources

```bash
# Remove test namespace
kubectl delete ns restore-test

# Remove test files
rm -rf /tmp/restore-test/
```

## Step 6: Update Backup Validation Matrix

Edit CLAUDE.md and update:
- "Last Restore Test" column with today's date
- "Test Result" column with Pass/Fail
- "Next Test Due" column with next due date

## Step 7: Document Results

First, ensure the test reports directory exists:
```bash
mkdir -p /home/psimmons/RUNBOOKS/backup-restore-tests/
```

Then create test report: `/home/psimmons/RUNBOOKS/backup-restore-tests/[YYYY-MM-DD]-[service].md`

```markdown
# Backup Restore Test: [Service]

**Date:** [YYYY-MM-DD]
**Backup Source:** [location]
**Restore Target:** [test location]

## Test Results
- Data integrity: [PASS/FAIL]
- Recovery time: [X minutes]
- Issues encountered: [none/describe]

## Notes
[Any observations or improvements needed]
```

## Testing Schedule

- **Production backups:** Test monthly (1st of month)
- **Configuration backups:** Test quarterly (Jan/Apr/Jul/Oct)
