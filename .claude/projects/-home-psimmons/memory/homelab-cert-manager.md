---
Category: reference
---
# Cert-Manager Patterns (Homelab)

**Core issue**: K8s overlay (10.42.x.x) ≠ homelab IPs (192.168.x.x). Cloudflare API token IP filtering silently blocks cert-manager pods.

## Key Rules
- Cloudflare token needs: Zone:Read, DNS:Read, DNS:Edit for petersimmons.com
- IP filtering: avoid, or whitelist `10.42.0.0/16` (K8s overlay). Without this, challenges show "Presented: true" but DNS records never appear
- Local DNS CNAMEs break DNS-01 challenges. Fix: `dnsPolicy: None` + nameservers `[1.1.1.1, 1.0.0.1]` on cert-manager deployment
- Cloudflare caches NXDOMAIN 1800s — CDN purge won't help, wait or use different record name
- If zone records don't resolve at all: `dig @a.gtld-servers.net <domain> NS` — check TLD NS delegation before debugging Cloudflare

## Quick Troubleshooting
```bash
kubectl describe certificate <name> -n <ns>    # Check Ready status
kubectl describe challenge <name> -n <ns>       # Check Presented/reason
nslookup _acme-challenge.<domain> 1.1.1.1      # Verify DNS record exists
kubectl get certificate -A                       # All should show Ready=True
```
