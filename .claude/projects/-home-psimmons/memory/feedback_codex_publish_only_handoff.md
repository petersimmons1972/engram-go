---
name: feedback_codex_publish_only_handoff
description: Codex↔Claude handoff is publish-only; the founder merges, not Claude
metadata:
  type: feedback
---

Ratified 2026-06-05 (Peter): the Claude↔Codex handoff is **publish-only**.

- **Codex = publish-only:** push branch → open PR → link the issue → apply `agent/codex/done` on the issue when CI is green. Then hand off. Codex does NOT merge and never pushes main (canonical: `~/.cache/claude-codex/protocol/operational-protocols.md`, Protocol 2/3/5).
- **Claude = review + stage only:** vet published PRs (cross-reference open PRs against `agent/codex/done` issues + green CI), flag holds, and prepare/stage the merge commands. Claude does NOT merge to main.
- **Founder = merges.** The per-push-to-main confirmation gate (CLAUDE.md Cost Guardrails) stays in force.

**Why:** I assumed "draft = unfinished" (wrong — see [[feedback_codex_draft_pr_semantics]]), then over-reached toward auto-merging. Peter corrected: "Stop assuming and coordinate protocols," and chose **Option A — publish-only handoff** over pre-authorizing Claude to merge. The harness classifier independently blocked a bulk merge at the same boundary.

**How to apply:** When asked to "approve and merge Codex PRs," review + vet + stage the batch and present ready-to-run merge commands for the founder. Do NOT add `gh pr merge` permission rules or merge autonomously. Do NOT work around a classifier denial on push-to-main. See [[feedback_run_bash_no_confirm]] (that preference covers read-only Bash, NOT merges).
