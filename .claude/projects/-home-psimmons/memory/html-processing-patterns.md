---
name: html-processing-patterns
description: SVG-safe HTML processing — extract-process-restore pattern with code example
type: reference
Category: reference
---
# HTML Processing Patterns - SVG Preservation

**Rule: NEVER use BeautifulSoup on SVG-containing HTML.** All parsers (html.parser, lxml, html5lib) lowercase `viewBox` → `viewbox` and mangle xmlns. Regex alone also fails — corrupts nested tag structure.

## Correct Pattern: Extract-Process-Restore

```python
import re
from bs4 import BeautifulSoup

def process_html_with_svg(body_html: str) -> str:
    # 1. Extract SVGs to placeholders
    svg_blocks = []
    def extract_svg(match):
        svg_blocks.append(match.group(1))
        return f'<!--SVG_PLACEHOLDER_{len(svg_blocks)-1}-->'
    html_no_svg = re.sub(r'(<svg[^>]*>.*?</svg>)', extract_svg, body_html, flags=re.DOTALL)

    # 2. Process HTML safely with BeautifulSoup
    soup = BeautifulSoup(html_no_svg, 'html.parser')
    # ... manipulations ...
    result = str(soup)

    # 3. Restore SVGs character-for-character
    for i, svg in enumerate(svg_blocks):
        result = result.replace(f'<!--SVG_PLACEHOLDER_{i}-->', svg)
    return result
```

SVG never touches BeautifulSoup. HTML gets proper structure parsing. SVGs restored exactly.
