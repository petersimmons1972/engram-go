---
name: next-js-troubleshooting
description: Use when troubleshooting Next.js deployments, especially Next.js 15+ breaking changes - async params, dynamic routes, standalone builds, static generation issues
---

# Next.js Troubleshooting

Common issues and solutions for Next.js deployments, with emphasis on Next.js 15+ breaking changes.

## When to Use

- Deploying Next.js applications (especially 15+)
- Dynamic routes returning 404
- Build succeeds but pages don't work
- Docker standalone builds missing data
- Static generation failures

## Critical Breaking Changes (Next.js 15+)

### Issue 1: Async Params (BREAKING CHANGE)

**Symptom:** All dynamic routes return "Not Found" despite pages building successfully.

**Cause:** Next.js 15+ changed `params` from synchronous object to Promise.

**Before (Next.js 14 and earlier):**
```typescript
export default function Page({ params }: { params: { vendor: string } }) {
  return <div>{params.vendor}</div>
}
```

**After (Next.js 15+):**
```typescript
export default async function Page({
  params
}: {
  params: Promise<{ vendor: string }>
}) {
  const { vendor } = await params
  return <div>{vendor}</div>
}
```

**Files to update:**
- All dynamic route pages: `app/[param]/page.tsx`
- All `generateMetadata()` functions
- All `generateStaticParams()` functions

**Fix:**
```typescript
// Update page components
export default async function Page({ params }: { params: Promise<{ id: string }> }) {
  const { id } = await params
  // ...
}

// Update generateMetadata
export async function generateMetadata({ params }: { params: Promise<{ id: string }> }) {
  const { id } = await params
  // ...
}

// Update generateStaticParams (if used)
export async function generateStaticParams() {
  const params = await params  // If params passed in
  // ...
}
```

### Issue 2: Dynamic Route Naming Limitations

**Symptom:** Routes with literal text between dynamic segments fail to generate. HTML files are 0 bytes (NoFallbackError).

**Problem Pattern:**
```typescript
// ❌ DOES NOT WORK in Next.js 15+
app/compare/[v1]-vs-[v2]/page.tsx
// URL: /compare/crowdstrike-vs-sentinelone
```

**Solution: Use single dynamic segment + parsing:**
```typescript
// ✅ WORKS
app/compare/[slug]/page.tsx
// URL: /compare/crowdstrike-vs-sentinelone

// In component, parse the slug:
export default async function Page({ params }: { params: Promise<{ slug: string }> }) {
  const { slug } = await params
  const [v1, v2] = slug.split('-vs-')
  // ...
}
```

**generateStaticParams for this pattern:**
```typescript
export async function generateStaticParams() {
  const comparisons = [
    { vendor1: 'crowdstrike', vendor2: 'sentinelone' },
    { vendor1: 'paloalto', vendor2: 'fortinet' },
  ]

  return comparisons.map(({ vendor1, vendor2 }) => ({
    slug: `${vendor1}-vs-${vendor2}`
  }))
}
```

### Issue 3: Data Loading in Standalone Builds

**Symptom:** JSON data files not found in Docker standalone builds. App works locally but fails in container.

**Cause:** Standalone output doesn't automatically bundle non-imported JSON files.

**Solution: Explicitly copy data files in Dockerfile:**
```dockerfile
# After standalone build
FROM node:20-alpine AS runner
WORKDIR /app

# Copy standalone output
COPY --from=builder /app/.next/standalone ./
COPY --from=builder /app/.next/static ./.next/static

# ✅ CRITICAL: Copy data files explicitly
COPY --from=builder /app/src ./src
COPY --from=builder /app/public ./public

# If data is in .next/server
COPY --from=builder /app/.next/server ./.next/server

USER nextjs
EXPOSE 3000
CMD ["node", "server.js"]
```

**Alternative: Import JSON in code (bundled automatically):**
```typescript
// ✅ Gets bundled in standalone
import data from '@/data/vendors.json'

// ❌ Won't be bundled
const data = fs.readFileSync('./data/vendors.json')
```

## Common Issues and Solutions

### Issue 4: Static Generation Failures

**Symptom:** `next build` succeeds but generates empty HTML files (0 bytes).

**Common Causes:**
1. Missing `generateStaticParams()` for dynamic routes
2. Data fetching errors during build (fail silently)
3. Invalid return values from `generateStaticParams()`

**Debug:**
```bash
# Build with verbose logging
next build --debug

# Check generated files
find .next/server/app -name "*.html" -size 0

# Check build output for errors
next build 2>&1 | grep -i error
```

**Fix:**
```typescript
// Ensure generateStaticParams returns valid array
export async function generateStaticParams() {
  const items = await fetchItems()

  // ✅ Return array of param objects
  return items.map(item => ({
    id: item.id
  }))

  // ❌ DON'T return undefined or invalid structure
}

// Handle errors gracefully
export default async function Page({ params }: { params: Promise<{ id: string }> }) {
  const { id } = await params

  try {
    const data = await fetchData(id)
    return <div>{data.name}</div>
  } catch (error) {
    // Return valid HTML even on error
    return <div>Not found</div>
  }
}
```

### Issue 5: Environment Variables in Docker

**Symptom:** `process.env.NEXT_PUBLIC_*` variables undefined in production.

**Cause:** Build-time vs runtime environment variable handling.

**Solution:**
```dockerfile
# Build-time variables (baked into bundle)
ARG NEXT_PUBLIC_API_URL
ENV NEXT_PUBLIC_API_URL=$NEXT_PUBLIC_API_URL

# Build
RUN npm run build

# Runtime variables (server-side only)
ENV API_SECRET=your-secret
```

**In next.config.js:**
```javascript
module.exports = {
  env: {
    // Available at build time and runtime
    CUSTOM_VAR: process.env.CUSTOM_VAR,
  },
  // For server-side only
  serverRuntimeConfig: {
    apiSecret: process.env.API_SECRET,
  },
}
```

### Issue 6: Image Optimization in Docker

**Symptom:** Images fail to load or return 500 errors.

**Cause:** Sharp dependency issues in Alpine containers.

**Solution:**
```dockerfile
# Install Sharp dependencies
FROM node:20-alpine
RUN apk add --no-cache \
    libc6-compat \
    vips-dev \
    fftw-dev \
    build-base \
    python3

# Or use standalone with built-in Sharp
COPY --from=builder /app/.next/standalone ./
```

**In next.config.js:**
```javascript
module.exports = {
  images: {
    loader: 'default',  // or 'custom'
    unoptimized: false,  // Set true to disable optimization if needed
  },
}
```

## Quick Diagnostic Checklist

**Build Issues:**
```bash
# Check Next.js version
npm list next

# Verbose build
next build --debug

# Check for 0-byte HTML files
find .next/server/app -name "*.html" -size 0

# Test specific route generation
curl http://localhost:3000/your-route
```

**Dynamic Route Issues:**
```bash
# Verify generateStaticParams output
# Add console.log in function, run build, check output

# Test route locally first
npm run dev
curl http://localhost:3000/your-dynamic-route
```

**Docker Issues:**
```bash
# Verify standalone output
ls -la .next/standalone

# Check data files copied
docker run -it your-image ls -la /app/src

# Test inside container
docker run -it your-image sh
node server.js &
curl http://localhost:3000
```

## Migration Checklist (Next.js 14 → 15+)

- [ ] Update all dynamic route page components to async with Promise params
- [ ] Update all `generateMetadata()` to await params
- [ ] Update all `generateStaticParams()` if they receive params
- [ ] Test all dynamic routes return content (not 404)
- [ ] Verify HTML files not 0 bytes after build
- [ ] Check Docker build includes data files
- [ ] Test production build locally before deploying

## Verification

**After fixing issues:**
```bash
# Clean build
rm -rf .next
next build

# Check for errors
next build 2>&1 | grep -i error | wc -l  # Should be 0

# Check HTML files generated
find .next/server/app -name "*.html" -size 0 | wc -l  # Should be 0

# Test dynamic routes
npm start
curl http://localhost:3000/your-dynamic-route  # Should return HTML, not 404

# Docker test
docker build -t test .
docker run -p 3000:3000 test
curl http://localhost:3000  # Should work
```

## Related

- **homelab-k8s-deployment**: Deploying Next.js to K8s after fixing issues
- **superpowers-verification-before-completion**: Before claiming deployment works
