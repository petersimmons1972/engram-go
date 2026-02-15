# Cert-Manager Patterns (Homelab)

## Core Concept

Kubernetes overlay network (10.42.x.x/16) differs from homelab external IPs (192.168.x.x). This causes silent failures with Cloudflare API token IP filtering.

## Cloudflare API Token Setup

- Token name: "Kubernetes - cert-manager"
- Permissions required: Zone:Zone:Read, Zone:DNS:Read, Zone:DNS:Edit
- Resources: petersimmons.com
- IP filtering: AVOID or whitelist `10.42.0.0/16` (K8s overlay network)
  - Without this, DNS-01 ACME challenges silently fail
  - Challenge shows "Presented: true" but DNS records never appear
  - No error in cert-manager logs because Cloudflare returns HTTP 200 at network level
- DNS01 Configuration: Uses Cloudflare API with fixed nameservers (1.1.1.1, 1.0.0.1)

## Troubleshooting

Certificate stuck in "IncorrectIssuer" or "Ready=False" for >7 days:
```bash
kubectl describe certificate <name> -n <namespace>
kubectl describe challenge <name> -n <namespace>
nslookup _acme-challenge.<domain> 1.1.1.1
```

Challenge shows "Presented: true" but DNS record missing:
1. API call reached Cloudflare but DNS record wasn't created
2. Check Cloudflare token IP filtering (is cert-manager pod IP whitelisted?)
3. Check Cloudflare zone is active and nameservers correct
4. Fix: Remove IP filtering or add 10.42.0.0/16 to whitelist

cert-manager pod network:
- Pod IP: 10.42.x.x (Kubernetes ClusterIP range)
- API calls originate from this range, NOT from homelab IPs

## Split-Horizon DNS Conflict

Local DNS CNAME records (domain.com -> traefik.petersimmons.com) break DNS-01 challenges.
cert-manager pod uses cluster DNS -> forwards to local DNS -> sees CNAME instead of TXT records.

Fix: Force cert-manager to use Cloudflare public DNS:
```bash
kubectl patch deployment cert-manager -n cert-manager --type='json' -p='[
  {"op": "add", "path": "/spec/template/spec/dnsPolicy", "value": "None"},
  {"op": "add", "path": "/spec/template/spec/dnsConfig", "value": {
    "nameservers": ["1.1.1.1", "1.0.0.1"],
    "searches": ["cert-manager.svc.cluster.local", "svc.cluster.local", "cluster.local"],
    "options": [{"name": "ndots", "value": "5"}]
  }}
]'
```

Verify: `kubectl get pod -n cert-manager -l app.kubernetes.io/name=cert-manager -o jsonpath='{.items[0].spec.dnsConfig.nameservers}'`

## Cloudflare DNS Cache Gotcha

Cloudflare caches NXDOMAIN responses for up to 1800 seconds (30 minutes). CDN cache purge does NOT clear DNS negative cache. If cert-manager fails initially, subsequent retries hit cached NXDOMAIN.

Diagnosis: Create test record with different name. If it resolves, negative cache is the issue. Wait for cache expiry.

## Registrar-Level DNS Bypass

When Cloudflare zone records don't resolve at all, check TLD-level NS delegation first:
```bash
dig @a.gtld-servers.net <domain>.com NS
```
If TLD returns CNAME instead of NS records, registrar-level configuration is bypassing Cloudflare entirely. Fix at registrar, not Cloudflare.

## Verification

- `health-check.sh` checks for stuck certificates and pending challenges
- Certs renew automatically ~30 days before expiry
- Monitor: `kubectl get certificate -A` should show all Ready=True

## Incidents

- 2026-01-24: DNS CNAME misconfiguration caused 58-day cert failure. Fixed with static DNS servers.
- 2026-01-25: Cloudflare IP filtering blocked cert-manager pod (10.42.2.247 outside 192.168.x.x whitelist). Silent failure.
