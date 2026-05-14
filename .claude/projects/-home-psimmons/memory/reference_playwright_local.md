---
name: reference-playwright-local
description: Playwright CLI available locally via clearwatch-checkout node_modules — use for live URL testing when SearXNG fails
metadata: 
  node_type: memory
  type: reference
  originSessionId: 61abe33a-c811-4282-b7de-9323185f1b1b
---

Playwright headless Chromium is available for live URL testing:

```bash
NODE_PATH=/home/psimmons/projects/clearwatch-checkout/node_modules node /tmp/your-script.js
```

- Binary: `/home/psimmons/.nvm/versions/node/v24.11.0/bin/playwright` (version 1.58.2)
- Chrome for Testing: 145.0.7632.6
- node_modules path: `/home/psimmons/projects/clearwatch-checkout/node_modules`

**When to use:** When `mcp__searxng__web_url_read` returns 429 (bot protection) or when you need JavaScript rendering. Confirmed to bypass Vercel 429 that blocks plain HTTP fetchers (tested against playgram.ai, 2026-05-12).

**Script pattern:**
```javascript
const { chromium } = require('playwright');
// userAgent: 'Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36...'
// waitUntil: 'domcontentloaded', timeout: 15000
```
