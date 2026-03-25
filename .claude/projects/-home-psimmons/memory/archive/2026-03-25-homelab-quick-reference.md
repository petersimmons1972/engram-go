---
Category: reference
---
# Homelab Quick Reference

## Triage Decision Tree

```
Service Down?
├─ Multiple? → Traefik, K8s, Proxmox (use homelab:trace-dependencies)
├─ Single K8s? → homelab:troubleshoot-common-issues
├─ Homepage? → homelab:troubleshoot-homepage
├─ Nextcloud? → homelab:troubleshoot-nextcloud
├─ Hardware? → homelab:troubleshoot-hardware
└─ Then: superpowers:systematic-debugging
```

## Top Known-Good Fixes (By Success Rate)

| Command | Success | MTTR | Service | Use When |
|---------|---------|------|---------|----------|
| `~/bin/mouse.sh` | 100% | 30s | Hardware | Mouse unresponsive after idle |
| `kubectl apply -f networkpolicy.yaml` | 100% | 15s | Homepage | Network policy mismatch |
| `kubectl rollout restart deployment/homepage` | 85% | 2m | Homepage | Pod issues after config change |

Full list: `~/.homelab/knowledge/fix-effectiveness.yaml`

## Warning Patterns

**Storage Pressure** (Tier 1 - Critical):
- Check: `ssh psimmons@192.168.0.100 'pvesm status' | grep zp3`
- Warning: >85% usage
- Impact: VM freeze within 48h (95% confidence)
- Action: Run cleanup immediately

**Homepage Restarts** (Tier 2 - Major):
- Check: `kubectl get pods -l app.kubernetes.io/name=homepage -o jsonpath='{.items[*].status.containerStatuses[*].restartCount}'`
- Warning: >3 restarts/hour
- Impact: CrashLoopBackOff imminent
- Action: Check network policy labels

**Certificate Renewal** (Tier 2 - Major):
- Check: `kubectl get certificate -A`
- Warning: Renewal stuck >7 days
- Impact: Service outage before expiry
- Action: Check cert-manager logs, verify Cloudflare API token

## Critical Commands

- Health: `~/bin/health-check.sh`
- Logs: [LOG-COMMANDS-REFERENCE.md](/home/psimmons/LOG-COMMANDS-REFERENCE.md)
- K8s Nodes: 192.168.0.131-139
- Proxmox: pve.petersimmons.com:100
- DNS: 192.168.0.231, .232

## DNS Architecture

| Scope | Tool | When |
|-------|------|------|
| Public subdomains (external access) | Cloudflare | `*.petersimmons.com` → public IPs or Argo Tunnel CNAMEs |
| Internal/local subdomains | **Unifi** (local DNS) | Cluster services, Traefik ingress, homelab hosts |
| cert-manager DNS-01 challenges | Cloudflare API | Automatic via cert-manager Cloudflare token |

**Key rule:** Traefik and cluster-internal service DNS entries go in **Unifi**, not Cloudflare. Cloudflare only handles external-facing records.

## Known Anti-Patterns

1. **Don't restart services before checking logs** - Treats symptom, not cause
2. **Don't use :latest tags in production** - Silent breaking changes
3. **Don't delete pods as first troubleshooting step** - Masks root cause
4. **Don't trust documentation over live state** - Docs represent point in time
5. **Don't assign static IPs in DHCP range** (192.168.0.2-.98) - IP conflicts

Full list: `~/.homelab/config/anti-patterns.yaml`
