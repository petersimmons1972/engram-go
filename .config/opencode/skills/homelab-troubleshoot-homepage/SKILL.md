---
name: homelab-troubleshoot-homepage
description: Use when troubleshooting Homepage-specific issues - the most problematic K8s service in the cluster. CRITICAL - Network policy label mismatch causes 90% of issues.
---

# Troubleshooting Homepage

**Homepage is the most problematic K8s app in this cluster.** 90% of issues are network policy label mismatches.

## Critical First Check (90% Fix Rate)

**ALWAYS check network policy labels first:**

```bash
# Check pod labels
kubectl get pods -l app.kubernetes.io/name=homepage -n default --show-labels

# Check network policy selectors
kubectl get networkpolicy -n default -o yaml | grep -A3 "podSelector:"
```

**Expected:** Labels MUST match exactly.
- ✅ Pod: `app.kubernetes.io/name=homepage`
- ✅ NetworkPolicy: `app.kubernetes.io/name: homepage`
- ❌ **WRONG:** `app: homepage` (causes all failures)

**Fix label mismatch:**
```bash
# Edit network policies file
vim /home/psimmons/projects/homepage-network-policies.yaml
# Change: app: homepage -> app.kubernetes.io/name: homepage
kubectl apply -f /home/psimmons/projects/homepage-network-policies.yaml
```

## Diagnostic Workflow

**Run in order, stop when problem found:**

### 1. Pod Status
```bash
kubectl get pods -l app.kubernetes.io/name=homepage -n default -o wide
kubectl logs -n default -l app.kubernetes.io/name=homepage --tail=50 | grep -i error
```

### 2. HTTP vs Content Check
```bash
# HTTP status (NOT sufficient alone!)
curl -k -s -o /dev/null -w "%{http_code}" https://homepage.petersimmons.com

# Check for errors in HTML (the real test)
curl -k -s https://homepage.petersimmons.com > /tmp/homepage-test.html
grep -i "api error\|something went wrong" /tmp/homepage-test.html
```

**Key Insight:** HTTP 200 does NOT mean Homepage is working. Must check HTML content.

### 3. Service Endpoints
```bash
kubectl get endpoints homepage -n default
# Must have active endpoints
```

## Top 3 Common Issues

### Issue 1: Network Policy Labels (90%)
**Symptom:** 502 Bad Gateway, pod running but unreachable
**Fix:** See "Critical First Check" above

### Issue 2: Search Widget Error
**Symptom:** Search bar shows "Something went wrong"
**Fix:** Use `provider: custom` not `provider: searxng`
```yaml
# CORRECT:
- search:
    provider: custom
    url: https://searxng.petersimmons.com/search?q=
```

### Issue 3: Widgets Show "API Error"
**Symptom:** Page loads, widgets display errors
**Causes:**
- Widget config removed from ConfigMap
- Backend service unreachable (Proxmox, Pi-hole)
- Network policy blocking widget API

**Fix:** Check widget configuration inline with services:
```yaml
- Proxmox:
    href: https://pve.petersimmons.com:8006
    widget:
      type: proxmox
      url: https://pve.petersimmons.com:8006
      username: root@pam!homepage
      password: YOUR_API_TOKEN
```

## Emergency Restoration (1 Minute)

```bash
# Restore known-good config
kubectl apply -f /home/psimmons/projects/kubernetes/homepage/configmap-updated.yaml
kubectl rollout restart deployment/homepage -n default
sleep 30

# Verify
curl -k -s https://homepage.petersimmons.com | grep -i "api error" || echo "OK"
# Browser: Ctrl+Shift+R (hard refresh)
```

## Verification Checklist

**ALL must pass:**
```bash
# Pod running
kubectl get pods -l app.kubernetes.io/name=homepage | grep "1/1.*Running"

# HTTP 200
curl -k -s -o /dev/null -w "%{http_code}" https://homepage.petersimmons.com  # = 200

# No errors in HTML
curl -k -s https://homepage.petersimmons.com | grep -i "api error\|something went wrong"  # Empty

# Visual in browser (Ctrl+Shift+R):
# - Services visible, widgets show data, icons load, search works
```

## Quick Fixes Reference

| Symptom | Most Likely Cause | Quick Fix |
|---------|------------------|-----------|
| 502 Gateway | Network policy labels | Check labels match (see Critical First Check) |
| HTTP 200 but broken widgets | HTML errors, not HTTP | Check HTML content with grep |
| "Something went wrong" in search | Wrong provider | Use `provider: custom` |
| Icons broken | URL-based icons fail | Use icon libraries: `mdi-*`, `si-*` |
| Demo content ("My First Group") | Browser cache | Hard refresh: Ctrl+Shift+R |

## Anti-Patterns

**Don't:**
- Trust HTTP 200 alone - check HTML content
- Restart pod before checking logs
- Use `:latest` tags
- Configure widgets in separate files

**Do:**
- Check network policy labels first (90% fix rate)
- Verify HTML content for errors
- Pin image versions
- Configure widgets inline with services

## Reference Files

- Instructions: `/home/psimmons/projects/custom-homepage/HOMEPAGE-INSTRUCTIONS.md`
- Golden Config: `/home/psimmons/projects/kubernetes/homepage/GOLDEN-CONFIG-BASELINE.md`
- Live Config: `/home/psimmons/projects/kubernetes/homepage/configmap-updated.yaml`
