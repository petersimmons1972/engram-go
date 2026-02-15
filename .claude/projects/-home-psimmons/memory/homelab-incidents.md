# Homelab Incident Summaries

Quick reference for past incidents. Full structured data in `~/.homelab/knowledge/failure-history.yaml`.

## Homepage Network Policy (2025-12-20)

Network policy label mismatch caused CrashLoopBackOff. Policy had `app=homepage`, deployment had `app.kubernetes.io/name=homepage`. Fixed by aligning labels. MTTR: 10 minutes. Pod delete didn't help (treats symptom not cause).

See: homelab-k8s-patterns.md "Network Policy Labels"

## Cert-Manager DNS CNAME Renewal Failure (2026-01-24)

Three certificates stuck Ready=False for 11-58 days. Local DNS CNAME records conflicted with DNS-01 ACME challenges. Fixed by patching cert-manager pod to use Cloudflare DNS (1.1.1.1) directly instead of cluster DNS.

See: homelab-cert-manager.md "Split-Horizon DNS Conflict"

## Cert-Manager Cloudflare IP Filtering (2026-01-25)

Challenge showed "Presented: true" but TXT records never appeared in DNS. Cloudflare API token IP filter silently rejected requests from cert-manager pod (10.42.x.x) because whitelist only included 192.168.x.x. No error in logs.

See: homelab-cert-manager.md "Cloudflare API Token Setup"

## Gmail Tracker Restart Loop (2026-01-25)

850 restarts in 8 days (~100/day). Deployment with `while true; sleep 43200` pattern killed by liveness probe every 15 minutes. Exit 137 with only 3Mi memory used (not OOM). Should be CronJob, not Deployment.

See: homelab-k8s-patterns.md "CronJob, Not Deployment, for Periodic Tasks"

## WordPress MCP Config Wrong File (2026-01-25)

REPEATED MISTAKE (twice same day). Applied MCP config to `~/.claude/mcp_servers.json` instead of `~/.claude.json`. Claude Code reads from `.claude.json`. Fix: Use `claude mcp add` CLI command. Learning system failed to prevent recurrence.

## Resume Website Deployment (2026-01-28)

Multiple issues during first deployment to websites namespace: Chainguard nginx needed writable temp dirs, Traefik IngressRoute case sensitivity, Cloudflare tunnel dashboard origin override, distroless image has no shell for exec probes. All resolved. 45 minutes.

See: homelab-k8s-patterns.md "Chainguard Images + PVC Permissions", "Cloudflare Tunnel on K8s"
