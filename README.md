# homelab-config

Home-directory configuration and Ansible playbooks for the petersimmons.com homelab.

This repository is checked out at `$HOME` on the operator workstation. It contains:

- The active `CLAUDE.md` (Claude Code operating instructions)
- `~/AGENTS.md` (agent/general roster behavior contract)
- `ansible/` — node-level configuration playbooks
- `.claude/` — Claude Code skills, agents, hooks (selective; most is .gitignored)
- `bin/` — operator scripts (`health-check.sh`, dispatch helpers, etc.)

## Directory map

```
ansible/                 K8s node configuration playbooks
  ansible.cfg
  inventory/hosts.ini    [k8s_workers] worker131..worker139, ansible_host pinned to IPs
  group_vars/            registry_ip, registry_host, etc.
  playbooks/             registry-hosts.yaml (and tests/ fixture suite)

bin/                     Operator scripts — most notably:
                         health-check.sh — fleet + service health probe

docs/                    Operator-facing reference:
                         advisory-protocol.md, engram-memory-rules.md,
                         container-images.md, k8s-firewall.md
```

## Relationships to other repos

This repo is **not** the application layer. It's the operator's local environment plus
the small set of playbooks that maintain node-level state. Application repositories
live elsewhere:

- **clearwatch** — revenue product (`~/projects/clearwatch`)
- **aifleet** — agent fleet controller (`~/projects/aifleet`)
- **longhorn-nfs / supabase-homelab** — storage and data layer
- **engram** — memory service (`~/projects/engram`)

## Running the Ansible playbook

The registry-hosts playbook ensures `registry.petersimmons.com` resolves at the
node level on every K8s worker (so containerd / crictl can pull images during
boot before CoreDNS is available).

From a fresh clone:

```bash
cd ansible

# Preferred: per-node form, lowest blast radius
ansible-playbook playbooks/registry-hosts.yaml --limit worker131.petersimmons.com

# Fleet-wide (only after per-node verification)
ansible-playbook playbooks/registry-hosts.yaml
```

The inventory pins `ansible_host` to numeric IPs so the playbook can run even when
DNS resolution is broken (which is exactly the failure mode it fixes).

### Running the fixture tests

```bash
cd ansible/playbooks/tests
./test-regexp.sh new    # expect 5/5 pass on the anchored regexp
```

## Bug tracking

GitHub Issues are the single source of truth for defects.
File before fixing; close with the commit reference.

See `CLAUDE.md` § Bug & Defect Tracking for the full rules.
