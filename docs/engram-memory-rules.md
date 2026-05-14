# Engram Memory — Full Rules

Reference doc for `~/CLAUDE.md`. Contains verbose rule detail and Eisenhower-only protocol.

---

## Full R-Rule Table

| Rule | Trigger | Action |
|------|---------|--------|
| **R1 — Recall at start** | First user message of a new conversation | `memory_recall("current project status recent work", project="global")`, then recall the request topic from the relevant project. Once per conversation. |
| **R2 — Recall before deciding** | Before proposing architecture/design, choosing between 2+ options, modifying infra (K8s/DNS/cert-manager/storage), or modifying a Clearwatch feature area | `memory_recall("<topic>", project="<project>")` |
| **R3 — Feedback after recall** | After every `memory_recall` | `memory_feedback` with the IDs that informed the answer. If results were absent/wrong where context should exist, store a MISS entry (`memory_type="error"`, `tags=["retrieval-miss"]`). |
| **R4 — Store after work** | Bug fix committed, decision made, pattern used 2+ times, or **end of session (extract wisdom/patterns before closing)** | `memory_store` with type: `decision` (include why) · `error` (include root cause) · `pattern` · `context` (`importance=1, project="global"` for session summary). |
| **R6 — Fallback** | Engram unreachable after 1 retry within 30s | Stage entry in `~/.claude/projects/-home-psimmons/memory/fallback.md` (format defined in that file). Flush all pending entries on reconnect. Staging only — never skip wisdom capture; file it here if Engram is down. |

## R7 — Eisenhower Dispute Tracking

*Eisenhower agent only.* Before adjudicating a user-raised dispute: `memory_recall("dispute-tracker <issue>", project="<project>")`. If count ≥3, escalate to founder instead of adjudicating. Store each adjudication:

```
content="DISPUTE: <desc> | VERDICT: <summary> | COUNT: N | LAST: <YYYY-MM-DD>"
tags=["dispute-tracker", "<project>"]
importance=1
```
