---
name: homelab-troubleshoot-nextcloud
description: Nextcloud-specific troubleshooting for production VM at 192.168.0.200. Verifies live state first (not docs), checks all 3 components (Apache, PostgreSQL, Redis), maintenance mode, background jobs, and automation health.
---

# Troubleshoot Nextcloud

## Overview

Systematic troubleshooting skill for the production Nextcloud instance at https://nextcloud.petersimmons.com (192.168.0.200). Follows the critical rule: **Verify live state FIRST, then docs.**

**Purpose**: Quickly diagnose and resolve Nextcloud issues by checking all service components in the correct order.

## When to Use

**Triggers**:
- Nextcloud is slow or unresponsive
- Users report file sync issues
- "503 Service Unavailable" errors
- User says "Nextcloud down" / "check Nextcloud" / "fix Nextcloud"
- Scheduled maintenance verification
- After infrastructure changes affecting 192.168.0.200

**When NOT to use**:
- General K8s troubleshooting (Nextcloud is VM-based, not K8s)
- DNS-only issues (check Pi-holes first)
- Traefik issues (Nextcloud has its own Apache)

## Quick Reference

| Component  | Service     | Expected State | Port |
|------------|-------------|----------------|------|
| Web Server | apache2     | active         | 80/443 |
| Database   | postgresql  | active         | 5432 |
| Cache      | redis-server| active         | 6379 |

**Server**: 192.168.0.200 (nextcloud.petersimmons.com)
**User**: psimmons (sudo for www-data commands)
**Install Dir**: /var/www/nextcloud
**Data Dir**: /var/nextcloud-data

## Troubleshooting Process

### Step 1: Quick Health Check (Remote)

**From local machine** (before SSH):

```bash
curl -I https://nextcloud.petersimmons.com
```

**Expected responses**:
- `HTTP/2 200` or `HTTP/2 302` - Healthy
- `HTTP/2 503` - Service issue (proceed to Step 2)
- `Connection refused` - Apache down or firewall
- `SSL error` - Certificate issue
- Timeout - Network or server unreachable

### Step 2: SSH and Service Status

```bash
ssh psimmons@192.168.0.200 "systemctl status apache2 postgresql redis-server --no-pager"
```

**Interpret results**:
- All "active (running)" - Services OK, check application layer
- apache2 failed - Web server issue (critical)
- postgresql failed - Database issue (critical)
- redis-server failed - Cache issue (degraded but functional)

### Step 3: Check Maintenance Mode

```bash
ssh psimmons@192.168.0.200 "sudo -u www-data php /var/www/nextcloud/occ maintenance:mode"
```

**Responses**:
- "Maintenance mode is currently disabled" - Normal operation
- "Maintenance mode is currently enabled" - Intentional or stuck

**To disable maintenance mode**:
```bash
ssh psimmons@192.168.0.200 "sudo -u www-data php /var/www/nextcloud/occ maintenance:mode --off"
```

### Step 4: Check Background Jobs

```bash
ssh psimmons@192.168.0.200 "sudo -u www-data php /var/www/nextcloud/occ background:job:list --limit=5"
```

**Look for**:
- Jobs with very old timestamps (stale)
- Large queue (backlog)
- Failed jobs

**Force run background jobs**:
```bash
ssh psimmons@192.168.0.200 "sudo -u www-data php /var/www/nextcloud/occ background:job:execute"
```

### Step 5: Check Logs

**Nextcloud monitoring log** (automation):
```bash
ssh psimmons@192.168.0.200 "tail -50 /var/log/nextcloud/monitoring.log"
```

**Apache error log**:
```bash
ssh psimmons@192.168.0.200 "sudo tail -50 /var/log/apache2/error.log"
```

**Nextcloud application log**:
```bash
ssh psimmons@192.168.0.200 "sudo tail -50 /var/www/nextcloud/data/nextcloud.log"
```

**Note**: Monitoring log is silent when healthy (by design) - check every 6 hours for status updates.

### Step 6: Common Fixes

**Restart Apache** (if needed):
```bash
ssh psimmons@192.168.0.200 "sudo systemctl restart apache2"
```

**Restart PostgreSQL** (if needed):
```bash
ssh psimmons@192.168.0.200 "sudo systemctl restart postgresql"
```

**Restart Redis** (if needed):
```bash
ssh psimmons@192.168.0.200 "sudo systemctl restart redis-server"
```

**Clear file locks** (stuck files):
```bash
ssh psimmons@192.168.0.200 "sudo -u www-data php /var/www/nextcloud/occ files:cleanup"
```

**Rescan files** (after manual changes):
```bash
ssh psimmons@192.168.0.200 "sudo -u www-data php /var/www/nextcloud/occ files:scan --all"
```

**Add missing database indices**:
```bash
ssh psimmons@192.168.0.200 "sudo -u www-data php /var/www/nextcloud/occ db:add-missing-indices"
```

## Automation Health

### Automated Tasks Reference

| Task               | Schedule         | Log Location                        |
|--------------------|------------------|-------------------------------------|
| Health monitoring  | Every 5 min      | /var/log/nextcloud/monitoring.log   |
| Backup verify      | Daily 2:30 AM    | /var/log/nextcloud/backup-check.log |
| Weekly maintenance | Sunday 3:00 AM   | /var/log/nextcloud/weekly-maintenance.log |
| Monthly review     | 1st 4:00 AM      | /var/log/nextcloud/monthly-maintenance.log |

### Check Automation Status

```bash
# Verify cron is running
ssh psimmons@192.168.0.200 "systemctl status cron --no-pager"

# View scheduled maintenance jobs
ssh psimmons@192.168.0.200 "cat /etc/cron.d/nextcloud-maintenance"

# Check backup timer status
ssh psimmons@192.168.0.200 "systemctl status nextcloud-backup.timer --no-pager"
```

### View Recent Automation Logs

```bash
# Last backup check
ssh psimmons@192.168.0.200 "tail -20 /var/log/nextcloud/backup-check.log"

# Last weekly maintenance
ssh psimmons@192.168.0.200 "tail -50 /var/log/nextcloud/weekly-maintenance.log"

# Last monthly maintenance
ssh psimmons@192.168.0.200 "tail -50 /var/log/nextcloud/monthly-maintenance.log"
```

## Output Format

After completing troubleshooting steps, output this summary:

```
NEXTCLOUD TROUBLESHOOTING (192.168.0.200)

Quick Health Check:
curl -I https://nextcloud.petersimmons.com
HTTP/2 200 OK
* Nextcloud responding

Service Status:
Apache2:    * active (running)
PostgreSQL: * active (running)
Redis:      * active (running)

Maintenance Mode: OFF
Background Jobs: 0 stale (healthy)

Automation:
Cron service:     * active
Last backup:      2026-01-18 (6 hours ago)
Last weekly maint: 2026-01-12

DIAGNOSIS: All systems healthy

RECOMMENDED ACTION: None required
```

## Common Issue Decision Tree

```
Nextcloud Issue?
|
+-- Can't reach at all?
|   +-- curl times out -> Check if VM is running (Proxmox)
|   +-- Connection refused -> Apache down, check service
|   +-- SSL error -> Certificate issue, check certbot
|
+-- 503 Service Unavailable?
|   +-- Apache running? -> No -> Restart Apache
|   +-- PostgreSQL running? -> No -> Check DB, restart if safe
|   +-- Redis running? -> No -> Restart Redis (non-critical)
|   +-- All running? -> Check maintenance mode
|
+-- Slow performance?
|   +-- Background jobs stale -> Force run jobs
|   +-- High memory/disk in logs -> Check resources
|   +-- Database slow -> Check postgresql logs
|
+-- Sync not working?
|   +-- Maintenance mode on -> Turn off
|   +-- File locks stuck -> Run files:cleanup
|   +-- Permissions changed -> Check www-data ownership
```

## Integration with Other Skills

**Before this skill**:
- `homelab:session-startup` - Get overall system health
- Check that Pi-holes (DNS) are working

**During troubleshooting**:
- `superpowers:systematic-debugging` - For complex issues
- `homelab:log-fix-result` - Record fix outcome

**After resolution**:
- `homelab:log-incident` - If significant outage
- `homelab:capture-learning` - If new failure mode discovered

## Documentation References

**Primary docs**:
- /home/psimmons/projects/nextcloud-deployment/MAINTENANCE-AUTOMATION-STATUS.md
- /home/psimmons/projects/nextcloud-deployment/TROUBLESHOOTING.md
- /home/psimmons/projects/nextcloud-deployment/README.md

**Remember**: Verify live state first. Docs may be outdated.

## Credential Locations (On Server)

Credentials are stored on the server only (not in code):
- `/root/.nextcloud-admin-password`
- `/root/.nextcloud-db-password`
- `/root/.nextcloud-redis-password`

## Example Session

```
User: "Nextcloud seems slow"

Assistant: I will troubleshoot Nextcloud using the homelab:troubleshoot-nextcloud skill.

[Runs curl -I https://nextcloud.petersimmons.com]
[SSHs to 192.168.0.200 and checks services]
[Checks maintenance mode and background jobs]

NEXTCLOUD TROUBLESHOOTING (192.168.0.200)

Quick Health Check:
curl -I https://nextcloud.petersimmons.com
HTTP/2 200 OK
* Nextcloud responding

Service Status:
Apache2:    * active (running)
PostgreSQL: * active (running)
Redis:      * active (running)

Maintenance Mode: OFF
Background Jobs: 15 stale jobs (backlogged)

DIAGNOSIS: Background job queue is stale - likely cause of slowness

RECOMMENDED ACTION: Force run background jobs

[Executes: sudo -u www-data php /var/www/nextcloud/occ background:job:execute]

Background jobs cleared. Nextcloud should be responsive now.
```
