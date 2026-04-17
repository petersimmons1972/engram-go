# Temporary Override — dispatching-parallel-agents

**Status:** TEMPORARY. Remove once no longer needed.
**Forked from:** superpowers v5.0.7 (commit `1f20bef`)
**Created:** 2026-04-17 during Opus 4.7 migration audit
**Upstream source:** `~/.claude/plugins/cache/superpowers-marketplace/superpowers/latest/skills/dispatching-parallel-agents/SKILL.md`

## Why this override exists

The obra/superpowers repo has explicit guidelines against "compliance" PRs and
speculative fixes without eval data. This change is currently speculative
(motivated by Anthropic's 4.7 announcement, not by observed 4.7 behavior), so
submitting upstream would be rejected.

Two wording changes were made to reduce the risk that Opus 4.7's more literal
instruction-following causes the skill to over-refuse parallel dispatch:

1. **"Don't use when" — failures-are-related clause.** Tied the judgment to
   Phase 1 investigation output rather than the probabilistic phrase
   "might fix others."
2. **"Exploratory debugging" — scope clause.** Tied the judgment to Phase 1
   output rather than the vague phrase "don't know what's broken yet."

## When to remove this override

Remove when either of the following is true:

- **(a) Upstream ships an equivalent change.** Check with:
  ```
  diff .claude/skills/dispatching-parallel-agents/SKILL.md \
       ~/.claude/plugins/cache/superpowers-marketplace/superpowers/latest/skills/dispatching-parallel-agents/SKILL.md
  ```
  If the upstream wording already achieves the same thing, delete this
  directory. User-scope skills take precedence over plugin-scope, so deleting
  restores upstream behavior.

- **(b) Real 4.7 use shows upstream wording isn't causing problems.** If after
  a representative period (~10 real dispatching decisions under 4.7) the
  upstream wording would have produced the same outcome as the override,
  delete the override. The override was defensive; without evidence of
  misbehavior, it's unjustified drift from the tuned upstream.

## If the upstream wording IS causing problems

Capture specific session evidence (dispatch that should have been parallel
but the model refused; or dispatch that was parallel when it shouldn't have
been). Then the change has a real problem statement and becomes an upstream
PR candidate per obra/superpowers contributor guidelines. Do not submit
without session evidence.

## Drift check

After any superpowers plugin update, run:

```
diff .claude/skills/dispatching-parallel-agents/SKILL.md \
     ~/.claude/plugins/cache/superpowers-marketplace/superpowers/latest/skills/dispatching-parallel-agents/SKILL.md
```

Review upstream changes. If upstream improved something unrelated to the
override, merge it into the override. If upstream improved something related
to the override, strongly consider removing the override.
