# Primer — writing this paper (first-timer's orientation)

A short guide to the form, so the scaffold in `noise-floor.md` makes sense. Delete before publishing.

## What makes it a paper and not a blog post
Three things, and only three:
1. **Pre-registration** — you decided the rules (scoring, stop conditions) *before* seeing the data. This is what stops
   "I tuned until it looked good." You have this; say so loudly.
2. **A real significance test** — McNemar on paired items, not "the average went up." This is the difference between
   "looks better" and "is better."
3. **Reproducibility** — someone else can re-run it and get your numbers. One command, full provenance. You have this too.

Everything else — the prose, the framing, the story — is craft. These three are the credential. A blog post can be
smarter than a paper; a paper is a blog post that *proved it couldn't be fooling itself.*

## The two-audience technique (your stated goal, and it's achievable)
- **The rigor layer** (numbers, methods, p-values) stays exact and is for reviewers / hiring managers. Never soften it.
- **The comprehension layer** (framing + the *In plain terms* boxes) makes every idea followable by a smart non-academic.
- These don't conflict because they're different sentences doing different jobs. The plain-terms box explains the *idea*;
  the table proves it. A reader understands everything and can reproduce nothing — which is correct.
- Your Ground Truth voice already does this: lead with the receipt ("this looked like +7.6; it was noise, p = 1.000"),
  then let the statistics sit underneath.

## What each section is *for* (so you know what "done" means)
| Section | Its job | Reader's question it answers |
|---|---|---|
| Abstract | The whole paper in 200 words | "Do I care?" |
| Introduction | Problem → gap → your 3 contributions | "What's new here?" |
| Setup | Exactly what was measured, on what | "Is this a fair test?" |
| Methodology | Why it can't be an artifact of *how* you measured | "Could this be a fluke of the method?" |
| Noise Floor | Your headline finding, its own section + figure | "What's the one thing to remember?" |
| Levers Tested | The results table — honest, including refutations | "What actually happened?" |
| Structure in/out | Your affirmative claim (if B4 lands) | "Does anything *work*?" |
| Negative results | Failures + a proof, framed as contributions | "What did you rule out?" |
| Discussion | What the field should take away | "So what?" |
| Limitations | Where it doesn't apply (says this *first*, before a reviewer does) | "Where are the edges?" |
| Related work | How it sits against prior art | "Do they know the literature?" |
| Reproducibility | One-command re-run | "Can I trust the numbers?" |

## The order to actually write it (not top-to-bottom)
1. **Results first** — fill §4 (noise floor) and §5 (levers table) from the registry. The paper is these two.
2. **Methodology** — you already ran it; just describe what you did.
3. **Setup** — mechanical.
4. **Discussion + Intro + Abstract last** — you can only frame the story once the results are on the page. Write the
   abstract dead last; it's the paper compressed.
5. **Related work + Limitations** — steady-state; fill as you go.

## Voice checklist (Ground Truth)
- Lead with the concrete number, then explain. No "revolutionary," no "we believe."
- Report your own refutations plainly — that's the credibility, not a weakness.
- Short sentences carry the claims. Save the long ones for nuance.
- Own the AI-assisted authorship if it comes up; don't apologize for it.

## Venue / publish path (matches reforge plan)
- **v0** — this system only, methodology + refutations. Ship as a blog + this doc + the open repo. *Publishable now.*
- **v1** — add the B4 (structure-as-input) result once the ledger ablation lands.
- **v2** — cross-system (Letta/Mem0/Zep/Cognee + engram); mirror to **arXiv** (cs.CL / cs.LG) or a workshop. A workshop
  paper is the low-friction credential; arXiv preprint is fine and citable without peer review.
- One rigorous public writeup has cleared multiple labs' interview bars for non-PhD candidates. This is that writeup.
