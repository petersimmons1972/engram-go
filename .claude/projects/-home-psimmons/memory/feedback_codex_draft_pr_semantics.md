---
name: feedback_codex_draft_pr_semantics
description: A draft Codex PR is finished-awaiting-merge, not work-in-progress
metadata:
  type: feedback
---

In the Claude↔Codex loop, a **draft Codex PR is NOT work-in-progress.** Codex's flow: push branch → open draft PR → link issue → wait for CI → apply `agent/codex/done` on the ISSUE when green/clean → leave the PR as draft (Codex confirmed this directly; canonical: `~/.cache/claude-codex/protocol/operational-protocols.md`).

**The merge-ready signal is: PR exists + linked issue labeled `agent/codex/done` + green CI** — NOT the PR's draft flag.

**Why:** I wrongly filtered out draft PRs as "unfinished" and nearly missed a real merge backlog. Peter and Codex both corrected me.

**How to apply:** To find merge-ready Codex PRs, cross-reference open PRs against `agent/codex/done` issues with green CI; do not exclude drafts. Merging itself is the founder's step — see [[feedback_codex_publish_only_handoff]].
