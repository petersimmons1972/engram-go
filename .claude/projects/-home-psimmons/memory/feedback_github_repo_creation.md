---
name: GitHub repo creation — just do it
description: When gh CLI is authenticated, create repos and push without asking the user first
type: feedback
Category: feedback
---

User has gh CLI authenticated as petersimmons1972. When a task calls for creating a GitHub repo, check auth with `gh auth status` and create + push directly. Do not ask the user to create the repo manually.

**Why:** User explicitly called this out — "You have the keys to create the repo. Stop asking me."

**How to apply:** Before asking the user to do anything on GitHub (create repo, set topics, push), check if `gh` is available and authenticated. If yes, just do it.
