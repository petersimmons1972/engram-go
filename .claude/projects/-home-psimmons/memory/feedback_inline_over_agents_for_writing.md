---
name: feedback-inline-over-agents-for-writing
description: User rejects Agent tool dispatch for document editing and writing tasks — prefers inline execution
metadata: 
  node_type: memory
  type: feedback
  originSessionId: 61abe33a-c811-4282-b7de-9323185f1b1b
---

Do NOT dispatch subagents via the Agent tool for document editing, writing, or reformatting tasks. Execute these inline in the current session.

**Why:** User explicitly rejected an Agent tool call for a document reformat task (playgram.ai magazine HTML, 2026-05-12), then switched model to Sonnet 4.6 and said "continue" — indicating they wanted inline execution, not subagent delegation.

**How to apply:** When a task involves editing markdown, writing HTML, reformatting documents, or structured content generation, do it directly with Edit/Write tools in the current session. Reserve Agent dispatch for genuinely parallel independent research tasks (web search, code analysis across repos) where isolation is the point. Subagent-driven-development skill is appropriate for multi-file code implementation, not document writing.
