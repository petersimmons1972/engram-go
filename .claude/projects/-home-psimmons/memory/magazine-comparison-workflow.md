---
name: Magazine-spread architectural comparison workflow
description: Reusable multi-general template for comparing two designs/products and producing a citable HTML magazine spread
type: pattern
category: generals-workflow
originSessionId: a15c0b00-0071-4065-ad0f-eae37b6c33fe
---
**When to use:** User asks to compare two architectures, designs, products, or approaches and wants a publication-grade output. Validated 2026-04-17 on engram-go vs. Iusztin GraphRAG (`~/reports/engram-vs-graphrag/index.html`).

**Team composition (9 roles, all surface in parallel where possible):**

| Role | General | Deliverable |
|---|---|---|
| Coordinator | Montgomery | Parallel dispatch, synthesis, schedule discipline |
| Capability analyst | Arnold | Gap matrix across N axes with days/weeks/months cost to close |
| Intelligence synthesis | Layton | Pro/con matrix, every claim HIGH/MED/LOW + source tag `[CODE]/[NOTE]/[INFERENCE]` |
| Tactical innovator | Galland | "Mutual theft" list — what each should steal from the other + dead-end trap for each |
| Paradigm | Yamamoto | What each cannot win at; 10-year question |
| Zero-context observer | Kulik | Five anti-patterns independent audit (complexity-as-expertise / metric surrogate / inverted load-bearing / sunk process / failure-obscuring degradation) |
| Designer | Mucha / Greiman / Cassandre | Single standalone HTML magazine spread |
| QA | Gordon Ramsay | Every claim must trace to a file path or cited line; kill unsourced claims |
| Journalist | Pyle (via `/write`) | LinkedIn dispatch in ground-truth voice, no keynote |

**Execution order:**

1. **Phase 1 — Explore.** Deep dive into BOTH sides. For code, get file paths + function names + concrete numbers. For the non-code side (blog, spec, talk), quote exactly what was disclosed AND flag what was NOT disclosed.
2. **Phase A parallel (4 agents):** Arnold / Layton / Galland / Yamamoto, each with the same fact sheet, each ≤500 words.
3. **Phase B parallel (1 agent):** Kulik with ONLY raw design descriptions, no Phase A output.
4. **Phase C synthesis:** Coordinator consolidates into 4 sections: Design Diff / Pro-Con / Mutual Theft / Observer Verdict. Writes `sources.md` citation manifest.
5. **Phase D Mucha renders HTML.** Must invoke `visual-output-standards` skill first. Inline SVG only, Google Fonts via `<link>` if site CSP allows. Required charts: topology diagram, scoring/mechanism viz, feature surface (bar or radial), capability radar, mutual-theft flow.
6. **Phase E Pyle dispatch** via `/write writer=pyle` — ground-truth, 250–350 words, one number per paragraph, ends on a land-line not CTA.

**Critical protocol — evidence asymmetry flag:**
When one side is code and the other is a blog post, **state the asymmetry at the top of the report**. Layton's line is reusable: "You cannot compare a system you have read against a system you have heard described, and claim the comparison is fair. It is not. It is the best comparison currently possible."

**Gotchas learned:**
- Don't let Mucha skip the `visual-output-standards` skill. Default output drifts toward generic AI infographic.
- If the site CSP is `script-src 'none'`, the HTML must be JS-free. Confirm CSP before rendering.
- Ramsay will find unsourced claims every time. Pre-write the citation manifest while the agents are running — don't wait for his audit to discover you're missing half the sources.
- Kulik's zero-context audit is most valuable when run in parallel to Phase A, not after. Running after contaminates him with the team's framing.

**Output locations (default):**
- `~/reports/<comparison-name>/index.html` — magazine spread
- `~/reports/<comparison-name>/sources.md` — citation manifest
- `~/reports/<comparison-name>/linkedin-dispatch.md` — Pyle

**Deployment:** See `static-site-pvc-deploys.md` for dropping the HTML on `www.petersimmons.com/reports/<name>/`.
