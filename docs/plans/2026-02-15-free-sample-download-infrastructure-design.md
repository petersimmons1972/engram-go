# Free Sample Download Infrastructure Design

**Date:** 2026-02-15
**Status:** Approved
**Goal:** Add free sample download to all 14 ClearWatch Research marketing variants to remove PRIMARY conversion blocker identified by Gordon Ramsay and CISO Validator

---

## Context

### Problem
Gordon Ramsay and CISO Validator both identified lack of risk reduction as the PRIMARY conversion blocker across all ClearWatch Research marketing variants:

- **Gordon**: "No sample or guarantee = asking for Michelin-level trust with zero proof"
- **CISO**: "Only Enterprise has a free sample. The other 13 ask for $495 sight-unseen from an unknown brand. That's a non-starter for first-time buyers."

### Current State
- 14 marketing variants (Fletcher deleted)
- Enterprise previously had free sample but lost it when buy button was added
- All variants currently ask for $495 payment with no risk reduction
- Estimated conversion impact: **20-30% uplift** with free sample

### Design Decisions

| Decision | Choice | Rationale |
|----------|--------|-----------|
| Analytics | Simple static download | Fast implementation, no API needed |
| PDF Strategy | Single shared PDF | Consistent experience, easier maintenance |
| Placement | Both hero + final CTA | Maximum visibility, multiple touchpoints |
| Styling | Match buy button style | Equal prominence removes decision anxiety |
| Implementation | Shared component in packages/ui | Proper architecture, reusable, maintainable |
| Import Strategy | Fix package imports (pnpm workspace) | Clean imports, no component copying |
| PDF Hosting | Dedicated assets.clearwatchresearch.com | Clean separation, scalable, professional |

---

## Architecture Overview

### Component Architecture

```
packages/ui/src/
├── BuyReportButton.tsx (existing)
├── DownloadSampleButton.tsx (new)
└── index.ts (export both)

apps/[variant]/
├── app/page.tsx (imports DownloadSampleButton)
├── components/ (local copies removed after import fix)
└── package.json (updated to import @clearwatch/ui)

k8s/
├── assets-deployment.yaml (new nginx pod)
├── assets-service.yaml (new service)
└── assets-ingressroute.yaml (new Traefik route)
```

### Flow

1. User visits any variant (e.g., `monitor.clearwatchresearch.com`)
2. Hero section shows: "Buy Report - $495" + "Download Free Sample"
3. Clicking "Download Free Sample" → `https://assets.clearwatchresearch.com/sample-report.pdf`
4. Final CTA section repeats both buttons
5. Same PDF served from single nginx pod for all 14 variants

### Infrastructure

- **New K8s deployment**: `assets` (nginx:alpine, 1 replica, minimal resources)
- **ConfigMap** with PDF file (or PersistentVolume if PDF > 1MB)
- **Service** + **IngressRoute** with TLS
- All 14 variants link to same endpoint

---

## Component Implementation

### DownloadSampleButton.tsx

**Location:** `packages/ui/src/DownloadSampleButton.tsx`

```typescript
'use client';

interface DownloadSampleButtonProps {
  variant: string;
  className?: string;
  children?: React.ReactNode;
}

export default function DownloadSampleButton({
  variant,
  className = '',
  children,
}: DownloadSampleButtonProps) {
  return (
    <a
      href="https://assets.clearwatchresearch.com/sample-report.pdf"
      target="_blank"
      rel="noopener noreferrer"
      className={className}
      download="clearwatch-research-sample-report.pdf"
    >
      {children || 'Download Free Sample Report'}
    </a>
  );
}
```

### Key Decisions

- **Simple anchor tag** - No fetch, no loading states, just direct download
- **target="_blank"** - Opens in new tab (doesn't lose place on marketing page)
- **download attribute** - Suggests filename to browser
- **variant prop** - Included for future analytics (not used yet, but ready)
- **Same prop structure as BuyReportButton** - Consistency

### Usage in Variants

```typescript
import { DownloadSampleButton } from '@clearwatch/ui';

// Hero section
<DownloadSampleButton
  variant="monitor"
  className="monitor-button"
/>

// Final CTA
<DownloadSampleButton
  variant="monitor"
  className="monitor-button-secondary"
/>
```

---

## Package Import Configuration

### Problem

Variants currently copy components due to build config issues. This creates maintenance burden and inconsistency.

### Solution

**packages/ui/package.json:**
```json
{
  "name": "@clearwatch/ui",
  "version": "1.0.0",
  "main": "./src/index.ts",
  "types": "./src/index.ts",
  "exports": {
    ".": "./src/index.ts"
  }
}
```

**packages/ui/src/index.ts:**
```typescript
export { default as BuyReportButton } from './BuyReportButton';
export { default as DownloadSampleButton } from './DownloadSampleButton';
```

**Each variant's package.json** (add dependency):
```json
{
  "dependencies": {
    "@clearwatch/ui": "workspace:*"
  }
}
```

**Root-level pnpm-workspace.yaml:**
```yaml
packages:
  - 'apps/*'
  - 'packages/*'
```

### Migration Path

1. Set up workspace config
2. Run `pnpm install` to link packages
3. Update imports in all variants
4. Remove copied component files
5. Test build on one variant, then deploy all

---

## Kubernetes Assets Service

### Deployment

**k8s/assets-deployment.yaml:**
```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: assets
  namespace: clearwatch
spec:
  replicas: 1
  selector:
    matchLabels:
      app: assets
  template:
    metadata:
      labels:
        app: assets
    spec:
      containers:
      - name: nginx
        image: nginx:alpine
        ports:
        - containerPort: 80
        volumeMounts:
        - name: assets-volume
          mountPath: /usr/share/nginx/html
        resources:
          requests:
            memory: "32Mi"
            cpu: "50m"
          limits:
            memory: "64Mi"
            cpu: "100m"
      volumes:
      - name: assets-volume
        configMap:
          name: assets-files
---
apiVersion: v1
kind: ConfigMap
metadata:
  name: assets-files
  namespace: clearwatch
binaryData:
  sample-report.pdf: <base64-encoded-pdf>
```

**Note:** ConfigMap has 1MB size limit. If PDF exceeds 1MB, use PersistentVolume instead.

### Service

**k8s/assets-service.yaml:**
```yaml
apiVersion: v1
kind: Service
metadata:
  name: assets-svc
  namespace: clearwatch
spec:
  selector:
    app: assets
  ports:
  - port: 80
    targetPort: 80
```

### IngressRoute

**k8s/assets-ingressroute.yaml:**
```yaml
apiVersion: traefik.io/v1alpha1
kind: IngressRoute
metadata:
  name: assets-https
  namespace: clearwatch
spec:
  entryPoints:
    - websecure
  routes:
    - kind: Rule
      match: Host(`assets.clearwatchresearch.com`)
      services:
        - name: assets-svc
          port: 80
  tls:
    secretName: clearwatchresearch-tls
```

---

## Deployment Strategy

### Phase 1: Infrastructure Setup

1. **Create sample PDF** (8-page comparison report showing methodology)
2. **Deploy assets service to K8s**:
   - Create ConfigMap with base64-encoded PDF
   - Apply deployment, service, IngressRoute
3. **Add DNS record**: `assets.clearwatchresearch.com` → Traefik ingress
4. **Verify accessibility**:
   ```bash
   curl -I https://assets.clearwatchresearch.com/sample-report.pdf
   # Should return: HTTP/2 200
   ```

### Phase 2: Package Setup

1. Create `pnpm-workspace.yaml` at repository root
2. Create `DownloadSampleButton.tsx` in `packages/ui/src/`
3. Export from `packages/ui/src/index.ts`
4. Update `packages/ui/package.json` with proper exports
5. Run `pnpm install` to link workspace packages
6. Verify package resolution: `pnpm why @clearwatch/ui`

### Phase 3: Variant Updates (Incremental Deployment)

**Step 1: Test with ONE variant (Monitor)**
1. Update `apps/monitor/package.json`:
   ```json
   {
     "dependencies": {
       "@clearwatch/ui": "workspace:*"
     }
   }
   ```
2. Update `apps/monitor/app/page.tsx`:
   - Import: `import { DownloadSampleButton } from '@clearwatch/ui';`
   - Add to hero section
   - Add to final CTA section
3. Remove `apps/monitor/components/BuyReportButton.tsx` (use shared version)
4. Build and test locally:
   ```bash
   cd apps/monitor
   npm run build
   npm run dev
   ```
5. Deploy to K8s
6. Verify both download buttons work

**Step 2: Deploy to remaining 13 variants**

Once Monitor verified working:
- Apply same changes to all 13 remaining variants
- Build all in parallel
- Deploy all to K8s
- Run verification checklist on each

### Testing Checklist (Per Variant)

- [ ] Hero download button renders with correct styling
- [ ] Hero download button matches buy button visual weight
- [ ] Final CTA download button renders with correct styling
- [ ] Final CTA download button matches buy button visual weight
- [ ] Both buttons link to `assets.clearwatchresearch.com/sample-report.pdf`
- [ ] PDF downloads successfully (opens in new tab)
- [ ] Suggested filename is `clearwatch-research-sample-report.pdf`
- [ ] Buy button still works (no regressions)
- [ ] Mobile responsive (buttons stack properly)
- [ ] Page layout not broken

### Rollback Plan

**If assets service fails:**
- Delete K8s resources (deployment, service, IngressRoute)
- Variants will show broken download links but buy flow unaffected

**If package imports break:**
- Revert variant to previous build
- Assets service remains operational
- Can fix imports without time pressure

**If PDF needs updating:**
- Update ConfigMap with new base64-encoded PDF
- Restart assets deployment: `kubectl rollout restart deployment/assets -n clearwatch`
- No variant changes needed

---

## Success Metrics

**Immediate (Day 1):**
- [ ] assets.clearwatchresearch.com returns 200 OK
- [ ] PDF downloads successfully from all 14 variants
- [ ] No buy button regressions

**Short-term (Week 1):**
- [ ] Monitor download click-through rate
- [ ] Measure conversion rate change (baseline vs post-launch)

**Target Impact:**
- **20-30% conversion uplift** (per Gordon/CISO analysis)
- Remove PRIMARY conversion blocker across entire fleet

---

## Future Enhancements (Not in Scope)

1. **Analytics tracking** - Add UTM parameters or event tracking to measure downloads per variant
2. **Variant-specific samples** - Create customized samples matching each variant's positioning
3. **Email capture** - Require email before download (builds lead list)
4. **A/B testing** - Test different sample lengths (4-page vs 8-page vs 12-page)
5. **Progressive enhancement** - Add loading states, error handling if moving to API-based download

---

## Files Changed

**New files:**
- `pnpm-workspace.yaml` (root)
- `packages/ui/src/DownloadSampleButton.tsx`
- `k8s/assets-deployment.yaml`
- `k8s/assets-service.yaml`
- `k8s/assets-ingressroute.yaml`

**Modified files:**
- `packages/ui/src/index.ts` (add export)
- `packages/ui/package.json` (add exports config)
- `apps/[all 14 variants]/package.json` (add @clearwatch/ui dependency)
- `apps/[all 14 variants]/app/page.tsx` (add DownloadSampleButton to hero + CTA)

**Deleted files:**
- `apps/[all 14 variants]/components/BuyReportButton.tsx` (use shared version instead)

---

## Appendix: Affected Variants

1. brutal
2. constitution
3. dreadnought
4. enterprise (re-add free sample that was lost)
5. hero
6. minimal
7. monitor
8. olympia
9. tang
10. trust
11. upholder
12. victory
13. yorktown
14. ~~fletcher~~ (deleted - Task #5)

**Total:** 13 active variants + 1 deleted = 14 originally evaluated
