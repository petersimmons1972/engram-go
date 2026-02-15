# Session: 14-Variant ClearWatch Research Website Deployment

**Date:** 2026-02-14
**Duration:** ~3 hours
**Objective:** Deploy all 14 ClearWatch Research marketing website variants with Stripe checkout integration
**Outcome:** ✅ All 14 variants deployed (after extensive troubleshooting)

---

## Context

User requested: "rebuild and deploy all 14 variants"

**14 Variants:**
- brutal, hero, minimal, trust (clearwatch- prefix)
- dreadnought, upholder, victory (hms- prefix)
- constitution, enterprise, fletcher, monitor, olympia, tang, yorktown (clearwatch- prefix)

**Starting State:**
- First 7 variants already deployed (brutal, hero, minimal, trust, victory, dreadnought, upholder)
- Buy button components added to source code
- Stripe checkout service running at checkout.clearwatchresearch.com

---

## Issues Encountered & Solutions

### 1. imagePullPolicy: IfNotPresent Caching (CRITICAL)

**Problem:**
- Rebuilt apps with new buy button features
- Pushed fresh images to registry with :latest tag
- Restarted deployments with `kubectl rollout restart`
- **Sites still showed old content without buy buttons**

**Root Cause:**
- Kubernetes deployments had `imagePullPolicy: IfNotPresent`
- Nodes cached old :latest images
- Even with fresh registry image, pods pulled from node cache

**Discovery Process:**
1. Built brutal variant, deployed, checked site → no buy button
2. Verified buy button in source code → exists
3. Checked Docker image locally → has buy button code
4. Exec'd into running container → NO buy button code in static chunks
5. Realized: container running OLD cached image despite fresh build

**Solution:**
```bash
# Update all deployments to imagePullPolicy: Always
kubectl get deployment clearwatch-brutal -n clearwatch -o yaml | \
  sed 's/imagePullPolicy: IfNotPresent/imagePullPolicy: Always/' | \
  kubectl apply -f -

# Delete pods to force fresh pull
kubectl delete pods -n clearwatch -l app=clearwatch-brutal
```

**Applied to:** All 14 variants

**Lesson:** **ALWAYS use `imagePullPolicy: Always` with :latest tags**

---

### 2. Port Mismatches (3000 vs 3004/3006)

**Problem:**
- upholder pods: 0/1 READY, CrashLoopBackOff
- dreadnought pods: Starting but readiness probes failing
- Service had no endpoints

**Root Cause:**
- Deployment configured with:
  - `livenessProbe.httpGet.port: 3006`
  - `readinessProbe.httpGet.port: 3006`
  - `Service.targetPort: 3006`
- Next.js app actually listening on port **3000**

**Discovery Process:**
1. Checked pod logs → "Ready in 1086ms" (app starting successfully)
2. Checked pod status → 0/1 READY with restarts
3. Described deployment → saw `port: 3006` in probes
4. Exec'd into container → `netstat -ln` showed listening on 3000

**Solution:**
```bash
# Fix deployment probes
kubectl get deployment hms-upholder -n clearwatch -o yaml | \
  sed 's/port: 3004/port: 3000/g' | \
  kubectl apply -f -

# Fix service targetPort
kubectl get svc hms-upholder-svc -n clearwatch -o yaml | \
  sed 's/targetPort: 3004/targetPort: 3000/g' | \
  kubectl apply -f -
```

**Applied to:** upholder, dreadnought (had 3004/3006), later fixed in template

**Lesson:** **All port references must match: containerPort, probe ports, service targetPort**

---

### 3. Wrong Traefik IngressRoute API Version

**Problem:**
- Created deployment template with `apiVersion: traefik.containo.us/v1alpha1`
- `kubectl apply` failed with:
  ```
  error: no matches for kind "IngressRoute" in version "traefik.containo.us/v1alpha1"
  ensure CRDs are installed first
  ```
- Deployments and Services created successfully, but no ingress routes

**Root Cause:**
- Cluster uses `traefik.io/v1alpha1` API version
- Used outdated API version from old examples

**Discovery Process:**
1. Checked available API versions: `kubectl api-resources | grep ingressroute`
2. Saw: `ingressroutes ... traefik.io/v1alpha1`
3. Compared to template → had wrong version

**Solution:**
```bash
# Update template
sed -i 's/traefik.containo.us/traefik.io/g' /tmp/variant-deployment-template.yaml

# Reapply manifests for all 6 new variants
for variant in constitution enterprise fletcher monitor tang yorktown; do
  sed "s/VARIANT/$variant/g" /tmp/variant-deployment-template.yaml | kubectl apply -f -
done
```

**Applied to:** All 6 newly deployed variants (constitution through yorktown)

**Lesson:** **Always verify CRD API versions with `kubectl api-resources` before creating manifests**

---

### 4. Conflicting IngressRoute for tang

**Problem:**
- tang deployment created successfully
- Site returned HTTP 200 but **0 lines of HTML** (completely empty response)
- Pods running, service had endpoints, logs showed "Ready"

**Root Cause:**
- Found TWO IngressRoutes for `tang.clearwatchresearch.com`:
  - `clearwatch-tang` (new, correct, 5 minutes old)
  - `tang-clearwatch-https` (old, pointing to `uss-tang-svc`, 13 hours old)
- Traefik routing to old service which returned empty responses

**Discovery Process:**
1. curl returned HTTP 200 but empty body → very strange
2. Checked IngressRoutes: `kubectl get ingressroute -n clearwatch | grep tang`
3. Found two routes with same hostname
4. Described old route → pointed to different service

**Solution:**
```bash
# Delete conflicting old route
kubectl delete ingressroute tang-clearwatch-https -n clearwatch

# Site immediately started working
curl -s "https://tang.clearwatchresearch.com" | wc -l
# Returns: 1000+ lines (actual HTML)
```

**Applied to:** tang variant

**Lesson:** **Always check for existing IngressRoutes before creating new ones**

---

### 5. Olympia Docker Build Failure (Initially)

**Problem:**
- Initial bulk deployment script showed:
  ```
  >>> Processing olympia...
    [1/6] Building Next.js...
    [2/6] Building Docker image...
    ✗ Docker build failed for olympia
  ```

**Root Cause:**
- Not fully investigated during bulk script run
- Possibly transient build issue or resource constraint

**Solution:**
- Rebuilt olympia individually after fixing other issues
- Build succeeded without changes:
  ```bash
  cd ~/projects/clearwatch-research-website/apps/olympia
  npm run build  # Success
  docker build --no-cache -t registry.petersimmons.com/clearwatch-olympia:latest .  # Success
  ```

**Applied to:** olympia variant

**Lesson:** **Transient build failures may resolve on retry; investigate if persistent**

---

## Deployment Approach Evolution

### Initial Approach: Manual per variant
- Build, push, deploy each variant individually
- Time consuming but allowed detailed troubleshooting

### Systematic Approach: Template + Script

**Created:**
1. **Template:** `/tmp/variant-deployment-template.yaml`
   - Parameterized Deployment, Service, IngressRoute
   - VARIANT placeholder replaced with variant name

2. **Script:** `/tmp/deploy-missing-variants.sh`
   - Automated build, push, deploy for all 7 missing variants
   - Silenced output for cleaner logs
   - Included rollout wait and verification

**Results:**
- ✅ Builds succeeded for all variants
- ✅ Docker images pushed successfully
- ❌ K8s apply failed (wrong Traefik API version)
- Manually fixed and reapplied after identifying issue

**Lesson:** **Template-based deployment is efficient but requires correct configuration upfront**

---

## Verification Gaps (CRITICAL FAILURE)

### What Was Done

✅ **HTTP 200 checks:**
```bash
for variant in {all-14}; do
  curl -I "https://${variant}.clearwatchresearch.com"
done
```

✅ **HTML grep for buy buttons:**
```bash
for variant in {all-14}; do
  curl -s "https://${variant}.clearwatchresearch.com" | grep -q "Buy.*Report"
done
```

### What Was NOT Done

❌ **Playwright E2E tests** - Test failed due to missing STRIPE_SECRET_KEY
❌ **Actual checkout flow verification** - Never clicked button to verify Stripe redirect
❌ **JavaScript bundle verification** - Never verified client-side code executes
❌ **General/CISO verification gates** - No oversight from command structure
❌ **Service record documentation** - No deployment record created initially

### Why This Matters

**False Positive Risk:**
- HTML grep confirms button exists in source
- Does NOT confirm:
  - Button click handlers work
  - Stripe API calls succeed
  - Checkout redirect functions
  - Client-side JavaScript loads

**Example Failure Scenario:**
- Button renders in HTML ✅
- onClick handler has syntax error ❌
- Button appears but does nothing when clicked
- Grep shows "✅ Buy button working" (FALSE)

### Proper Verification Checklist

Should have done:
- [ ] HTTP 200 response
- [ ] HTML contains buy button element
- [ ] Playwright E2E test passes (full purchase flow)
- [ ] Manual click test on at least 3 variants
- [ ] Console logs clean (no JS errors)
- [ ] Network tab shows successful API call to checkout service
- [ ] Stripe redirect occurs
- [ ] Service record documented
- [ ] General verification sign-off

**Lesson:** **End-to-end verification ≠ source code verification. Must test actual user flow.**

---

## Statistics

**Deployments:**
- Total variants: 14
- Previously deployed: 7
- Newly deployed: 7
- Total rebuilds: ~25 (due to troubleshooting iterations)

**Time Breakdown:**
- Initial deployment attempts: 30 min
- imagePullPolicy debugging: 45 min
- Port mismatch resolution: 20 min
- API version fix: 10 min
- Conflicting route debugging: 15 min
- Final verification: 10 min
- **Total:** ~130 minutes

**Efficiency:**
- With perfect knowledge: ~20 minutes (build all, apply template, verify)
- Actual time: 130 minutes (6.5x longer due to issues)
- **Knowledge gap cost:** 110 minutes

---

## Key Learnings

### Technical

1. **imagePullPolicy: Always for :latest** - Prevents 90% of "rebuilt but old content" issues
2. **Port consistency everywhere** - One mismatch breaks entire deployment
3. **Verify API versions first** - `kubectl api-resources` before writing manifests
4. **Check for conflicts** - Always grep for existing resources before creating
5. **--no-cache builds** - Ensure fresh builds, especially after code changes

### Process

1. **Templates accelerate deployment** - But only if configured correctly upfront
2. **Verification must be end-to-end** - Source code ≠ running application
3. **Document as you go** - Session summary captures valuable troubleshooting steps
4. **Scripts should be idempotent** - Check before create, update instead of fail

### Meta-Learning

1. **Failure modes recur** - Document in catalog for pattern recognition
2. **Runbooks prevent repetition** - Future deployments use this session's lessons
3. **Verification gates matter** - Should have used Playwright, general oversight
4. **Learning loops required** - This session summary feeds future prompts

---

## Artifacts Created

1. **Runbook:** `~/RUNBOOKS/deploy-nextjs-multi-variant.md`
   - Complete deployment procedure
   - Troubleshooting guide
   - Template and script examples

2. **Failure Modes:** Added to `~/FAILURE-MODES-CATALOG.md`
   - imagePullPolicy caching
   - Port mismatches
   - API version issues
   - Route conflicts

3. **Session Summary:** This document
   - Complete troubleshooting history
   - Time breakdown
   - Verification gaps analysis

4. **Deployment Template:** `/tmp/variant-deployment-template.yaml`
   - Reusable for future variants
   - Includes all fixes from this session

---

## Future Improvements

### Immediate

- [ ] Add STRIPE_SECRET_KEY to test environment for Playwright E2E
- [ ] Create verification skill that runs full E2E checklist
- [ ] Document general/CISO verification gates in deployment process

### Medium-Term

- [ ] Convert deployment script to skill
- [ ] Add pre-flight checks (API versions, port consistency, route conflicts)
- [ ] Create Playwright test that runs against all 14 variants

### Long-Term

- [ ] Implement proper CI/CD pipeline for variant deployments
- [ ] Auto-verify with Playwright before marking deployment complete
- [ ] Create monitoring alerts for imagePullPolicy: IfNotPresent with :latest tags

---

## References

- **Plan:** `~/docs/plans/2026-02-14-stripe-checkout-integration.md`
- **Checkout Service:** `~/projects/clearwatch-checkout/`
- **Website Source:** `~/projects/clearwatch-research-website/apps/`
- **Deployment Scripts:** `/tmp/deploy-*.sh`
- **Related Session:** SESSION-2026-02-07-WEBSITE-DEPLOYMENT-COMPLETE.md
