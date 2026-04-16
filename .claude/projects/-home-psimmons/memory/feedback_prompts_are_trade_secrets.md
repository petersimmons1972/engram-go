---
name: Agent Prompts Are Trade Secrets
description: Never publish agent prompts, briefs, writing rules, or workflow instructions to any public location
type: feedback
Category: feedback
---

The prompts, briefs, and workflow instructions used to dispatch AI agents are proprietary methodology and trade secrets. They must never be published to any location that is not explicitly the user's own private storage.

**Why:** The prompts are the load-bearing intellectual property behind the agent workflow. Publishing them gives away the discipline that makes the work distinctive. Specifically: the 13 writing rules given to Murrow, the role definitions for Layton/Rommel/Zero-context/Ramsay, the structural document briefs, the editorial constraints — all of this is the user's own methodology and is not for public consumption.

**The incident:** On 2026-04-07 the methodology.html page initially included a section titled "The Prompt That Produced The Document" which listed Murrow's 13 writing rules verbatim plus a link to a `methodology-prompts.md` file in the public GitHub repo. The user said "Do not publish Murrow's prompt. That is a trade secret." Then followed up: "make sure that isn't published period. Anywhere except my own personal private areas." The trade-secret content was live for ~30 minutes before being redacted and re-signed. The methodology-prompts.md archive file was untracked in git so it was never actually pushed to GitHub, but it was on disk in a project that has a public remote — risk vector even if not exposed.

**How to apply:**
- Never paste the contents of an agent brief, system prompt, writing rules list, role definition, or workflow instruction into any HTML, markdown, or other file destined for publication.
- Never reference the existence of "the prompt" with a link to it.
- Talk about what the workflow DOES (multi-agent, named roles, fresh-eyes review, confidence tags, source traceability) without revealing HOW the agents are instructed to do it.
- Local archives of prompt content belong in `~/.config/locked-shields/private-archive/` (chmod 700 dir, chmod 600 files) — NOT in `~/projects/locked-shields/` or anywhere with a public git remote.
- When tempted to publish a "transparency" or "methodology" page, write the page about the OUTCOMES and the DATA, not the prompts. Transparency about sources is good. Transparency about prompts is giving away the kitchen.
- If the user explicitly asks for the prompt to be published, ask twice and document the decision in writing. The default is never publish.

**The redacted methodology page now describes editorial discipline in the abstract** — honesty enforcement, confidence tags, source traceability, independent review, error re-grading — without listing any actual rules from the briefs. Use that page as the model for any future "how we work" content.
