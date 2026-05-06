# Benched Skills

Skills moved here are not auto-loaded at session start.
To activate one, move it back to `~/.claude/skills/`.

| Skill | Purpose |
|---|---|
| `adapt` | Responsive design — breakpoints, fluid layouts, touch targets |
| `animate` | Add animations, transitions, micro-interactions |
| `arrange` | Layout, spacing, visual rhythm fixes |
| `bolder` | Amplify safe/bland designs for more visual impact |
| `clarify` | Improve UX copy, error messages, microcopy |
| `colorize` | Add color to monochromatic designs |
| `delight` | Joy, personality, memorable micro-interactions |
| `onboard` | Onboarding flows, empty states, first-run experiences |
| `overdrive` | Technically ambitious UI — shaders, spring physics, 60fps |
| `quieter` | Tone down aggressive/overstimulating designs |
| `typeset` | Fix typography: fonts, hierarchy, sizing, readability |
| `audit` | Technical code-level quality check — accessibility, performance, anti-patterns |
| `critique` | Evaluate UX — visual hierarchy, IA, emotional resonance, cognitive load |
| `distill` | Strip designs to their essence — declutter, reduce noise |
| `harden` | Improve interface resilience — error handling, i18n, edge cases |
| `normalize` | Realign UI that has drifted from the design system |
| `optimize` | Diagnose and fix UI performance — loading, rendering, animations |
| `polish` | Final pre-ship pass — alignment, typography micro-details, hover states |
| `shape` | Plan UX/UI for a feature before writing code — discovery + design brief |
| `archive` | (purpose TBD — restore and document on first use) |
| `armies` | Multi-agent campaign coordination patterns |
| `art-direction` | Establish visual identity for a repo — VISUAL-IDENTITY.md authoring |
| `extract` | Extract reusable components/tokens/patterns into design system |
| `generals` | Generals library (lives at `~/projects/generals/skills`) |
| `marketing-psychology` | Apply psychological principles and mental models to marketing |
| `playwright-testing` | Debug flaky Playwright tests — race conditions, timing, locators |

Benched 2026-05-04 based on 90-day telemetry showing zero invocations. UI/design cluster (audit, critique, distill, harden, normalize, optimize, polish, shape, extract, art-direction) likely wakes together when UI work resumes.

## Reactivate a skill

```bash
mv ~/.claude/skills/bench/<name> ~/.claude/skills/
```
