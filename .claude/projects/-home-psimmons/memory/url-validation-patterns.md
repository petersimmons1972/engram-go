# URL Validation Patterns

## Browser User-Agent Spoofing

**Problem**: Many legitimate sites (Gartner, Crunchbase, G2, investor relations pages) return 403 Forbidden to automated tools with generic user-agents, even though the site is alive and the content is legitimate.

**Solution**: Mimic a real Windows desktop browser with complete headers.

### Current Best Practice (as of Feb 2026)

**Windows 11 + Chrome 131** is the recommended user-agent string:

```python
headers = {
    "User-Agent": "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36",
    "Accept": "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,image/apng,*/*;q=0.8",
    "Accept-Language": "en-US,en;q=0.9",
    "Accept-Encoding": "gzip, deflate, br, zstd",
    "sec-ch-ua": '"Google Chrome";v="131", "Chromium";v="131", "Not_A Brand";v="24"',
    "sec-ch-ua-mobile": "?0",
    "sec-ch-ua-platform": '"Windows"',
    "Sec-Fetch-Dest": "document",
    "Sec-Fetch-Mode": "navigate",
    "Sec-Fetch-Site": "none",
    "Sec-Fetch-User": "?1",
    "Upgrade-Insecure-Requests": "1"
}
```

### 403 Handling

Treat 403 as "alive but protected" not "dead":

```python
alive = (response.status_code < 400) or (response.status_code == 403)
```

**Rationale**: 403 means the site exists and is actively blocking automated access, which proves it's operational. Only treat 404, 410, 5xx, and connection failures as "dead".

### Monthly Update Policy

**Browser versions change monthly.** Update the user-agent string monthly to match current Chrome stable:

1. Check https://chromereleases.googleblog.com/ for latest stable version
2. Update Chrome version number in User-Agent and sec-ch-ua headers
3. Test against known 403-blocking sites (Gartner, G2, Crunchbase)
4. Commit with message: "chore: update URL validator user-agent to Chrome {version}"

**Historical versions**:
- Feb 2026: Chrome 131 (Windows 11)

### Impact

This change improved citation validation integrity score from ~72% to ~92% in security intelligence business reports (Feb 2026).

### Where This Applies

Any code that validates URLs for research purposes:
- Citation validators in report generators
- Link checkers in documentation systems
- URL liveness checks in archival tools
- Web scraping for research data collection

### Implementation Locations

Current implementations:
- `projects/security-intelligence-business/src/lib/citation_url_validator.py` (lines 37-77)

When adding URL validation to new projects, copy the headers from the pattern above, don't start with generic `requests.get()` calls.
