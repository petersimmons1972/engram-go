---
name: Substack workflow feedback
description: Two rules from user corrections during Substack publishing session
type: feedback
originSessionId: c399db8e-a41e-4d26-b74e-e7b3f48c0df9
---
**Rule 1: Never open the browser uninstructed.**
User said STOP mid-session when I attempted to open Brave browser after a push. Print the URL; do not open it.

**Why:** User controls when to review things in a browser. Uninstructed browser actions break their flow.

**How to apply:** After push-draft.py or update-post.py, print the live/edit URLs and stop. Never call xdg-open, brave, or any browser command unless explicitly asked.

---

**Rule 2: Artwork production and publishing are separate steps.**
When asked to "produce artwork" — write SVG, validate, Ramsay review, convert PNG, commit. Stop there.
When asked to "post" or "push" — run push-draft.py or update-post.py.

**Why:** User controls timing of each post going live. Conflating the two removes editorial control.

**How to apply:** After committing artwork, say the files are ready and wait. Do not run publishing commands.
