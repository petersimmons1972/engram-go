# HTML Processing Patterns - SVG Preservation

## The Problem

When post-processing HTML that contains SVG charts, standard approaches fail:

### ❌ BeautifulSoup Alone
- **ALL parsers** (html.parser, lxml, html5lib) mangle SVG
- Lowercases `viewBox` → `viewbox`
- Strips/modifies `xmlns` attributes
- Breaks SVG rendering

### ❌ Regex Alone
- Hard to parse nested HTML correctly
- Easily corrupts tag structure
- Created bugs like: `<div class="pull-quote">div class="pull-quote"`
- Duplicates or malforms opening tags

## ✅ Correct Pattern: Extract-Process-Restore

```python
import re
from bs4 import BeautifulSoup

def process_html_with_svg(body_html: str) -> str:
    """Process HTML while preserving SVG content exactly."""

    # Step 1: Extract SVG content
    svg_pattern = r'(<svg[^>]*>.*?</svg>)'
    svg_blocks = []

    def extract_svg(match):
        svg_blocks.append(match.group(1))
        return f'<!--SVG_PLACEHOLDER_{len(svg_blocks)-1}-->'

    body_html_no_svg = re.sub(svg_pattern, extract_svg, body_html, flags=re.DOTALL)

    # Step 2: Use BeautifulSoup to manipulate HTML
    soup = BeautifulSoup(body_html_no_svg, 'html.parser')

    # ... do your processing ...
    # soup.find(), soup.insert_after(), etc.

    result_html = str(soup)

    # Step 3: Restore SVG content
    for i, svg_content in enumerate(svg_blocks):
        result_html = result_html.replace(f'<!--SVG_PLACEHOLDER_{i}-->', svg_content)

    return result_html
```

## Why This Works

1. **SVG never touches BeautifulSoup** - replaced with HTML comment placeholders
2. **BeautifulSoup can parse HTML correctly** - no SVG to confuse it
3. **SVG restored character-for-character** - exact preservation
4. **HTML structure correct** - BeautifulSoup handles nesting/closing tags

## Real-World Impact

**Before (regex approach)**: Corrupted HTML
```html
<div class="pull-quote">div class="pull-quote"  <!-- BROKEN -->
<h2 id="...">h2Title</h2>  <!-- BROKEN -->
```

**After (extract-process-restore)**: Clean HTML
```html
<div class="pull-quote">  <!-- CORRECT -->
  <div class="pull-quote-label">Key Insight</div>
<h2 id="...">Title</h2>  <!-- CORRECT -->
<svg xmlns="...">...</svg>  <!-- PRESERVED EXACTLY -->
```

## When to Use This

- ✅ Adding visual breaks to HTML with charts
- ✅ Injecting elements into HTML with embedded SVG
- ✅ Any BeautifulSoup processing when SVG present
- ✅ Post-processing generated HTML reports

## When NOT to Use This

- ❌ HTML has no SVG content (just use BeautifulSoup)
- ❌ Need to process SVG content itself (then use XML parser)
- ❌ Simple string replacement (use str.replace())

## Lessons Learned

1. **BeautifulSoup is not SVG-aware** - designed for HTML, not XML namespaces
2. **Regex is not HTML-aware** - can't handle nested structures reliably
3. **Combine tools strategically** - regex for extraction, BeautifulSoup for structure
4. **Test with actual content** - don't assume HTML processors are lossless
