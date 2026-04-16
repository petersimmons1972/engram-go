---
name: Code blocks always get copy buttons
description: Any HTML page with code samples must include a copy-to-clipboard button on each code block
type: feedback
Category: feedback
---

Code blocks in any HTML page I build must have a copy button. No exceptions.

**Why:** Code that people can't easily copy is friction that stops adoption. If the samples work, they need to be one click away from the user's terminal.

**How to apply:** Whenever building or updating an HTML page that contains `<pre>` or `<code>` blocks with usable samples:
- Add a copy button positioned absolutely in the top-right of each code block
- Use the Clipboard API with an `execCommand` fallback for older browsers
- Style to match the page's aesthetic (e.g., brass/gold for Art Deco pages)
- Show brief "✓ Copied" confirmation on click, revert after 2 seconds
- This applies to project showcases, documentation pages, and any static HTML with code samples
