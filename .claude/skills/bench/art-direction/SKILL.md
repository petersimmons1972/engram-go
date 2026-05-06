---
name: art-direction
description: "Use when a repo needs a visual identity established or refreshed, or when docs/assets/VISUAL-IDENTITY.md does not exist. Runs a short discovery conversation about the project's personality and aesthetic preferences, then authors VISUAL-IDENTITY.md from the answers. Also checks for installed design skills (like ui-ux-pro-max) that can serve as design reference."
---

# Art Direction — Visual Identity Discovery

This skill runs when `docs/assets/VISUAL-IDENTITY.md` does not exist in the target repo, or when the user explicitly asks to establish or refresh the visual identity.

The goal: author a `VISUAL-IDENTITY.md` that reflects *this project's* personality — not a template, not an imposed style. Two repos using this skill should end up looking completely different if their projects are different.

---

## Step 1: Check for Installed Design Skills

Before running the discovery conversation, check if the user has a design skill installed:

Scan `~/.claude/skills/` for any skill with "design", "ui", "ux", or "visual" in the name or description.

If multiple matching skills are found, list them all and ask the user to choose one before continuing.

If one is found, prompt:

> "I found [skill-name] installed. Would you like to use it as your visual design reference? Options:
> - **yes** — use [skill-name] as the design foundation, pull palette and typography from it
> - **no** — I'll ask you questions and build a custom identity from scratch
> - **both** — ask me the questions, then we'll pick specific options from [skill-name] that match your answers"

**Recommended default:** `ui-ux-pro-max` from skills.sh gives access to 50 styles, 21 palettes, and 50 font pairings. It's the recommended starting point if you do not have a strong visual preference. See the [skills.sh listing](https://skills.sh/nextlevelbuilder/ui-ux-pro-max-skill/ui-ux-pro-max) for install instructions.

If no design skill is found, proceed directly to Step 2.

---

## Step 2: Discovery Conversation

Ask these questions one at a time. Wait for each answer before asking the next. All questions are optional — pressing enter / "skip" / "no preference" is a valid answer.

**Question 1 — Personality:**

> "How should this repo feel? Pick one or more, or describe it in your own words:
>
> - Precise/technical — clean lines, monospace aesthetic, minimal color
> - Warm/accessible — rounded, friendly, inviting
> - Bold/loud — high contrast, commanding, designed to stop you mid-scroll
> - Minimal/professional — restrained palette, lots of whitespace, serious
> - Playful/fun — color, illustration, personality
> - Something else — just describe it"

**Question 2 — Aesthetic reference (optional):**

> "Any era, movement, or visual style that resonates with this project?
>
> Examples: 1920s travel posters, 1980s neon, Bauhaus, Swiss International Style, woodblock prints, technical blueprints, hand-drawn zines, brutalism, Y2K...
>
> Or paste colors/fonts you already like. Or skip this entirely."

**Question 3 — Existing brand materials (optional):**

> "Do you have any existing brand materials for this project? A logo, hex codes, a design system, or a style guide? If so, paste what you have — or describe it."

**Question 4 — Visual references (optional):**

> "Any repos, websites, or docs whose visual design you admire? Even a vague reference ('something like Stripe's docs' or 'warm like Notion but darker') gives me something to work with."

---

## Step 3: Synthesize and Author VISUAL-IDENTITY.md

Based on the discovery answers, author `docs/assets/VISUAL-IDENTITY.md` using this structure. Every section must be filled with specific, concrete values derived from the answers — no placeholders.

```markdown
# Visual Identity: [REPO NAME]

## Theme
[1-sentence aesthetic statement — what does this repo LOOK like and feel like?]

## Palette
| Role | Hex | Usage |
|------|-----|-------|
| Background | #... | page/chart backgrounds |
| Primary text | #... | headings, key values |
| Secondary text | #... | body text, descriptions |
| Muted | #... | captions, footnotes |
| Accent | #... | highlights, calls to action |

## Typography mood
[serif/sans/mono preference, formal/casual register, weight preferences, letter-spacing]

## Visual personality
[2-3 sentences describing the aesthetic in concrete terms. Not "clean and modern" — specific. Example: "Industrial precision: right angles, monospace everywhere, nothing decorative unless it's load-bearing."]

## Imagery style
[What kinds of images, illustrations, or artwork suit this project? Be specific about subject matter, period, medium, and mood.]

## SVG constraints
- viewBox: [standard dimensions — 750x420 landscape is a safe default]
- Corner treatment: [rounded (4-8px radius) / sharp (0px)]
- Gradient style: [radial from center / linear top-to-bottom / flat]
- Font family: [primary font with web-safe fallbacks]

## Existing assets
[List any committed SVGs, logos, or posters already in docs/assets/, or "None yet"]
```

**Palette derivation rules:**
- Derive from discovery answers. If the user said "dark and technical" — dark background, high-contrast text, minimal accent.
- If no color preference was given and no design skill is installed, pick a purposeful palette based on the project's domain (dev tool? security? consumer app?) rather than defaulting to generic gray.
- Always include all 5 color roles: Background, Primary text, Secondary text, Muted, Accent. Muted is for captions/footnotes and may be a lighter version of Secondary text — but it must be present.
- Every color must pass WCAG AA contrast (4.5:1) against its background.

**If the user chose "both" in Step 1:** Read the referenced design skill file. Identify its palette and typography sections. Select the style/palette option that best aligns with the user's discovery answers. Note the source skill and the specific option selected in the VISUAL-IDENTITY.md under Existing assets (e.g., "Palette: Cascade Night from ui-ux-pro-max, style #14").

---

## Step 4: Write and Commit VISUAL-IDENTITY.md

Write the synthesized content to `docs/assets/VISUAL-IDENTITY.md` using the Write tool or equivalent. Create the directory if it does not exist. Then commit:

```bash
mkdir -p docs/assets
git add docs/assets/VISUAL-IDENTITY.md
git commit -m "docs: establish visual identity for [repo-name]"
```

Signal completion by outputting:

```
VISUAL-IDENTITY.md authored at docs/assets/VISUAL-IDENTITY.md — art-direction complete
```

If this skill was invoked from the `github-docs` pipeline, the pipeline will now continue from Step 3 (Audit). If running standalone, the skill is complete.

---

## Style Vocabulary (Reference Only)

These historical design styles are vocabulary, not mandates. They exist to help describe aesthetics when users have no other reference point. A project might draw on Cassandre's geometric clarity while using completely different colors and a modern font stack.

| Style | Characteristics | When it fits |
|-------|----------------|--------------|
| **Cassandre (Streamline Moderne)** | Bold geometric shapes, strong diagonals, limited palette, machine-age precision | Geometric, system-heavy, infrastructure, CLI tools |
| **Mucha (Art Nouveau)** | Flowing lines, botanical ornament, warm tones, human-centered | Organic, warm, human-centered projects |
| **Rand (Modernism)** | Grid-based, mark-centric, timeless, reduction to essentials | Professional, corporate, brand systems |
| **Toulouse-Lautrec (Belle Epoque)** | Strong silhouettes, flat color, commanding advertising presence | High-impact, bold, event-style projects |
| **Savignac (Mid-century)** | Character-driven, visual humor, warmth, broad appeal | Fun, accessible, community-oriented |
| **Greiman (Digital Modernism)** | Layered information, complex hierarchy, digital-native systems | Data-heavy, UI-heavy, technically complex |

These are starting points for vocabulary. The goal is always a visual identity that feels *right for this specific project* — not a recreation of any historical style.
