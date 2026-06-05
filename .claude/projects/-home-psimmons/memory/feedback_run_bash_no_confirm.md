---
name: feedback_run_bash_no_confirm
description: Run read-only/diagnostic Bash without asking for confirmation each time
metadata:
  type: feedback
---

When working a task, do NOT pause to ask permission before running read-only/diagnostic Bash commands — just run them ("Stop asking about Bash commands. Run them").

**Why:** Per-command confirmation prompts slow the work to a crawl; the task is already authorized.

**How to apply:** Run diagnostic/read-only Bash freely in service of the assigned task. This does NOT extend to the hard gates: push to main/master, prod deploys, >$5 compute, data-loss ops, or merging Codex PRs ([[feedback_codex_publish_only_handoff]]) — those still need explicit founder confirmation.
