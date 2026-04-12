# Visual Identity: engram-go (Engram 3.0)

**Artist Commander:** April Greiman  
**Aesthetic:** Digital-era information systems — layered, grid-dense, complex hierarchy, memory palace  
**Established:** 2026-04-11

---

## Theme

Engram 3.0 is a memory palace built from code: a dark technical substrate lit by signal — the four search paths that fire simultaneously on every recall, the graph edges that accumulate with use, the layers of context that persist across sessions. Greiman's aesthetic is the right vocabulary: depth through layering, hierarchy through density, complexity made navigable through grid discipline.

---

## Palette

| Role          | Hex       | Usage                                        |
|---------------|-----------|----------------------------------------------|
| Background    | `#0d1117` | SVG and poster backgrounds                   |
| Surface-1     | `#161b22` | Panel backgrounds, card fills                |
| Surface-2     | `#21262d` | Borders, dividers, secondary panels          |
| Text-primary  | `#f0f6fc` | Headings, labels, high-contrast body text    |
| Text-muted    | `#8b949e` | Secondary labels, captions, footnotes        |
| BM25          | `#e3b341` | Keyword search signal — amber/gold           |
| Cosine        | `#3fb950` | Vector semantic signal — green               |
| Recency       | `#1f6feb` | Time-decay signal — blue                     |
| Graph         | `#bc8cff` | Knowledge graph enrichment — violet          |
| Go cyan       | `#00ADD8` | Go language accent, primary action color     |
| Accent-hot    | `#ff6b6b` | Error states, critical flags, warnings       |

**Signal palette rationale:** The four search signals each own a color. BM25 amber/gold (catalog, keyword), cosine green (semantic, meaning), recency blue (time, decay), graph violet (connection, inference). These are used consistently across all scoring diagrams and architecture panels. Never mix them.

---

## Typography Mood

Sans-serif. Weight-forward headings with open tracking. Technical mono for code, tool names, and identifiers. The Greiman approach: type as architecture, not decoration — labels that belong to the grid rather than floating above it.

- **Headings:** `font-family: system-ui, -apple-system, sans-serif` — regular to bold, wide tracking on display text
- **Body/labels:** same stack, 14–16px, `#f0f6fc` or `#8b949e` depending on hierarchy level
- **Code/tool names:** `font-family: 'SF Mono', 'Consolas', 'Liberation Mono', monospace` — `#e3b341` or `#00ADD8` tint for tool identifiers
- **Caption/source lines:** 11–12px, `#8b949e`, italic where attribution is required

---

## Poster Style

Greiman's late-1980s digital collage applied to the memory-system concept. Layered geometric forms at different opacities, signal-color gradients radiating outward, grid lines visible as structure rather than hidden. The human figure is absent — this is the machine's aesthetic, not the user's.

**Reference prompt fragment (for AI generation if needed):**
```
April Greiman digital collage style, late 1980s California New Wave design, 
dark technical substrate #0d1117, layered geometric forms with transparency, 
four-signal color palette (amber gold, semantic green, time-decay blue, violet graph),
grid-dense composition, Go language cyan accent #00ADD8, memory systems theme,
complex information hierarchy made readable through color discipline,
digital grain texture, no human figures, technical sublime aesthetic
```

---

## SVG Constraints

- **viewBox standard:**
  - Landscape panels: `0 0 900 420` (scoring, architecture)
  - Wide hero: `0 0 1000 300` (hero.svg)
  - Portrait: `0 0 512 768` (artwork/posters)
  - Square: `0 0 600 600` (icons, thumbnails)
- **Corner treatment:** 8px radius on panels, sharp on grid lines and dividers
- **Gradient style:** radial from center for spotlight effects; linear top-to-bottom for panel depth; flat fills for signal-color blocks (no gradients on colored signal elements — they should read clean)
- **Font family in SVG:** `font-family="system-ui, -apple-system, Arial, sans-serif"` for labels; `font-family="'Courier New', monospace"` for tool names and code
- **xmlns:** Required on every SVG root — `xmlns="http://www.w3.org/2000/svg"`
- **Defs block:** All gradients and filters declared in `<defs>` — never inline
- **Text elements:** All readable text as `<text>` elements — no paths for text
- **Opacity for layering:** Greiman's signature — panels at 0.05–0.15 opacity overlapping to create depth; grid lines at 0.08–0.12 opacity

---

## Grid System

Every composition is anchored to a visible or implied grid. Panel boundaries align. Labels align to panel edges or centers — never floating. Signal-color elements respect their lane (BM25 top, cosine next, recency next, graph bottom in vertical layouts; or left-to-right in horizontal signal flows).

For architecture diagrams: component boxes should align on a 40px or 60px grid. Connection lines are orthogonal or 45°, never organic curves (that is Mucha's vocabulary, not Greiman's).

---

## Per-Page Artist Assignment

| Page | Visual type | Notes |
|------|-------------|-------|
| README / hero | Hero banner + command table | Full Greiman color system |
| scoring.md / how-it-works.md | Scoring diagram, knowledge graph | Signal colors canonical here |
| architecture.md | Component architecture diagram | Grid-aligned, orthogonal connections |
| getting-started.md | Quick-start flow | Simplified, clean — same palette |
| tools.md | Session workflow | Three-moment pattern visualized |
| connecting.md | IDE ecosystem | Show Claude Code + IDE connections |
| operations.md | DB schema | Technical, schema-style diagram |

---

## Existing Assets

```
docs/hero.svg             — hero banner (needs v3.0 update: aria-label, tagline, badge count)
docs/scoring.svg          — four-signal scoring (needs v3.0 footer update)
docs/architecture.svg     — system architecture (needs v3.0 footer update)
docs/ide-ecosystem.svg    — IDE connection diagram (needs v3.0 footer update)
docs/quick-start-flow.svg — getting started flow
docs/session-workflow.svg — three-moment session pattern
docs/knowledge-graph.svg  — graph traversal visualization
docs/db-schema.svg        — five-table schema
docs/context-reduction.svg — summary vs full mode context savings
docs/claude-advisor.svg   — Claude advisor feature diagram
docs/memory-loss-cycle.svg — pain point: context loss cycle
docs/claude-reason-flow.svg — memory_reason flow
```

---

## Reference Implementation

- **armies** docs: WWII propaganda poster aesthetic (Toulouse-Lautrec) — reference for SVG structure patterns and poster-manifest.md format
- **engram-go** is the reference implementation for Greiman aesthetic within this system

---

*Visual Identity established 2026-04-11 | Artist Commander: Greiman | Version: 3.0*
