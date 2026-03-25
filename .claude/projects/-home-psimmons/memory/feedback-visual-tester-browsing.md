---
name: Visual-tester as browser fallback
description: Use the visual-tester k8s pod (Playwright/Chromium) to browse websites when WebFetch or web search fails due to bot detection or 403s
type: feedback
Category: feedback
---

When WebFetch or web search tools fail to load a website (403, bot detection, blocked), use the visual-tester k8s pod as a fallback browser. It runs Playwright with Chromium and can render any webpage.

**Why:** Many websites block non-browser user agents. The visual-tester pod has a real Chromium browser that presents as a legitimate macOS user.

**How to apply:**
1. Create a small Python script that navigates to the URL and takes a screenshot or extracts content
2. Copy it into the pod and execute:
   ```bash
   kubectl exec -n clearwatch visual-tester-POD-ID -- python3 -c "
   from playwright.sync_api import sync_playwright
   with sync_playwright() as p:
       browser = p.chromium.launch()
       page = browser.new_page()
       page.goto('https://example.com')
       print(page.content())
       browser.close()
   "
   ```
3. The pod has Playwright pre-installed at `/ms-playwright`
4. Two replicas available: check with `kubectl get pods -n clearwatch -l app=visual-tester`
