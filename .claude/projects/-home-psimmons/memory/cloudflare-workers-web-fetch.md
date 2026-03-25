---
Category: reference
name: Cloudflare Workers web fetch capability
description: Cloudflare Workers can make outbound HTTP fetch() requests to read public websites — useful as a distributed edge fetch layer for non-bot-protected sites
type: reference
---

# Cloudflare Workers — Outbound Web Fetch

Workers can make outbound `fetch()` to any public URL from 300+ global PoPs (AS13335 IPs).

## Good Use Cases
- Vendor press releases, blog posts, documentation, public GitHub data, open pricing pages
- Parallel crawl of many URLs without hammering from a single IP

## Limitations
- **No browser runtime** — no DOM/JS execution; React SPAs and DataDome-protected sites return empty shells
- **AS13335 is known infrastructure** — DataDome, Cloudflare Bot Management recognize Worker IPs as non-residential
- **No Playwright/Puppeteer** — Cloudflare Browser Rendering API runs Chromium but still on AS13335

## Blocked Sites (require residential proxy instead)
- G2 reviews (DataDome — confirmed blocked 2026-03-12, issue #2567)
- Gartner Peer Insights
- LinkedIn (auth + bot detection)

For blocked sites: residential proxy (BrightData, Oxylabs) — see issue #2567.
