# Ansible Drift Alerting

**Issue:** #43
**Owner:** ops
**Last updated:** 2026-06-02

## Overview

A daily systemd timer runs `ansible-playbook --check --diff` against the
`registry-hosts` playbook. If ansible exits with code 2 (tasks would change),
the script automatically files a GitHub issue tagged `severity/serious` in the
`petersimmons1972/homelab-config` repo.

## Components

| Component | Location |
| --------- | -------- |
| Drift check script | `scripts/ansible-drift-notify.sh` (in repo) |
| systemd service | `~/.config/systemd/user/ansible-drift-check.service` (machine-local) |
| systemd timer | `~/.config/systemd/user/ansible-drift-check.timer` (machine-local) |
| Log directory | `~/.local/state/ansible/drift-YYYYMMDD.log` |

## Setup (first-time on a new machine)

1. Clone the repo and ensure `scripts/ansible-drift-notify.sh` is executable:

   ```bash
   chmod +x ~/projects/homelab-config/scripts/ansible-drift-notify.sh
   ```

2. Copy the systemd units from `docs/runbooks/` (or re-create them per the
   spec in this file) to `~/.config/systemd/user/`:

   ```bash
   # units are machine-local — not tracked in git
   cp /path/to/ansible-drift-check.service ~/.config/systemd/user/
   cp /path/to/ansible-drift-check.timer   ~/.config/systemd/user/
   ```

3. Reload systemd and enable the timer:

   ```bash
   systemctl --user daemon-reload
   systemctl --user enable --now ansible-drift-check.timer
   ```

4. Verify the timer is loaded:

   ```bash
   systemctl --user list-timers ansible-drift-check.timer
   ```

## Manual run

```bash
~/projects/homelab-config/scripts/ansible-drift-notify.sh
```

Or trigger via systemd:

```bash
systemctl --user start ansible-drift-check.service
journalctl --user -u ansible-drift-check.service -f
```

## Exit codes

| Code | Meaning |
| ---- | ------- |
| 0 | No drift — all tasks would be no-ops |
| 2 | Drift detected — GitHub issue filed |
| other | ansible-playbook error — check log file |

## Overriding the playbook target

Set `ANSIBLE_DRIFT_PLAYBOOK` before running:

```bash
ANSIBLE_DRIFT_PLAYBOOK=~/projects/homelab-config/ansible/playbooks/other.yaml \
  ~/projects/homelab-config/scripts/ansible-drift-notify.sh
```

## Log rotation

Logs are named `drift-YYYYMMDD.log` and accumulate in `~/.local/state/ansible/`.
They are not auto-rotated. Prune with:

```bash
find ~/.local/state/ansible -name 'drift-*.log' -mtime +30 -delete
```

## GitHub label requirement

The script uses `--label "severity/serious"`. Ensure that label exists in the
repo before the first automated run:

```bash
gh label create "severity/serious" --repo petersimmons1972/homelab-config \
  --color "d93f0b" --description "Serious operational issue"
```
